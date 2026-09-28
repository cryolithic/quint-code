package change

import (
	"strings"
	"testing"
)

func TestT01BR10IndirectProofIgnoresIrrelevantBranchOrder(t *testing.T) {
	history, snapshots := t01br9History(t, 260)
	long := history[len(history)-1]
	record := long.doc.Record
	p := t01br9MakeEdition(t, record, long.doc.Body, 9400, nil, snapshots)
	missingRef := "spec-20260927-0000ffff@sha256:" + strings.Repeat("1", 64)
	missing := t01br9MakeEdition(t, record, long.doc.Body, 9258, []string{missingRef}, snapshots)
	for _, branch := range []struct {
		name    string
		edition t01br9Edition
	}{{"long", long}, {"missing", missing}} {
		for _, p2ID := range []int{8000, 9500} {
			name := branch.name + "/proof-first"
			if p2ID == 9500 {
				name = branch.name + "/irrelevant-first"
			}
			t.Run(name, func(t *testing.T) {
				p2 := t01br9MakeEdition(t, record, long.doc.Body, p2ID, []string{p.ref}, snapshots)
				merge := t01br9MakeEdition(t, record, long.doc.Body, p2ID+20000, []string{branch.edition.ref, p2.ref}, snapshots)
				old := []SectionPatch{t01br9Patch(p, "Original P", "P_OWNER")}
				selected := []SectionPatch{t01br9Patch(merge, "Resolved P", "")}
				got := t01br9Revise(t, old, selected, merge)
				t01br9ExpectReady(t, got, "P_OWNER")
			})
		}
	}
}

func TestT01BR10UnresolvedRequiredBranchRefuses(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	a := history[0]
	record := a.doc.Record
	z := t01br9MakeEdition(t, record, a.doc.Body, 9500, nil, snapshots)
	missingRef := "spec-20260927-0000ffff@sha256:" + strings.Repeat("1", 64)
	missing := t01br9MakeEdition(t, record, a.doc.Body, 9501, []string{missingRef}, snapshots)
	merge := t01br9MakeEdition(t, record, a.doc.Body, 9502, []string{a.ref, missing.ref}, snapshots)
	old := []SectionPatch{t01br9Patch(a, "Original A", "A_OWNER"), t01br9Patch(z, "Original Z", "Z_OWNER")}
	selected := []SectionPatch{t01br9Patch(merge, "Resolved A", "")}
	got := t01br9Revise(t, old, selected, merge)
	t01br9ExpectConflict(t, got, "ambiguous_revision_correspondence", "patches[0]")
	if !t01br4HasDiagnosticAt(got.Diagnostics, "ambiguous_revision_correspondence", "patches[1]") {
		t.Fatalf("unresolved second owner has no precise refusal: %+v", got.Diagnostics)
	}

	onlyMissing := t01br9Revise(t, old[:1], []SectionPatch{t01br9Patch(missing, "Unproven A", "")}, missing)
	t01br9ExpectConflict(t, onlyMissing, "ambiguous_revision_correspondence", "patches[0]")
	if !strings.Contains(onlyMissing.Diagnostics[0].Message, "missing or invalid") {
		t.Fatalf("required missing branch lacks cause: %+v", onlyMissing.Diagnostics)
	}
}

func TestT01BR10AncestorOwnersCannotCollapseIntoOnePatch(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	a := history[0]
	record := a.doc.Record
	b := t01br9MakeEdition(t, record, a.doc.Body, 9400, []string{a.ref}, snapshots)
	c := t01br9MakeEdition(t, record, a.doc.Body, 9401, []string{b.ref}, snapshots)
	oldA := t01br9Patch(a, "Outstanding A", "A_OWNER")
	oldB := t01br9Patch(b, "Outstanding B", "B_OWNER")
	selected := []SectionPatch{t01br9Patch(c, "Resolved combined intent", "")}
	for _, old := range [][]SectionPatch{{oldA, oldB}, {oldB, oldA}} {
		got := t01br9Revise(t, old, selected, c)
		t01br9ExpectConflict(t, got, "ambiguous_revision_correspondence", "patches[0]")
		if !t01br4HasDiagnosticAt(got.Diagnostics, "ambiguous_revision_correspondence", "patches[1]") {
			t.Fatalf("one old owner was silently removed: %+v", got.Diagnostics)
		}
	}
}

func TestT01BR10ExplicitUnrelatedRemovalKeepsProvenOwner(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	a := history[0]
	record := a.doc.Record
	z := t01br9MakeEdition(t, record, a.doc.Body, 9600, nil, snapshots)
	b := t01br9MakeEdition(t, record, a.doc.Body, 9601, []string{z.ref}, snapshots)
	old := []SectionPatch{t01br9Patch(a, "Removed independent edit", "A_OWNER"), t01br9Patch(z, "Original Z", "Z_OWNER")}
	selected := []SectionPatch{t01br9Patch(b, "Resolved Z", "")}
	t01br9ExpectReady(t, t01br9Revise(t, old, selected, b), "Z_OWNER")
}

func TestT01BR10ExactRetainedOwnerIsNotBlamedForUncertainAddition(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	a := history[0]
	record := a.doc.Record
	z := t01br9MakeEdition(t, record, a.doc.Body, 9700, nil, snapshots)
	b := t01br9MakeEdition(t, record, a.doc.Body, 9701, []string{a.ref}, snapshots)
	missingRef := "spec-20260927-0000ffff@sha256:" + strings.Repeat("1", 64)
	y := t01br9MakeEdition(t, record, a.doc.Body, 9702, []string{missingRef}, snapshots)
	old := []SectionPatch{t01br9Patch(z, "Original Z", "Z_OWNER"), t01br9Patch(a, "Original A", "A_OWNER")}
	selected := []SectionPatch{t01br9Patch(z, "Resolved Z", ""), t01br9Patch(b, "Resolved A", ""), t01br9Patch(y, "Uncertain addition", "")}
	got := t01br9Revise(t, old, selected, z, b, y)
	t01br9ExpectConflict(t, got, "ambiguous_revision_correspondence", "patches[1]")
	for _, diagnostic := range got.Diagnostics {
		if diagnostic.Path == "patches[0]" {
			t.Fatalf("exact retained owner was blamed for an unrelated uncertainty: %+v", got.Diagnostics)
		}
	}
	if len(got.Raw) != 0 || got.Change.Patches != nil {
		t.Fatalf("uncertain addition produced a successor: %+v", got)
	}
}

func TestT01BR10MissingBranchCannotHideSecondNewOwner(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	a := history[0]
	record := a.doc.Record
	b := t01br9MakeEdition(t, record, a.doc.Body, 9800, []string{a.ref}, snapshots)
	missingRef := "spec-20260927-0000ffff@sha256:" + strings.Repeat("1", 64)
	y := t01br9MakeEdition(t, record, a.doc.Body, 9801, []string{missingRef}, snapshots)
	old := []SectionPatch{t01br9Patch(a, "Original A", "A_OWNER")}
	selected := []SectionPatch{t01br9Patch(b, "Resolved A", ""), t01br9Patch(y, "Uncertain second patch", "")}
	got := t01br9Revise(t, old, selected, b, y)
	t01br9ExpectConflict(t, got, "ambiguous_revision_correspondence", "patches[0]")
}
