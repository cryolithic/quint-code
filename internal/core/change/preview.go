package change

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

// Preview never changes inputs. Snapshot verification is repeated here rather
// than trusting a caller to associate a digest with the correct carrier bytes.
func Preview(c Change, bases map[string]Basis) PreviewResult {
	result := PreviewResult{Kind: "ready", Outputs: []Output{}, Losses: []Loss{}, Diagnostics: Validate(c)}
	if carrier.HasErrors(result.Diagnostics) {
		result.Kind = "invalid"
		return finish(result)
	}
	if c.State != "open" {
		result.Kind = "conflict"
		result.Diagnostics = append(result.Diagnostics, diag("archived_change", "state", "Reopen as a successor before applying"))
		return finish(result)
	}
	for _, patch := range c.Patches {
		basis, ok := bases[patch.Base]
		if !ok {
			result.Diagnostics = append(result.Diagnostics, diag("missing_authored_base", patch.Base, "Exact authored snapshot is unavailable"))
			continue
		}
		if len(basis.Contested) > 1 {
			result.Diagnostics = append(result.Diagnostics, diag("contested_authored_base", patch.Base, "Current spec has competing heads requiring explicit resolution"))
			continue
		}
		if basis.CurrentRef == "" {
			result.Diagnostics = append(result.Diagnostics, diag("current_head_unknown", patch.Base, "Current head basis is unavailable"))
			continue
		}
		if basis.CurrentRef != patch.Base {
			result.Diagnostics = append(result.Diagnostics, diag("stale_authored_base", patch.Base, "Authored edition differs from the selected current head; explicitly rebase"))
			continue
		}
		ref, _ := carrier.ParseRef(patch.Base)
		snap, err := carrier.ReadSnapshot(basis.Snapshot, ref.Digest)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, diag("invalid_authored_snapshot", patch.Base, err.Error()))
			continue
		}
		d := snap.Document()
		if !d.Valid() || d.Record.Kind != "spec" || d.Record.ID != ref.RecordID {
			result.Diagnostics = append(result.Diagnostics, diag("invalid_authored_base", patch.Base, "Snapshot is not the addressed valid spec"))
			continue
		}
		out, losses, ds := patchSection(d, patch)
		result.Losses = append(result.Losses, losses...)
		result.Diagnostics = append(result.Diagnostics, ds...)
		if out != nil {
			result.Outputs = append(result.Outputs, *out)
		}
	}
	if carrier.HasErrors(result.Diagnostics) {
		result.Kind = "conflict"
		result.Outputs = []Output{}
	}
	return finish(result)
}

// PreviewDocument also checks a saved change's authored YAML before any of its
// supplied claims can be used as a specification delta. Historical carriers
// remain readable even when their typed normalization would lose meaning.
func PreviewDocument(d Document, bases map[string]Basis) PreviewResult {
	result := Preview(d.Change, bases)
	normalized, err := Encode(d.Change, d.Body)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diag("change_normalization_unavailable", "", err.Error()))
	} else {
		rewritten := []string{}
		if CreatedAtTimestampNormalization(d.Raw, normalized) {
			rewritten = append(rewritten, "created_at")
		}
		result.Diagnostics = append(result.Diagnostics, carrier.SemanticYAMLLosses(d.Raw, normalized, carrier.SemanticRetention{RewrittenPaths: rewritten, Schema: Change{}})...)
	}
	if carrier.HasErrors(result.Diagnostics) {
		result.Kind = "conflict"
		result.Outputs = []Output{}
	}
	result.Digest = ""
	return finish(result)
}
func finish(r PreviewResult) PreviewResult {
	// Digest excludes itself. It binds deterministic previews, including all losses
	// and conflicts; random publication IDs/timestamps do not exist here.
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		a, b := r.Diagnostics[i], r.Diagnostics[j]
		return a.Path+"\x00"+a.Code+"\x00"+a.Message < b.Path+"\x00"+b.Code+"\x00"+b.Message
	})
	b, err := json.Marshal(r)
	if err != nil {
		r.Kind = "invalid"
		r.Outputs = []Output{}
		r.Diagnostics = append(r.Diagnostics, diag("unserializable_preview", "", err.Error()))
		r.Digest = ""
		return r
	}
	r.Digest = carrier.Digest(b)
	return r
}
func patchSection(d carrier.Document, p SectionPatch) (*Output, []Loss, []carrier.Diagnostic) {
	// Decode from the saved bytes to own every nested map/slice independently.
	r := carrier.Parse(d.Raw).Record
	var losses []Loss
	var ds []carrier.Diagnostic
	bad := func(code, field, msg string) { ds = append(ds, diag(code, p.Base+field, msg)) }
	if r.Retirement != nil {
		bad("retired_lineage", "", "A retired section requires a new lineage")
		return nil, losses, ds
	}
	for n, op := range p.Operations {
		field := fmt.Sprintf(".operations[%d]", n)
		idx := -1
		for i, cl := range r.Claims {
			if cl.ID == op.ClaimID {
				idx = i
				break
			}
		}
		switch op.Op {
		case "ADDED":
			id := op.Claim.ID
			existing := -1
			for i, cl := range r.Claims {
				if cl.ID == id {
					existing = i
					break
				}
			}
			if contains(r.RetiredClaimIDs, id) {
				bad("retired_claim_id", field, "Removed/renamed claim ID cannot be reused")
				continue
			}
			if existing >= 0 {
				if !sameValue(r.Claims[existing], *op.Claim) {
					bad("claim_already_exists", field, "ADDED cannot overwrite different content")
				}
				continue
			}
			cl, err := cloneClaim(*op.Claim)
			if err != nil {
				bad("invalid_claim", field, err.Error())
				continue
			}
			r.Claims = append(r.Claims, cl)
		case "MODIFIED":
			if idx < 0 {
				bad("claim_not_found", field, "MODIFIED target is absent")
				continue
			}
			before := r.Claims[idx]
			replacement, err := cloneClaim(*op.Claim)
			if err != nil {
				bad("invalid_claim", field, err.Error())
				continue
			}
			if replacement.Extra == nil {
				replacement.Extra = carrier.Extra{}
			}
			for k, v := range before.Extra {
				if _, set := replacement.Extra[k]; !set {
					replacement.Extra[k] = v
				}
			}
			replacement.Checks = retainBindingExtras(before.Checks, replacement.Checks)
			replacement.ImplementedBy = retainBindingExtras(before.ImplementedBy, replacement.ImplementedBy)
			replacement.EvidenceInputs = retainEvidenceInputExtras(before.EvidenceInputs, replacement.EvidenceInputs)
			if ambiguousBindingEdit(before.Checks, replacement.Checks) {
				bad("ambiguous_binding_correspondence", field+".claim.checks", "Duplicate check refs cannot identify the edited binding")
			}
			if ambiguousBindingEdit(before.ImplementedBy, replacement.ImplementedBy) {
				bad("ambiguous_binding_correspondence", field+".claim.implemented_by", "Duplicate implementation refs cannot identify the edited binding")
			}
			if ambiguousEvidenceInputEdit(before.EvidenceInputs, replacement.EvidenceInputs) {
				bad("ambiguous_binding_correspondence", field+".claim.evidence_inputs", "Duplicate evidence input refs cannot identify the edited binding")
			}
			removedFields := map[string]bool{}
			for _, k := range op.RemoveFields {
				if removedFields[k] {
					bad("duplicate_removal", field, "Unknown field removal repeated")
					continue
				}
				removedFields[k] = true
				old, exists := before.Extra[k]
				if !exists {
					bad("unknown_field_removal", field, "Only existing extension fields may be removed")
					continue
				}
				delete(replacement.Extra, k)
				losses = append(losses, Loss{Base: p.Base, ClaimID: before.ID, Kind: "extension_removed", ID: k, Before: old, Reason: op.Reason})
			}
			remove := map[string]bool{}
			for _, id := range op.RemoveExamples {
				if remove[id] {
					bad("duplicate_removal", field, "Example removal repeated")
				}
				remove[id] = true
			}
			oldExamples := map[string]carrier.Example{}
			for _, ex := range before.Examples {
				oldExamples[ex.ID] = ex
			}
			for id := range remove {
				if _, exists := oldExamples[id]; !exists {
					bad("unknown_example_removal", field, "Removed example ID does not exist: "+id)
				}
			}
			if replacement.Examples == nil {
				replacement.Examples = append([]carrier.Example{}, before.Examples...)
			}
			kept := replacement.Examples[:0]
			for _, ex := range replacement.Examples {
				if !remove[ex.ID] {
					// Replacing the known scenario fields does not silently remove
					// extensions carried by that stable scenario identity.
					if old, exists := oldExamples[ex.ID]; exists {
						if ex.Extra == nil {
							ex.Extra = carrier.Extra{}
						}
						for k, v := range old.Extra {
							if _, supplied := ex.Extra[k]; !supplied {
								ex.Extra[k] = v
							}
						}
					}
					kept = append(kept, ex)
				}
			}
			replacement.Examples = kept
			next := map[string]bool{}
			for _, ex := range replacement.Examples {
				next[ex.ID] = true
			}
			for _, ex := range before.Examples {
				if !next[ex.ID] && !remove[ex.ID] {
					bad("scenario_loss", field, "Missing scenario must be explicitly removed: "+ex.ID)
				}
				if remove[ex.ID] {
					losses = append(losses, Loss{Base: p.Base, ClaimID: before.ID, Kind: "example_removed", ID: ex.ID, Before: ex, Reason: op.Reason})
				}
			}
			r.Claims[idx] = replacement
		case "REMOVED":
			if idx < 0 {
				bad("claim_not_found", field, "REMOVED target is absent")
				continue
			}
			old := r.Claims[idx]
			losses = append(losses, Loss{Base: p.Base, ClaimID: old.ID, Kind: "claim_removed", Before: old, Reason: op.Reason})
			r.Claims = append(r.Claims[:idx], r.Claims[idx+1:]...)
			r.RetiredClaimIDs = append(r.RetiredClaimIDs, old.ID)
		case "RENAMED":
			if idx < 0 {
				bad("claim_not_found", field, "RENAMED target is absent")
				continue
			}
			used := contains(r.RetiredClaimIDs, op.NewID)
			for _, cl := range r.Claims {
				used = used || cl.ID == op.NewID
			}
			if used {
				bad("claim_id_in_use", field, "New ID is present or previously retired")
				continue
			}
			r.Claims[idx].ID = op.NewID
			r.RetiredClaimIDs = append(r.RetiredClaimIDs, op.ClaimID)
			for i := range r.Claims {
				for j, ref := range r.Claims[i].Refs {
					if ref == op.ClaimID {
						r.Claims[i].Refs[j] = op.NewID
					}
				}
			}
			losses = append(losses, Loss{Base: p.Base, ClaimID: op.ClaimID, Kind: "claim_address_changed", Before: op.ClaimID, After: op.NewID, Reason: op.Reason})
		}
	}
	body := bytes.Clone(d.Body)
	if p.Body != nil {
		if carrier.Digest(d.Body) != p.ExpectedBodyDigest {
			bad("body_digest_conflict", ".body", "Body differs from explicitly expected content")
		} else {
			body = []byte(*p.Body)
			if !bytes.Equal(body, d.Body) {
				losses = append(losses, Loss{Base: p.Base, Kind: "body_replaced", Before: string(d.Body), After: *p.Body, Reason: p.BodyChangeReason})
			}
		}
	}
	if len(d.Record.Claims) > 0 && len(r.Claims) == 0 {
		if strings.TrimSpace(p.RetireReason) == "" {
			bad("retirement_required", ".retire_reason", "Removing the last claim requires an explicit retirement reason")
		} else {
			r.Retirement = &carrier.Retirement{Reason: p.RetireReason}
		}
	} else if p.RetireReason != "" {
		bad("invalid_retirement", ".retire_reason", "Retirement is conditional on removal of the last claim")
	}
	sort.Strings(r.RetiredClaimIDs)
	ds = append(ds, carrier.Validate(r, false)...)
	if carrier.HasErrors(ds) {
		return nil, losses, ds
	}
	if sameValue(r, d.Record) && bytes.Equal(body, d.Body) {
		return nil, losses, ds
	}
	r.ID = ""
	r.CreatedAt = ""
	r.UpdatedAt = ""
	r.WriteReceipt = nil
	if r.Format == "haft/2" {
		r.Status = ""
		r.Origin = "agent_edit"
		r.OperatorConfirmed = false
	} else {
		r.Status = "proposed"
		r.Origin = "agent_proposal"
		r.OperatorConfirmed = false
	}
	r.Supersedes = []string{p.Base}
	r.SupersedeReason = "Apply authored specification delta"
	ds = append(ds, sectionSemanticLosses(d, r, body, p)...)
	if carrier.HasErrors(ds) {
		return nil, losses, ds
	}
	return &Output{Base: p.Base, Successor: r, Body: body}, losses, ds
}
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func retainBindingExtras(before, after []carrier.Binding) []carrier.Binding {
	old, unique := uniqueBindingsByRef(before)
	if !unique || !uniqueBindingRefs(after) {
		return after
	}
	for index := range after {
		prior, retained := old[after[index].Ref]
		if !retained {
			continue
		}
		// The typed replacement cannot distinguish omission from an explicit
		// removal of this nested value. Keep the old value for the same ref;
		// the semantic guard still catches an unrepresentable retained tag.
		if after[index].InterpretationBasis == nil {
			after[index].InterpretationBasis = prior.InterpretationBasis
		}
		if after[index].Extra == nil {
			after[index].Extra = carrier.Extra{}
		}
		for key, value := range prior.Extra {
			if _, supplied := after[index].Extra[key]; !supplied {
				after[index].Extra[key] = value
			}
		}
	}
	return after
}

func uniqueBindingsByRef(bindings []carrier.Binding) (map[string]carrier.Binding, bool) {
	byRef := make(map[string]carrier.Binding, len(bindings))
	for _, binding := range bindings {
		if binding.Ref == "" {
			return nil, false
		}
		if _, duplicate := byRef[binding.Ref]; duplicate {
			return nil, false
		}
		byRef[binding.Ref] = binding
	}
	return byRef, true
}

func uniqueBindingRefs(bindings []carrier.Binding) bool {
	_, unique := uniqueBindingsByRef(bindings)
	return unique
}

func ambiguousBindingEdit(before, after []carrier.Binding) bool {
	if uniqueBindingRefs(before) && uniqueBindingRefs(after) {
		return false
	}
	if len(after) == 0 || disjointBindingRefs(before, after) || unchangedBindingPrefix(before, after) {
		return false
	}
	for _, item := range before {
		if len(item.Extra) != 0 || item.InterpretationBasis != nil {
			return !reflect.DeepEqual(before, after)
		}
	}
	return false
}

func disjointBindingRefs(before, after []carrier.Binding) bool {
	old := map[string]bool{}
	for _, item := range before {
		old[item.Ref] = true
	}
	for _, item := range after {
		if old[item.Ref] {
			return false
		}
	}
	return true
}

func unchangedBindingPrefix(before, after []carrier.Binding) bool {
	if len(after) < len(before) {
		return false
	}
	return reflect.DeepEqual(before, after[:len(before)])
}

func retainEvidenceInputExtras(before, after []carrier.EvidenceInput) []carrier.EvidenceInput {
	old, unique := uniqueEvidenceInputsByRef(before)
	if !unique {
		return after
	}
	if _, unique = uniqueEvidenceInputsByRef(after); !unique {
		return after
	}
	for index := range after {
		prior, retained := old[after[index].Ref]
		if !retained {
			continue
		}
		if after[index].Extra == nil {
			after[index].Extra = carrier.Extra{}
		}
		for key, value := range prior.Extra {
			if _, supplied := after[index].Extra[key]; !supplied {
				after[index].Extra[key] = value
			}
		}
	}
	return after
}

func uniqueEvidenceInputsByRef(inputs []carrier.EvidenceInput) (map[string]carrier.EvidenceInput, bool) {
	byRef := make(map[string]carrier.EvidenceInput, len(inputs))
	for _, input := range inputs {
		if input.Ref == "" {
			return nil, false
		}
		if _, duplicate := byRef[input.Ref]; duplicate {
			return nil, false
		}
		byRef[input.Ref] = input
	}
	return byRef, true
}

func ambiguousEvidenceInputEdit(before, after []carrier.EvidenceInput) bool {
	_, oldUnique := uniqueEvidenceInputsByRef(before)
	_, newUnique := uniqueEvidenceInputsByRef(after)
	if oldUnique && newUnique {
		return false
	}
	if len(after) == 0 || disjointEvidenceRefs(before, after) || unchangedEvidencePrefix(before, after) {
		return false
	}
	for _, item := range before {
		if len(item.Extra) != 0 {
			return !reflect.DeepEqual(before, after)
		}
	}
	return false
}

func disjointEvidenceRefs(before, after []carrier.EvidenceInput) bool {
	old := map[string]bool{}
	for _, item := range before {
		old[item.Ref] = true
	}
	for _, item := range after {
		if old[item.Ref] {
			return false
		}
	}
	return true
}

func unchangedEvidencePrefix(before, after []carrier.EvidenceInput) bool {
	if len(after) < len(before) {
		return false
	}
	return reflect.DeepEqual(before, after[:len(before)])
}
func sameValue(a, b any) bool {
	aa, e := json.Marshal(a)
	bb, f := json.Marshal(b)
	return e == nil && f == nil && bytes.Equal(aa, bb)
}
func cloneClaim(c carrier.Claim) (carrier.Claim, error) {
	// YAML retains extension keys in the single authored namespace.
	r := carrier.Record{Claims: []carrier.Claim{c}}
	b, err := carrier.Encode(r, nil)
	if err != nil {
		return carrier.Claim{}, err
	}
	front, _, err := carrier.SplitFrontmatter(b)
	if err != nil {
		return carrier.Claim{}, err
	}
	node, ds := carrier.ParseYAML(front)
	if carrier.HasErrors(ds) {
		return carrier.Claim{}, fmt.Errorf("%v", ds)
	}
	var copy carrier.Record
	if err := node.Decode(&copy); err != nil {
		return carrier.Claim{}, err
	}
	if c.Examples != nil && len(c.Examples) == 0 {
		copy.Claims[0].Examples = []carrier.Example{}
	}
	if c.Checks != nil && len(c.Checks) == 0 {
		copy.Claims[0].Checks = []carrier.Binding{}
	}
	if c.ImplementedBy != nil && len(c.ImplementedBy) == 0 {
		copy.Claims[0].ImplementedBy = []carrier.Binding{}
	}
	if c.EvidenceInputs != nil && len(c.EvidenceInputs) == 0 {
		copy.Claims[0].EvidenceInputs = []carrier.EvidenceInput{}
	}
	return copy.Claims[0], nil
}

// Materialize completes a preview with caller-supplied publication metadata.
// Acceptance is an explicit caller assertion in the trusted local model, never
// inferred from the predecessor, a check result, or this function invocation.
func Materialize(out Output, m Metadata) ([]byte, []carrier.Diagnostic) {
	r := out.Successor
	old, err := carrier.ParseRef(out.Base)
	if err != nil || m.ID == old.RecordID {
		return nil, []carrier.Diagnostic{diag("invalid_successor_id", "id", "Successor needs a new record identity")}
	}
	r.ID = m.ID
	r.CreatedAt = m.CreatedAt
	r.UpdatedAt = ""
	r.WriteReceipt = m.Receipt
	if r.Format == "haft/2" && (m.Status != "" || m.OperatorConfirmed || m.Origin != "" && m.Origin != "agent_edit") {
		return nil, []carrier.Diagnostic{diag("v2_authority_metadata", "metadata", "A v2 spec edit cannot carry status, confirmation or inferred operator provenance")}
	}
	if m.Status != "" {
		r.Status = m.Status
	}
	if m.Origin != "" {
		r.Origin = m.Origin
	}
	r.OperatorConfirmed = m.OperatorConfirmed
	if m.Supersedes != nil {
		r.Supersedes = append([]string{}, m.Supersedes...)
	}
	if !contains(r.Supersedes, out.Base) {
		return nil, []carrier.Diagnostic{diag("missing_predecessor", "supersedes", "Authored base must remain an explicit predecessor")}
	}
	if m.SupersedeReason != "" {
		r.SupersedeReason = m.SupersedeReason
	}
	ds := carrier.Validate(r, false)
	if carrier.HasErrors(ds) {
		return nil, ds
	}
	raw, err := carrier.Encode(r, out.Body)
	if err != nil {
		return nil, append(ds, diag("encode", "", err.Error()))
	}
	return raw, ds
}
