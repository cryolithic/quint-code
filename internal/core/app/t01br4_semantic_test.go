package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01BR4RememberKeepsSchemaStringsAndOptionalAbsence(t *testing.T) {
	note := "---\nkind: note\ntitle: Controller scalar\nabout: domain:Billing.OrderCancellation\n%s---\nRationale.\n"
	evidence := "---\nformat: haft/1\nkind: evidence\ntitle: Cancellation property result\nstatus: active\norigin: agent_proposal\nabout: domain:Billing.OrderCancellation\ncreated_at: \"2026-09-23T10:00:00Z\"\nclaim: The total remained unchanged\nobserved_at: 2026-09-23T09:59:00Z\nmethod: Property test\nsource: reports/cancel-property.txt\nbasis:\n  kind: code\n  ref: build:cancel-fixture-001\n  conditions: Go property fixture\n---\nController observation.\n"
	cases := []struct {
		name    string
		raw     string
		title   string
		observe string
	}{
		{"observed-unquoted", evidence, "Cancellation property result", "2026-09-23T09:59:00Z"},
		{"updated-unquoted", fmt.Sprintf(note, "updated_at: 2026-09-26T00:00:00Z\n"), "Controller scalar", ""},
		{"reopen-date", fmt.Sprintf(note, "reopen_when: 2026-12-01\n"), "Controller scalar", ""},
		{"numeric-title", strings.Replace(fmt.Sprintf(note, ""), "title: Controller scalar", "title: 2026", 1), "2026", ""},
		{"empty-supersedes", fmt.Sprintf(note, "supersedes: []\n"), "Controller scalar", ""},
		{"null-supersedes", fmt.Sprintf(note, "supersedes: null\n"), "Controller scalar", ""},
		{"operator-false", fmt.Sprintf(note, "operator_confirmed: false\n"), "Controller scalar", ""},
		{"empty-reopen", fmt.Sprintf(note, "reopen_when: \"\"\n"), "Controller scalar", ""},
		{"space-time-extension", fmt.Sprintf(note, "x-value: 2026-09-25 10:00:00\n"), "Controller scalar", ""},
		{"lower-t-extension", fmt.Sprintf(note, "x-value: 2026-09-25t10:00:00Z\n"), "Controller scalar", ""},
		{"short-date-extension", fmt.Sprintf(note, "x-value: 2026-9-5T10:00:00Z\n"), "Controller scalar", ""},
		{"plus-infinity-extension", fmt.Sprintf(note, "x-value: +.inf\n"), "Controller scalar", ""},
		{"unknown-empty-sequence", fmt.Sprintf(note, "x-value: []\n"), "Controller scalar", ""},
		{"unknown-null", fmt.Sprintf(note, "x-value: null\n"), "Controller scalar", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := service(t)
			q := Request{Operation: "remember", RequestID: "t01br4-" + tc.name, Carrier: tc.raw}
			run(t, s, q, "written")
			beforeReplay := readView(t, s)
			if len(beforeReplay.Documents) != 1 || beforeReplay.Documents[0].Record.Title != tc.title || beforeReplay.Documents[0].Record.ObservedAt != tc.observe {
				t.Fatalf("schema string was not retained: %+v", beforeReplay.Documents)
			}
			run(t, s, q, "replayed")
			afterReplay := readView(t, s)
			if afterReplay.Generation != beforeReplay.Generation || !bytes.Equal(afterReplay.Documents[0].Raw, beforeReplay.Documents[0].Raw) {
				t.Fatal("identical retry changed the original publication")
			}
			if strings.Contains(tc.raw, "x-value:") {
				if _, present := afterReplay.Documents[0].Record.Extra["x-value"]; !present {
					t.Fatal("unknown extension was omitted")
				}
			}
		})
	}
}

func TestT01BR4RememberStillRefusesRetainedExtensionLoss(t *testing.T) {
	note := "---\nkind: note\ntitle: Controller scalar\nabout: domain:Billing.OrderCancellation\n%s---\nRationale.\n"
	for _, tc := range []struct{ name, extra, path string }{
		{"custom", "x-value: !vendor abc\n", "x-value"},
		{"large-number", "x-value: 123456789012345678901234\n", "x-value"},
		{"integral-float", "x-value: 1.0\n", "x-value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := service(t)
			before := readView(t, s)
			refused := run(t, s, Request{Operation: "remember", RequestID: "t01br4-refuse-" + tc.name, Carrier: fmt.Sprintf(note, tc.extra)}, "invalid")
			t01br3AssertLossPath(t, refused.Diagnostics, tc.path)
			after := readView(t, s)
			if after.Generation != before.Generation || len(after.Documents) != 0 {
				t.Fatal("semantic refusal published a carrier")
			}
		})
	}
}

func TestT01BR4ChangeCreateUsesChangeSchemaWithoutLosingExtensions(t *testing.T) {
	raw := "---\nformat: haft.change/1\nid: chg-20260926-010bc017\nchange_key: chg-20260926-010bc017\ntitle: 2026\nintent: Inspect a bounded change\nstate: open\ncreated_at: 2026-09-26T00:00:00Z\nno_spec_change_reason: Administrative change\ntasks: []\nx-empty: []\n---\nAuthored rationale.\n"
	s := service(t)
	q := Request{Operation: "change", Action: "create", RequestID: "t01br4-change-schema", Carrier: raw}
	run(t, s, q, "written")
	run(t, s, q, "replayed")
	docs := changeDocuments(readView(t, s))
	if len(docs) != 1 || docs[0].Change.Title != "2026" || docs[0].Change.Format != change.Format {
		t.Fatalf("change schema string or carrier lost: %+v", docs)
	}
	if _, present := docs[0].Change.Extra["x-empty"]; !present {
		t.Fatal("change extension was omitted")
	}

	other := service(t)
	before := readView(t, other)
	tagged := strings.Replace(raw, "x-empty: []", "x-empty: !vendor abc", 1)
	refused := run(t, other, Request{Operation: "change", Action: "create", RequestID: "t01br4-change-tag", Carrier: tagged}, "invalid")
	t01br3AssertLossPath(t, refused.Diagnostics, "x-empty")
	after := readView(t, other)
	if after.Generation != before.Generation || len(changeDocuments(after)) != 0 {
		t.Fatal("tagged change extension was published")
	}
}

func TestT01BR4SpecFormsKeepDeclaredNestedStringsAndOptionalAbsence(t *testing.T) {
	for _, format := range []string{"haft/1", "haft/2"} {
		t.Run(format, func(t *testing.T) {
			s := service(t)
			legacy := ""
			if format == "haft/1" {
				legacy = "status: proposed\norigin: agent_proposal\n"
			}
			raw := "---\nformat: " + format + "\nkind: spec\ntitle: 2026\n" + legacy + "about: domain:Billing.OrderCancellation\nslug: drafted-rule\nreceiving_use: Inspect the local rule\nclaims:\n  - id: rule\n    kind: definition\n    text: 42\n    refs: []\n---\nBody.\n"
			run(t, s, Request{Operation: "remember", RequestID: "spec-" + format, Carrier: raw}, "written")
			got := readView(t, s).Documents[0]
			if got.Record.Title != "2026" || got.Record.Claims[0].Text != "42" || got.Record.Format != format {
				t.Fatalf("declared string/optional content changed: %+v", got.Record)
			}
			run(t, s, Request{Operation: "remember", RequestID: "spec-" + format, Carrier: raw}, "replayed")
			if !bytes.Equal(readView(t, s).Documents[0].Raw, got.Raw) {
				t.Fatal("identical retry changed original spec bytes")
			}
		})
	}
}

func TestT01BR4SelectedReauthorPreservesKnownStringMeaning(t *testing.T) {
	s := service(t)
	old, _ := seedT01BActiveSpec(t, s)
	raw := strings.Replace(string(old.Raw), "title: Cancel preserves total", "title: 2026", 1)
	raw = strings.Replace(raw, "    text: Cancel preserves total", "    text: 42", 1)
	before, ref := t01br3ExternalSpec(t, s, old, raw)
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "ready")
	again := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "ready")
	if preview.Basis["preview_digest"] != again.Basis["preview_digest"] || len(preview.Data.(map[string]any)["material_losses"].([]string)) != 0 {
		t.Fatalf("declared strings produced a false reauthor loss: %+v", preview)
	}
	q := Request{Operation: "change", Action: "reauthor_apply", Ref: ref, RequestID: "t01br4-reauthor-strings", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	run(t, s, q, "written")
	run(t, s, q, "replayed")
	view := readView(t, s)
	if len(view.Documents) != 2 || !bytes.Equal(view.Documents[0].Raw, before.Raw) {
		t.Fatal("reauthor changed the predecessor or published multiple successors")
	}
	var successor carrier.Document
	for _, doc := range view.Documents {
		if doc.Record.Format == "haft/2" {
			successor = doc
		}
	}
	if successor.Record.Title != "2026" || successor.Record.Claims[0].Text != "42" || successor.Record.Supersedes[0] != ref {
		t.Fatalf("reauthor lost declared string meaning or lineage: %+v", successor.Record)
	}
}

func TestT01BR4PublicRevisionCannotWaiveIgnoredTaskTag(t *testing.T) {
	s := service(t)
	id := "chg-20260927-00000001"
	c := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Inspect lifecycle", Intent: "Keep authored task", State: "open", CreatedAt: s.now(), NoSpecChangeReason: "No spec delta", Tasks: []change.Task{{ID: "one", Text: "Inspect source", Extra: carrier.Extra{"x-owner": "abc"}}}}
	raw, err := change.Encode(c, []byte("Rationale.\n"))
	if err != nil {
		t.Fatal(err)
	}
	run(t, s, Request{Operation: "change", Action: "create", RequestID: "t01br4-task-create", Carrier: string(raw)}, "written")
	path := filepath.Join(s.Root, ".haft", "changes", id+".md")
	tagged := strings.Replace(string(raw), "x-owner: abc", "x-owner: !vendor abc", 1)
	if tagged == string(raw) {
		t.Fatal("task tag fixture was not inserted")
	}
	if err := os.WriteFile(path, []byte(tagged), 0600); err != nil {
		t.Fatal(err)
	}
	before := readView(t, s)
	empty := []change.Task{}
	q := Request{Operation: "change", Action: "archive", Ref: id, RequestID: "t01br4-task-archive", Revision: &change.Revision{Reason: "Preserve completed review", Tasks: &empty}}
	full := run(t, s, q, "conflict")
	t01br3AssertLossPath(t, full.Diagnostics, "tasks[0].x-owner")
	refused := public(t, s, q)
	if refused.Kind != "conflict" || !refused.IsError || delivery.Size(refused) > delivery.Budget {
		t.Fatalf("ignored task exempted retained tag: %+v", refused)
	}
	part := collectPart(t, s, deliveredPart(t, s, refused, "data_diagnostics"))
	if !bytes.Contains(part, []byte("tasks[0].x-owner")) {
		t.Fatalf("public continuation omitted retained task path: %s", part)
	}
	after := readView(t, s)
	if after.Generation != before.Generation || len(changeDocuments(after)) != 1 || !bytes.Equal(after.Files["changes/"+id+".md"], before.Files["changes/"+id+".md"]) {
		t.Fatal("refused archive changed the tagged predecessor")
	}
}

func TestT01BR4PublicBindingEditCannotRetagRetainedExtension(t *testing.T) {
	s := service(t)
	old, _ := seed(t, s)
	raw := string(old.Raw)
	needle := "covers: New and paid states; 1000 generated signed integer totals, seed 23; no universal proof."
	index := strings.Index(raw, needle)
	if index < 0 {
		t.Fatal("binding scope fixture was not found")
	}
	lineStart := strings.LastIndex(raw[:index], "\n") + 1
	indent := raw[lineStart:index]
	raw = strings.Replace(raw, needle, needle+"\n"+indent+"x-owner: !vendor abc", 1)
	if raw == string(old.Raw) {
		t.Fatal("binding tag fixture was not inserted")
	}
	path := filepath.Join(s.Root, ".haft", "specs", old.Record.ID+".md")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	before := readView(t, s)
	base := before.Documents[0]
	if !base.Valid() {
		t.Fatalf("tagged predecessor became unreadable: %+v", base.Diagnostics)
	}
	ref := base.Record.ID + "@" + base.Edition
	claim := base.Record.Claims[0]
	claim.Checks[0].Covers += " Clarified coverage."
	id := "chg-20260927-00000002"
	c := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Clarify check", Intent: "Retain the check extension", State: "open", CreatedAt: s.now(), Patches: []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify check coverage"}}}}}
	changeRaw, err := change.Encode(c, []byte("Rationale.\n"))
	if err != nil {
		t.Fatal(err)
	}
	run(t, s, Request{Operation: "change", Action: "create", RequestID: "t01br4-binding-create", Carrier: string(changeRaw), Snapshots: map[string][]byte{base.Edition: before.CurrentSnapshots[base.Edition]}}, "written")
	preApply := readView(t, s)
	preview := public(t, s, Request{Operation: "change", Action: "preview", Ref: id})
	if preview.Kind != "conflict" || !preview.IsError || delivery.Size(preview) > delivery.Budget {
		t.Fatalf("binding preview hid retained tag loss: %+v", preview)
	}
	t01br3AssertLossPath(t, preview.Diagnostics, "claims[0].checks[0].x-owner")
	for _, action := range []string{"apply", "sync"} {
		q := Request{Operation: "change", Action: action, Ref: id, RequestID: "t01br4-refuse-" + action, ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
		refused := public(t, s, q)
		if refused.Kind != "conflict" || !refused.IsError {
			t.Fatalf("%s published the retained binding tag loss: %+v", action, refused)
		}
	}
	after := readView(t, s)
	if after.Generation != preApply.Generation || len(after.Documents) != len(preApply.Documents) || !bytes.Equal(after.Documents[0].Raw, base.Raw) {
		t.Fatal("preview/apply/sync changed the tagged predecessor")
	}
}
