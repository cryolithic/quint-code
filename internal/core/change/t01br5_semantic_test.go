package change

import (
	"bytes"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func t01br5Basis(t *testing.T, record carrier.Record, body []byte, rawReplacement func(string) string) (carrier.Document, string, map[string]Basis) {
	t.Helper()
	raw, err := carrier.Encode(record, body)
	if err != nil {
		t.Fatal(err)
	}
	if rawReplacement != nil {
		raw = []byte(rawReplacement(string(raw)))
	}
	doc := carrier.Parse(raw)
	if !doc.Valid() {
		t.Fatal(doc.Diagnostics)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := doc.Record.ID + "@" + digest
	return doc, ref, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}}
}

func TestT01BR5RevisionKeepsSameIDTaskMeaning(t *testing.T) {
	for _, tagged := range []bool{false, true} {
		for _, omit := range []bool{false, true} {
			name := "plain/copied"
			if tagged {
				name = "tagged/copied"
			}
			if omit {
				name = strings.Replace(name, "copied", "omitted", 1)
			}
			t.Run(name, func(t *testing.T) {
				field := ""
				if tagged {
					field = "tasks"
				}
				raw, snapshot, ref, bases := t01br4RevisionSource(t, field, "open")
				original := bytes.Clone(snapshot)
				edit := t01br4RevisionEdit("update", ref)
				intent := "A revised intent without task replacement"
				edit.Intent = &intent
				tasks := Parse(raw).Change.Tasks
				if omit {
					tasks[0].Extra = nil
				}
				edit.Tasks = &tasks
				got := Revise(ref, snapshot, edit, bases)
				if tagged && (got.Kind != "conflict" || !t01br4HasDiagnosticAt(got.Diagnostics, "unsupported_yaml_tag_conversion", "tasks[0].x-owner")) {
					t.Fatalf("same-ID tagged task was retagged: %+v", got)
				}
				if !tagged && (got.Kind != "ready" || got.Change.Tasks[0].Extra["x-owner"] != "abc") {
					t.Fatalf("same-ID ordinary task extension was lost: %+v", got)
				}
				if !bytes.Equal(snapshot, original) {
					t.Fatal("revision changed predecessor snapshot")
				}
			})
		}
	}
}

func TestT01BR5TaskFieldEditKeepsCopiedExtensionGuard(t *testing.T) {
	raw, snapshot, ref, bases := t01br4RevisionSource(t, "tasks", "open")
	edit := t01br4RevisionEdit("update", ref)
	tasks := Parse(raw).Change.Tasks
	tasks[0].Text = "Clarified task text"
	edit.Tasks = &tasks
	got := Revise(ref, snapshot, edit, bases)
	if got.Kind != "conflict" || !t01br4HasDiagnosticAt(got.Diagnostics, "unsupported_yaml_tag_conversion", "tasks[0].x-owner") {
		t.Fatalf("task field edit retagged copied extension: %+v", got)
	}
	// Omitting the extension while changing the same task's declared content is
	// an explicit whole-item replacement, retained from the earlier contract.
	tasks[0].Extra = nil
	got = Revise(ref, snapshot, edit, bases)
	if got.Kind != "ready" {
		t.Fatalf("explicit task replacement was refused: %+v", got)
	}
}

func TestT01BR5RevisionMatchesReorderedPatchesAndOperations(t *testing.T) {
	base, baseRef, bases, candidate := setup(t)
	second := base.Record
	second.ID = "spec-20260923-00000009"
	_, secondRef, secondBases := t01br5Basis(t, second, base.Body, nil)
	for ref, basis := range secondBases {
		bases[ref] = basis
	}
	firstClaim := base.Record.Claims[0]
	firstClaim.Text = "Clarified first claim"
	firstClaim.Checks[0].Extra = carrier.Extra{"x-owner": "abc"}
	secondClaim := base.Record.Claims[1]
	secondClaim.Text = "Clarified second claim"
	first := SectionPatch{Base: baseRef, Operations: []Operation{
		{Op: "MODIFIED", ClaimID: firstClaim.ID, Claim: &firstClaim, Reason: "First claim"},
		{Op: "MODIFIED", ClaimID: secondClaim.ID, Claim: &secondClaim, Reason: "Second claim"},
	}}
	other := SectionPatch{Base: secondRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: secondClaim.ID, Claim: &secondClaim, Reason: "Other section"}}}
	candidate.Patches = []SectionPatch{first, other}
	raw, err := Encode(candidate, []byte("Rationale.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("x-owner: abc")) {
		t.Fatal("reordered fixture omitted extension")
	}
	for _, tagged := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "tagged"}[tagged], func(t *testing.T) {
			source := raw
			if tagged {
				source = bytes.Replace(raw, []byte("x-owner: abc"), []byte("x-owner: !vendor abc"), 1)
			}
			_, snapshot, digest, err := carrier.NewSnapshot(source, carrier.InterpretationBasis{})
			if err != nil {
				t.Fatal(err)
			}
			ref := candidate.ID + "@" + digest
			parsed := Parse(source)
			if carrier.HasErrors(parsed.Diagnostics) {
				t.Fatal(parsed.Diagnostics)
			}
			edit := t01br4RevisionEdit("update", ref)
			reordered := parsed.Change.Patches
			reordered[0].Operations[0], reordered[0].Operations[1] = reordered[0].Operations[1], reordered[0].Operations[0]
			reordered[0], reordered[1] = reordered[1], reordered[0]
			edit.Patches = reordered
			intent := "Reorder without changing first claim meaning"
			edit.Intent = &intent
			result := Revise(ref, snapshot, edit, bases)
			if tagged && (result.Kind != "conflict" || !t01br4HasDiagnosticAt(result.Diagnostics, "unsupported_yaml_tag_conversion", "patches[0].operations[0].claim.checks[0].x-owner")) {
				t.Fatalf("reorder waived retained operation meaning: %+v", result)
			}
			if !tagged && result.Kind != "ready" {
				t.Fatalf("plain patch/operation reorder was refused: %+v", result)
			}
		})
	}
}

func TestT01BR5RebaseKeepsUnchangedNestedBindingMeaning(t *testing.T) {
	base, oldRef, _, c := setup(t)
	claim := changeClaim(base)
	claim.Checks[0].Extra = carrier.Extra{"x-owner": "abc"}
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify the claim"}}
	raw, err := Encode(c, []byte("Original rationale.\n"))
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "x-owner: abc", "x-owner: !vendor abc", 1))
	parsed := Parse(raw)
	if carrier.HasErrors(parsed.Diagnostics) {
		t.Fatal(parsed.Diagnostics)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	changeRef := c.ID + "@" + digest
	newBase := base.Record
	newBase.ID = "spec-20260923-00000009"
	newBase.Supersedes = []string{oldRef}
	newBase.SupersedeReason = "Advance the exact authored basis"
	_, nextRef, bases := t01br5Basis(t, newBase, base.Body, nil)
	if nextRef == oldRef {
		t.Fatal("rebase fixture did not move its exact base")
	}
	edit := t01br4RevisionEdit("rebase", changeRef)
	edit.Patches = parsed.Change.Patches
	edit.Patches[0].Base = nextRef
	got := Revise(changeRef, snapshot, edit, bases)
	if got.Kind != "conflict" || !t01br4HasDiagnosticAt(got.Diagnostics, "unsupported_yaml_tag_conversion", "patches[0].operations[0].claim.checks[0].x-owner") {
		t.Fatalf("rebase waived unchanged nested binding: %+v", got)
	}
}

func TestT01BR5NestedClaimAndExampleLeavesKeepTaggedSibling(t *testing.T) {
	for _, field := range []string{"claim", "example"} {
		t.Run(field, func(t *testing.T) {
			base, _, _, c := setup(t)
			old := carrier.Extra{"x-meta": map[string]any{"a": "abc", "b": 2}}
			if field == "claim" {
				base.Record.Claims[0].Extra = old
			} else {
				base.Record.Claims[0].Examples[0].Extra = old
			}
			doc, ref, bases := t01br5Basis(t, base.Record, base.Body, func(raw string) string {
				return strings.Replace(raw, "a: abc", "a: !vendor abc", 1)
			})
			c.Patches[0].Base = ref
			claim := doc.Record.Claims[0]
			if field == "claim" {
				claim.Extra["x-meta"].(map[string]any)["b"] = 3
			} else {
				claim.Examples[0].Extra["x-meta"].(map[string]any)["b"] = 3
			}
			c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit one mapping leaf"}}
			preview := Preview(c, bases)
			path := "claims[0].x-meta.a"
			if field == "example" {
				path = "claims[0].examples[0].x-meta.a"
			}
			if preview.Kind != "conflict" || !t01br4HasDiagnosticAt(preview.Diagnostics, "unsupported_yaml_tag_conversion", path) {
				t.Fatalf("unchanged nested sibling was retagged: %+v", preview)
			}
		})
	}
}

func TestT01BR5ExplicitExtensionAndExampleRemoval(t *testing.T) {
	base, _, _, candidate := setup(t)
	base.Record.Claims[0].Extra = carrier.Extra{"x-meta": map[string]any{"a": "abc", "b": 2}}
	base.Record.Claims[0].Examples[0].Extra = carrier.Extra{"x-owner": "abc"}
	doc, ref, bases := t01br5Basis(t, base.Record, base.Body, func(raw string) string {
		return strings.Replace(raw, "a: abc", "a: !vendor abc", 1)
	})
	candidate.Patches[0].Base = ref
	claim := doc.Record.Claims[0]
	claim.Extra["x-meta"].(map[string]any)["a"] = "authored replacement"
	candidate.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Replace the tagged leaf"}}
	if got := Preview(candidate, bases); got.Kind != "ready" {
		t.Fatalf("addressed leaf replacement was refused: %+v", got)
	}
	claim.Extra = nil
	candidate.Patches[0].Operations[0] = Operation{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, RemoveFields: []string{"x-meta"}, Reason: "Remove the whole extension"}
	if got := Preview(candidate, bases); got.Kind != "ready" || len(got.Outputs[0].Successor.Claims[0].Extra) != 0 {
		t.Fatalf("explicit extension removal was refused: %+v", got)
	}
	claim = doc.Record.Claims[0]
	claim.Examples = []carrier.Example{}
	candidate.Patches[0].Operations[0] = Operation{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, RemoveExamples: []string{"paid-order"}, Reason: "Remove the scenario"}
	if got := Preview(candidate, bases); got.Kind != "ready" || len(got.Outputs[0].Successor.Claims[0].Examples) != 0 {
		t.Fatalf("explicit scenario removal was refused: %+v", got)
	}
}

func TestT01BR5BindingSequenceElementEditAndRetainedTag(t *testing.T) {
	for _, tagged := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "tagged-sibling"}[tagged], func(t *testing.T) {
			base, _, _, c := setup(t)
			base.Record.Claims[0].Checks[0].Extra = carrier.Extra{"x-meta": []any{"a", "b"}}
			var replace func(string) string
			if tagged {
				replace = func(raw string) string { return strings.Replace(raw, "- a\n", "- !vendor a\n", 1) }
			}
			doc, ref, bases := t01br5Basis(t, base.Record, base.Body, replace)
			c.Patches[0].Base = ref
			claim := doc.Record.Claims[0]
			claim.Checks[0].Extra["x-meta"].([]any)[1] = "c"
			c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit second sequence element"}}
			preview := Preview(c, bases)
			if !tagged && preview.Kind != "ready" {
				t.Fatalf("ordinary sequence element edit was refused: %+v", preview)
			}
			if tagged && (preview.Kind != "conflict" || !t01br4HasDiagnosticAt(preview.Diagnostics, "unsupported_yaml_tag_conversion", "claims[0].checks[0].x-meta[0]")) {
				t.Fatalf("retained tagged sequence sibling was lost: %+v", preview)
			}
		})
	}
}

func TestT01BR5PlainDuplicateBindingsAllowEditsAndRemoval(t *testing.T) {
	for _, field := range []string{"checks", "implemented_by", "evidence_inputs"} {
		for _, action := range []string{"edit", "add", "remove", "replace"} {
			t.Run(field+"/"+action, func(t *testing.T) {
				base, _, _, c := setup(t)
				claim := &base.Record.Claims[0]
				switch field {
				case "checks":
					second := claim.Checks[0]
					second.Covers = "Distinct second coverage"
					claim.Checks = append(claim.Checks, second)
				case "implemented_by":
					second := claim.ImplementedBy[0]
					second.Covers = "Distinct second implementation"
					claim.ImplementedBy = append(claim.ImplementedBy, second)
				case "evidence_inputs":
					claim = &base.Record.Claims[2]
					claim.EvidenceInputs = []carrier.EvidenceInput{{Ref: "ev-20260926-00000001@" + carrier.Digest([]byte("evidence")) + "#check-1", Applicability: "First scope"}}
					second := claim.EvidenceInputs[0]
					second.Applicability = "Distinct second scope"
					claim.EvidenceInputs = append(claim.EvidenceInputs, second)
				}
				doc, ref, bases := t01br5Basis(t, base.Record, base.Body, nil)
				c.Patches[0].Base = ref
				index := 0
				if field == "evidence_inputs" {
					index = 2
				}
				changed := doc.Record.Claims[index]
				switch field {
				case "checks":
					changed.Checks = t01br5ChangeBindings(changed.Checks, action)
				case "implemented_by":
					changed.ImplementedBy = t01br5ChangeBindings(changed.ImplementedBy, action)
				case "evidence_inputs":
					changed.EvidenceInputs = t01br5ChangeEvidence(changed.EvidenceInputs, action)
				}
				c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: changed.ID, Claim: &changed, Reason: "Edit valid duplicate selector content"}}
				preview := Preview(c, bases)
				if preview.Kind != "ready" || len(preview.Outputs) != 1 {
					t.Fatalf("plain duplicate %s was refused: %+v", action, preview)
				}
			})
		}
	}
}

func t01br5ChangeBindings(before []carrier.Binding, action string) []carrier.Binding {
	updated := append([]carrier.Binding{}, before...)
	switch action {
	case "edit":
		updated[1].Covers = "Edited second coverage"
	case "add":
		third := updated[0]
		third.Covers = "Third coverage"
		updated = append(updated, third)
	case "remove":
		return []carrier.Binding{}
	case "replace":
		ref := "test:other_test.go::TestOther"
		if strings.HasPrefix(updated[0].Ref, "sym:") {
			ref = "sym:order.go::Order.Replace"
		}
		return []carrier.Binding{{Ref: ref, Covers: "Replacement coverage"}}
	}
	return updated
}

func t01br5ChangeEvidence(before []carrier.EvidenceInput, action string) []carrier.EvidenceInput {
	updated := append([]carrier.EvidenceInput{}, before...)
	switch action {
	case "edit":
		updated[1].Applicability = "Edited second scope"
	case "add":
		third := updated[0]
		third.Applicability = "Third scope"
		updated = append(updated, third)
	case "remove":
		return []carrier.EvidenceInput{}
	case "replace":
		return []carrier.EvidenceInput{{Ref: "ev-20260926-00000001@" + carrier.Digest([]byte("other")) + "#check-2", Applicability: "Replacement scope"}}
	}
	return updated
}
