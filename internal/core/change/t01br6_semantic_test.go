package change

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func t01br6ChangeSnapshot(t *testing.T, candidate Change) ([]byte, []byte, string) {
	t.Helper()
	raw, err := Encode(candidate, []byte("Original rationale.\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, digest, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	return raw, snapshot, candidate.ID + "@" + digest
}

func TestT01BR6OmittedExamplesSurviveRevisionAndExplicitRemoval(t *testing.T) {
	base, ref, bases, candidate := setup(t)
	claim := changeClaim(base)
	claim.Examples = nil
	candidate.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify the rule"}}
	ready(t, Preview(candidate, bases))
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, candidate)
	for _, supplied := range []bool{false, true} {
		edit := t01br4RevisionEdit("update", changeRef)
		intent := "Clarified intent"
		edit.Intent = &intent
		if supplied {
			edit.Patches = candidate.Patches
		}
		result := Revise(changeRef, snapshot, edit, bases)
		if result.Kind != "ready" || result.Change.Patches[0].Operations[0].Claim.Examples != nil || bytes.Contains(result.Raw, []byte("examples: []")) {
			t.Fatalf("omitted scenarios became an authored empty list: %+v", result)
		}
		preview := Preview(result.Change, bases)
		if preview.Kind != "ready" || len(preview.Outputs[0].Successor.Claims[0].Examples) != 1 {
			t.Fatalf("revised proposal lost the base scenario: %+v", preview)
		}
	}
	next := base.Record
	next.ID = "spec-20260923-00000008"
	next.Supersedes = []string{ref}
	next.SupersedeReason = "Advance the exact rule basis"
	next.WriteReceipt = nil
	_, nextRef, nextBases := t01br5Basis(t, next, base.Body, nil)
	edit := t01br4RevisionEdit("rebase", changeRef)
	edit.Patches = []SectionPatch{{Base: nextRef, Operations: candidate.Patches[0].Operations}}
	result := Revise(changeRef, snapshot, edit, nextBases)
	if result.Kind != "ready" || result.Change.Patches[0].Operations[0].Claim.Examples != nil || result.Preview == nil || result.Preview.Kind != "ready" {
		t.Fatalf("rebased omitted scenarios were lost: %+v", result)
	}
	if got := result.Preview.Outputs[0].Successor.Claims[0].Examples; len(got) != 1 || got[0].ID != "paid-order" {
		t.Fatalf("rebased scenario is absent: %+v", got)
	}
	claim.Examples = []carrier.Example{}
	candidate.Patches[0].Operations[0].Claim = &claim
	if preview := Preview(candidate, bases); preview.Kind != "conflict" || !code(preview.Diagnostics, "scenario_loss") {
		t.Fatalf("explicit empty list silently removed a scenario: %+v", preview)
	}
	candidate.Patches[0].Operations[0].RemoveExamples = []string{"paid-order"}
	preview := Preview(candidate, bases)
	if preview.Kind != "ready" || len(preview.Losses) != 1 || preview.Losses[0].Kind != "example_removed" {
		t.Fatalf("named scenario removal was refused or hidden: %+v", preview)
	}
}

func TestT01BR6DeclaredListsAndNamedExampleRemoval(t *testing.T) {
	base, _, _, candidate := setup(t)
	base.Record.Claims[0].Examples = append(base.Record.Claims[0].Examples, carrier.Example{ID: "new-sample", Text: "Another case"})
	base.Record.Claims[0].Extra = carrier.Extra{"x-retired": "Keep unless removed"}
	doc, ref, bases := t01br5Basis(t, base.Record, base.Body, nil)
	claim := changeClaim(doc)
	claim.Examples = []carrier.Example{}
	candidate.Tasks = []Task{{ID: "check", Text: "Inspect result", Results: []string{"first", "second"}, Extra: carrier.Extra{"x-owner": "local"}}}
	candidate.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Remove scenario", RemoveExamples: []string{"paid-order"}, RemoveFields: []string{"x-retired"}}}
	candidate.Patches[0].Base = ref
	// The old proposal removes one scenario and the extension. The revision
	// removes both scenarios while retaining the extension.
	claim.Examples = []carrier.Example{{ID: "new-sample", Text: "Another case"}}
	candidate.Patches[0].Operations[0].Claim = &claim
	ready(t, Preview(candidate, bases))
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, candidate)
	parsed := Parse(candidateRaw(t, candidate)).Change
	parsed.Tasks[0].Results = nil
	parsed.Patches[0].Operations[0].RemoveFields = nil
	parsed.Patches[0].Operations[0].RemoveExamples = append([]string{"new-sample"}, parsed.Patches[0].Operations[0].RemoveExamples...)
	parsed.Patches[0].Operations[0].Claim.Examples = []carrier.Example{}
	edit := t01br4RevisionEdit("update", changeRef)
	edit.Tasks = &parsed.Tasks
	edit.Patches = parsed.Patches
	result := Revise(changeRef, snapshot, edit, bases)
	if result.Kind != "ready" || len(result.Change.Tasks[0].Results) != 0 || len(result.Change.Patches[0].Operations[0].RemoveFields) != 0 || !reflect.DeepEqual(result.Change.Patches[0].Operations[0].RemoveExamples, []string{"new-sample", "paid-order"}) {
		t.Fatalf("actual list edits were refused or altered: %+v", result)
	}
	if result.Change.Tasks[0].Extra["x-owner"] != "local" {
		t.Fatalf("task extension was dropped: %+v", result.Change.Tasks[0])
	}
	preview := Preview(result.Change, bases)
	if preview.Kind != "ready" || len(preview.Outputs[0].Successor.Claims[0].Examples) != 0 || len(preview.Losses) != 2 || preview.Outputs[0].Successor.Claims[0].Extra["x-retired"] != "Keep unless removed" {
		t.Fatalf("revised named removal failed preview: %+v", preview)
	}
}

func candidateRaw(t *testing.T, candidate Change) []byte {
	t.Helper()
	raw, err := Encode(candidate, []byte("Original rationale.\n"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestT01BR6RepeatedSequencePositionalEdit(t *testing.T) {
	base, _, _, candidate := setup(t)
	base.Record.Claims[0].Extra = carrier.Extra{"x-steps": []any{"a", "a"}}
	doc, ref, bases := t01br5Basis(t, base.Record, base.Body, nil)
	claim := doc.Record.Claims[0]
	claim.Extra["x-steps"] = []any{"a", "b"}
	candidate.Patches[0].Base = ref
	candidate.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit the second step"}}
	preview := Preview(candidate, bases)
	if preview.Kind != "ready" || !reflect.DeepEqual(preview.Outputs[0].Successor.Claims[0].Extra["x-steps"], []any{"a", "b"}) {
		t.Fatalf("unambiguous repeated value edit was refused: %+v", preview)
	}
	claim.Extra["x-steps"] = []any{"a", "a"}
	candidate.Patches[0].Operations[0].Claim = &claim
	if got := Preview(candidate, bases); got.Kind != "ready" {
		t.Fatalf("unchanged repeated sequence was refused: %+v", got)
	}
}

func TestT01BR6WholeTaskReplacementReportsRemovedExtensions(t *testing.T) {
	raw, snapshot, ref, bases := t01br4RevisionSource(t, "tasks", "open")
	tasks := Parse(raw).Change.Tasks
	tasks[0].Text = "Replace the whole task"
	tasks[0].Extra = nil
	edit := t01br4RevisionEdit("update", ref)
	edit.Tasks = &tasks
	result := Revise(ref, snapshot, edit, bases)
	if result.Kind != "ready" || !reflect.DeepEqual(result.Losses, []RevisionLoss{{Kind: "task_extension_replaced", Path: "tasks[0]", RemovedFields: []string{"x-owner"}, Reason: "Changed task omitted its extension map; this is a whole-task replacement"}}) {
		t.Fatalf("whole-task replacement has no honest loss receipt: %+v", result)
	}
	if len(result.Change.Tasks[0].Extra) != 0 {
		t.Fatalf("whole-task replacement retained withdrawn extensions: %+v", result.Change.Tasks[0])
	}
	unchangedTasks := Parse(raw).Change.Tasks
	edit.Tasks = &unchangedTasks
	unchanged := Revise(ref, snapshot, edit, bases)
	if unchanged.Kind != "conflict" || len(unchanged.Losses) != 0 {
		t.Fatalf("unchanged tagged task was treated as replacement: %+v", unchanged)
	}
}

func TestT01BR6UnchangedTaskEmptyResultsRetainsOmittedOrdinaryExtension(t *testing.T) {
	raw, snapshot, ref, bases := t01br4RevisionSource(t, "", "open")
	tasks := Parse(raw).Change.Tasks
	tasks[0].Results = []string{}
	tasks[0].Extra = nil
	edit := t01br4RevisionEdit("update", ref)
	edit.Tasks = &tasks
	result := Revise(ref, snapshot, edit, bases)
	if result.Kind != "ready" || len(result.Losses) != 0 || result.Change.Tasks[0].Extra["x-owner"] != "abc" {
		t.Fatalf("semantic no-op in results replaced task metadata: %+v", result)
	}
}

func TestT01BR6DisjointDuplicateBindingReplacementKeepsMixedGuard(t *testing.T) {
	for _, field := range []string{"checks", "implemented_by", "evidence_inputs"} {
		t.Run(field, func(t *testing.T) {
			base, _, _, candidate := setup(t)
			claim := base.Record.Claims[0]
			old := carrier.Binding{Ref: "test:old_test.go::TestOld", Covers: "Old scope", Extra: carrier.Extra{"x-owner": "local"}}
			switch field {
			case "checks":
				claim.Checks = []carrier.Binding{old, old}
			case "implemented_by":
				old.Ref = "sym:old.go::Old"
				claim.ImplementedBy = []carrier.Binding{old, old}
			case "evidence_inputs":
				claim.Kind = "guard"
				claim.EvidenceInputs = []carrier.EvidenceInput{{Ref: "ev-20260927-00000001@" + carrier.Digest([]byte("evidence")) + "#use", Applicability: "One", Extra: carrier.Extra{"x-owner": "local"}}, {Ref: "ev-20260927-00000001@" + carrier.Digest([]byte("evidence")) + "#use", Applicability: "Two", Extra: carrier.Extra{"x-owner": "local"}}}
			}
			base.Record.Claims[0] = claim
			doc, ref, bases := t01br5Basis(t, base.Record, base.Body, nil)
			changed := doc.Record.Claims[0]
			switch field {
			case "checks":
				changed.Checks = []carrier.Binding{{Ref: "test:new_test.go::TestNew", Covers: "New scope"}}
			case "implemented_by":
				changed.ImplementedBy = []carrier.Binding{{Ref: "sym:new.go::New", Covers: "New implementation"}}
			case "evidence_inputs":
				changed.EvidenceInputs = []carrier.EvidenceInput{{Ref: "ev-20260927-00000002@" + carrier.Digest([]byte("new evidence")) + "#use", Applicability: "New scope"}}
			}
			candidate.Patches[0].Base = ref
			candidate.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: changed.ID, Claim: &changed, Reason: "Replace the exact list"}}
			preview := Preview(candidate, bases)
			if preview.Kind != "ready" {
				t.Fatalf("disjoint replacement refused: %+v", preview)
			}
			mixed := changed
			switch field {
			case "checks":
				mixed.Checks = []carrier.Binding{doc.Record.Claims[0].Checks[0], changed.Checks[0]}
			case "implemented_by":
				mixed.ImplementedBy = []carrier.Binding{doc.Record.Claims[0].ImplementedBy[0], changed.ImplementedBy[0]}
			case "evidence_inputs":
				mixed.EvidenceInputs = []carrier.EvidenceInput{doc.Record.Claims[0].EvidenceInputs[0], changed.EvidenceInputs[0]}
			}
			candidate.Patches[0].Operations[0].Claim = &mixed
			if refused := Preview(candidate, bases); refused.Kind != "conflict" || !code(refused.Diagnostics, "ambiguous_binding_correspondence") {
				t.Fatalf("mixed retained/removed duplicate meaning was accepted: %+v", refused)
			}
		})
	}
}

func TestT01BR6RebaseUsesExactLineageAcrossReorderedPatches(t *testing.T) {
	fixture, _, _, candidate := setup(t)
	oldAlpha := fixture.Record
	oldAlpha.ID = "spec-20260923-00000041"
	oldAlpha.Claims = []carrier.Claim{{ID: "rule", Kind: "definition", Text: "Alpha rule"}}
	_, alphaRef, alphaOldBases := t01br5Basis(t, oldAlpha, fixture.Body, nil)
	oldBeta := fixture.Record
	oldBeta.ID = "spec-20260923-00000042"
	oldBeta.Claims = []carrier.Claim{{ID: "rule", Kind: "definition", Text: "Beta rule"}, {ID: "fee", Kind: "definition", Text: "Beta fee"}}
	_, betaRef, betaOldBases := t01br5Basis(t, oldBeta, fixture.Body, nil)
	alphaRule := oldAlpha.Claims[0]
	alphaRule.Text = "Revised alpha rule"
	betaRule := oldBeta.Claims[0]
	betaRule.Text = "Revised beta rule"
	betaFee := oldBeta.Claims[1]
	betaFee.Text = "Revised beta fee"
	candidate.Patches = []SectionPatch{
		{Base: alphaRef, Extra: carrier.Extra{"x-owner": "ALPHA_ONLY"}, Operations: []Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &alphaRule, Reason: "Edit alpha"}}},
		{Base: betaRef, Extra: carrier.Extra{"x-owner": "BETA_ONLY"}, Operations: []Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaRule, Reason: "Edit beta rule"}, {Op: "MODIFIED", ClaimID: "fee", Claim: &betaFee, Reason: "Edit beta fee"}}},
	}
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, candidate)
	newAlpha := oldAlpha
	newAlpha.ID = "spec-20260923-00000043"
	newAlpha.Supersedes = []string{alphaRef}
	newAlpha.SupersedeReason = "Exact alpha successor"
	newAlpha.Claims = []carrier.Claim{{ID: "rule2", Kind: "definition", Text: "Alpha rule two"}}
	_, newAlphaRef, alphaBases := t01br5Basis(t, newAlpha, fixture.Body, nil)
	newBeta := oldBeta
	newBeta.ID = "spec-20260923-00000044"
	newBeta.Supersedes = []string{betaRef}
	newBeta.SupersedeReason = "Exact beta successor"
	_, newBetaRef, betaBases := t01br5Basis(t, newBeta, fixture.Body, nil)
	for ref, basis := range betaBases {
		alphaBases[ref] = basis
	}
	alphaParsed, _ := carrier.ParseRef(alphaRef)
	betaParsed, _ := carrier.ParseRef(betaRef)
	captured := map[string][]byte{alphaParsed.Digest: alphaOldBases[alphaRef].Snapshot, betaParsed.Digest: betaOldBases[betaRef].Snapshot}
	for ref, basis := range alphaBases {
		basis.CapturedSnapshots = captured
		alphaBases[ref] = basis
	}
	alphaRule2 := newAlpha.Claims[0]
	alphaRule2.Text = "Revised alpha rule two"
	edit := t01br4RevisionEdit("rebase", changeRef)
	edit.Patches = []SectionPatch{
		{Base: newBetaRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaRule, Reason: "Edit beta rule"}, {Op: "MODIFIED", ClaimID: "fee", Claim: &betaFee, Reason: "Edit beta fee"}}},
		{Base: newAlphaRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: "rule2", Claim: &alphaRule2, Reason: "Edit alpha again"}}},
	}
	result := Revise(changeRef, snapshot, edit, alphaBases)
	if result.Kind != "ready" || result.Preview == nil || result.Preview.Kind != "ready" {
		t.Fatalf("exact multi-patch rebase failed: %+v", result)
	}
	if got := []any{result.Change.Patches[0].Extra["x-owner"], result.Change.Patches[1].Extra["x-owner"]}; !reflect.DeepEqual(got, []any{"BETA_ONLY", "ALPHA_ONLY"}) {
		t.Fatalf("patch ownership crossed lineages: %+v", got)
	}
	if got := Parse(result.Raw).Change.Patches; got[0].Extra["x-owner"] != "BETA_ONLY" || got[1].Extra["x-owner"] != "ALPHA_ONLY" {
		t.Fatalf("saved change has crossed owners: %+v", got)
	}
	if got := result.Preview.Outputs; len(got) != 2 || got[0].Successor.Claims[1].Text != "Revised beta fee" || got[1].Successor.Claims[0].ID != "rule2" {
		t.Fatalf("rebased effects differ from the selected lineages: %+v", got)
	}
	// Removing alpha while adding an unrelated section is a pair of actual
	// patch edits. Its old owner must not be copied to the new section.
	other := oldAlpha
	other.ID = "spec-20260923-00000047"
	other.Claims = []carrier.Claim{{ID: "other", Kind: "definition", Text: "Other rule"}}
	_, otherRef, otherBases := t01br5Basis(t, other, fixture.Body, nil)
	for ref, basis := range alphaBases {
		otherBases[ref] = basis
	}
	otherClaim := other.Claims[0]
	otherClaim.Text = "Edited other rule"
	edit.Patches = []SectionPatch{edit.Patches[0], {Base: otherRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: "other", Claim: &otherClaim, Reason: "Edit other"}}}}
	replaced := Revise(changeRef, snapshot, edit, otherBases)
	if replaced.Kind != "ready" || replaced.Change.Patches[0].Extra["x-owner"] != "BETA_ONLY" || len(replaced.Change.Patches[1].Extra) != 0 {
		t.Fatalf("unmatched patch removal/addition crossed owners: %+v", replaced)
	}
	// No exact lineage and a shared operation context leave correspondence
	// genuinely unresolved; never select a patch by array order or score.
	uncertain := oldBeta
	uncertain.ID = "spec-20260923-00000045"
	uncertain.Supersedes = nil
	uncertain.SupersedeReason = ""
	_, uncertainRef, uncertainBases := t01br5Basis(t, uncertain, fixture.Body, nil)
	uncertainOther := uncertain
	uncertainOther.ID = "spec-20260923-00000046"
	_, uncertainOtherRef, otherBases := t01br5Basis(t, uncertainOther, fixture.Body, nil)
	for ref, basis := range otherBases {
		uncertainBases[ref] = basis
	}
	edit.Patches = []SectionPatch{{Base: uncertainRef, Operations: result.Change.Patches[0].Operations}, {Base: uncertainOtherRef, Operations: result.Change.Patches[0].Operations}}
	refused := Revise(changeRef, snapshot, edit, uncertainBases)
	if refused.Kind != "conflict" || !code(refused.Diagnostics, "ambiguous_revision_correspondence") {
		t.Fatalf("unproven competing correspondence was accepted: %+v", refused)
	}
}

func TestT01BR6SameOperationContextAloneCannotMovePatchOwner(t *testing.T) {
	base, oldRef, _, candidate := setup(t)
	claim := changeClaim(base)
	candidate.Patches[0].Extra = carrier.Extra{"x-owner": "OLD_ONLY"}
	candidate.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit old section"}}
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, candidate)
	unrelated := base.Record
	unrelated.ID = "spec-20260923-00000048"
	unrelated.Supersedes = nil
	unrelated.SupersedeReason = ""
	_, unrelatedRef, bases := t01br5Basis(t, unrelated, base.Body, nil)
	if unrelatedRef == oldRef {
		t.Fatal("unrelated base fixture retained exact identity")
	}
	edit := t01br4RevisionEdit("rebase", changeRef)
	edit.Patches = []SectionPatch{{Base: unrelatedRef, Operations: candidate.Patches[0].Operations}}
	result := Revise(changeRef, snapshot, edit, bases)
	if result.Kind != "conflict" || !t01br4HasDiagnosticAt(result.Diagnostics, "ambiguous_revision_correspondence", "patches[0]") || len(result.Raw) != 0 {
		t.Fatalf("one overlapping operation cross-copied an unproven owner: %+v", result)
	}
}
