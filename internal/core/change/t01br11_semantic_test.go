package change

import (
	"bytes"
	"strings"
	"testing"
)

func t01br11ExpectOwnersAndMeaning(t *testing.T, got RevisionResult, selected []SectionPatch, editions []t01br9Edition, owners ...string) {
	t.Helper()
	t01br9ExpectReady(t, got, owners...)
	for index, edition := range editions {
		output := got.Preview.Outputs[index]
		successor := output.Successor
		if got.Change.Patches[index].Base != edition.ref || !bytes.Equal(output.Body, edition.doc.Body) || successor.About != edition.doc.Record.About || successor.ReceivingUse != edition.doc.Record.ReceivingUse {
			t.Fatalf("patch %d changed selected basis or unrelated meaning: %+v", index, successor)
		}
		claim := selected[index].Operations[0].Claim
		if successor.Claims[0].ID != claim.ID || successor.Claims[0].Kind != claim.Kind || successor.Claims[0].Text != claim.Text {
			t.Fatalf("patch %d lost authored claim meaning: %+v", index, successor.Claims)
		}
	}
}

func TestT01BR11CompleteFrontierIgnoresAsymmetricDepth(t *testing.T) {
	for _, depth := range []int{1, 2, 3} {
		t.Run(strings.Repeat("hop/", depth), func(t *testing.T) {
			history, snapshots := t01br9History(t, 260)
			a := history[len(history)-1]
			record := a.doc.Record
			b := t01br9MakeEdition(t, record, a.doc.Body, 10000, []string{a.ref}, snapshots)
			z := t01br9MakeEdition(t, record, a.doc.Body, 11000, nil, snapshots)
			y := z
			for hop := 1; hop <= depth; hop++ {
				y = t01br9MakeEdition(t, record, a.doc.Body, 11000+hop, []string{y.ref}, snapshots)
			}
			old := []SectionPatch{t01br9Patch(a, "Old A edit", "A_OWNER"), t01br9Patch(z, "Old Z edit", "Z_OWNER")}
			selected := []SectionPatch{t01br9Patch(b, "New A edit", ""), t01br9Patch(y, "New Z edit", "")}
			got := t01br9Revise(t, old, selected, b, y)
			t01br11ExpectOwnersAndMeaning(t, got, selected, []t01br9Edition{b, y}, "A_OWNER", "Z_OWNER")
			if depth != 3 {
				return
			}
			got = t01br9Revise(t, []SectionPatch{old[1], old[0]}, []SectionPatch{selected[1], selected[0]}, y, b)
			t01br11ExpectOwnersAndMeaning(t, got, []SectionPatch{selected[1], selected[0]}, []t01br9Edition{y, b}, "Z_OWNER", "A_OWNER")
		})
	}
}

func TestT01BR11CompleteFrontierIgnoresLexicalDiscoveryOrder(t *testing.T) {
	for _, branch := range []string{"long", "missing"} {
		for _, proofFirst := range []bool{true, false} {
			name := branch + "/irrelevant-first"
			if proofFirst {
				name = branch + "/proof-first"
			}
			t.Run(name, func(t *testing.T) {
				history, snapshots := t01br9History(t, 260)
				a := history[len(history)-1]
				record := a.doc.Record
				if branch == "missing" {
					missing := "spec-20260927-0000ffff@sha256:" + strings.Repeat("1", 64)
					a = t01br9MakeEdition(t, record, a.doc.Body, 12000, []string{missing}, snapshots)
				}
				b1 := t01br9MakeEdition(t, record, a.doc.Body, 12001, []string{a.ref}, snapshots)
				b := t01br9MakeEdition(t, record, a.doc.Body, 12002, []string{b1.ref}, snapshots)
				z := t01br9MakeEdition(t, record, a.doc.Body, 13000, nil, snapshots)
				root := t01br9MakeEdition(t, record, a.doc.Body, 14000, nil, snapshots)
				proofID := 15000
				if proofFirst {
					proofID = 13500
				}
				proof := t01br9MakeEdition(t, record, a.doc.Body, proofID, []string{z.ref}, snapshots)
				y := t01br9MakeEdition(t, record, a.doc.Body, 16000, []string{root.ref, proof.ref}, snapshots)
				old := []SectionPatch{t01br9Patch(a, "Old A edit", "A_OWNER"), t01br9Patch(z, "Old Z edit", "Z_OWNER")}
				selected := []SectionPatch{t01br9Patch(b, "New A edit", ""), t01br9Patch(y, "New Z edit", "")}
				got := t01br9Revise(t, old, selected, b, y)
				t01br11ExpectOwnersAndMeaning(t, got, selected, []t01br9Edition{b, y}, "A_OWNER", "Z_OWNER")
			})
		}
	}
}

func TestT01BR11CompleteForkFrontierIsIndependentOfDepth(t *testing.T) {
	for _, depth := range []int{1, 2, 3} {
		t.Run(strings.Repeat("fork/", depth), func(t *testing.T) {
			history, snapshots := t01br9History(t, 1)
			a := history[0]
			record := a.doc.Record
			b := t01br9MakeEdition(t, record, a.doc.Body, 17000, []string{a.ref}, snapshots)
			c := t01br9MakeEdition(t, record, a.doc.Body, 17001, []string{b.ref}, snapshots)
			e := a
			for hop := 1; hop <= depth; hop++ {
				e = t01br9MakeEdition(t, record, a.doc.Body, 18000+hop, []string{e.ref}, snapshots)
			}
			old := []SectionPatch{t01br9Patch(a, "Old A edit", "A_OWNER"), t01br9Patch(b, "Old B edit", "B_OWNER")}
			selected := []SectionPatch{t01br9Patch(c, "New B edit", ""), t01br9Patch(e, "New A edit", "")}
			got := t01br9Revise(t, old, selected, c, e)
			t01br11ExpectOwnersAndMeaning(t, got, selected, []t01br9Edition{c, e}, "B_OWNER", "A_OWNER")
			if depth != 2 {
				return
			}
			got = t01br9Revise(t, []SectionPatch{old[1], old[0]}, []SectionPatch{selected[1], selected[0]}, e, c)
			t01br11ExpectOwnersAndMeaning(t, got, []SectionPatch{selected[1], selected[0]}, []t01br9Edition{e, c}, "A_OWNER", "B_OWNER")
		})
	}
}

func TestT01BR11FrontierErrorsAreScopedToRequiredProof(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	root := history[0]
	record := root.doc.Record
	falseRef := "spec-20260927-0000eeee@" + strings.Split(root.ref, "@")[1]
	a := t01br9MakeEdition(t, record, root.doc.Body, 19000, []string{falseRef}, snapshots)
	z := t01br9MakeEdition(t, record, root.doc.Body, 19001, nil, snapshots)
	b := t01br9MakeEdition(t, record, root.doc.Body, 19002, []string{a.ref}, snapshots)
	y := t01br9MakeEdition(t, record, root.doc.Body, 19003, []string{z.ref}, snapshots)
	old := []SectionPatch{t01br9Patch(a, "Old A edit", "A_OWNER"), t01br9Patch(z, "Old Z edit", "Z_OWNER")}
	selected := []SectionPatch{t01br9Patch(b, "New A edit", ""), t01br9Patch(y, "New Z edit", "")}
	got := t01br9Revise(t, old, selected, b, y)
	t01br11ExpectOwnersAndMeaning(t, got, selected, []t01br9Edition{b, y}, "A_OWNER", "Z_OWNER")

	unknown := t01br9MakeEdition(t, record, root.doc.Body, 19004, []string{falseRef}, snapshots)
	selected = []SectionPatch{t01br9Patch(b, "New A edit", ""), t01br9Patch(unknown, "Unproven Z edit", "")}
	got = t01br9Revise(t, old, selected, b, unknown)
	t01br9ExpectConflict(t, got, "ambiguous_revision_correspondence", "patches[1]")
	if len(got.Raw) != 0 || !strings.Contains(got.Diagnostics[0].Message, "does not match its ref") {
		t.Fatalf("required mismatched ref gained a successor or lost its cause: %+v", got)
	}
}

func TestT01BR11FrontierPromotionKeepsOneSnapshotBudget(t *testing.T) {
	history, snapshots := t01br9History(t, 260)
	b := history[len(history)-1]
	c := t01br9MakeEdition(t, b.doc.Record, b.doc.Body, 20000, []string{b.ref}, snapshots)
	selected := []SectionPatch{t01br9Patch(c, "Combined edit", "")}
	for _, test := range []struct {
		name      string
		oldIndex  int
		wantBound bool
	}{
		{name: "256 total snapshots find both owners", oldIndex: 4},
		{name: "257 total snapshots cannot prove both owners", oldIndex: 3, wantBound: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := history[test.oldIndex]
			lineage := capturedRevisionLineage(c.ref, c.basis, map[string]bool{a.ref: true, b.ref: true})
			wantEarlierOwner := !test.wantBound
			if !lineage.ancestors[b.ref] || lineage.ancestors[a.ref] != wantEarlierOwner || strings.Contains(lineage.problem, "256-snapshot bound") != test.wantBound {
				t.Fatalf("frontier promotion changed the total search budget: %+v", lineage)
			}
			old := []SectionPatch{t01br9Patch(a, "Earlier A edit", "A_OWNER"), t01br9Patch(b, "Later B edit", "B_OWNER")}
			got := t01br9Revise(t, old, selected, c)
			t01br9ExpectConflict(t, got, "ambiguous_revision_correspondence", "patches[0]")
			if !t01br4HasDiagnosticAt(got.Diagnostics, "ambiguous_revision_correspondence", "patches[1]") || len(got.Raw) != 0 {
				t.Fatalf("two old owners were collapsed into one: %+v", got)
			}
			hitBound := false
			for _, diagnostic := range got.Diagnostics {
				hitBound = hitBound || strings.Contains(diagnostic.Message, "256-snapshot bound")
			}
			if hitBound != test.wantBound {
				t.Fatalf("frontier promotion changed the shared snapshot budget: %+v", got.Diagnostics)
			}
		})
	}
}
