package change

import (
	"strconv"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func t01br4BindingBasis(t *testing.T, field string, tagged bool) (carrier.Document, string, map[string]Basis, Change, int) {
	t.Helper()
	d, _, _, c := setup(t)
	index := 0
	switch field {
	case "checks":
		d.Record.Claims[0].Checks[0].Extra = carrier.Extra{"x-owner": "abc"}
	case "implemented_by":
		d.Record.Claims[0].ImplementedBy[0].Extra = carrier.Extra{"x-owner": "abc"}
	case "evidence_inputs":
		index = 2
		d.Record.Claims[index].EvidenceInputs = []carrier.EvidenceInput{{
			Ref:           "ev-20260926-00000001@" + carrier.Digest([]byte("evidence")) + "#check-1",
			Applicability: "The observed cancellation run",
			Extra:         carrier.Extra{"x-owner": "abc"},
		}}
	default:
		t.Fatalf("unsupported field %q", field)
	}
	raw, err := carrier.Encode(d.Record, d.Body)
	if err != nil {
		t.Fatal(err)
	}
	if tagged {
		old := "x-owner: abc"
		if !strings.Contains(string(raw), old) {
			t.Fatalf("fixture does not contain %q", old)
		}
		raw = []byte(strings.Replace(string(raw), old, "x-owner: !vendor abc", 1))
	}
	parsed := carrier.Parse(raw)
	if !parsed.Valid() {
		t.Fatalf("invalid fixture: %+v", parsed.Diagnostics)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := parsed.Record.ID + "@" + digest
	c.Patches[0].Base = ref
	return parsed, ref, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}}, c, index
}

func t01br4BindingEdit(claim carrier.Claim, field string, omitExtra bool) carrier.Claim {
	switch field {
	case "checks":
		claim.Checks[0].Covers += " Revised coverage."
		if omitExtra {
			claim.Checks[0].Extra = nil
		}
	case "implemented_by":
		claim.ImplementedBy[0].Covers += " Revised implementation scope."
		if omitExtra {
			claim.ImplementedBy[0].Extra = nil
		}
	case "evidence_inputs":
		claim.EvidenceInputs[0].Applicability += " Revised applicability."
		if omitExtra {
			claim.EvidenceInputs[0].Extra = nil
		}
	}
	return claim
}

func t01br4HasDiagnosticAt(ds []carrier.Diagnostic, code, path string) bool {
	for _, diagnostic := range ds {
		if diagnostic.Code == code && diagnostic.Path == path {
			return true
		}
	}
	return false
}

func TestT01BR4SameRefBindingEditsGuardRetainedExtensions(t *testing.T) {
	for _, field := range []string{"checks", "implemented_by", "evidence_inputs"} {
		for _, omit := range []bool{false, true} {
			name := field + "/copied"
			if omit {
				name = field + "/omitted"
			}
			t.Run(name, func(t *testing.T) {
				d, _, bases, c, index := t01br4BindingBasis(t, field, true)
				claim := t01br4BindingEdit(d.Record.Claims[index], field, omit)
				c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify only the binding scope"}}
				preview := Preview(c, bases)
				path := "claims[" + strconv.Itoa(index) + "]." + field + "[0].x-owner"
				if preview.Kind != "conflict" || !t01br4HasDiagnosticAt(preview.Diagnostics, "unsupported_yaml_tag_conversion", path) || len(preview.Outputs) != 0 || len(preview.Losses) != 0 {
					t.Fatalf("retained binding extension changed without a loss: %+v", preview)
				}
			})
		}
		t.Run(field+"/ordinary-control", func(t *testing.T) {
			d, _, bases, c, index := t01br4BindingBasis(t, field, false)
			claim := t01br4BindingEdit(d.Record.Claims[index], field, true)
			c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify only the binding scope"}}
			preview := Preview(c, bases)
			if preview.Kind != "ready" || len(preview.Outputs) != 1 || len(preview.Losses) != 0 {
				t.Fatalf("ordinary retained binding was refused: %+v", preview)
			}
			got := preview.Outputs[0].Successor.Claims[index]
			var owner any
			switch field {
			case "checks":
				owner = got.Checks[0].Extra["x-owner"]
			case "implemented_by":
				owner = got.ImplementedBy[0].Extra["x-owner"]
			case "evidence_inputs":
				owner = got.EvidenceInputs[0].Extra["x-owner"]
			}
			if owner != "abc" {
				t.Fatalf("ordinary extension was dropped: %+v", got)
			}
		})
	}
}

func TestT01BR4BindingReorderAndMultipleOperationsKeepSourcePaths(t *testing.T) {
	d, _, bases, c, _ := t01br4BindingBasis(t, "checks", true)
	claim := d.Record.Claims[0]
	second := carrier.Binding{Ref: "test:order_test.go::TestCancelStates", Covers: "State cases", Extra: carrier.Extra{"x-order": "second"}}
	claim.Checks = append(claim.Checks, second)
	claim.Checks[0], claim.Checks[1] = claim.Checks[1], claim.Checks[0]
	claim.Checks[1].Covers += " Clarified coverage."
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Add and reorder checks"}}
	preview := Preview(c, bases)
	if preview.Kind != "conflict" || !t01br4HasDiagnosticAt(preview.Diagnostics, "unsupported_yaml_tag_conversion", "claims[0].checks[0].x-owner") {
		t.Fatalf("reorder lost source correspondence: %+v", preview)
	}
	// An earlier binding removal must not exempt a finally retained value.
	removed := d.Record.Claims[0]
	removed.Checks = []carrier.Binding{second}
	c.Patches[0].Operations = []Operation{
		{Op: "MODIFIED", ClaimID: claim.ID, Claim: &removed, Reason: "Temporarily replace the binding"},
		{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Restore the old ref and clarify coverage"},
	}
	preview = Preview(c, bases)
	if preview.Kind != "conflict" || !t01br4HasDiagnosticAt(preview.Diagnostics, "unsupported_yaml_tag_conversion", "claims[0].checks[0].x-owner") {
		t.Fatalf("intermediate removal exempted retained value: %+v", preview)
	}
}

func TestT01BR4OrdinaryBindingReorderAndExplicitReplacement(t *testing.T) {
	d, _, _, c, _ := t01br4BindingBasis(t, "checks", false)
	d.Record.Claims[0].Checks = append(d.Record.Claims[0].Checks, carrier.Binding{
		Ref:    "test:order_test.go::TestCancelStates",
		Covers: "State cases",
		Extra:  carrier.Extra{"x-order": "second"},
	})
	raw, err := carrier.Encode(d.Record, d.Body)
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + digest
	c.Patches[0].Base = ref
	bases := map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}}
	original := carrier.Parse(raw).Record.Claims[0]
	reordered := original
	reordered.Checks = append([]carrier.Binding{}, original.Checks...)
	reordered.Checks[0], reordered.Checks[1] = reordered.Checks[1], reordered.Checks[0]
	reordered.Checks[1].Covers += " A narrower observed property."
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: original.ID, Claim: &reordered, Reason: "Reorder checks and clarify coverage"}}
	preview := Preview(c, bases)
	if preview.Kind != "ready" || len(preview.Outputs) != 1 || len(preview.Losses) != 0 {
		t.Fatalf("ordinary reorder was refused: %+v", preview)
	}
	got := preview.Outputs[0].Successor.Claims[0].Checks
	if got[0].Ref != "test:order_test.go::TestCancelStates" || got[0].Extra["x-order"] != "second" || got[1].Extra["x-owner"] != "abc" {
		t.Fatalf("reordered bindings lost their extensions: %+v", got)
	}
	replacement := original
	replacement.Checks = []carrier.Binding{original.Checks[1]}
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: original.ID, Claim: &replacement, Reason: "Replace the selected check"}}
	preview = Preview(c, bases)
	if preview.Kind != "ready" || len(preview.Outputs) != 1 || len(preview.Outputs[0].Successor.Claims[0].Checks) != 1 {
		t.Fatalf("explicit binding replacement was refused: %+v", preview)
	}
}

func TestT01BR4ExplicitRemovalOfTaggedBindingDoesNotBlock(t *testing.T) {
	d, _, bases, c, _ := t01br4BindingBasis(t, "checks", true)
	claim := d.Record.Claims[0]
	claim.Checks = []carrier.Binding{{Ref: "test:order_test.go::TestCancelStates", Covers: "State cases"}}
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Replace the original check with a new selector"}}
	preview := Preview(c, bases)
	if preview.Kind != "ready" || len(preview.Outputs) != 1 || len(preview.Outputs[0].Successor.Claims[0].Checks) != 1 {
		t.Fatalf("explicit binding replacement was confused with retained tag loss: %+v", preview)
	}
}

func TestT01BR4DuplicateBindingRefCannotWaiveRetainedExtension(t *testing.T) {
	d, _, _, c, _ := t01br4BindingBasis(t, "checks", true)
	duplicate := d.Record.Claims[0].Checks[0]
	duplicate.Covers = "Distinct second scope"
	d.Record.Claims[0].Checks = append(d.Record.Claims[0].Checks, duplicate)
	raw, err := carrier.Encode(d.Record, d.Body)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.ReplaceAll(string(raw), "x-owner: abc", "x-owner: !vendor abc"))
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + digest
	c.Patches[0].Base = ref
	claim := carrier.Parse(raw).Record.Claims[0]
	claim.Checks[0].Covers += " Clarified."
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify ambiguous duplicated selector"}}
	preview := Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "conflict" || !code(preview.Diagnostics, "ambiguous_binding_correspondence") || len(preview.Outputs) != 0 || len(preview.Losses) != 0 {
		t.Fatalf("duplicate refs gave an unsafe exemption: %+v", preview)
	}
}

func TestT01BR4DuplicateRefExtensionOnlyEditRefusesAmbiguity(t *testing.T) {
	d, _, _, c, _ := t01br4BindingBasis(t, "checks", false)
	duplicate := d.Record.Claims[0].Checks[0]
	duplicate.Covers = "Distinct second scope"
	d.Record.Claims[0].Checks = append(d.Record.Claims[0].Checks, duplicate)
	raw, err := carrier.Encode(d.Record, d.Body)
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + digest
	c.Patches[0].Base = ref
	claim := carrier.Parse(raw).Record.Claims[0]
	claim.Checks[0].Extra["x-owner"] = "changed"
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit one ambiguous extension"}}
	preview := Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "conflict" || len(preview.Outputs) != 0 || !t01br4HasDiagnosticAt(preview.Diagnostics, "ambiguous_binding_correspondence", c.Patches[0].Base+".operations[0].claim.checks") {
		t.Fatalf("duplicate refs silently accepted an extension-only edit: %+v", preview)
	}
}

func TestT01BR4DuplicateBindingRefAllowsUnchangedListButRefusesReorder(t *testing.T) {
	d, _, _, c, _ := t01br4BindingBasis(t, "checks", false)
	duplicate := d.Record.Claims[0].Checks[0]
	duplicate.Covers = "Distinct second scope"
	d.Record.Claims[0].Checks = append(d.Record.Claims[0].Checks, duplicate)
	raw, err := carrier.Encode(d.Record, d.Body)
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + digest
	c.Patches[0].Base = ref
	base := carrier.Parse(raw).Record.Claims[0]
	textOnly := base
	textOnly.Text += " Clarified prose."
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: base.ID, Claim: &textOnly, Reason: "Clarify prose without touching duplicated refs"}}
	bases := map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}}
	preview := Preview(c, bases)
	if preview.Kind != "ready" || len(preview.Outputs) != 1 {
		t.Fatalf("unchanged duplicate binding list was refused: %+v", preview)
	}
	reordered := base
	reordered.Checks = append([]carrier.Binding{}, base.Checks...)
	reordered.Checks[0], reordered.Checks[1] = reordered.Checks[1], reordered.Checks[0]
	c.Patches[0].Operations[0].Claim = &reordered
	preview = Preview(c, bases)
	if preview.Kind != "conflict" || !code(preview.Diagnostics, "ambiguous_binding_correspondence") {
		t.Fatalf("ambiguous duplicate-ref reorder was accepted: %+v", preview)
	}
}

func TestT01BR4ExplicitBindingExtensionEditKeepsSiblingsGuarded(t *testing.T) {
	d, _, bases, c, _ := t01br4BindingBasis(t, "checks", false)
	claim := d.Record.Claims[0]
	claim.Checks[0].Extra["x-owner"] = "changed"
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Correct the local extension"}}
	preview := Preview(c, bases)
	if preview.Kind != "ready" || len(preview.Outputs) != 1 || preview.Outputs[0].Successor.Claims[0].Checks[0].Extra["x-owner"] != "changed" {
		t.Fatalf("explicit extension edit was refused: %+v", preview)
	}
}

func TestT01BR4BindingKnownEditDoesNotExemptNestedBasisExtension(t *testing.T) {
	d, _, _, _ := setup(t)
	d.Record.Claims[0].Checks[0].InterpretationBasis = map[string]any{"scope": "old", "x-owner": "abc"}
	raw, err := carrier.Encode(d.Record, d.Body)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "x-owner: abc", "x-owner: !vendor abc", 1))
	parsed := carrier.Parse(raw)
	if !parsed.Valid() {
		t.Fatal(parsed.Diagnostics)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := parsed.Record.ID + "@" + digest
	c := Change{Format: Format, ID: "chg-20260926-00000001", ChangeKey: "chg-20260926-00000001", Title: "Clarify nested basis", Intent: "Retain unknown nested meaning", State: "open", CreatedAt: "2026-09-26T10:00:00Z", Patches: []SectionPatch{{Base: ref}}}
	claim := parsed.Record.Claims[0]
	claim.Checks[0].Covers += " Clarified."
	claim.Checks[0].InterpretationBasis.(map[string]any)["scope"] = "new"
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify known scope only"}}
	preview := Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "conflict" || !t01br4HasDiagnosticAt(preview.Diagnostics, "unsupported_yaml_tag_conversion", "claims[0].checks[0].interpretation_basis.x-owner") {
		t.Fatalf("nested retained value was waived: %+v", preview)
	}
}

func TestT01BR4OmittedNestedBasisRetainsOrdinaryValue(t *testing.T) {
	d, _, _, c := setup(t)
	d.Record.Claims[0].Checks[0].InterpretationBasis = map[string]any{"scope": "bounded", "x-owner": "abc"}
	raw, err := carrier.Encode(d.Record, d.Body)
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + digest
	c.Patches[0].Base = ref
	claim := carrier.Parse(raw).Record.Claims[0]
	claim.Checks[0].Covers += " Clarified."
	claim.Checks[0].InterpretationBasis = nil
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify coverage only"}}
	preview := Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "ready" || len(preview.Outputs) != 1 {
		t.Fatalf("ordinary omitted nested basis was refused: %+v", preview)
	}
	got := preview.Outputs[0].Successor.Claims[0].Checks[0].InterpretationBasis.(map[string]any)
	if got["scope"] != "bounded" || got["x-owner"] != "abc" {
		t.Fatalf("nested basis was lost: %+v", got)
	}
}

func TestT01BR4SameRefBindingExactNumberGuard(t *testing.T) {
	d, _, _, c, _ := t01br4BindingBasis(t, "checks", false)
	raw := strings.Replace(string(d.Raw), "x-owner: abc", "x-owner: 18446744073709551616", 1)
	_, snapshot, digest, err := carrier.NewSnapshot([]byte(raw), carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + digest
	c.Patches[0].Base = ref
	claim := t01br4BindingEdit(carrier.Parse([]byte(raw)).Record.Claims[0], "checks", false)
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify coverage only"}}
	preview := Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "conflict" || !t01br4HasDiagnosticAt(preview.Diagnostics, "unsupported_yaml_value_conversion", "claims[0].checks[0].x-owner") {
		t.Fatalf("large retained number lost exact type without refusal: %+v", preview)
	}
}
