package change

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

type t01br9Edition struct {
	doc   carrier.Document
	ref   string
	basis Basis
}

func t01br9MakeEdition(t *testing.T, record carrier.Record, body []byte, id int, parents []string, snapshots map[string][]byte) t01br9Edition {
	t.Helper()
	record.ID = fmt.Sprintf("spec-20260927-%08x", id)
	record.Supersedes = parents
	record.WriteReceipt = nil
	if len(parents) > 0 {
		record.SupersedeReason = "Preserve exact predecessor meaning"
	}
	doc, ref, bases := t01br5Basis(t, record, body, nil)
	parsed, err := carrier.ParseRef(ref)
	if err != nil {
		t.Fatal(err)
	}
	basis := bases[ref]
	snapshots[parsed.Digest] = basis.Snapshot
	basis.CapturedSnapshots = snapshots
	return t01br9Edition{doc: doc, ref: ref, basis: basis}
}

func t01br9History(t *testing.T, count int) ([]t01br9Edition, map[string][]byte) {
	t.Helper()
	seed, _, _, _ := setup(t)
	record := seed.Record
	record.Claims = []carrier.Claim{{ID: "rule", Kind: "definition", Text: "Original rule"}}
	snapshots := map[string][]byte{}
	editions := make([]t01br9Edition, 0, count)
	for index := range count {
		var parents []string
		if index > 0 {
			parents = []string{editions[index-1].ref}
		}
		editions = append(editions, t01br9MakeEdition(t, record, seed.Body, 9000+index, parents, snapshots))
	}
	return editions, snapshots
}

func t01br9Patch(edition t01br9Edition, text, owner string) SectionPatch {
	claim := edition.doc.Record.Claims[0]
	claim.Text = text
	patch := SectionPatch{Base: edition.ref, Operations: []Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Resolve the exact edition"}}}
	if owner != "" {
		patch.Extra = carrier.Extra{"x-owner": owner}
	}
	return patch
}

func t01br9Revise(t *testing.T, original, selected []SectionPatch, editions ...t01br9Edition) RevisionResult {
	t.Helper()
	_, _, _, source := setup(t)
	source.Patches = original
	_, snapshot, ref := t01br6ChangeSnapshot(t, source)
	oldBytes := bytes.Clone(snapshot)
	edit := t01br4RevisionEdit("rebase", ref)
	edit.Patches = selected
	bases := map[string]Basis{}
	for _, edition := range editions {
		bases[edition.ref] = edition.basis
	}
	result := Revise(ref, snapshot, edit, bases)
	if !bytes.Equal(snapshot, oldBytes) {
		t.Fatal("revision changed the exact predecessor snapshot")
	}
	return result
}

func t01br9ExpectReady(t *testing.T, result RevisionResult, owners ...string) {
	t.Helper()
	if result.Kind != "ready" || result.Preview == nil || result.Preview.Kind != "ready" || len(result.Raw) == 0 || len(result.Change.Patches) != len(owners) {
		t.Fatalf("exact rebase did not produce a ready successor: %+v", result)
	}
	for index, owner := range owners {
		actual, present := result.Change.Patches[index].Extra["x-owner"]
		if owner != "" && actual != owner || owner == "" && present {
			t.Fatalf("patch %d took another owner's extension: %+v", index, result.Change.Patches[index].Extra)
		}
		selected := result.Change.Patches[index].Operations[0].Claim
		found := false
		for _, claim := range result.Preview.Outputs[index].Successor.Claims {
			if claim.ID == selected.ID && claim.Text == selected.Text {
				found = true
			}
		}
		if !found {
			t.Fatalf("patch %d previewed a different meaning than %+v", index, selected)
		}
	}
}

func t01br9ExpectConflict(t *testing.T, result RevisionResult, code, path string) {
	t.Helper()
	if result.Kind != "conflict" || len(result.Raw) != 0 || !t01br4HasDiagnosticAt(result.Diagnostics, code, path) {
		t.Fatalf("required correspondence refusal changed outcome: %+v", result)
	}
}

func capturedRevisionLineage(ref string, basis Basis, oldBases map[string]bool) revisionLineage {
	patches := []SectionPatch{{Base: ref}}
	selected := map[int]Basis{0: basis}
	lineages := capturedRevisionLineages(patches, selected, oldBases)
	return lineages[0]
}

func TestT01BR9ProvenParentsStopLongHistoryAtOwnerFrontier(t *testing.T) {
	history, snapshots := t01br9History(t, 260)
	a := history[len(history)-1]
	record := a.doc.Record
	b := t01br9MakeEdition(t, record, a.doc.Body, 9300, []string{a.ref}, snapshots)
	z := t01br9MakeEdition(t, record, a.doc.Body, 9301, nil, snapshots)
	y := t01br9MakeEdition(t, record, a.doc.Body, 9302, []string{z.ref}, snapshots)
	old := []SectionPatch{t01br9Patch(a, "Original A", "A_OWNER"), t01br9Patch(z, "Original Z", "Z_OWNER")}
	selected := []SectionPatch{t01br9Patch(b, "Resolved A", ""), t01br9Patch(y, "Resolved Z", "")}
	t01br9ExpectReady(t, t01br9Revise(t, old, selected, b, y), "A_OWNER", "Z_OWNER")
	t01br9ExpectReady(t, t01br9Revise(t, old, []SectionPatch{selected[1], selected[0]}, y, b), "Z_OWNER", "A_OWNER")
	t01br9ExpectReady(t, t01br9Revise(t, []SectionPatch{old[1], old[0]}, selected, b, y), "A_OWNER", "Z_OWNER")

	for _, parents := range [][]string{{a.ref, b.ref}, {b.ref, a.ref}} {
		merge := t01br9MakeEdition(t, record, a.doc.Body, 9303+len(parents), parents, snapshots)
		got := t01br9Revise(t, []SectionPatch{t01br9Patch(b, "Original B", "B_OWNER")}, []SectionPatch{t01br9Patch(merge, "Resolved merge", "")}, merge)
		t01br9ExpectReady(t, got, "B_OWNER")
	}
}

func TestT01BR9ExactRetainedPatchIgnoresAddedSectionHistory(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	z := history[0]
	record := z.doc.Record
	missing := "spec-20260927-0000ffff@sha256:" + strings.Repeat("1", 64)
	y := t01br9MakeEdition(t, record, z.doc.Body, 9400, []string{missing}, snapshots)
	old := []SectionPatch{t01br9Patch(z, "Original Z", "Z_OWNER")}
	selected := []SectionPatch{t01br9Patch(z, "Resolved Z", ""), t01br9Patch(y, "New section", "")}
	t01br9ExpectReady(t, t01br9Revise(t, old, selected, z, y), "Z_OWNER", "")
}

func TestT01BR9DuplicateSelectedBaseKeepsStructuralOutcome(t *testing.T) {
	history, _ := t01br9History(t, 1)
	z := history[0]
	old := []SectionPatch{t01br9Patch(z, "Original Z", "Z_OWNER")}
	selected := []SectionPatch{t01br9Patch(z, "First", ""), t01br9Patch(z, "Second", "")}
	got := t01br9Revise(t, old, selected, z)
	if got.Kind != "invalid" || len(got.Raw) != 0 || !t01br4HasDiagnosticAt(got.Diagnostics, "duplicate_base", "patches[1]") || code(got.Diagnostics, "ambiguous_revision_correspondence") {
		t.Fatalf("correspondence hid duplicate_base: %+v", got)
	}
}

func TestT01BR9UniqueOwnerAndRequiredUnresolvedPaths(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	a := history[0]
	record := a.doc.Record
	z := t01br9MakeEdition(t, record, a.doc.Body, 9500, nil, snapshots)
	b := t01br9MakeEdition(t, record, a.doc.Body, 9501, []string{a.ref}, snapshots)
	c := t01br9MakeEdition(t, record, a.doc.Body, 9502, []string{a.ref}, snapshots)
	merge := t01br9MakeEdition(t, record, a.doc.Body, 9503, []string{a.ref, z.ref}, snapshots)
	old := []SectionPatch{t01br9Patch(a, "Original A", "A_OWNER"), t01br9Patch(z, "Original Z", "Z_OWNER")}
	t01br9ExpectConflict(t, t01br9Revise(t, old, []SectionPatch{t01br9Patch(merge, "One merged patch", "")}, merge), "ambiguous_revision_correspondence", "patches[0]")
	t01br9ExpectConflict(t, t01br9Revise(t, old[:1], []SectionPatch{t01br9Patch(b, "First", ""), t01br9Patch(c, "Second", "")}, b, c), "ambiguous_revision_correspondence", "patches[0]")

	missing := "spec-20260927-0000ffff@sha256:" + strings.Repeat("1", 64)
	unresolved := t01br9MakeEdition(t, record, a.doc.Body, 9504, []string{missing}, snapshots)
	t01br9ExpectConflict(t, t01br9Revise(t, old[:1], []SectionPatch{t01br9Patch(unresolved, "Unresolved", "")}, unresolved), "ambiguous_revision_correspondence", "patches[0]")
	uncertainMerge := t01br9MakeEdition(t, record, a.doc.Body, 9505, []string{a.ref, missing}, snapshots)
	t01br9ExpectConflict(t, t01br9Revise(t, old, []SectionPatch{t01br9Patch(uncertainMerge, "Possibly two owners", "")}, uncertainMerge), "ambiguous_revision_correspondence", "patches[0]")
}

func TestT01BR9SnapshotAndLinkBounds(t *testing.T) {
	history, _ := t01br9History(t, 260)
	tip := history[len(history)-1]
	for _, test := range []struct {
		name      string
		oldIndex  int
		wantReady bool
	}{
		{name: "256 snapshots prove the edge", oldIndex: 3, wantReady: true},
		{name: "257 snapshots cannot prove the edge", oldIndex: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			old := history[test.oldIndex]
			got := t01br9Revise(t, []SectionPatch{t01br9Patch(old, "Original", "HISTORICAL_OWNER")}, []SectionPatch{t01br9Patch(tip, "Resolved", "")}, tip)
			if test.wantReady {
				t01br9ExpectReady(t, got, "HISTORICAL_OWNER")
				return
			}
			t01br9ExpectConflict(t, got, "ambiguous_revision_correspondence", "patches[0]")
			if !strings.Contains(got.Diagnostics[0].Message, "256-snapshot bound") {
				t.Fatalf("snapshot limit has no precise cause: %+v", got.Diagnostics)
			}
		})
	}

	seed, _, _, _ := setup(t)
	record := seed.Record
	record.Claims = []carrier.Claim{{ID: "rule", Kind: "definition", Text: "Original rule"}}
	snapshots := map[string][]byte{}
	unrelated := t01br9MakeEdition(t, record, seed.Body, 9700, nil, snapshots)
	dense := make([]t01br9Edition, 0, 209)
	for index := range 208 {
		parents := []string{}
		for prior := max(0, index-5); prior < index; prior++ {
			if index == 207 && prior == index-5 {
				continue
			}
			parents = append(parents, dense[prior].ref)
		}
		dense = append(dense, t01br9MakeEdition(t, record, seed.Body, 9800+index, parents, snapshots))
	}
	tip1024 := dense[len(dense)-1]
	tip1025 := t01br9MakeEdition(t, record, seed.Body, 10100, []string{tip1024.ref}, snapshots)
	addition := func(base t01br9Edition) SectionPatch {
		claim := carrier.Claim{ID: "new-rule", Kind: "definition", Text: "Explicit unrelated replacement"}
		return SectionPatch{Base: base.ref, Operations: []Operation{{Op: "ADDED", Claim: &claim, Reason: "Replace the previous patch"}}}
	}
	old := []SectionPatch{t01br9Patch(unrelated, "Original unrelated", "UNRELATED_OWNER")}
	t01br9ExpectReady(t, t01br9Revise(t, old, []SectionPatch{addition(tip1024)}, tip1024), "")
	got := t01br9Revise(t, old, []SectionPatch{addition(tip1025)}, tip1025)
	t01br9ExpectConflict(t, got, "ambiguous_revision_correspondence", "patches[0]")
	if !strings.Contains(got.Diagnostics[0].Message, "1024-link bound") {
		t.Fatalf("link limit has no precise cause: %+v", got.Diagnostics)
	}
}

func TestT01BR9InvalidRefAndMismatchedSnapshotCannotProveOwnership(t *testing.T) {
	history, snapshots := t01br9History(t, 1)
	old := history[0]
	invalid := capturedRevisionLineage("invalid-ref", Basis{CurrentRef: "invalid-ref"}, map[string]bool{old.ref: true})
	if !strings.Contains(invalid.problem, "invalid predecessor ref") || invalid.known {
		t.Fatalf("invalid exact ref gained ancestry: %+v", invalid)
	}
	parsed, err := carrier.ParseRef(old.ref)
	if err != nil {
		t.Fatal(err)
	}
	falseRef := "spec-20260927-0000eeee@" + parsed.Digest
	selected := t01br9MakeEdition(t, old.doc.Record, old.doc.Body, 9900, []string{falseRef}, snapshots)
	lineage := capturedRevisionLineage(selected.ref, selected.basis, map[string]bool{old.ref: true})
	if !lineage.known || !strings.Contains(lineage.problem, "does not match its ref") {
		t.Fatalf("wrong-record snapshot proved ancestry: %+v", lineage)
	}
	t01br9ExpectConflict(t, t01br9Revise(t, []SectionPatch{t01br9Patch(old, "Original", "OLD_OWNER")}, []SectionPatch{t01br9Patch(selected, "Resolved", "")}, selected), "ambiguous_revision_correspondence", "patches[0]")
}
