package transport

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

type t01br5Case struct {
	name     string
	prepare  func(string) string
	modify   func(*carrier.Claim)
	conflict string
	path     string
}

func t01br5EvidenceSeed(t *testing.T, client *t01aProtocolClient, tool, root, format string) (store.View, carrier.Document) {
	t.Helper()
	baseRaw := strings.Replace(t01br5Spec(format), "kind: definition", "kind: guard", 1)
	seed := client.mustCall(t, tool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br5-evidence-spec", Carrier: baseRaw})
	if seed.Kind != "written" || seed.IsError {
		t.Fatalf("cannot seed guard: kind=%s diagnostics=%+v", seed.Kind, seed.Diagnostics)
	}
	view, err := (store.Store{Root: root}).Read(context.Background())
	if err != nil || len(view.Documents) != 1 {
		t.Fatalf("cannot read guard: %v", err)
	}
	base := view.Documents[0]
	ref := base.Record.ID + "@" + base.Edition
	evidenceRaw := fmt.Sprintf("---\nformat: haft/1\nid: ev-20260927-00000001\nkind: evidence\ntitle: Local check\nstatus: active\norigin: agent_proposal\nabout: domain:Billing.Semantic\nclaim: One observation\nobserved_at: 2026-09-27T00:00:00Z\nmethod: Local fixture\nsource: reports/local.txt\nbasis:\n  kind: code\n  ref: build:local-fixture\n  conditions: Isolated test\nuses:\n  - id: check-1\n    target: %s#rule\n    polarity: supports\n    scope: One local observation\n  - id: check-2\n    target: %s#rule\n    polarity: supports\n    scope: Second local observation\n---\nEvidence body.\n", ref, ref)
	evidence := client.mustCall(t, tool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br5-evidence-use", Carrier: evidenceRaw})
	if evidence.Kind != "written" || evidence.IsError {
		t.Fatalf("cannot seed evidence use: kind=%s diagnostics=%+v", evidence.Kind, evidence.Diagnostics)
	}
	view, err = (store.Store{Root: root}).Read(context.Background())
	if err != nil || len(view.Documents) != 2 {
		t.Fatalf("cannot read evidence: %v", err)
	}
	var evidenceDoc carrier.Document
	for _, doc := range view.Documents {
		if doc.Record.Kind == "evidence" {
			evidenceDoc = doc
		}
	}
	if evidenceDoc.Record.ID == "" {
		t.Fatal("evidence use was not retained")
	}
	evidenceRef := evidenceDoc.Record.ID + "@" + evidenceDoc.Edition + "#check-1"
	path := filepath.Join(root, ".haft", "specs", base.Record.ID+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := "      text: Original meaning\n"
	addition := "      evidence_inputs:\n        - ref: " + evidenceRef + "\n          applicability: First scope\n" +
		"        - ref: " + evidenceRef + "\n          applicability: Second scope\n"
	authored := strings.Replace(string(raw), line, line+addition, 1)
	if authored == string(raw) {
		t.Fatal("evidence input fixture did not attach to the guard")
	}
	if err := os.WriteFile(path, []byte(authored), 0600); err != nil {
		t.Fatal(err)
	}
	view, err = (store.Store{Root: root}).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range view.Documents {
		if doc.Record.Kind == "spec" {
			if !doc.Valid() || len(doc.Record.Claims[0].EvidenceInputs) != 2 {
				t.Fatalf("evidence input fixture is not readable: %+v", doc.Diagnostics)
			}
			return view, doc
		}
	}
	t.Fatal("authored guard is missing")
	return store.View{}, carrier.Document{}
}

func TestT01BR5PublicDuplicateEvidenceEditsBothFormatsAndProfiles(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, action := range []string{"edit", "add", "remove", "replace", "tagged-edit", "disjoint", "tagged-disjoint"} {
				t.Run(profile.name+"/"+format+"/"+action, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)
					before, base := t01br5EvidenceSeed(t, client, profile.writeTool, root, format)
					ref := base.Record.ID + "@" + base.Edition
					if action == "tagged-edit" || action == "disjoint" || action == "tagged-disjoint" {
						path := filepath.Join(root, ".haft", "specs", base.Record.ID+".md")
						owner := "abc"
						if action == "tagged-edit" || action == "tagged-disjoint" {
							owner = "!vendor abc"
						}
						tagged := strings.Replace(string(base.Raw), "applicability: First scope", "applicability: First scope\n          x-owner: "+owner, 1)
						if err := os.WriteFile(path, []byte(tagged), 0600); err != nil {
							t.Fatal(err)
						}
						before, _ = (store.Store{Root: root}).Read(context.Background())
						for _, doc := range before.Documents {
							if doc.Record.Kind == "spec" {
								base = doc
							}
						}
						ref = base.Record.ID + "@" + base.Edition
					}
					claim := base.Record.Claims[0]
					switch action {
					case "edit", "tagged-edit":
						claim.EvidenceInputs[1].Applicability = "Edited second scope"
					case "add":
						claim.EvidenceInputs = append(claim.EvidenceInputs, carrier.EvidenceInput{Ref: claim.EvidenceInputs[0].Ref, Applicability: "Third scope"})
					case "remove":
						claim.EvidenceInputs = []carrier.EvidenceInput{}
					case "replace":
						claim.EvidenceInputs = []carrier.EvidenceInput{{Ref: claim.EvidenceInputs[0].Ref, Applicability: "Replacement scope"}}
					case "disjoint", "tagged-disjoint":
						for _, doc := range before.Documents {
							if doc.Record.Kind == "evidence" {
								claim.EvidenceInputs = []carrier.EvidenceInput{{Ref: doc.Record.ID + "@" + doc.Edition + "#check-2", Applicability: "New pinned use"}}
							}
						}
					}
					id := "chg-20260927-00000015"
					candidate := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Edit evidence use", Intent: "Keep exact scope", State: "open", CreatedAt: "2026-09-27T00:00:00Z", Patches: []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit selected evidence input"}}}}}
					changeRaw, err := change.Encode(candidate, []byte("Rationale.\n"))
					if err != nil {
						t.Fatal(err)
					}
					created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "t01br5-evidence-create", Carrier: string(changeRaw), Snapshots: map[string][]byte{base.Edition: before.CurrentSnapshots[base.Edition]}})
					if created.Kind != "written" || created.IsError {
						t.Fatalf("evidence change create failed: kind=%s diagnostics=%+v", created.Kind, created.Diagnostics)
					}
					preview := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "preview", Ref: id})
					if action == "tagged-edit" {
						if preview.Kind != "conflict" {
							t.Fatalf("tagged duplicate evidence input was changed: kind=%s diagnostics=%+v", preview.Kind, preview.Diagnostics)
						}
						t01br4MCPDiagnostic(t, client, profile.readTool, preview, "ambiguous_binding_correspondence", ref+".operations[0].claim.evidence_inputs")
						return
					}
					if preview.Kind != "ready" || preview.IsError {
						t.Fatalf("plain evidence edit was refused: kind=%s diagnostics=%+v", preview.Kind, preview.Diagnostics)
					}
					apply := app.Request{Format: delivery.Format, Operation: "change", Action: "apply", Ref: id, RequestID: "t01br5-evidence-apply", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
					written := client.mustCall(t, profile.changeTool, apply)
					if written.Kind != "written" || written.IsError {
						t.Fatalf("evidence edit was not published: kind=%s diagnostics=%+v", written.Kind, written.Diagnostics)
					}
					if replay := client.mustCall(t, profile.changeTool, apply); replay.Kind != "replayed" {
						t.Fatalf("evidence edit did not replay: kind=%s diagnostics=%+v", replay.Kind, replay.Diagnostics)
					}
					latest, err := (store.Store{Root: root}).Read(context.Background())
					if err != nil || !bytes.Equal(latest.Files["specs/"+base.Record.ID+".md"], base.Raw) {
						t.Fatalf("evidence edit changed predecessor bytes: %v", err)
					}
					successor := t01br6Successor(t, latest, ref).Record.Claims[0]
					switch action {
					case "edit":
						if successor.EvidenceInputs[1].Applicability != "Edited second scope" {
							t.Fatalf("edited applicability was not published: %+v", successor.EvidenceInputs)
						}
					case "add":
						if len(successor.EvidenceInputs) != 3 || successor.EvidenceInputs[2].Applicability != "Third scope" {
							t.Fatalf("third evidence input was not published: %+v", successor.EvidenceInputs)
						}
					case "remove":
						if len(successor.EvidenceInputs) != 0 {
							t.Fatalf("evidence inputs were not cleared: %+v", successor.EvidenceInputs)
						}
					case "replace":
						if len(successor.EvidenceInputs) != 1 || successor.EvidenceInputs[0].Applicability != "Replacement scope" {
							t.Fatalf("evidence input replacement was not published: %+v", successor.EvidenceInputs)
						}
					case "disjoint", "tagged-disjoint":
						if len(successor.EvidenceInputs) != 1 || successor.EvidenceInputs[0].Ref != claim.EvidenceInputs[0].Ref || successor.EvidenceInputs[0].Applicability != "New pinned use" || len(successor.EvidenceInputs[0].Extra) != 0 {
							t.Fatalf("disjoint pinned evidence replacement was altered: %+v", successor.EvidenceInputs)
						}
					}
				})
			}
		}
	}
}

func TestT01BR5PublicTaskRevisionRetainsNestedMeaning(t *testing.T) {
	cases := []struct {
		name, action, initialState string
		tagged, omit, changedText  bool
		conflict                   bool
	}{
		{"update-tagged-copied", "update", "open", true, false, false, true},
		{"update-tagged-omitted", "update", "open", true, true, false, true},
		{"update-tagged-task-field", "update", "open", true, false, true, true},
		{"update-plain-omitted", "update", "open", false, true, false, false},
		{"archive-tagged", "archive", "open", true, false, false, true},
		{"archive-plain", "archive", "open", false, false, false, false},
		{"reopen-tagged", "reopen", "archived", true, false, false, true},
		{"reopen-plain", "reopen", "archived", false, false, false, false},
	}
	for _, profile := range t01br4MCPProfiles() {
		for _, scenario := range cases {
			t.Run(profile.name+"/"+scenario.name, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				id := "chg-20260927-00000021"
				initial := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Task revision", Intent: "Initial intent", State: scenario.initialState, CreatedAt: "2026-09-27T00:00:00Z", NoSpecChangeReason: "Local administrative fixture", Tasks: []change.Task{{ID: "inspect", Text: "Inspect the local rule", Extra: carrier.Extra{"x-owner": "abc"}}}}
				raw, err := change.Encode(initial, []byte("Original rationale.\n"))
				if err != nil {
					t.Fatal(err)
				}
				created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "t01br5-revision-create", Carrier: string(raw)})
				if created.Kind != "written" || created.IsError {
					t.Fatalf("change seed failed: kind=%s diagnostics=%+v", created.Kind, created.Diagnostics)
				}
				path := filepath.Join(root, ".haft", "changes", id+".md")
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if scenario.tagged {
					tagged := bytes.Replace(original, []byte("x-owner: abc"), []byte("x-owner: !vendor abc"), 1)
					if bytes.Equal(tagged, original) {
						t.Fatal("tag fixture was not inserted")
					}
					original = tagged
					if err := os.WriteFile(path, original, 0600); err != nil {
						t.Fatal(err)
					}
				}
				before, err := (store.Store{Root: root}).Read(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				revision := &change.Revision{ID: "chg-20260927-00000022", Reason: "Record bounded successor"}
				intent := "Revised intent"
				revision.Intent = &intent
				tasks := change.Parse(original).Change.Tasks
				if scenario.omit {
					tasks[0].Extra = nil
				}
				if scenario.changedText {
					tasks[0].Text = "Clarified task text"
				}
				revision.Tasks = &tasks
				result := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: scenario.action, Ref: id, RequestID: "t01br5-revision", Revision: revision})
				if scenario.conflict {
					if result.Kind != "conflict" || !result.IsError {
						t.Fatalf("retained task tag was changed: kind=%s diagnostics=%+v", result.Kind, result.Diagnostics)
					}
					t01br4MCPDiagnostic(t, client, profile.readTool, result, "unsupported_yaml_tag_conversion", "tasks[0].x-owner")
					after, err := (store.Store{Root: root}).Read(context.Background())
					if err != nil || after.Generation != before.Generation || !bytes.Equal(after.Files["changes/"+id+".md"], original) {
						t.Fatalf("refused revision changed old bytes or generation: %v", err)
					}
					return
				}
				if result.Kind != "written" || result.IsError {
					t.Fatalf("plain revision failed: kind=%s diagnostics=%+v", result.Kind, result.Diagnostics)
				}
				after, err := (store.Store{Root: root}).Read(context.Background())
				if err != nil || !bytes.Equal(after.Files["changes/"+id+".md"], original) {
					t.Fatalf("revision changed predecessor bytes: %v", err)
				}
				successor := change.Parse(after.Files["changes/chg-20260927-00000022.md"])
				if carrier.HasErrors(successor.Diagnostics) || successor.Change.Tasks[0].Extra["x-owner"] != "abc" {
					t.Fatalf("ordinary task extension did not survive: %+v", successor)
				}
			})
		}
	}
}

func TestT01BR5PublicRebaseKeepsUnchangedNestedBindingTag(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, customTag := range []bool{false, true} {
				name := "plain"
				if customTag {
					name = "tagged"
				}
				t.Run(profile.name+"/"+format+"/"+name, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)
					seeded := client.mustCall(t, profile.writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br5-rebase-old", Carrier: t01br5Spec(format)})
					if seeded.Kind != "written" || seeded.IsError {
						t.Fatalf("old base failed: kind=%s diagnostics=%+v", seeded.Kind, seeded.Diagnostics)
					}
					view, err := (store.Store{Root: root}).Read(context.Background())
					if err != nil || len(view.Documents) != 1 {
						t.Fatalf("old base unavailable: %v", err)
					}
					base := view.Documents[0]
					oldRef := base.Record.ID + "@" + base.Edition
					claim := base.Record.Claims[0]
					claim.Text = "Clarified rule text"
					claim.Checks[0].Extra = carrier.Extra{"x-owner": "abc"}
					id := "chg-20260927-00000031"
					candidate := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Rebase one patch", Intent: "Keep operation meaning", State: "open", CreatedAt: "2026-09-27T00:00:00Z", Patches: []change.SectionPatch{{Base: oldRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify rule"}}}}}
					changeRaw, err := change.Encode(candidate, []byte("Rationale.\n"))
					if err != nil {
						t.Fatal(err)
					}
					created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "t01br5-rebase-change", Carrier: string(changeRaw), Snapshots: map[string][]byte{base.Edition: view.CurrentSnapshots[base.Edition]}})
					if created.Kind != "written" || created.IsError {
						t.Fatalf("old change failed: kind=%s diagnostics=%+v", created.Kind, created.Diagnostics)
					}
					path := filepath.Join(root, ".haft", "changes", id+".md")
					original, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					authored := original
					if customTag {
						authored = bytes.Replace(original, []byte("x-owner: abc"), []byte("x-owner: !vendor abc"), 1)
						if bytes.Equal(authored, original) {
							t.Fatal("nested change tag was not inserted")
						}
						if err := os.WriteFile(path, authored, 0600); err != nil {
							t.Fatal(err)
						}
					}
					view, err = (store.Store{Root: root}).Read(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					next := base.Record
					next.ID = "spec-20260927-00000032"
					next.CreatedAt = "2026-09-27T01:00:00Z"
					next.WriteReceipt = nil
					next.Supersedes = []string{oldRef}
					next.SupersedeReason = "Replace local basis for exact rebase"
					nextRaw, err := carrier.Encode(next, base.Body)
					if err != nil {
						t.Fatal(err)
					}
					published := client.mustCall(t, profile.writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br5-rebase-new", Carrier: string(nextRaw), ExpectedGeneration: view.Generation, ExpectedHeads: []string{oldRef}})
					if published.Kind != "written" || published.IsError {
						t.Fatalf("new base failed: kind=%s diagnostics=%+v", published.Kind, published.Diagnostics)
					}
					view, err = (store.Store{Root: root}).Read(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					newRef := ""
					for _, doc := range view.Documents {
						if doc.Record.ID == next.ID {
							newRef = doc.Record.ID + "@" + doc.Edition
						}
					}
					if newRef == "" || newRef == oldRef {
						t.Fatal("new exact base was not published")
					}
					patches := change.Parse(authored).Change.Patches
					patches[0].Base = newRef
					before := view.Generation
					result := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "t01br5-rebase", Revision: &change.Revision{ID: "chg-20260927-00000032", Reason: "Rebase exact local patch", Patches: patches}})
					if !customTag {
						if result.Kind != "written" || result.IsError {
							t.Fatalf("plain rebase was refused: kind=%s diagnostics=%+v", result.Kind, result.Diagnostics)
						}
						if replay := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "t01br5-rebase", Revision: &change.Revision{ID: "chg-20260927-00000032", Reason: "Rebase exact local patch", Patches: patches}}); replay.Kind != "replayed" {
							t.Fatalf("plain rebase did not replay: kind=%s diagnostics=%+v", replay.Kind, replay.Diagnostics)
						}
						return
					}
					if result.Kind != "conflict" || !result.IsError {
						t.Fatalf("rebase retagged unchanged binding: kind=%s diagnostics=%+v", result.Kind, result.Diagnostics)
					}
					t01br4MCPDiagnostic(t, client, profile.readTool, result, "unsupported_yaml_tag_conversion", "patches[0].operations[0].claim.checks[0].x-owner")
					view, err = (store.Store{Root: root}).Read(context.Background())
					if err != nil || view.Generation != before || !bytes.Equal(view.Files["changes/"+id+".md"], authored) {
						t.Fatalf("refused rebase changed history: %v", err)
					}
				})
			}
		}
	}
}

func t01br5Spec(format string) string {
	legacy := "status: proposed\norigin: agent_proposal\n"
	if format == "haft/2" {
		legacy = ""
	}
	return "---\nformat: " + format + "\nid: spec-20260927-00000011\nkind: spec\ntitle: Binding basis\n" + legacy +
		"about: domain:Billing.Semantic\nslug: local-rule\nreceiving_use: Review a local claim\nclaims:\n" +
		"  - id: rule\n    kind: definition\n    text: Original meaning\n" +
		"    checks:\n      - ref: test:rule_test.go::TestRule\n        covers: First coverage\n" +
		"      - ref: test:rule_test.go::TestRule\n        covers: Second coverage\n" +
		"    implemented_by:\n      - ref: sym:rule.go::Rule.Apply\n        covers: First implementation\n" +
		"      - ref: sym:rule.go::Rule.Apply\n        covers: Second implementation\n" +
		"    examples:\n      - id: sample\n        given: A sample\n        when: Rule applies\n        then: It holds\n        x-meta: {a: abc, b: 2}\n" +
		"    x-meta: {a: abc, b: 2}\n---\nOriginal body.\n"
}

func t01br5Cases() []t01br5Case {
	return []t01br5Case{
		{name: "duplicate-check-edit", modify: func(c *carrier.Claim) { c.Checks[1].Covers = "Edited coverage" }},
		{name: "duplicate-implementation-add", modify: func(c *carrier.Claim) {
			c.ImplementedBy = append(c.ImplementedBy, carrier.Binding{Ref: c.ImplementedBy[0].Ref, Covers: "Third implementation"})
		}},
		{name: "duplicate-check-replace", modify: func(c *carrier.Claim) {
			c.Checks = []carrier.Binding{{Ref: "test:other_test.go::TestOther", Covers: "Replacement coverage"}}
		}},
		{name: "duplicate-check-extra-disjoint", prepare: func(raw string) string {
			return t01br5AddBindingExtension(raw, "abc")
		}, modify: func(c *carrier.Claim) {
			c.Checks = []carrier.Binding{{Ref: "test:other_test.go::TestOther", Covers: "Replacement coverage"}}
		}},
		{name: "duplicate-check-tag-disjoint", prepare: func(raw string) string {
			return t01br5AddBindingExtension(raw, "!vendor abc")
		}, modify: func(c *carrier.Claim) {
			c.Checks = []carrier.Binding{{Ref: "test:other_test.go::TestOther", Covers: "Replacement coverage"}}
		}},
		{name: "duplicate-implementation-extra-disjoint", prepare: func(raw string) string {
			return strings.Replace(raw, "covers: First implementation", "covers: First implementation\n          x-owner: abc", 1)
		}, modify: func(c *carrier.Claim) {
			c.ImplementedBy = []carrier.Binding{{Ref: "sym:new.go::New", Covers: "New implementation"}}
		}},
		{name: "claim-leaf-edit", modify: func(c *carrier.Claim) { c.Extra["x-meta"].(map[string]any)["b"] = 3 }},
		{name: "claim-retained-tag", prepare: func(raw string) string {
			return t01br5ReplaceLast(raw, "a: abc", "a: !vendor abc")
		}, modify: func(c *carrier.Claim) { c.Extra["x-meta"].(map[string]any)["b"] = 3 },
			conflict: "unsupported_yaml_tag_conversion", path: "claims[0].x-meta.a"},
		{name: "claim-explicit-leaf-replace", prepare: func(raw string) string {
			return t01br5ReplaceLast(raw, "a: abc", "a: !vendor abc")
		}, modify: func(c *carrier.Claim) { c.Extra["x-meta"].(map[string]any)["a"] = "replaced" }},
		{name: "example-retained-tag", prepare: func(raw string) string {
			return strings.Replace(raw, "a: abc", "a: !vendor abc", 1)
		}, modify: func(c *carrier.Claim) { c.Examples[0].Extra["x-meta"].(map[string]any)["b"] = 3 },
			conflict: "unsupported_yaml_tag_conversion", path: "claims[0].examples[0].x-meta.a"},
		{name: "sequence-element-edit", prepare: func(raw string) string {
			return t01br5ClaimSequence(raw, "[a, b]")
		}, modify: func(c *carrier.Claim) { c.Extra["x-meta"].([]any)[1] = "c" }},
		{name: "sequence-retained-tag", prepare: func(raw string) string {
			return t01br5ClaimSequence(raw, "[!vendor a, b]")
		}, modify: func(c *carrier.Claim) { c.Extra["x-meta"].([]any)[1] = "c" },
			conflict: "unsupported_yaml_tag_conversion", path: "claims[0].x-meta[0]"},
		{name: "sequence-ambiguous-move", prepare: func(raw string) string {
			return t01br5ClaimSequence(raw, "[a, b]")
		}, modify: func(c *carrier.Claim) { c.Extra["x-meta"] = []any{"b", "a"} },
			conflict: "ambiguous_yaml_correspondence", path: "claims[0].x-meta"},
		{name: "sequence-unsupported-shape", prepare: func(raw string) string {
			return t01br5ClaimSequence(raw, "[a, b]")
		}, modify: func(c *carrier.Claim) { c.Extra["x-meta"] = map[string]any{"a": "a", "b": "b"} },
			conflict: "ambiguous_yaml_correspondence", path: "claims[0].x-meta"},
		{name: "duplicate-retained-tag", prepare: func(raw string) string {
			return t01br5AddBindingExtension(raw, "!vendor abc")
		}, modify: func(c *carrier.Claim) { c.Checks[1].Covers = "Edited coverage" },
			conflict: "ambiguous_binding_correspondence", path: ".operations[0].claim.checks"},
		{name: "duplicate-retained-number", prepare: func(raw string) string {
			return t01br5AddBindingExtension(raw, "123456789012345678901234")
		}, modify: func(c *carrier.Claim) { c.Checks[1].Covers = "Edited coverage" },
			conflict: "ambiguous_binding_correspondence", path: ".operations[0].claim.checks"},
	}
}

func t01br5ReplaceLast(raw, old, next string) string {
	index := strings.LastIndex(raw, old)
	if index < 0 {
		return raw
	}
	return raw[:index] + next + raw[index+len(old):]
}

func t01br5ClaimSequence(raw, sequence string) string {
	start := strings.LastIndex(raw, "x-meta:\n")
	if start < 0 {
		return raw
	}
	end := strings.Index(raw[start:], "\nreceiving_use:")
	if end < 0 {
		return raw
	}
	return raw[:start] + "x-meta: " + sequence + raw[start+end:]
}

func t01br5AddBindingExtension(raw, value string) string {
	old := "covers: First coverage"
	index := strings.Index(raw, old)
	if index < 0 {
		return raw
	}
	line := strings.LastIndex(raw[:index], "\n") + 1
	indent := raw[line:index]
	return raw[:index] + old + "\n" + indent + "x-owner: " + value + raw[index+len(old):]
}

func TestT01BR5PublicBindingEffectsBothFormatsAndProfiles(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, scenario := range t01br5Cases() {
				t.Run(profile.name+"/"+format+"/"+scenario.name, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)
					seed := client.mustCall(t, profile.writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br5-seed", Carrier: t01br5Spec(format)})
					if seed.Kind != "written" || seed.IsError {
						t.Fatalf("cannot seed %s: %+v", format, seed)
					}
					path := filepath.Join(root, ".haft", "specs", "spec-20260927-00000011.md")
					original, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					authored := string(original)
					if scenario.prepare != nil {
						authored = scenario.prepare(authored)
						if authored == string(original) {
							t.Fatal("external fixture did not alter the source")
						}
						if err := os.WriteFile(path, []byte(authored), 0600); err != nil {
							t.Fatal(err)
						}
					}
					before, err := (store.Store{Root: root}).Read(context.Background())
					if err != nil || len(before.Documents) != 1 || !before.Documents[0].Valid() {
						t.Fatalf("authored %s base is not readable: %v %+v", format, err, before)
					}
					base := before.Documents[0]
					if base.Record.Format != format {
						t.Fatalf("wrong actual format: %s", base.Record.Format)
					}
					claim := base.Record.Claims[0]
					scenario.modify(&claim)
					ref := base.Record.ID + "@" + base.Edition
					id := "chg-20260927-00000012"
					candidate := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Edit local meaning", Intent: "Exercise exact public writer semantics", State: "open", CreatedAt: "2026-09-27T00:00:00Z", Patches: []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit one selected claim"}}}}}
					changeRaw, err := change.Encode(candidate, []byte("Rationale.\n"))
					if err != nil {
						t.Fatal(err)
					}
					created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "t01br5-create", Carrier: string(changeRaw), Snapshots: map[string][]byte{base.Edition: before.CurrentSnapshots[base.Edition]}})
					if created.Kind != "written" || created.IsError {
						t.Fatalf("cannot create change: %+v", created)
					}
					preApply, err := (store.Store{Root: root}).Read(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					preview := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "preview", Ref: id})
					if scenario.conflict != "" {
						if preview.Kind != "conflict" || !preview.IsError {
							t.Fatalf("retained meaning was published: %+v", preview)
						}
						lossPath := scenario.path
						if strings.HasPrefix(lossPath, ".operations") {
							lossPath = ref + lossPath
						}
						t01br4MCPDiagnostic(t, client, profile.readTool, preview, scenario.conflict, lossPath)
						for _, action := range []string{"apply", "sync"} {
							refused := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: action, Ref: id, RequestID: "t01br5-" + action, ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]})
							if refused.Kind != "conflict" || !refused.IsError {
								t.Fatalf("%s did not refuse retained loss: %+v", action, refused)
							}
						}
						latest, err := (store.Store{Root: root}).Read(context.Background())
						if err != nil || latest.Generation != preApply.Generation || !bytes.Equal(latest.Files["specs/"+base.Record.ID+".md"], []byte(authored)) {
							t.Fatalf("refusal changed base bytes or generation: %v %+v", err, latest)
						}
						return
					}
					if preview.Kind != "ready" || preview.IsError || preview.Basis["preview_digest"] == "" {
						t.Fatalf("plain edit was refused: %+v", preview)
					}
					apply := app.Request{Format: delivery.Format, Operation: "change", Action: "apply", Ref: id, RequestID: "t01br5-apply", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
					written := client.mustCall(t, profile.changeTool, apply)
					if written.Kind != "written" || written.IsError {
						t.Fatalf("plain edit was not published: %+v", written)
					}
					if replay := client.mustCall(t, profile.changeTool, apply); replay.Kind != "replayed" || replay.IsError {
						t.Fatalf("old request did not replay: %+v", replay)
					}
					after, err := (store.Store{Root: root}).Read(context.Background())
					if err != nil || !bytes.Equal(after.Files["specs/"+base.Record.ID+".md"], []byte(authored)) || len(after.Documents) != 2 {
						t.Fatalf("publication changed predecessor or output count: %v %+v", err, after)
					}
					published := t01br6Successor(t, after, ref).Record.Claims[0]
					switch scenario.name {
					case "duplicate-check-edit":
						if published.Checks[1].Covers != "Edited coverage" || published.Examples[0].Extra["x-meta"] == nil {
							t.Fatalf("check edit or scenario extension was lost: %+v", published)
						}
					case "duplicate-implementation-add":
						if len(published.ImplementedBy) != 3 || published.ImplementedBy[2].Covers != "Third implementation" {
							t.Fatalf("implementation addition was lost: %+v", published.ImplementedBy)
						}
					case "duplicate-check-replace", "duplicate-check-extra-disjoint", "duplicate-check-tag-disjoint":
						if len(published.Checks) != 1 || published.Checks[0].Ref != "test:other_test.go::TestOther" || published.Checks[0].Covers != "Replacement coverage" || len(published.Checks[0].Extra) != 0 {
							t.Fatalf("disjoint check replacement was altered: %+v", published.Checks)
						}
					case "duplicate-implementation-extra-disjoint":
						if len(published.ImplementedBy) != 1 || published.ImplementedBy[0].Ref != "sym:new.go::New" || published.ImplementedBy[0].Covers != "New implementation" || len(published.ImplementedBy[0].Extra) != 0 {
							t.Fatalf("disjoint implementation replacement was altered: %+v", published.ImplementedBy)
						}
					case "claim-leaf-edit":
						if published.Extra["x-meta"].(map[string]any)["a"] != "abc" || published.Extra["x-meta"].(map[string]any)["b"] != 3 {
							t.Fatalf("claim leaf or sibling was lost: %+v", published.Extra)
						}
					case "claim-explicit-leaf-replace":
						if published.Extra["x-meta"].(map[string]any)["a"] != "replaced" || published.Extra["x-meta"].(map[string]any)["b"] != 2 {
							t.Fatalf("claim leaf replacement was lost: %+v", published.Extra)
						}
					case "sequence-element-edit":
						if !reflect.DeepEqual(published.Extra["x-meta"], []any{"a", "c"}) {
							t.Fatalf("extension sequence edit was lost: %+v", published.Extra)
						}
					}
				})
			}
		}
	}
}
