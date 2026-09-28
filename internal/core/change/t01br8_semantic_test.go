package change

import (
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func TestT01BR8ConvergentAncestryAndUniqueOwner(t *testing.T) {
	base, oldRef, oldBases, original := setup(t)
	claim := changeClaim(base)
	original.Patches[0].Extra = carrier.Extra{"x-owner": "OLD_OWNER"}
	original.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Original edit"}}
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, original)

	middle := base.Record
	middle.ID = "spec-20260923-00000081"
	middle.Supersedes = []string{oldRef}
	middle.SupersedeReason = "Proposed exact successor"
	middle.WriteReceipt = nil
	middleDoc, middleRef, middleBases := t01br5Basis(t, middle, base.Body, nil)
	merge := middleDoc.Record
	merge.ID = "spec-20260923-00000082"
	merge.Supersedes = []string{oldRef, middleRef}
	merge.SupersedeReason = "Accept both exact heads"
	mergeDoc, mergeRef, mergeBases := t01br5Basis(t, merge, base.Body, nil)
	child := mergeDoc.Record
	child.ID = "spec-20260923-00000083"
	child.Supersedes = []string{mergeRef}
	child.SupersedeReason = "Continue after acceptance"
	childDoc, childRef, childBases := t01br5Basis(t, child, base.Body, nil)
	oldDigest := strings.Split(oldRef, "@")[1]
	middleDigest := strings.Split(middleRef, "@")[1]
	mergeDigest := strings.Split(mergeRef, "@")[1]
	captured := map[string][]byte{oldDigest: oldBases[oldRef].Snapshot, middleDigest: middleBases[middleRef].Snapshot, mergeDigest: mergeBases[mergeRef].Snapshot}
	for ref, basis := range childBases {
		basis.CapturedSnapshots = captured
		childBases[ref] = basis
	}
	childBases[oldRef] = oldBases[oldRef]
	edit := t01br4RevisionEdit("rebase", changeRef)
	claim = childDoc.Record.Claims[0]
	claim.Text = "Resolved after merge"
	edit.Patches = []SectionPatch{{Base: childRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Resolve current claim"}}}}
	result := Revise(changeRef, snapshot, edit, childBases)
	if result.Kind != "ready" || result.Preview == nil || result.Preview.Kind != "ready" || result.Change.Patches[0].Extra["x-owner"] != "OLD_OWNER" {
		t.Fatalf("convergent descendant lost its unique old owner: %+v", result)
	}

	// With two old patches, the same descendant would claim both owners.
	two := original
	two.Patches = append([]SectionPatch{}, original.Patches...)
	two.Patches = append(two.Patches, SectionPatch{Base: middleRef, Extra: carrier.Extra{"x-owner": "MIDDLE_OWNER"}, Operations: original.Patches[0].Operations})
	items, diagnostics := matchRevisionItems(two, Change{Patches: edit.Patches}, childBases)
	if len(items.patches) != 0 || !code(diagnostics, "ambiguous_revision_correspondence") {
		t.Fatalf("many old owners chose one merged patch: items=%+v diagnostics=%+v", items, diagnostics)
	}

	missing := childBases[childRef]
	missing.CapturedSnapshots = map[string][]byte{oldDigest: captured[oldDigest], middleDigest: captured[middleDigest]}
	childBases[childRef] = missing
	refused := Revise(changeRef, snapshot, edit, childBases)
	if refused.Kind != "conflict" || !code(refused.Diagnostics, "ambiguous_revision_correspondence") || len(refused.Raw) != 0 {
		t.Fatalf("missing merge snapshot granted ownership: %+v", refused)
	}
}

func TestT01BR8OmittedProposalExamplesAreNotWithdrawal(t *testing.T) {
	for _, kind := range []string{"MODIFIED", "ADDED"} {
		t.Run(kind, func(t *testing.T) {
			base, _, bases, original := setup(t)
			claim := changeClaim(base)
			if kind == "ADDED" {
				claim = carrier.Claim{ID: "new-rule", Kind: "definition", Text: "New rule"}
			}
			claim.Examples = []carrier.Example{{ID: "keep", Text: "Keep", Extra: carrier.Extra{"x-owner": "RETAINED"}}, {ID: "drop", Text: "Drop"}}
			if kind == "MODIFIED" {
				claim.Examples = append(append([]carrier.Example{}, base.Record.Claims[0].Examples...), claim.Examples...)
			}
			operation := Operation{Op: kind, Claim: &claim, Reason: "Propose scenarios"}
			if kind == "MODIFIED" {
				operation.ClaimID = claim.ID
			}
			original.Patches[0].Operations = []Operation{operation}
			_, snapshot, changeRef := t01br6ChangeSnapshot(t, original)
			edit := t01br4RevisionEdit("update", changeRef)
			omitted := claim
			omitted.Text = "Clarify text only"
			omitted.Examples = nil
			operation.Claim = &omitted
			edit.Patches = []SectionPatch{{Base: original.Patches[0].Base, Operations: []Operation{operation}}}
			refused := Revise(changeRef, snapshot, edit, bases)
			if refused.Kind != "conflict" || !code(refused.Diagnostics, "unsupported_yaml_content_conversion") || len(refused.Raw) != 0 {
				t.Fatalf("omitted proposal scenarios were withdrawn: %+v", refused)
			}
			explicit := omitted
			explicit.Examples = []carrier.Example{}
			if kind == "MODIFIED" {
				explicit.Examples = append([]carrier.Example{}, base.Record.Claims[0].Examples...)
			}
			operation.Claim = &explicit
			edit.Patches[0].Operations = []Operation{operation}
			accepted := Revise(changeRef, snapshot, edit, bases)
			if accepted.Kind != "ready" {
				t.Fatalf("explicit proposal-only clear was refused: %+v", accepted)
			}
		})
	}
}

func TestT01BR8RebaseUsesSelectedCurrentExamples(t *testing.T) {
	base, oldRef, oldBases, original := setup(t)
	claim := changeClaim(base)
	original.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Outstanding edit"}}
	_, snapshot, changeRef := t01br6ChangeSnapshot(t, original)
	current := base.Record
	current.ID = "spec-20260923-00000084"
	current.Supersedes = []string{oldRef}
	current.SupersedeReason = "Advance exact scenarios"
	current.WriteReceipt = nil
	current.Claims = append([]carrier.Claim{}, base.Record.Claims...)
	current.Claims[0].Examples = []carrier.Example{}
	currentDoc, currentRef, bases := t01br5Basis(t, current, base.Body, nil)
	oldDigest := strings.Split(oldRef, "@")[1]
	currentBasis := bases[currentRef]
	currentBasis.CapturedSnapshots = map[string][]byte{oldDigest: oldBases[oldRef].Snapshot}
	bases[currentRef] = currentBasis
	edit := t01br4RevisionEdit("rebase", changeRef)
	selected := currentDoc.Record.Claims[0]
	selected.Text = "Rebased current edit"
	selected.Examples = []carrier.Example{}
	edit.Patches = []SectionPatch{{Base: currentRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: selected.ID, Claim: &selected, Reason: "Resolve scenarios"}}}}
	result := Revise(changeRef, snapshot, edit, bases)
	if result.Kind != "ready" || result.Preview == nil || result.Preview.Kind != "ready" || len(result.Preview.Outputs[0].Successor.Claims[0].Examples) != 0 {
		t.Fatalf("old example blocked explicit current-base resolution: %+v", result)
	}

	added := current
	added.ID = "spec-20260923-00000085"
	added.Claims = append([]carrier.Claim{}, base.Record.Claims...)
	added.Claims[0].Examples = append(append([]carrier.Example{}, base.Record.Claims[0].Examples...), carrier.Example{ID: "current-only", Text: "Current scenario"})
	addedDoc, addedRef, addedBases := t01br5Basis(t, added, base.Body, nil)
	addedClaim := addedDoc.Record.Claims[0]
	addedClaim.Examples = append([]carrier.Example{}, base.Record.Claims[0].Examples...)
	edit.Patches = []SectionPatch{{Base: addedRef, Operations: []Operation{{Op: "MODIFIED", ClaimID: addedClaim.ID, Claim: &addedClaim, Reason: "Resolve current scenarios"}}}}
	refused := Revise(changeRef, snapshot, edit, addedBases)
	if refused.Kind != "conflict" || !code(refused.Diagnostics, "scenario_loss") || len(refused.Raw) != 0 {
		t.Fatalf("new current-base scenario gained a proposal-only exemption: %+v", refused)
	}
}
