package change

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func TestT01BR7ExactCapturedTransitiveAncestry(t *testing.T) {
	base, oldRef, oldBases, original := setup(t)
	oldClaim := changeClaim(base)
	original.Patches[0].Extra = carrier.Extra{"x-owner": "ORIGINAL_PATCH_OWNER"}
	original.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: oldClaim.ID, Claim: &oldClaim, Reason: "Edit original"}}
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, original)

	middle := base.Record
	middle.ID = "spec-20260923-00000071"
	middle.Supersedes = []string{oldRef}
	middle.SupersedeReason = "First exact successor"
	middle.WriteReceipt = nil
	middle.Claims = append([]carrier.Claim(nil), base.Record.Claims...)
	middle.Claims[0].ID = "rule2"
	middle.RetiredClaimIDs = []string{base.Record.Claims[0].ID}
	_, middleRef, middleBases := t01br5Basis(t, middle, base.Body, nil)

	current := middle
	current.ID = "spec-20260923-00000072"
	current.Supersedes = []string{middleRef}
	current.SupersedeReason = "Second exact successor"
	current.Claims = append([]carrier.Claim(nil), middle.Claims...)
	current.Claims[0].Text = "Current rule two"
	current.RetiredClaimIDs = nil
	currentDoc, currentRef, currentBases := t01br5Basis(t, current, base.Body, nil)
	currentBasis := currentBases[currentRef]
	oldDigest := strings.Split(oldRef, "@")[1]
	middleDigest := strings.Split(middleRef, "@")[1]
	currentBasis.CapturedSnapshots = map[string][]byte{oldDigest: oldBases[oldRef].Snapshot, middleDigest: middleBases[middleRef].Snapshot}
	currentBases[currentRef] = currentBasis
	currentBases[oldRef] = oldBases[oldRef]

	claim := currentDoc.Record.Claims[0]
	claim.Text = "Rebased rule two"
	edit := t01br4RevisionEdit("rebase", changeRef)
	edit.Patches = []SectionPatch{{Base: currentRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit current"}}}}
	result := Revise(changeRef, snapshot, edit, currentBases)
	if result.Kind != "ready" || result.Preview == nil || result.Preview.Kind != "ready" || result.Change.Patches[0].Extra["x-owner"] != "ORIGINAL_PATCH_OWNER" {
		t.Fatalf("exact two-step rename lost patch ownership: %+v", result)
	}
	if got := result.Preview.Outputs[0].Successor.Claims[0]; got.ID != "rule2" || got.Text != "Rebased rule two" {
		t.Fatalf("wrong selected successor content: %+v", got)
	}

	incomplete := currentBases[currentRef]
	incomplete.CapturedSnapshots = nil
	refused := Revise(changeRef, snapshot, edit, map[string]Basis{currentRef: incomplete})
	if refused.Kind != "conflict" || !code(refused.Diagnostics, "ambiguous_revision_correspondence") || len(refused.Raw) != 0 {
		t.Fatalf("incomplete intermediate ancestry was guessed: %+v", refused)
	}

	merged := current
	merged.ID = "spec-20260923-00000073"
	merged.Supersedes = []string{middleRef, oldRef}
	merged.SupersedeReason = "Merge branches"
	mergedDoc, mergedRef, mergedBases := t01br5Basis(t, merged, base.Body, nil)
	mergedBasis := mergedBases[mergedRef]
	mergedBasis.CapturedSnapshots = map[string][]byte{oldDigest: oldBases[oldRef].Snapshot, middleDigest: middleBases[middleRef].Snapshot}
	mergedBases[mergedRef] = mergedBasis
	edit.Patches[0].Base = mergedRef
	edit.Patches[0].Operations[0].Claim = &mergedDoc.Record.Claims[0]
	mergedResult := Revise(changeRef, snapshot, edit, mergedBases)
	if mergedResult.Kind != "ready" || mergedResult.Change.Patches[0].Extra["x-owner"] != "ORIGINAL_PATCH_OWNER" {
		t.Fatalf("convergent exact ancestry lost its one patch owner: %+v", mergedResult)
	}
	branch := middle
	branch.ID = "spec-20260923-00000076"
	branch.Supersedes = []string{oldRef}
	branch.SupersedeReason = "Competing exact successor"
	branchDoc, branchRef, branchBases := t01br5Basis(t, branch, base.Body, nil)
	branchBasis := branchBases[branchRef]
	branchBasis.CapturedSnapshots = map[string][]byte{oldDigest: oldBases[oldRef].Snapshot}
	branchBases[branchRef] = branchBasis
	middleBasis := middleBases[middleRef]
	middleBasis.CapturedSnapshots = map[string][]byte{oldDigest: oldBases[oldRef].Snapshot}
	middleBases[middleRef] = middleBasis
	edit.Patches = []SectionPatch{
		{Base: middleRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: "rule2", Claim: &middle.Claims[0], Reason: "Edit first branch"}}},
		{Base: branchRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: "rule2", Claim: &branchDoc.Record.Claims[0], Reason: "Edit second branch"}}},
	}
	for ref, basis := range middleBases {
		branchBases[ref] = basis
	}
	refused = Revise(changeRef, snapshot, edit, branchBases)
	if refused.Kind != "conflict" || !code(refused.Diagnostics, "ambiguous_revision_correspondence") || len(refused.Raw) != 0 {
		t.Fatalf("one old patch chose a competing exact branch: %+v", refused)
	}

	unrelated := base.Record
	unrelated.ID = "spec-20260923-00000074"
	unrelated.Supersedes = nil
	unrelated.SupersedeReason = ""
	unrelated.Claims = []carrier.Claim{{ID: "other", Kind: "definition", Text: "Independent content"}}
	unrelatedDoc, unrelatedRef, unrelatedBases := t01br5Basis(t, unrelated, base.Body, nil)
	newClaim := unrelatedDoc.Record.Claims[0]
	newClaim.Text = "Edited independent content"
	edit.Patches = []SectionPatch{{Base: unrelatedRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: "other", Claim: &newClaim, Reason: "Add independent patch"}}}}
	replaced := Revise(changeRef, snapshot, edit, unrelatedBases)
	if replaced.Kind != "ready" || len(replaced.Change.Patches[0].Extra) != 0 {
		t.Fatalf("unrelated explicit replacement copied the old owner: %+v", replaced)
	}
}

func TestT01BR7RebaseKeepsExactBaseDiagnostics(t *testing.T) {
	base, oldRef, _, original := setup(t)
	claim := changeClaim(base)
	original.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit old"}}
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, original)
	next := base.Record
	next.ID = "spec-20260923-00000075"
	next.Supersedes = []string{oldRef}
	next.SupersedeReason = "Next exact edition"
	next.WriteReceipt = nil
	_, nextRef, nextBases := t01br5Basis(t, next, base.Body, nil)
	claim.Text = "Edit next"
	edit := t01br4RevisionEdit("rebase", changeRef)
	edit.Patches = []SectionPatch{{Base: nextRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit next"}}}}
	valid := nextBases[nextRef]
	cases := []struct {
		name  string
		bases map[string]Basis
		code  string
	}{
		{"missing", nil, "missing_authored_base"},
		{"corrupt", map[string]Basis{nextRef: {Snapshot: []byte("corrupt"), CurrentRef: nextRef}}, "invalid_authored_snapshot"},
		{"stale", map[string]Basis{nextRef: {Snapshot: valid.Snapshot, CurrentRef: oldRef}}, "stale_authored_base"},
		{"unknown-head", map[string]Basis{nextRef: {Snapshot: valid.Snapshot}}, "current_head_unknown"},
		{"multiple-heads", map[string]Basis{nextRef: {Snapshot: valid.Snapshot, Contested: []string{oldRef, nextRef}}}, "contested_authored_base"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Revise(changeRef, snapshot, edit, tc.bases)
			if result.Kind != "conflict" || !code(result.Diagnostics, tc.code) || code(result.Diagnostics, "ambiguous_revision_correspondence") || len(result.Raw) != 0 {
				t.Fatalf("exact base diagnostic was masked: %+v", result)
			}
		})
	}
}

func TestT01BR7ProposalOnlyExampleEdits(t *testing.T) {
	for _, operationKind := range []string{"MODIFIED", "ADDED"} {
		for _, editKind := range []string{"partial", "clear", "rename"} {
			t.Run(operationKind+"/"+editKind, func(t *testing.T) {
				base, _, bases, original := setup(t)
				claim := changeClaim(base)
				if operationKind == "ADDED" {
					claim = carrier.Claim{ID: "new-rule", Kind: "definition", Text: "New rule"}
				}
				claim.Examples = []carrier.Example{{ID: "keep", Text: "Keep proposed scenario", Extra: carrier.Extra{"x-owner": "retained"}}, {ID: "drop", Text: "Withdraw proposed scenario"}}
				operation := Operation{Op: operationKind, Claim: &claim, Reason: "Propose examples"}
				if operationKind == "MODIFIED" {
					operation.ClaimID = claim.ID
					claim.Examples = append([]carrier.Example{base.Record.Claims[0].Examples[0]}, claim.Examples...)
				}
				original.Patches[0].Operations = []Operation{operation}
				ready(t, Preview(original, bases))
				_, snapshot, changeRef := t01br6ChangeSnapshot(t, original)
				revised := Parse(candidateRaw(t, original)).Change.Patches
				claimNext := *revised[0].Operations[0].Claim
				switch editKind {
				case "partial":
					claimNext.Examples = claimNext.Examples[:len(claimNext.Examples)-1]
				case "clear":
					if operationKind == "MODIFIED" {
						claimNext.Examples = claimNext.Examples[:1]
					} else {
						claimNext.Examples = []carrier.Example{}
					}
				case "rename":
					claimNext.Examples[len(claimNext.Examples)-1].ID = "renamed"
				}
				revised[0].Operations[0].Claim = &claimNext
				edit := t01br4RevisionEdit("update", changeRef)
				edit.Patches = revised
				result := Revise(changeRef, snapshot, edit, bases)
				if result.Kind != "ready" {
					t.Fatalf("explicit proposal-only example edit refused: %+v", result)
				}
				preview := Preview(result.Change, bases)
				if preview.Kind != "ready" {
					t.Fatalf("revised examples cannot publish: %+v", preview)
				}
				var published carrier.Claim
				for _, next := range preview.Outputs[0].Successor.Claims {
					if next.ID == claimNext.ID {
						published = next
					}
				}
				if published.ID == "" {
					t.Fatalf("edited claim absent from successor: %+v", preview.Outputs[0].Successor.Claims)
				}
				if editKind == "partial" && !reflect.DeepEqual(published.Examples[len(published.Examples)-1].Extra, carrier.Extra{"x-owner": "retained"}) {
					t.Fatalf("surviving example extension lost: %+v", published.Examples)
				}
				for _, example := range published.Examples {
					if example.ID == "drop" {
						t.Fatalf("withdrawn proposal-only example survived: %+v", published.Examples)
					}
				}
				if editKind == "clear" && operationKind == "ADDED" && len(published.Examples) != 0 {
					t.Fatalf("explicit clear retained proposal-only examples: %+v", published.Examples)
				}
				if editKind == "rename" && published.Examples[len(published.Examples)-1].ID != "renamed" {
					t.Fatalf("proposal-only rename did not publish: %+v", published.Examples)
				}
			})
		}
	}
}

func TestT01BR7ProposalRemovalDoesNotWaiveBaseScenarioRules(t *testing.T) {
	base, _, bases, original := setup(t)
	claim := changeClaim(base)
	claim.Examples = append(claim.Examples, carrier.Example{ID: "proposal-only", Text: "New case"})
	original.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Add one proposed case"}}
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, original)
	parsed := Parse(candidateRaw(t, original)).Change
	modified := *parsed.Patches[0].Operations[0].Claim
	modified.Examples = []carrier.Example{{ID: "proposal-only", Text: "New case"}}
	parsed.Patches[0].Operations[0].Claim = &modified
	edit := t01br4RevisionEdit("update", changeRef)
	edit.Patches = parsed.Patches
	result := Revise(changeRef, snapshot, edit, bases)
	if result.Kind != "conflict" || !code(result.Diagnostics, "unsupported_yaml_content_conversion") || len(result.Raw) != 0 {
		t.Fatalf("omitting a base scenario gained a proposal-only exemption: %+v", result)
	}
	parsed = Parse(candidateRaw(t, original)).Change
	parsed.Patches[0].Operations[0].RemoveExamples = []string{"proposal-only"}
	edit.Patches = parsed.Patches
	result = Revise(changeRef, snapshot, edit, bases)
	if result.Kind != "ready" {
		t.Fatalf("ordinary revision cannot retain an explicit invalid removal for preview: %+v", result)
	}
	preview := Preview(result.Change, bases)
	if preview.Kind != "conflict" || !code(preview.Diagnostics, "unknown_example_removal") {
		t.Fatalf("proposal-only ID was accepted as a base removal: %+v", preview)
	}
}

func TestT01BR7DuplicateMovesPreservePositionalEdits(t *testing.T) {
	cases := []struct {
		before []any
		after  []any
		moved  bool
	}{
		{[]any{"a", "a"}, []any{"a", "b"}, false},
		{[]any{"a", "a"}, []any{"b", "a"}, false},
		{[]any{"a", "a", "b", "b"}, []any{"b", "b", "a", "a"}, true},
		{[]any{"a", "a", "b", "b"}, []any{"b", "b", "a"}, true},
		{[]any{"a", "a", "b", "b"}, []any{"x", "b", "b", "a", "a"}, true},
	}
	for _, tc := range cases {
		if got := sequenceMoved(tc.before, tc.after); got != tc.moved {
			t.Fatalf("sequence move mismatch for %v -> %v: got %v", tc.before, tc.after, got)
		}
	}
	base, _, _, original := setup(t)
	base.Record.Claims[0].Extra = carrier.Extra{"x-steps": []any{"a", "a", "b", "b"}}
	doc, ref, bases := t01br5Basis(t, base.Record, base.Body, func(raw string) string {
		return strings.Replace(raw, "- a\n", "- !vendor a\n", 2)
	})
	if bytes.Count(doc.Raw, []byte("!vendor a")) != 2 {
		t.Fatal("tagged fixture did not survive capture")
	}
	for _, values := range [][]any{{"b", "b", "a", "a"}, {"x", "b", "b", "a", "a"}, {"b", "b", "a"}} {
		claim := base.Record.Claims[0]
		claim.Extra = carrier.Extra{"x-steps": values}
		original.Patches[0].Base = ref
		original.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Reorder steps"}}
		preview := Preview(original, bases)
		if preview.Kind != "conflict" || !code(preview.Diagnostics, "ambiguous_yaml_correspondence") {
			t.Fatalf("duplicate tag move %v was accepted: %+v", values, preview)
		}
	}
}
