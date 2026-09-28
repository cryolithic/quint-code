package carrier

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func v2Spec(t *testing.T, old Document, id string, predecessor string) Document {
	t.Helper()
	r := old.Record
	r.Format = "haft/2"
	r.ID = id
	r.Status = ""
	r.OperatorConfirmed = false
	r.LegacyStatus = ""
	r.Origin = "agent_edit"
	r.Supersedes = nil
	r.SupersedeReason = ""
	if predecessor != "" {
		r.Supersedes = []string{predecessor}
		r.SupersedeReason = "Reauthor selected spec content"
	}
	raw, err := Encode(r, old.Body)
	if err != nil {
		t.Fatal(err)
	}
	d := Parse(raw)
	requireValid(t, d)
	return d
}

func TestV2SpecHasContentWithoutAuthoredStatus(t *testing.T) {
	d := v2Spec(t, fixture(t, "valid/spec.md"), "spec-20260925-00000001", "")
	if bytes.Contains(d.Raw, []byte("status:")) || bytes.Contains(d.Raw, []byte("operator_confirmed:")) {
		t.Fatal("v2 normalized carrier wrote a confirmation field")
	}
	if d.Record.Status != "" || d.Record.OperatorConfirmed || d.Record.Origin != "agent_edit" {
		t.Fatalf("v2 content acquired authority metadata: %+v", d.Record)
	}
	if Project([]Document{d}, nil).Entries[0].State != Current {
		t.Fatal("v2 spec was not projected as current content")
	}
	for _, field := range []string{"status: ''\n", "operator_confirmed: false\n", "legacy_status: ''\n"} {
		raw := bytes.Replace(d.Raw, []byte("kind: spec\n"), []byte("kind: spec\n"+field), 1)
		if !hasCode(Parse(raw).Diagnostics, "v2_authority_field") {
			t.Fatalf("presence of %q slipped into v2 content", field)
		}
	}
	r := d.Record
	r.Status = Proposed
	if !hasCode(Validate(r, false), "v2_authority_field") {
		t.Fatal("typed v2 status passed validation")
	}
	r = d.Record
	r.Origin = "operator_request"
	if !hasCode(Validate(r, false), "invalid_origin") {
		t.Fatal("v1 acceptance provenance became v2 edit provenance")
	}
	for _, format := range []string{"haft/0", "haft/3", "haft/20"} {
		raw := bytes.Replace(d.Raw, []byte("format: haft/2"), []byte("format: "+format), 1)
		parsed := Parse(raw)
		if !hasCode(parsed.Diagnostics, "unsupported_format") || Project([]Document{parsed}, nil).Entries[0].State != Invalid {
			t.Fatalf("unknown %s entered the semantic projection", format)
		}
	}
	decision := fixture(t, "valid/decision.md")
	decision.Record.Format = "haft/2"
	if !hasCode(Validate(decision.Record, false), "unsupported_kind_format") {
		t.Fatal("v2 decision bypassed the binding boundary")
	}
}

func TestV2SelectedReauthorPreservesHistoricalBytesAndClaims(t *testing.T) {
	snaps := map[string][]byte{}
	old, oldRef := snap(t, fixture(t, "valid/spec.md"), InterpretationBasis{}, snaps)
	originalRaw := bytes.Clone(old.Raw)
	originalSnapshot := bytes.Clone(snaps[old.Edition])
	next := v2Spec(t, old, "spec-20260925-00000002", oldRef)
	if !hasCode(ValidateSuccessor(next.Record, Project([]Document{old}, snaps), snaps, []string{oldRef}), "reauthor_required") {
		t.Fatal("ordinary successor admission bypassed explicit reauthor")
	}
	if ds := ValidateReauthorSuccessor(next.Record, Project([]Document{old}, snaps), snaps, oldRef); HasErrors(ds) {
		t.Fatalf("selected reauthor rejected: %+v", ds)
	}
	next, nextRef := snap(t, next, InterpretationBasis{}, snaps)
	p := Project([]Document{old, next}, snaps)
	if p.Entries[0].State != Superseded || p.Entries[1].State != Current || !reflect.DeepEqual(p.Heads(old.Record.ID), []string{nextRef}) {
		t.Fatalf("wrong v1/v2 lineage projection: %+v", p.Entries)
	}
	if got := p.Resolve("spec:order-cancel#L1"); got.Kind != "found" || got.Document.Record.Format != "haft/2" {
		t.Fatalf("live alias did not select v2 content: %+v", got)
	}
	if got := p.Resolve(oldRef + "#L1"); got.Kind != "found" || got.Document.Record.Format != "haft/1" || got.Claim.Text != "Cancel preserves total" {
		t.Fatalf("historical pinned claim changed: %+v", got)
	}
	if !bytes.Equal(old.Raw, originalRaw) || !bytes.Equal(snaps[old.Edition], originalSnapshot) {
		t.Fatal("predecessor carrier/snapshot bytes changed")
	}
	if !reflect.DeepEqual(old.Record.Claims, next.Record.Claims) || !reflect.DeepEqual(old.Record.Extra, next.Record.Extra) || !bytes.Equal(old.Body, next.Body) {
		t.Fatal("claim IDs, unknown extensions or prose changed during reauthor")
	}
	changed := edit(t, old, func(r *Record) { r.Title = "Concurrent manual edit" })
	if !hasCode(ValidateReauthorBasis(Project([]Document{changed}, snaps), snaps, oldRef), "stale_reauthor_ref") {
		t.Fatal("stale selected bytes passed reauthor CAS")
	}
}

func TestV2ReauthorRequiresSoleSelectedHead(t *testing.T) {
	snaps := map[string][]byte{}
	active, activeRef := snap(t, fixture(t, "valid/spec.md"), InterpretationBasis{}, snaps)
	proposed := edit(t, active, func(r *Record) {
		r.ID = "spec-20260925-00000003"
		r.Status = Proposed
		r.Origin = "agent_proposal"
		r.OperatorConfirmed = false
		r.Supersedes = []string{activeRef}
		r.SupersedeReason = "Draft revision"
	})
	proposed, proposalRef := snap(t, proposed, InterpretationBasis{}, snaps)
	p := Project([]Document{active, proposed}, snaps)
	if p.Entries[0].State != Active || p.Entries[1].State != Proposed {
		t.Fatal("v1 active/proposed interpretation changed before reauthor")
	}
	if !hasCode(ValidateReauthorBasis(p, snaps, proposalRef), "reauthor_head_conflict") {
		t.Fatal("selected proposal silently displaced a distinct active head")
	}
	if ds := ValidateReauthorBasis(p, snaps, activeRef); HasErrors(ds) {
		t.Fatalf("selected active head cannot be reauthored: %+v", ds)
	}
	standalone := Project([]Document{proposed}, snaps)
	if ds := ValidateReauthorBasis(standalone, snaps, proposalRef); HasErrors(ds) {
		t.Fatalf("sole proposed head rejected: %+v", ds)
	}
	proposedContent := v2Spec(t, proposed, "spec-20260925-00000008", proposalRef)
	if ds := ValidateReauthorSuccessor(proposedContent.Record, standalone, snaps, proposalRef); HasErrors(ds) {
		t.Fatalf("sole proposed reauthor rejected: %+v", ds)
	}
	proposedContent, _ = snap(t, proposedContent, InterpretationBasis{}, snaps)
	proposedProjection := Project([]Document{proposed, proposedContent}, snaps)
	if proposedProjection.Entries[0].State != Superseded || proposedProjection.Entries[1].State != Current {
		t.Fatal("explicit proposed reauthor did not create current content")
	}
	if old := proposedProjection.Resolve(proposalRef); old.Kind != "found" || old.Document.Record.Status != Proposed || old.Document.Record.Origin != "agent_proposal" {
		t.Fatal("historical proposed interpretation changed")
	}
	migrated := edit(t, active, func(r *Record) {
		r.ID = "spec-20260925-00000004"
		r.Slug = "migrated-cancel"
		r.Origin = "migrated_9x"
		r.OperatorConfirmed = false
	})
	migrated, migratedRef := snap(t, migrated, InterpretationBasis{}, snaps)
	historical := Project([]Document{migrated}, snaps)
	if historical.Entries[0].State != Historical {
		t.Fatal("v1 migrated status changed")
	}
	if ds := ValidateReauthorBasis(historical, snaps, migratedRef); HasErrors(ds) {
		t.Fatalf("selected migrated historical record rejected: %+v", ds)
	}
	migratedContent := v2Spec(t, migrated, "spec-20260925-00000009", migratedRef)
	if ds := ValidateReauthorSuccessor(migratedContent.Record, historical, snaps, migratedRef); HasErrors(ds) {
		t.Fatalf("selected migrated reauthor rejected: %+v", ds)
	}
	migratedProjection := Project([]Document{migrated, migratedContent}, snaps)
	if migratedProjection.Entries[0].State != Historical || migratedProjection.Entries[1].State != Current {
		t.Fatal("v2 reauthor promoted historical v1 authority")
	}
	if old := migratedProjection.Resolve(migratedRef); old.Kind != "found" || old.Document.Record.Origin != "migrated_9x" {
		t.Fatal("historical migrated interpretation changed")
	}
}

func TestV2ChangeSuccessorAndCompetingCurrentContent(t *testing.T) {
	snaps := map[string][]byte{}
	first, firstRef := snap(t, v2Spec(t, fixture(t, "valid/spec.md"), "spec-20260925-00000005", ""), InterpretationBasis{}, snaps)
	second := v2Spec(t, first, "spec-20260925-00000006", firstRef)
	base := Project([]Document{first}, snaps)
	if ds := ValidateSuccessor(second.Record, base, snaps, []string{firstRef}); HasErrors(ds) {
		t.Fatalf("v2 content successor rejected: %+v", ds)
	}
	second, secondRef := snap(t, second, InterpretationBasis{}, snaps)
	updated := Project([]Document{first, second}, snaps)
	if updated.Entries[0].State != Superseded || updated.Entries[1].State != Current || !reflect.DeepEqual(updated.Heads(first.Record.ID), []string{secondRef}) {
		t.Fatalf("v2 content edit did not replace exact edition: %+v", updated.Entries)
	}
	third := v2Spec(t, first, "spec-20260925-00000007", firstRef)
	third.Record.Title = "Competing edit"
	thirdRaw, err := Encode(third.Record, third.Body)
	if err != nil {
		t.Fatal(err)
	}
	third, thirdRef := snap(t, Parse(thirdRaw), InterpretationBasis{}, snaps)
	competing := Project([]Document{first, second, third}, snaps)
	if competing.Resolve("spec:order-cancel").Kind != "conflict" || len(competing.Heads(first.Record.ID)) != 2 {
		t.Fatalf("current content conflict hidden: %+v", competing.Entries)
	}
	if competing.Entries[1].State != ContentContested || competing.Entries[2].State != ContentContested || !strings.Contains(strings.Join(competing.Heads(first.Record.ID), " "), thirdRef) {
		t.Fatal("competing v2 heads chose a winner")
	}
	if !hasCode(ValidateSuccessor(second.Record, base, snaps, nil), "head_conflict") {
		t.Fatal("stale v2 head CAS passed")
	}
}

func TestV2SequentialContentRetirementIndependentOfIDOrder(t *testing.T) {
	orders := [][]string{
		{"00000001", "00000002", "00000003"},
		{"00000001", "00000003", "00000002"},
		{"00000002", "00000001", "00000003"},
		{"00000002", "00000003", "00000001"},
		{"00000003", "00000001", "00000002"},
		{"00000003", "00000002", "00000001"},
		{"00000005", "00000004", "00000003", "00000002", "00000001"},
	}
	for _, order := range orders {
		t.Run(strings.Join(order, "-"), func(t *testing.T) {
			snaps := map[string][]byte{}
			base := fixture(t, "valid/spec.md")
			written := make([]Document, 0, len(order))
			previous := ""
			for _, suffix := range order {
				id := "spec-20260925-" + suffix
				next := v2Spec(t, base, id, previous)
				next, previous = snap(t, next, InterpretationBasis{}, snaps)
				written = append(written, next)
				base = next
			}
			sort.Slice(written, func(i, j int) bool {
				return written[i].Record.ID < written[j].Record.ID
			})
			p := Project(written, snaps)
			for _, e := range p.Entries {
				want := Superseded
				if e.Ref == previous {
					want = Current
				}
				if e.State != want {
					t.Fatalf("%s is %s, want %s: %+v", e.Ref, e.State, want, p.Entries)
				}
			}
			first := "spec-20260925-" + order[0]
			if !reflect.DeepEqual(p.Heads(first), []string{previous}) {
				t.Fatalf("heads=%v, want only %s", p.Heads(first), previous)
			}
			resolved := p.Resolve("spec:order-cancel")
			if resolved.Kind != "found" || resolved.Document.Record.ID != "spec-20260925-"+order[len(order)-1] {
				t.Fatalf("linear chain did not resolve latest content: %+v", resolved)
			}
		})
	}
}

func TestV2ExactPredecessorMismatchDoesNotRetireChangedLiveEdition(t *testing.T) {
	snaps := map[string][]byte{}
	base := fixture(t, "valid/spec.md")
	first := v2Spec(t, base, "spec-20260925-00000003", "")
	first, firstRef := snap(t, first, InterpretationBasis{}, snaps)
	second := v2Spec(t, first, "spec-20260925-00000002", firstRef)
	second, secondRef := snap(t, second, InterpretationBasis{}, snaps)
	third := v2Spec(t, second, "spec-20260925-00000001", secondRef)
	changed := edit(t, second, func(r *Record) { r.Title = "Changed live intermediate edition" })
	p := Project([]Document{third, changed, first}, snaps)
	if p.Entries[1].State != ContentContested || p.Entries[0].State != ContentContested {
		t.Fatalf("changed live predecessor was retired: %+v", p.Entries)
	}
	if !hasCode(p.Entries[0].Diagnostics, "predecessor_edition_changed") {
		t.Fatal("exact predecessor mismatch was not diagnosed")
	}
	if p.Resolve("spec:order-cancel").Kind != "conflict" {
		t.Fatal("changed live predecessor and separately published successor chose a winner")
	}
}

func TestMixedV1ActiveAndV2CurrentHeadsExposeExactConflict(t *testing.T) {
	snaps := map[string][]byte{}
	old, oldRef := snap(t, fixture(t, "valid/spec.md"), InterpretationBasis{}, snaps)
	content := v2Spec(t, old, "spec-20260925-00000002", oldRef)
	content, contentRef := snap(t, content, InterpretationBasis{}, snaps)
	accepted := edit(t, old, func(r *Record) {
		r.ID = "spec-20260925-00000003"
		r.Supersedes = []string{oldRef}
		r.SupersedeReason = "Independent accepted revision"
	})
	accepted, acceptedRef := snap(t, accepted, InterpretationBasis{}, snaps)
	wantHeads := []string{contentRef, acceptedRef}
	sort.Strings(wantHeads)
	for _, documents := range [][]Document{{old, content, accepted}, {accepted, content, old}} {
		p := Project(documents, snaps)
		if got := p.Heads(old.Record.ID); !reflect.DeepEqual(got, wantHeads) {
			t.Fatalf("mixed heads=%v, want %v", got, wantHeads)
		}
		if p.Resolve("spec:order-cancel").Kind != "conflict" {
			t.Fatal("mixed v1/v2 heads did not conflict")
		}
		for _, e := range p.Entries {
			if e.Ref == oldRef && e.State != Superseded {
				t.Fatal("shared exact predecessor was not retired")
			}
			if e.Ref != contentRef && e.Ref != acceptedRef {
				continue
			}
			want := Contested
			if e.Ref == contentRef {
				want = ContentContested
			}
			if e.State != want || !hasCode(e.Diagnostics, "mixed_format_heads") {
				t.Fatalf("mixed head lacks contest state/diagnostic: %+v", e)
			}
			for _, d := range e.Diagnostics {
				if d.Code == "mixed_format_heads" && (!strings.Contains(d.Message, contentRef) || !strings.Contains(d.Message, acceptedRef) || !strings.Contains(d.Message, "separate authorized repair")) {
					t.Fatalf("mixed diagnostic omits participants or repair boundary: %+v", d)
				}
			}
		}
		if !hasCode(ValidateReauthorBasis(p, snaps, acceptedRef), "reauthor_head_conflict") {
			t.Fatal("mixed branches were silently admitted to selected reauthor")
		}
	}
}
