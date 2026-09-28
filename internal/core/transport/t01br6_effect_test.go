package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

func t01br6View(t *testing.T, root string) store.View {
	t.Helper()
	view, err := (store.Store{Root: root}).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func t01br6Document(t *testing.T, view store.View, id string) carrier.Document {
	t.Helper()
	for _, doc := range view.Documents {
		if doc.Record.ID == id {
			return doc
		}
	}
	t.Fatalf("document %s is absent", id)
	return carrier.Document{}
}

func t01br6Successor(t *testing.T, view store.View, ref string) carrier.Document {
	t.Helper()
	for _, doc := range view.Documents {
		for _, parent := range doc.Record.Supersedes {
			if parent == ref {
				return doc
			}
		}
	}
	t.Fatalf("successor of %s is absent", ref)
	return carrier.Document{}
}

func t01br6Create(t *testing.T, client *t01aProtocolClient, tool string, candidate change.Change, base carrier.Document, view store.View, requestID string) {
	t.Helper()
	raw, err := change.Encode(candidate, []byte("Rationale.\n"))
	if err != nil {
		t.Fatal(err)
	}
	created := client.mustCall(t, tool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: requestID, Carrier: string(raw), Snapshots: map[string][]byte{base.Edition: view.CurrentSnapshots[base.Edition]}})
	if created.Kind != "written" || created.IsError {
		t.Fatalf("change creation failed: kind=%s diagnostics=%+v", created.Kind, created.Diagnostics)
	}
}

func t01br6Preview(t *testing.T, client *t01aProtocolClient, tool, id string) delivery.Response {
	t.Helper()
	return client.mustCall(t, tool, app.Request{Format: delivery.Format, Operation: "change", Action: "preview", Ref: id})
}

func t01br6Apply(t *testing.T, client *t01aProtocolClient, tool, id, action, requestID string, preview delivery.Response) {
	t.Helper()
	request := app.Request{Format: delivery.Format, Operation: "change", Action: action, Ref: id, RequestID: requestID, ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	written := client.mustCall(t, tool, request)
	if written.Kind != "written" || written.IsError || delivery.Size(written) > delivery.Budget {
		t.Fatalf("%s failed: kind=%s diagnostics=%+v", action, written.Kind, written.Diagnostics)
	}
	replay := client.mustCall(t, tool, request)
	if replay.Kind != "replayed" || replay.IsError || delivery.Size(replay) > delivery.Budget {
		t.Fatalf("%s replay failed: kind=%s diagnostics=%+v", action, replay.Kind, replay.Diagnostics)
	}
}

func TestT01BR6PublicOmittedExamplesAndNamedRemoval(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				seed := client.mustCall(t, profile.writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br6-seed", Carrier: t01br5Spec(format)})
				if seed.Kind != "written" || seed.IsError {
					t.Fatalf("seed failed: %+v", seed)
				}
				view := t01br6View(t, root)
				base := t01br6Document(t, view, "spec-20260927-00000011")
				oldBytes := bytes.Clone(base.Raw)
				baseRef := base.Record.ID + "@" + base.Edition
				claim := base.Record.Claims[0]
				claim.Text = "Revised local rule"
				claim.Examples = nil
				id := "chg-20260927-00000061"
				candidate := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Keep local scenario", Intent: "Clarify the rule", State: "open", CreatedAt: "2026-09-27T00:00:00Z", Patches: []change.SectionPatch{{Base: baseRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify rule"}}}}}
				t01br6Create(t, client, profile.changeTool, candidate, base, view, "t01br6-create")
				if first := t01br6Preview(t, client, profile.changeTool, id); first.Kind != "ready" || first.IsError {
					t.Fatalf("initial omitted-example preview failed: %+v", first)
				}
				intent := "Clarified intent only"
				update := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "update", Ref: id, RequestID: "t01br6-update", Revision: &change.Revision{ID: "chg-20260927-00000062", Reason: "Clarify intent", Intent: &intent}})
				if update.Kind != "written" || update.IsError {
					t.Fatalf("intent-only update failed: %+v", update)
				}
				view = t01br6View(t, root)
				revisedRaw := view.Files["changes/chg-20260927-00000062.md"]
				revised := change.Parse(revisedRaw)
				if carrier.HasErrors(revised.Diagnostics) || revised.Change.Patches[0].Operations[0].Claim.Examples != nil || bytes.Contains(revisedRaw, []byte("examples: []")) {
					t.Fatalf("intent update authored an empty examples list: %+v", revised)
				}
				preview := t01br6Preview(t, client, profile.changeTool, revised.Change.ID)
				if preview.Kind != "ready" || preview.IsError {
					t.Fatalf("revised omitted-example preview failed: %+v", preview)
				}
				action := "apply"
				if format == "haft/2" {
					action = "sync"
				}
				t01br6Apply(t, client, profile.changeTool, revised.Change.ID, action, "t01br6-apply", preview)
				view = t01br6View(t, root)
				published := t01br6Successor(t, view, baseRef)
				if len(published.Record.Claims[0].Examples) != 1 || published.Record.Claims[0].Examples[0].ID != "sample" || published.Record.Claims[0].Text != "Revised local rule" || !bytes.Equal(view.Files["specs/"+base.Record.ID+".md"], oldBytes) {
					t.Fatalf("omitted scenario, rule edit or predecessor was lost: %+v", published.Record.Claims[0])
				}
				publishedRef := published.Record.ID + "@" + published.Edition
				claim = published.Record.Claims[0]
				claim.Examples = []carrier.Example{}
				emptyID := "chg-20260927-00000063"
				empty := change.Change{Format: change.Format, ID: emptyID, ChangeKey: emptyID, Title: "Remove one scenario", Intent: "Explicitly remove sample", State: "open", CreatedAt: "2026-09-27T01:00:00Z", Patches: []change.SectionPatch{{Base: publishedRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Remove sample"}}}}}
				t01br6Create(t, client, profile.changeTool, empty, published, view, "t01br6-empty-create")
				refused := t01br6Preview(t, client, profile.changeTool, emptyID)
				if refused.Kind != "conflict" || !refused.IsError {
					t.Fatalf("unnamed explicit empty was accepted: %+v", refused)
				}
				t01br4MCPDiagnostic(t, client, profile.readTool, refused, "scenario_loss", publishedRef+".operations[0]")
				empty.Patches[0].Operations[0].RemoveExamples = []string{"sample"}
				named := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "update", Ref: emptyID, RequestID: "t01br6-named", Revision: &change.Revision{ID: "chg-20260927-00000064", Reason: "Name the removed scenario", Patches: empty.Patches}})
				if named.Kind != "written" || named.IsError {
					t.Fatalf("named removal revision failed: %+v", named)
				}
				removed := t01br6Preview(t, client, profile.changeTool, "chg-20260927-00000064")
				if removed.Kind != "ready" || removed.IsError {
					t.Fatalf("named removal preview failed: %+v", removed)
				}
				t01br6Apply(t, client, profile.changeTool, "chg-20260927-00000064", "apply", "t01br6-remove", removed)
				view = t01br6View(t, root)
				withoutExample := t01br6Successor(t, view, publishedRef)
				if len(withoutExample.Record.Claims[0].Examples) != 0 || !bytes.Equal(view.Files["specs/"+published.Record.ID+".md"], published.Raw) {
					t.Fatalf("named removal or predecessor retention failed: %+v", withoutExample.Record.Claims[0])
				}
			})
		}
	}
}

func TestT01BR6PublicTaskReplacementLosses(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := t.TempDir()
			client := t01aStartClient(t, app.Service{Root: root}, profile.name)
			client.list(t)
			id := "chg-20260927-00000071"
			raw := fmt.Sprintf("---\nformat: haft.change/1\nid: %s\nchange_key: %s\ntitle: Task replacement\nintent: Review task results\nstate: open\ncreated_at: 2026-09-27T00:00:00Z\nno_spec_change_reason: Local task only\ntasks:\n  - id: inspect\n    text: Inspect source\n    done: false\n    x-owner: abc\n---\nRationale.\n", id, id)
			created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "t01br6-task-create", Carrier: raw})
			if created.Kind != "written" || created.IsError {
				t.Fatalf("task seed failed: %+v", created)
			}
			path := filepath.Join(root, ".haft", "changes", id+".md")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			tagged := bytes.Replace(original, []byte("x-owner: abc"), []byte("x-owner: !vendor abc"), 1)
			if err := os.WriteFile(path, tagged, 0600); err != nil {
				t.Fatal(err)
			}
			tasks := change.Parse(tagged).Change.Tasks
			tasks[0].Text = "Replace the task"
			tasks[0].Extra = nil
			result := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "update", Ref: id, RequestID: "t01br6-task-update", Revision: &change.Revision{ID: "chg-20260927-00000072", Reason: "Replace task and its metadata", Tasks: &tasks}})
			if result.Kind != "written" || result.IsError || delivery.Size(result) > delivery.Budget {
				t.Fatalf("task replacement failed: %+v", result)
			}
			parts := t01aParts(t, client, result)
			loss := parts["diagnostics"]
			page := t01aRead(t, client, profile.readTool, loss.Request)
			body, err := json.Marshal(page.Data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(body, []byte("task_extension_replaced")) || !bytes.Contains(body, []byte("x-owner")) || !bytes.Contains(body, []byte("tasks[0]")) {
				t.Fatalf("replacement loss omitted removed field: %s", body)
			}
			view := t01br6View(t, root)
			if !bytes.Equal(view.Files["changes/"+id+".md"], tagged) || bytes.Contains(view.Files["changes/chg-20260927-00000072.md"], []byte("x-owner")) {
				t.Fatalf("replacement changed predecessor or retained removed extension")
			}
		})
	}
}

func TestT01BR6GuidanceStatesRevisionBoundaries(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		client := t01aStartClient(t, app.Service{Root: t.TempDir()}, profile.name)
		for _, listed := range client.list(t) {
			if listed.Name != profile.changeTool {
				continue
			}
			for _, phrase := range []string{"omitted examples", "whole-task replacement", "remove_fields", "revision diagnostics"} {
				if !strings.Contains(strings.ToLower(listed.Description), phrase) {
					t.Fatalf("%s guidance lacks %q", profile.name, phrase)
				}
			}
		}
	}
}

func TestT01BR6PublicReorderedMultiPatchRebase(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				alphaRaw := t01br5Spec(format)
				betaRaw := strings.Replace(alphaRaw, "spec-20260927-00000011", "spec-20260927-00000012", 1)
				betaRaw = strings.Replace(betaRaw, "slug: local-rule", "slug: local-fee", 1)
				betaRaw = strings.Replace(betaRaw, "about: domain:Billing.Semantic", "about: domain:Billing.Fee", 1)
				betaRaw = strings.Replace(betaRaw, "---\nOriginal body.", "  - id: fee\n    kind: definition\n    text: Base fee\n---\nOriginal body.", 1)
				for index, raw := range []string{alphaRaw, betaRaw} {
					seed := client.mustCall(t, profile.writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: fmt.Sprintf("t01br6-old-%d", index), Carrier: raw})
					if seed.Kind != "written" || seed.IsError {
						t.Fatalf("old base %d failed: %+v", index, seed)
					}
				}
				view := t01br6View(t, root)
				alpha := t01br6Document(t, view, "spec-20260927-00000011")
				beta := t01br6Document(t, view, "spec-20260927-00000012")
				alphaRef := alpha.Record.ID + "@" + alpha.Edition
				betaRef := beta.Record.ID + "@" + beta.Edition
				alphaRule := alpha.Record.Claims[0]
				alphaRule.Text = "Edited alpha rule"
				betaRule := beta.Record.Claims[0]
				betaRule.Text = "Edited beta rule"
				betaFee := beta.Record.Claims[1]
				betaFee.Text = "Edited beta fee"
				id := "chg-20260927-00000081"
				old := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Two lineage edit", Intent: "Edit separate rules", State: "open", CreatedAt: "2026-09-27T00:00:00Z", Patches: []change.SectionPatch{
					{Base: alphaRef, Extra: carrier.Extra{"x-owner": "ALPHA_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &alphaRule, Reason: "Edit alpha"}}},
					{Base: betaRef, Extra: carrier.Extra{"x-owner": "BETA_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaRule, Reason: "Edit beta rule"}, {Op: "MODIFIED", ClaimID: "fee", Claim: &betaFee, Reason: "Edit beta fee"}}},
				}}
				oldChange, err := change.Encode(old, []byte("Rationale.\n"))
				if err != nil {
					t.Fatal(err)
				}
				created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "t01br6-two-create", Carrier: string(oldChange), Snapshots: map[string][]byte{alpha.Edition: view.CurrentSnapshots[alpha.Edition], beta.Edition: view.CurrentSnapshots[beta.Edition]}})
				if created.Kind != "written" || created.IsError {
					t.Fatalf("old change failed: %+v", created)
				}
				if preview := t01br6Preview(t, client, profile.changeTool, id); preview.Kind != "ready" || preview.IsError {
					t.Fatalf("old exact bases did not preview: %+v", preview)
				}
				oldBytes := bytes.Clone(t01br6View(t, root).Files["changes/"+id+".md"])
				newAlpha := alpha.Record
				newAlpha.ID = "spec-20260927-00000013"
				newAlpha.Supersedes = []string{alphaRef}
				newAlpha.SupersedeReason = "Exact alpha successor"
				newAlpha.RetiredClaimIDs = []string{"rule"}
				newAlpha.Claims = append([]carrier.Claim(nil), alpha.Record.Claims...)
				newAlpha.Claims[0].ID = "rule2"
				newBeta := beta.Record
				newBeta.ID = "spec-20260927-00000014"
				newBeta.Supersedes = []string{betaRef}
				newBeta.SupersedeReason = "Exact beta successor"
				for index, successor := range []carrier.Record{newAlpha, newBeta} {
					view = t01br6View(t, root)
					parent := alphaRef
					body := alpha.Body
					if index == 1 {
						parent = betaRef
						body = beta.Body
					}
					successor.WriteReceipt = nil
					raw, err := carrier.Encode(successor, body)
					if err != nil {
						t.Fatal(err)
					}
					written := client.mustCall(t, profile.writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: fmt.Sprintf("t01br6-new-%d", index), Carrier: string(raw), ExpectedGeneration: view.Generation, ExpectedHeads: []string{parent}})
					if written.Kind != "written" || written.IsError {
						t.Fatalf("exact successor %d failed: %+v", index, written)
					}
				}
				view = t01br6View(t, root)
				alphaNext := t01br6Document(t, view, newAlpha.ID)
				betaNext := t01br6Document(t, view, newBeta.ID)
				alphaNextRef := alphaNext.Record.ID + "@" + alphaNext.Edition
				betaNextRef := betaNext.Record.ID + "@" + betaNext.Edition
				alphaRule2 := alphaNext.Record.Claims[0]
				alphaRule2.Text = "Edited alpha rule two"
				patches := []change.SectionPatch{
					{Base: betaNextRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaRule, Reason: "Edit beta rule"}, {Op: "MODIFIED", ClaimID: "fee", Claim: &betaFee, Reason: "Edit beta fee"}}},
					{Base: alphaNextRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule2", Claim: &alphaRule2, Reason: "Edit alpha again"}}},
				}
				rebased := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "t01br6-rebase", Revision: &change.Revision{ID: "chg-20260927-00000082", Reason: "Rebase exact lineages", Patches: patches}})
				if rebased.Kind != "written" || rebased.IsError || delivery.Size(rebased) > delivery.Budget {
					t.Fatalf("multi-patch rebase failed: %+v", rebased)
				}
				view = t01br6View(t, root)
				saved := change.Parse(view.Files["changes/chg-20260927-00000082.md"])
				if carrier.HasErrors(saved.Diagnostics) || saved.Change.Patches[0].Base != betaNextRef || saved.Change.Patches[0].Extra["x-owner"] != "BETA_ONLY" || saved.Change.Patches[1].Base != alphaNextRef || saved.Change.Patches[1].Extra["x-owner"] != "ALPHA_ONLY" || !bytes.Equal(view.Files["changes/"+id+".md"], oldBytes) {
					t.Fatalf("saved rebase crossed owners or changed predecessor: %+v", saved.Change.Patches)
				}
				preview := t01br6Preview(t, client, profile.changeTool, saved.Change.ID)
				if preview.Kind != "ready" || preview.IsError {
					t.Fatalf("reordered rebase preview failed: %+v", preview)
				}
				t01br6Apply(t, client, profile.changeTool, saved.Change.ID, "apply", "t01br6-two-apply", preview)
				view = t01br6View(t, root)
				alphaPublished := t01br6Successor(t, view, alphaNextRef).Record.Claims[0]
				betaPublished := t01br6Successor(t, view, betaNextRef).Record.Claims
				if alphaPublished.ID != "rule2" || alphaPublished.Text != "Edited alpha rule two" || len(betaPublished) != 2 || betaPublished[0].Text != "Edited beta rule" || betaPublished[1].Text != "Edited beta fee" || !bytes.Equal(view.Files["specs/"+newAlpha.ID+".md"], alphaNext.Raw) || !bytes.Equal(view.Files["specs/"+newBeta.ID+".md"], betaNext.Raw) {
					t.Fatalf("multi-patch publication changed ownership, content or predecessor")
				}
			})
		}
	}
}

func TestT01BR6PublicActualDeclaredListEdits(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				raw := strings.Replace(t01br5Spec(format), "    x-meta: {a: abc, b: 2}\n---", "      - id: new-sample\n        text: Another case\n    x-meta: {a: abc, b: 2}\n    x-retired: Keep me\n---", 1)
				seed := client.mustCall(t, profile.writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br6-list-seed", Carrier: raw})
				if seed.Kind != "written" || seed.IsError {
					t.Fatalf("list base failed: %+v", seed)
				}
				view := t01br6View(t, root)
				base := t01br6Document(t, view, "spec-20260927-00000011")
				ref := base.Record.ID + "@" + base.Edition
				claim := base.Record.Claims[0]
				claim.Text = "Revised rule"
				id := "chg-20260927-00000091"
				old := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Edit declared lists", Intent: "Keep selected meaning", State: "open", CreatedAt: "2026-09-27T00:00:00Z", Tasks: []change.Task{{ID: "inspect", Text: "Inspect local result", Results: []string{"first", "second"}, Extra: carrier.Extra{"x-owner": "local"}}}, Patches: []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Remove old scenarios and field", RemoveExamples: []string{"sample", "new-sample"}, RemoveFields: []string{"x-retired"}}}}}}
				t01br6Create(t, client, profile.changeTool, old, base, view, "t01br6-list-create")
				if preview := t01br6Preview(t, client, profile.changeTool, id); preview.Kind != "ready" || preview.IsError {
					t.Fatalf("initial explicit removal did not preview: %+v", preview)
				}
				oldBytes := bytes.Clone(t01br6View(t, root).Files["changes/"+id+".md"])
				newTasks := append([]change.Task(nil), old.Tasks...)
				newTasks[0].Results = nil
				newPatches := append([]change.SectionPatch(nil), old.Patches...)
				newPatches[0].Operations = append([]change.Operation(nil), old.Patches[0].Operations...)
				newClaim := claim
				newClaim.Examples = []carrier.Example{}
				newPatches[0].Operations[0].Claim = &newClaim
				newPatches[0].Operations[0].RemoveExamples = []string{"new-sample", "sample"}
				newPatches[0].Operations[0].RemoveFields = nil
				updated := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "update", Ref: id, RequestID: "t01br6-list-update", Revision: &change.Revision{ID: "chg-20260927-00000092", Reason: "Clear result, remove examples and retain field", Tasks: &newTasks, Patches: newPatches}})
				if updated.Kind != "written" || updated.IsError {
					t.Fatalf("list revision failed: %+v", updated)
				}
				view = t01br6View(t, root)
				saved := change.Parse(view.Files["changes/chg-20260927-00000092.md"])
				if carrier.HasErrors(saved.Diagnostics) || len(saved.Change.Tasks[0].Results) != 0 || saved.Change.Tasks[0].Extra["x-owner"] != "local" || len(saved.Change.Patches[0].Operations[0].RemoveFields) != 0 || !bytes.Equal(view.Files["changes/"+id+".md"], oldBytes) {
					t.Fatalf("saved list revision changed wrong meaning: %+v", saved.Change)
				}
				if got := saved.Change.Patches[0].Operations[0].RemoveExamples; len(got) != 2 || got[0] != "new-sample" || got[1] != "sample" || saved.Change.Patches[0].Operations[0].Claim.Examples == nil {
					t.Fatalf("named reordered removal or explicit empty was lost: %+v", got)
				}
				preview := t01br6Preview(t, client, profile.changeTool, saved.Change.ID)
				if preview.Kind != "ready" || preview.IsError {
					t.Fatalf("revised list preview failed: %+v", preview)
				}
				t01br6Apply(t, client, profile.changeTool, saved.Change.ID, "apply", "t01br6-list-apply", preview)
				view = t01br6View(t, root)
				out := t01br6Successor(t, view, ref).Record.Claims[0]
				if len(out.Examples) != 0 || out.Extra["x-retired"] != "Keep me" || out.Extra["x-meta"] == nil || out.Text != "Revised rule" || !bytes.Equal(view.Files["specs/"+base.Record.ID+".md"], base.Raw) {
					t.Fatalf("published list effect lost retained content: %+v", out)
				}
			})
		}
	}
}
