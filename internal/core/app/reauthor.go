package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/store"
)

type reauthorBasis struct {
	selected    carrier.Document
	prototype   carrier.Record
	body        []byte
	snapshot    []byte
	data        map[string]any
	digest      string
	diagnostics []carrier.Diagnostic
}

func (s Service) reauthor(ctx context.Context, q Request, r Result, v store.View, payload []byte) Result {
	if q.Action == "reauthor_apply" && (q.RequestID == "" || q.ExpectedGeneration == "" || q.PreviewDigest == "") {
		return failure(r, "apply_preconditions_required", "Reauthor apply requires request_id, expected_generation and preview_digest from reauthor_preview")
	}
	if q.Action == "reauthor_apply" && q.ExpectedGeneration != v.Generation {
		return conflict(r, "concurrent_write", "Memory generation differs from the reauthor preview basis")
	}
	b := selectedReauthorBasis(q.Ref, v)
	r.Diagnostics = append(r.Diagnostics, b.diagnostics...)
	r.Data = b.data
	r.Basis["preview_digest"] = b.digest
	if carrier.HasErrors(b.diagnostics) {
		r.Kind = "conflict"
		return r
	}
	if q.Action == "reauthor_preview" {
		r.Kind = "ready"
		return r
	}
	if q.PreviewDigest != b.digest {
		return conflict(r, "preview_changed", "Re-read and assess the current reauthor preview")
	}
	next := b.prototype
	var err error
	next.ID, err = s.newID("spec")
	if err != nil {
		return unavailable(r, err)
	}
	next.CreatedAt = s.now()
	next.WriteReceipt = receipt(q, payload)
	if ds := carrier.ValidateReauthorSuccessor(next, v.Projection, v.AllSnapshots(), q.Ref); carrier.HasErrors(ds) {
		r.Diagnostics = append(r.Diagnostics, ds...)
		r.Kind = "conflict"
		return r
	}
	raw, err := carrier.Encode(next, b.body)
	if err != nil {
		return failure(r, "encode", err.Error())
	}
	if parsed := carrier.Parse(raw); !parsed.Valid() {
		r.Diagnostics = append(r.Diagnostics, parsed.Diagnostics...)
		r.Kind = "invalid"
		return r
	}
	ref, _ := carrier.ParseRef(q.Ref)
	outputs := appendSnapshot(nil, ref.Digest, b.snapshot)
	outputs, newRef, err := addRecord(outputs, raw, next, v)
	if err != nil {
		return failure(r, "interpretation_basis", err.Error())
	}
	staged := publicationSnapshots(v, outputs)
	newDoc := carrier.Parse(raw)
	newDoc.Edition = strings.Split(newRef, "@")[1]
	docs := append(append([]carrier.Document{}, v.Documents...), newDoc)
	prospective := carrier.ProjectCaptured(docs, v.CurrentSnapshots, staged)
	entry := prospective.Entries[len(v.Documents)]
	r.Diagnostics = append(r.Diagnostics, entry.Diagnostics...)
	if entry.State == carrier.Invalid || carrier.HasErrors(entry.Diagnostics) {
		r.Kind = "conflict"
		return r
	}
	r = s.publish(ctx, q, r, v, payload, outputs, []string{q.Ref})
	receipt, ok := r.Data.(store.Result)
	if !ok {
		return r
	}
	return reauthorReceipt(r, q, receipt)
}

// reauthorReceipt uses exact outputs recorded by the durable transaction. It
// does not recompute an edition from the current project head on replay.
func reauthorReceipt(r Result, q Request, receipt store.Result) Result {
	if receipt.Kind != "written" && receipt.Kind != "replayed" && receipt.Kind != "recovered" {
		return r
	}
	if len(receipt.PublishedRefs) != 1 {
		r.Diagnostics = append(r.Diagnostics, carrier.Diagnostic{Code: "published_ref_unavailable", Severity: "warning", Message: "Committed output identity could not be recovered from the durable receipt"})
		return r
	}
	ref := receipt.PublishedRefs[0]
	r.Data = map[string]any{
		"predecessor_ref":         q.Ref,
		"published_successor_ref": ref,
		"new_ref":                 ref,
		"preview_digest":          q.PreviewDigest,
		"exact_read_request":      map[string]any{"format": "haft.api/2", "operation": "recall", "ref": ref},
		"published_content":       map[string]any{"format": "haft/2", "state_at_publication": carrier.Current, "live_currentness": "not_reassessed"},
		"decision_authority":      map[string]any{"assessment": "not_assessed", "meaning": "Content publication does not accept or supersede a binding decision"},
		"implementation":          map[string]any{"assessment": "not_verified"},
		"evidence":                map[string]any{"assessment": "not_run_or_transferred"},
		"receipt":                 receipt,
	}
	return r
}

func selectedReauthorBasis(address string, v store.View) reauthorBasis {
	b := reauthorBasis{data: map[string]any{}}
	ref, err := carrier.ParseRef(address)
	if err != nil || !ref.Pinned() || ref.ClaimID != "" {
		b.diagnostics = append(b.diagnostics, reauthorDiagnostic("exact_spec_ref_required", "ref", "Select one exact pinned whole-spec v1 edition"))
		return b
	}
	entries := []carrier.Entry{}
	for _, entry := range v.Projection.Entries {
		if entry.Ref == address {
			entries = append(entries, entry)
		}
	}
	if len(entries) != 1 || entries[0].Document.Record.Format != "haft/1" || entries[0].Document.Record.Kind != "spec" {
		b.diagnostics = append(b.diagnostics, reauthorDiagnostic("selected_v1_spec_unavailable", "ref", "The selected exact v1 spec is not one valid live carrier"))
		return b
	}
	b.selected = entries[0].Document
	b.body = b.selected.Body
	b.snapshot = v.CurrentSnapshots[ref.Digest]
	if len(b.snapshot) == 0 {
		b.diagnostics = append(b.diagnostics, reauthorDiagnostic("selected_snapshot_unavailable", "ref", "The selected live edition has no matching captured snapshot bytes"))
		return b
	}
	b.diagnostics = append(b.diagnostics, carrier.ValidateReauthorBasis(v.Projection, v.AllSnapshots(), address)...)
	proto := carrier.Parse(b.selected.Raw).Record
	proto.Format = "haft/2"
	proto.ID = ""
	proto.CreatedAt = ""
	proto.UpdatedAt = ""
	proto.Status = ""
	proto.LegacyStatus = ""
	proto.OperatorConfirmed = false
	proto.Origin = "agent_edit"
	proto.WriteReceipt = nil
	proto.Supersedes = []string{address}
	proto.SupersedeReason = "Explicit selected v1 spec reauthoring to v2 current content"
	b.prototype = proto
	prototypeBytes, err := carrier.Encode(proto, b.body)
	if err != nil {
		b.diagnostics = append(b.diagnostics, reauthorDiagnostic("prototype_encode", "", err.Error()))
		return b
	}
	retention := carrier.SemanticRetention{RewrittenPaths: []string{
		"format", "id", "created_at", "updated_at", "status", "legacy_status",
		"operator_confirmed", "origin", "write_receipt", "supersedes", "supersede_reason",
	}}
	semanticLosses := carrier.SemanticYAMLLosses(b.selected.Raw, prototypeBytes, retention)
	b.diagnostics = append(b.diagnostics, semanticLosses...)
	materialLosses := []string{}
	for _, loss := range semanticLosses {
		materialLosses = append(materialLosses, loss.Path+": "+loss.Message)
	}
	claimIDs := make([]string, 0, len(proto.Claims))
	implementation := []map[string]string{}
	checks := []map[string]string{}
	for _, claim := range proto.Claims {
		claimIDs = append(claimIDs, claim.ID)
		for _, link := range claim.ImplementedBy {
			implementation = append(implementation, map[string]string{"claim_id": claim.ID, "ref": link.Ref, "covers": link.Covers})
		}
		for _, check := range claim.Checks {
			checks = append(checks, map[string]string{"claim_id": claim.ID, "ref": check.Ref, "covers": check.Covers})
		}
	}
	decisionCandidates := specDecisionCandidates(proto, v.Projection)
	decisions := reauthorBindingDecisions(decisionCandidates)
	authority := decisionCandidateAssessment(v.Coverage, len(decisions), "same_about_or_shared_declared_selector")
	authority["binding_decisions"] = decisions
	authority["reauthor_effect"] = "Publishing current spec content does not accept, supersede or assess a binding decision"
	heads := v.Projection.Heads(ref.RecordID)
	sort.Strings(heads)
	pending := pendingV1ReauthorProposals(address, ref.RecordID, v.Projection)
	b.data = map[string]any{
		"selected_ref":      address,
		"memory_generation": v.Generation,
		"old": map[string]any{"format": "haft/1", "state": entries[0].State, "status": b.selected.Record.Status, "origin": b.selected.Record.Origin,
			"carrier_digest": carrier.Digest(b.selected.Raw), "snapshot_digest": ref.Digest, "carrier": string(b.selected.Raw)},
		"current_heads":        heads,
		"pending_v1_proposals": pending,
		"proposal_consequence": "Selected reauthor leaves listed v1 proposals unchanged and unaccepted; carrying their content into current v2 requires a separate authored change against the new content.",
		"new_content": map[string]any{"format": "haft/2", "state": carrier.Current, "origin": "agent_edit", "prototype_carrier": string(prototypeBytes), "prototype_digest": carrier.Digest(prototypeBytes), "claim_ids": claimIDs,
			"body_digest": carrier.Digest(b.body), "source_count": len(proto.Sources), "predecessor_ref": address,
			"id_created_at_receipt": "allocated only by apply"},
		"field_changes":      []string{"format: haft/1 -> haft/2", "status/operator_confirmed/legacy_status: omitted in v2; original values retained in exact predecessor", "origin: agent_edit for new authored content; predecessor origin retained in exact predecessor", "supersedes: selected exact predecessor"},
		"material_losses":    materialLosses,
		"normalization_note": "Prototype frontmatter is re-encoded: comments and YAML lexical form are not carried forward. Exact predecessor carrier, snapshot and body bytes remain available.",
		"decision_authority": authority,
		"implementation":     map[string]any{"declared_only": implementation, "assessment": "not_verified"},
		"evidence":           map[string]any{"declared_checks_only": checks, "new_edition_uses": 0, "assessment": "not_run_or_transferred", "meaning": "Exact historical evidence remains attached to its original edition"},
	}
	bytes, err := json.Marshal(b.data)
	if err != nil {
		b.diagnostics = append(b.diagnostics, reauthorDiagnostic("preview_encode", "", err.Error()))
		return b
	}
	b.digest = carrier.Digest(bytes)
	b.data["preview_digest"] = b.digest
	return b
}

func pendingV1ReauthorProposals(selectedRef, recordID string, p carrier.Projection) []map[string]string {
	byRef := map[string]carrier.Entry{}
	for _, entry := range p.Entries {
		byRef[entry.Ref] = entry
	}
	proposals := []map[string]string{}
	for _, ref := range p.ProposalHeads(recordID) {
		if ref == selectedRef {
			continue
		}
		entry := byRef[ref]
		if entry.Document.Record.Format != "haft/1" || entry.Document.Record.Kind != "spec" {
			continue
		}
		proposals = append(proposals, map[string]string{"ref": ref, "title": entry.Document.Record.Title, "state": entry.State})
	}
	return proposals
}

func reauthorBindingDecisions(candidates []specDecisionCandidate) []map[string]any {
	decisions := []map[string]any{}
	for _, candidate := range candidates {
		decision := candidate.Entry.Document.Record
		decisions = append(decisions, map[string]any{
			"ref": candidate.Entry.Ref, "title": decision.Title, "state": candidate.Entry.State,
			"chosen": decision.Chosen, "about": decision.About, "match_kinds": candidate.MatchKinds,
		})
	}
	return decisions
}

func reauthorDiagnostic(code, path, message string) carrier.Diagnostic {
	return carrier.Diagnostic{Code: code, Path: path, Message: message, Severity: "error"}
}
