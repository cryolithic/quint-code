package transport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

func t01br9AppliedOutputs(t *testing.T, view store.View, requestID, previewDigest string, expected ...string) {
	t.Helper()
	for path, raw := range view.Files {
		if !strings.HasPrefix(path, "changes/applied-") {
			continue
		}
		var log struct {
			Preview string   `json:"preview_digest"`
			Outputs []string `json:"outputs"`
			Receipt struct {
				RequestID string `json:"request_id"`
			} `json:"write_receipt"`
		}
		if err := json.Unmarshal(raw, &log); err != nil {
			t.Fatal(err)
		}
		if log.Receipt.RequestID != requestID {
			continue
		}
		if log.Preview != previewDigest || len(log.Outputs) != len(expected) {
			t.Fatalf("applied receipt did not bind exact preview and output count: %+v", log)
		}
		for index, ref := range expected {
			if log.Outputs[index] != ref {
				t.Fatalf("applied receipt changed output %d: %+v", index, log.Outputs)
			}
		}
		return
	}
	t.Fatalf("no applied receipt for request %q", requestID)
}

func TestT01BR9PublicTwoDirectParentsKeepReorderedOwners(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				alpha := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000901", "br9-alpha", true, false)
				beta := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000902", "br9-beta", true, false)
				alphaRef := alpha.Record.ID + "@" + alpha.Edition
				betaRef := beta.Record.ID + "@" + beta.Edition
				alphaClaim := alpha.Record.Claims[0]
				alphaClaim.Text = "Original alpha intent"
				betaClaim := beta.Record.Claims[0]
				betaClaim.Text = "Original beta intent"
				id := "chg-20260927-00000901"
				original := t01br7Change(id, []change.SectionPatch{
					{Base: alphaRef, Extra: carrier.Extra{"x-owner": "ALPHA_OWNER"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &alphaClaim, Reason: "Edit alpha"}}},
					{Base: betaRef, Extra: carrier.Extra{"x-owner": "BETA_OWNER"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaClaim, Reason: "Edit beta"}}},
				})
				raw, err := change.Encode(original, []byte("Rationale.\n"))
				if err != nil {
					t.Fatal(err)
				}
				view := t01br6View(t, root)
				created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "br9-two-parents", Carrier: string(raw), Snapshots: map[string][]byte{alpha.Edition: view.CurrentSnapshots[alpha.Edition], beta.Edition: view.CurrentSnapshots[beta.Edition]}})
				if created.Kind != "written" || created.IsError {
					t.Fatalf("two-patch source was not created: %+v", created)
				}
				if preview := t01br6Preview(t, client, profile.changeTool, id); preview.Kind != "ready" || preview.IsError {
					t.Fatalf("two-patch source did not preview: %+v", preview)
				}
				originalView := t01br6View(t, root)
				oldChange := bytes.Clone(originalView.Files["changes/"+id+".md"])
				oldAlpha := bytes.Clone(originalView.Files["specs/"+alpha.Record.ID+".md"])
				oldBeta := bytes.Clone(originalView.Files["specs/"+beta.Record.ID+".md"])
				alphaCurrent := t01br7Advance(t, client, root, profile.changeTool, alpha, "0911", false)
				betaCurrent := t01br7Advance(t, client, root, profile.changeTool, beta, "0912", false)
				alphaCurrentRef := t01br8PublicExact(t, client, profile.readTool, alphaCurrent.Record.ID)
				betaCurrentRef := t01br8PublicExact(t, client, profile.readTool, betaCurrent.Record.ID)
				if alphaCurrentRef != alphaCurrent.Record.ID+"@"+alphaCurrent.Edition || betaCurrentRef != betaCurrent.Record.ID+"@"+betaCurrent.Edition {
					t.Fatal("public exact current refs differ from the applied editions")
				}
				view = t01br6View(t, root)
				alphaBytes := bytes.Clone(view.Files["specs/"+alphaCurrent.Record.ID+".md"])
				betaBytes := bytes.Clone(view.Files["specs/"+betaCurrent.Record.ID+".md"])
				alphaClaim = alphaCurrent.Record.Claims[0]
				alphaClaim.Text = "Resolved alpha intent"
				betaClaim = betaCurrent.Record.Claims[0]
				betaClaim.Text = "Resolved beta intent"
				revisionID := "chg-20260927-00000913"
				revision := change.Revision{ID: revisionID, Reason: "Use the two exact current parents", Patches: []change.SectionPatch{
					{Base: betaCurrentRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaClaim, Reason: "Resolve beta"}}},
					{Base: alphaCurrentRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &alphaClaim, Reason: "Resolve alpha"}}},
				}}
				written := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "br9-reorder", Revision: &revision})
				if written.Kind != "written" || written.IsError {
					t.Fatalf("two proven parents refused rebase: %+v", written)
				}
				view = t01br6View(t, root)
				saved := change.Parse(view.Files["changes/"+revisionID+".md"])
				if carrier.HasErrors(saved.Diagnostics) || saved.Change.Patches[0].Base != betaCurrentRef || saved.Change.Patches[0].Extra["x-owner"] != "BETA_OWNER" || saved.Change.Patches[1].Base != alphaCurrentRef || saved.Change.Patches[1].Extra["x-owner"] != "ALPHA_OWNER" || !bytes.Equal(view.Files["changes/"+id+".md"], oldChange) {
					t.Fatalf("public revision crossed owners or changed old bytes: %+v", saved)
				}
				preview := t01br6Preview(t, client, profile.changeTool, revisionID)
				if preview.Kind != "ready" || preview.IsError {
					t.Fatalf("reordered direct-parent revision did not preview: %+v", preview)
				}
				t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "br9-publish-reordered", preview)
				view = t01br6View(t, root)
				alphaPublished := t01br6Successor(t, view, alphaCurrentRef)
				betaPublished := t01br6Successor(t, view, betaCurrentRef)
				alphaPublishedRef := t01br8PublicExact(t, client, profile.readTool, alphaPublished.Record.ID)
				betaPublishedRef := t01br8PublicExact(t, client, profile.readTool, betaPublished.Record.ID)
				if alphaPublishedRef != alphaPublished.Record.ID+"@"+alphaPublished.Edition || betaPublishedRef != betaPublished.Record.ID+"@"+betaPublished.Edition || alphaPublished.Record.Claims[0].Text != "Resolved alpha intent" || betaPublished.Record.Claims[0].Text != "Resolved beta intent" || alphaPublished.Record.WriteReceipt == nil || betaPublished.Record.WriteReceipt == nil || alphaPublished.Record.WriteReceipt.RequestID != "br9-publish-reordered" || betaPublished.Record.WriteReceipt.RequestID != "br9-publish-reordered" || !bytes.Equal(view.Files["specs/"+alphaCurrent.Record.ID+".md"], alphaBytes) || !bytes.Equal(view.Files["specs/"+betaCurrent.Record.ID+".md"], betaBytes) || !bytes.Equal(view.Files["specs/"+alpha.Record.ID+".md"], oldAlpha) || !bytes.Equal(view.Files["specs/"+beta.Record.ID+".md"], oldBeta) {
					t.Fatalf("published direct-parent effects or predecessor bytes changed: alpha=%+v beta=%+v", alphaPublished, betaPublished)
				}
				t01br9AppliedOutputs(t, view, "br9-publish-reordered", preview.Basis["preview_digest"], betaPublishedRef, alphaPublishedRef)
			})
		}
	}
}

func TestT01BR9PublicExactAdditionAndDuplicateBase(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				retained := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000931", "br9-retained", true, false)
				added := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000932", "br9-added", true, false)
				retainedRef := retained.Record.ID + "@" + retained.Edition
				addedRef := added.Record.ID + "@" + added.Edition
				oldClaim := retained.Record.Claims[0]
				oldClaim.Text = "Original retained intent"
				id := "chg-20260927-00000931"
				original := t01br7Change(id, []change.SectionPatch{{Base: retainedRef, Extra: carrier.Extra{"x-owner": "RETAINED_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &oldClaim, Reason: "Edit retained"}}}})
				t01br6Create(t, client, profile.changeTool, original, retained, t01br6View(t, root), "br9-original")
				if preview := t01br6Preview(t, client, profile.changeTool, id); preview.Kind != "ready" || preview.IsError {
					t.Fatalf("original single patch did not preview: %+v", preview)
				}
				before := t01br6View(t, root)
				oldBytes := bytes.Clone(before.Files["changes/"+id+".md"])
				oldRetained := bytes.Clone(before.Files["specs/"+retained.Record.ID+".md"])
				oldAdded := bytes.Clone(before.Files["specs/"+added.Record.ID+".md"])
				retainedClaim := retained.Record.Claims[0]
				retainedClaim.Text = "Resolved retained intent"
				duplicate := change.SectionPatch{Base: retainedRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &retainedClaim, Reason: "Edit retained"}}}
				revisionID := "chg-20260927-00000933"
				refused := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "br9-duplicate", Revision: &change.Revision{ID: revisionID, Reason: "Try duplicate selected bases", Patches: []change.SectionPatch{duplicate, duplicate}}})
				if refused.Kind != "invalid" || !refused.IsError {
					t.Fatalf("duplicate base lost its structural outcome: %+v", refused)
				}
				t01br4MCPDiagnostic(t, client, profile.readTool, refused, "duplicate_base", "patches[1]")
				after := t01br6View(t, root)
				if after.Generation != before.Generation || !bytes.Equal(after.Files["changes/"+id+".md"], oldBytes) || len(after.Files["changes/"+revisionID+".md"]) != 0 {
					t.Fatal("duplicate base refusal changed durable state")
				}
				addedClaim := added.Record.Claims[0]
				addedClaim.Text = "Explicit new section edit"
				selected := []change.SectionPatch{duplicate, {Base: addedRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &addedClaim, Reason: "Edit added"}}}}
				written := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "br9-add-section", Revision: &change.Revision{ID: revisionID, Reason: "Retain exact patch and add a section", Patches: selected}})
				if written.Kind != "written" || written.IsError {
					t.Fatalf("exact retained patch plus new section refused: %+v", written)
				}
				view := t01br6View(t, root)
				saved := change.Parse(view.Files["changes/"+revisionID+".md"])
				if carrier.HasErrors(saved.Diagnostics) || saved.Change.Patches[0].Extra["x-owner"] != "RETAINED_ONLY" || len(saved.Change.Patches[1].Extra) != 0 || !bytes.Equal(view.Files["changes/"+id+".md"], oldBytes) {
					t.Fatalf("added section borrowed a retained owner: %+v", saved)
				}
				preview := t01br6Preview(t, client, profile.changeTool, revisionID)
				if preview.Kind != "ready" || preview.IsError {
					t.Fatalf("exact retained plus added section did not preview: %+v", preview)
				}
				t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "br9-publish-addition", preview)
				view = t01br6View(t, root)
				retainedPublished := t01br6Successor(t, view, retainedRef)
				addedPublished := t01br6Successor(t, view, addedRef)
				retainedPublishedRef := t01br8PublicExact(t, client, profile.readTool, retainedPublished.Record.ID)
				addedPublishedRef := t01br8PublicExact(t, client, profile.readTool, addedPublished.Record.ID)
				if retainedPublishedRef != retainedPublished.Record.ID+"@"+retainedPublished.Edition || addedPublishedRef != addedPublished.Record.ID+"@"+addedPublished.Edition || retainedPublished.Record.Claims[0].Text != "Resolved retained intent" || addedPublished.Record.Claims[0].Text != "Explicit new section edit" || retainedPublished.Record.WriteReceipt == nil || addedPublished.Record.WriteReceipt == nil || retainedPublished.Record.WriteReceipt.RequestID != "br9-publish-addition" || addedPublished.Record.WriteReceipt.RequestID != "br9-publish-addition" || !bytes.Equal(view.Files["specs/"+retained.Record.ID+".md"], oldRetained) || !bytes.Equal(view.Files["specs/"+added.Record.ID+".md"], oldAdded) || !bytes.Equal(view.Files["changes/"+id+".md"], oldBytes) {
					t.Fatalf("added section publication lost meaning or receipt: retained=%+v added=%+v", retainedPublished, addedPublished)
				}
				t01br9AppliedOutputs(t, view, "br9-publish-addition", preview.Basis["preview_digest"], retainedPublishedRef, addedPublishedRef)
			})
		}
	}
}
