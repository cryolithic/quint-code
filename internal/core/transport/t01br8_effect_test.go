package transport

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func t01br8Seed(t *testing.T, client *t01aProtocolClient, root, tool, format, id, key string, active, example bool) carrier.Document {
	t.Helper()
	legacy := ""
	if format == "haft/1" {
		legacy = "status: proposed\norigin: agent_proposal\n"
		if active {
			legacy = "status: active\norigin: operator_request\noperator_confirmed: true\n"
		}
	}
	scenario := ""
	if example {
		scenario = "    examples:\n      - id: e\n        text: Original base scenario\n"
	}
	raw := fmt.Sprintf("---\nformat: %s\nid: %s\nkind: spec\ntitle: Revision boundary\n%sabout: domain:T01BR8.%s\nslug: %s\nreceiving_use: Preserve exact scenario intent\nclaims:\n  - id: rule\n    kind: definition\n    text: Original rule\n%s---\nFixture body.\n", format, id, legacy, key, key, scenario)
	written := client.mustCall(t, tool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "seed-" + key, Carrier: raw})
	if written.Kind != "written" || written.IsError {
		t.Fatalf("seed failed: %+v", written)
	}
	return t01br6Document(t, t01br6View(t, root), id)
}

// A proposed edition may lose unpinned alias lookup to an accepted head.
// Search metadata identifies its exact ref; recall then proves that selection.
func t01br8PublicExact(t *testing.T, client *t01aProtocolClient, readTool, id string) string {
	t.Helper()
	search := client.mustCall(t, readTool, app.Request{Format: delivery.Format, Operation: "recall", Query: id})
	if search.Kind != "results" || search.IsError {
		t.Fatalf("public search cannot locate %s: %+v", id, search)
	}
	data, ok := search.Data.(map[string]any)
	if !ok {
		t.Fatalf("search metadata is absent: %+v", search)
	}
	hits, ok := data["first_hits"].([]any)
	if !ok {
		t.Fatalf("search hits are absent: %+v", search)
	}
	for _, value := range hits {
		hit, ok := value.(map[string]any)
		if !ok || hit["record_id"] != id {
			continue
		}
		ref, ok := hit["ref"].(string)
		if !ok || !strings.HasPrefix(ref, id+"@") {
			t.Fatalf("search returned no exact edition for %s: %+v", id, hit)
		}
		selected := client.mustCall(t, readTool, app.Request{Format: delivery.Format, Operation: "recall", Ref: ref})
		if selected.Kind != "found" || selected.IsError || selected.Data.(map[string]any)["exact_ref"] != ref {
			t.Fatalf("exact public recall did not select %s: %+v", ref, selected)
		}
		return ref
	}
	t.Fatalf("search did not return exact edition %s: %+v", id, search)
	return ""
}

func t01br8Accept(t *testing.T, client *t01aProtocolClient, root, writeTool, readTool, format string, accepted, proposal carrier.Document, id string) carrier.Document {
	t.Helper()
	acceptedRef := accepted.Record.ID + "@" + accepted.Edition
	proposalRef := t01br8PublicExact(t, client, readTool, proposal.Record.ID)
	if proposalRef != proposal.Record.ID+"@"+proposal.Edition {
		t.Fatalf("public proposal ref differs from captured bytes: %s", proposalRef)
	}
	merged := proposal.Record
	merged.ID = id
	merged.Supersedes = []string{acceptedRef, proposalRef}
	merged.SupersedeReason = "Accept the exact proposed content"
	merged.WriteReceipt = nil
	expected := []string{proposalRef}
	if format == "haft/1" {
		merged.Status = carrier.Active
		merged.Origin = "operator_request"
		merged.OperatorConfirmed = true
		expected = []string{acceptedRef}
	}
	raw, err := carrier.Encode(merged, proposal.Body)
	if err != nil {
		t.Fatal(err)
	}
	written := client.mustCall(t, writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "accept-" + id, Carrier: string(raw), ExpectedHeads: expected})
	if written.Kind != "written" || written.IsError {
		t.Fatalf("exact two-parent acceptance failed: %+v", written)
	}
	selected := t01br8PublicExact(t, client, readTool, id)
	result := t01br6Document(t, t01br6View(t, root), id)
	if selected != result.Record.ID+"@"+result.Edition {
		t.Fatalf("accepted exact ref differs from saved carrier: %s", selected)
	}
	return result
}

func TestT01BR8PublicAcceptedMergeAndDescendantRebase(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, scenario := range []string{"direct", "descendant", "renamed-descendant"} {
				t.Run(profile.name+"/"+format+"/"+scenario, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)
					accepted := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000801", "merge", true, false)
					proposal := t01br7Advance(t, client, root, profile.changeTool, accepted, "0821", false)
					proposalRef := t01br8PublicExact(t, client, profile.readTool, proposal.Record.ID)
					claim := proposal.Record.Claims[0]
					claim.Text = "Outstanding edit on proposal"
					id := "chg-20260927-00000801"
					owner := scenario != "direct"
					patch := change.SectionPatch{Base: proposalRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &claim, Reason: "Original edit"}}}
					if owner {
						patch.Extra = carrier.Extra{"x-owner": "PROPOSAL_PATCH_OWNER"}
					}
					original := t01br7Change(id, []change.SectionPatch{patch})
					t01br6Create(t, client, profile.changeTool, original, proposal, t01br6View(t, root), "outstanding")
					if first := t01br6Preview(t, client, profile.changeTool, id); first.Kind != "ready" || first.IsError {
						t.Fatalf("outstanding exact proposal did not preview: %+v", first)
					}
					before := t01br6View(t, root)
					oldChange := bytes.Clone(before.Files["changes/"+id+".md"])
					acceptedBytes := bytes.Clone(before.Files["specs/"+accepted.Record.ID+".md"])
					proposalBytes := bytes.Clone(before.Files["specs/"+proposal.Record.ID+".md"])
					merged := t01br8Accept(t, client, root, profile.writeTool, profile.readTool, format, accepted, proposal, "spec-20260927-00000803")
					target := merged
					if scenario != "direct" {
						target = t01br7Advance(t, client, root, profile.changeTool, merged, "0822", scenario == "renamed-descendant")
						if exact := t01br8PublicExact(t, client, profile.readTool, target.Record.ID); exact != target.Record.ID+"@"+target.Edition {
							t.Fatalf("descendant public exact ref differs from selected target: %s", exact)
						}
					}
					targetRef := target.Record.ID + "@" + target.Edition
					next := target.Record.Claims[0]
					next.Text = "Resolved after accepted ancestry"
					revisionID := "chg-20260927-00000802"
					updated := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "resolve-merge", Revision: &change.Revision{ID: revisionID, Reason: "Use the proven current edition", Patches: []change.SectionPatch{{Base: targetRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: next.ID, Claim: &next, Reason: "Resolve current claim"}}}}}})
					if updated.Kind != "written" || updated.IsError {
						t.Fatalf("exact merged ancestry refused: %+v", updated)
					}
					view := t01br6View(t, root)
					saved := change.Parse(view.Files["changes/"+revisionID+".md"])
					if carrier.HasErrors(saved.Diagnostics) || saved.Change.Patches[0].Base != targetRef || !bytes.Equal(view.Files["changes/"+id+".md"], oldChange) {
						t.Fatalf("merged rebase changed old history or selected base: %+v", saved)
					}
					if owner && saved.Change.Patches[0].Extra["x-owner"] != "PROPOSAL_PATCH_OWNER" || !owner && len(saved.Change.Patches[0].Extra) != 0 {
						t.Fatalf("merged rebase has wrong patch owner: %+v", saved.Change.Patches[0].Extra)
					}
					preview := t01br6Preview(t, client, profile.changeTool, revisionID)
					if preview.Kind != "ready" || preview.IsError {
						t.Fatalf("merged rebase cannot preview: %+v", preview)
					}
					t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "publish-merge", preview)
					view = t01br6View(t, root)
					published := t01br6Successor(t, view, targetRef)
					if published.Record.Claims[0].ID != next.ID || published.Record.Claims[0].Text != "Resolved after accepted ancestry" || published.Record.WriteReceipt == nil || published.Record.WriteReceipt.RequestID != "publish-merge" {
						t.Fatalf("merged rebase did not publish the selected meaning and receipt: %+v", published.Record)
					}
					if exact := t01br8PublicExact(t, client, profile.readTool, published.Record.ID); exact != published.Record.ID+"@"+published.Edition {
						t.Fatalf("published public exact ref differs from saved successor: %s", exact)
					}
					if !bytes.Equal(view.Files["specs/"+accepted.Record.ID+".md"], acceptedBytes) || !bytes.Equal(view.Files["specs/"+proposal.Record.ID+".md"], proposalBytes) {
						t.Fatal("merged rebase changed predecessor native bytes")
					}
				})
			}
		}
	}
}

func TestT01BR8PublicReorderedAcceptedLineagesKeepOwners(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				alpha := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000851", "alpha-merge", true, false)
				beta := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000852", "beta-merge", true, false)
				alphaProposal := t01br7Advance(t, client, root, profile.changeTool, alpha, "0853", false)
				betaProposal := t01br7Advance(t, client, root, profile.changeTool, beta, "0854", false)
				alphaRef := t01br8PublicExact(t, client, profile.readTool, alphaProposal.Record.ID)
				betaRef := t01br8PublicExact(t, client, profile.readTool, betaProposal.Record.ID)
				alphaClaim := alphaProposal.Record.Claims[0]
				alphaClaim.Text = "Original alpha edit"
				betaClaim := betaProposal.Record.Claims[0]
				betaClaim.Text = "Original beta edit"
				id := "chg-20260927-00000851"
				original := t01br7Change(id, []change.SectionPatch{
					{Base: alphaRef, Extra: carrier.Extra{"x-owner": "ALPHA_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &alphaClaim, Reason: "Edit alpha"}}},
					{Base: betaRef, Extra: carrier.Extra{"x-owner": "BETA_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaClaim, Reason: "Edit beta"}}},
				})
				raw, err := change.Encode(original, []byte("Rationale.\n"))
				if err != nil {
					t.Fatal(err)
				}
				view := t01br6View(t, root)
				created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "two-merge-lineages", Carrier: string(raw), Snapshots: map[string][]byte{alphaProposal.Edition: view.CurrentSnapshots[alphaProposal.Edition], betaProposal.Edition: view.CurrentSnapshots[betaProposal.Edition]}})
				if created.Kind != "written" || created.IsError {
					t.Fatalf("two-patch proposal creation failed: %+v", created)
				}
				if initial := t01br6Preview(t, client, profile.changeTool, id); initial.Kind != "ready" || initial.IsError {
					t.Fatalf("original two-patch proposal cannot preview: %+v", initial)
				}
				oldBytes := bytes.Clone(t01br6View(t, root).Files["changes/"+id+".md"])
				alphaMerged := t01br8Accept(t, client, root, profile.writeTool, profile.readTool, format, alpha, alphaProposal, "spec-20260927-00000855")
				betaMerged := t01br8Accept(t, client, root, profile.writeTool, profile.readTool, format, beta, betaProposal, "spec-20260927-00000856")
				acceptedView := t01br6View(t, root)
				alphaBytes := bytes.Clone(acceptedView.Files["specs/"+alphaMerged.Record.ID+".md"])
				betaBytes := bytes.Clone(acceptedView.Files["specs/"+betaMerged.Record.ID+".md"])
				alphaMergedRef := alphaMerged.Record.ID + "@" + alphaMerged.Edition
				betaMergedRef := betaMerged.Record.ID + "@" + betaMerged.Edition
				alphaNext := alphaMerged.Record.Claims[0]
				alphaNext.Text = "Resolved alpha after acceptance"
				betaNext := betaMerged.Record.Claims[0]
				betaNext.Text = "Resolved beta after acceptance"
				ref := "chg-20260927-00000852"
				rebased := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "reorder-merged-lineages", Revision: &change.Revision{ID: ref, Reason: "Use both exact accepted bases", Patches: []change.SectionPatch{
					{Base: betaMergedRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaNext, Reason: "Resolve beta"}}},
					{Base: alphaMergedRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &alphaNext, Reason: "Resolve alpha"}}},
				}}})
				if rebased.Kind != "written" || rebased.IsError {
					t.Fatalf("reordered accepted lineages refused: %+v", rebased)
				}
				view = t01br6View(t, root)
				saved := change.Parse(view.Files["changes/"+ref+".md"])
				if carrier.HasErrors(saved.Diagnostics) || saved.Change.Patches[0].Extra["x-owner"] != "BETA_ONLY" || saved.Change.Patches[1].Extra["x-owner"] != "ALPHA_ONLY" || !bytes.Equal(view.Files["changes/"+id+".md"], oldBytes) {
					t.Fatalf("accepted lineage owners crossed in saved proposal: %+v", saved)
				}
				preview := t01br6Preview(t, client, profile.changeTool, ref)
				if preview.Kind != "ready" || preview.IsError {
					t.Fatalf("reordered accepted patches cannot preview: %+v", preview)
				}
				t01br6Apply(t, client, profile.changeTool, ref, "apply", "publish-reordered-merge", preview)
				view = t01br6View(t, root)
				alphaPublished := t01br6Successor(t, view, alphaMergedRef)
				betaPublished := t01br6Successor(t, view, betaMergedRef)
				if alphaPublished.Record.Claims[0].Text != "Resolved alpha after acceptance" || betaPublished.Record.Claims[0].Text != "Resolved beta after acceptance" || alphaPublished.Record.WriteReceipt == nil || betaPublished.Record.WriteReceipt == nil || alphaPublished.Record.WriteReceipt.RequestID != "publish-reordered-merge" || betaPublished.Record.WriteReceipt.RequestID != "publish-reordered-merge" || !bytes.Equal(view.Files["specs/"+alphaMerged.Record.ID+".md"], alphaBytes) || !bytes.Equal(view.Files["specs/"+betaMerged.Record.ID+".md"], betaBytes) {
					t.Fatalf("published merged lineages crossed: alpha=%+v beta=%+v", alphaPublished, betaPublished)
				}
			})
		}
	}
}

func TestT01BR8PublicManyOldPatchesCannotClaimOneMerge(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				accepted := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000871", "ambiguous-merge", true, false)
				proposal := t01br7Advance(t, client, root, profile.changeTool, accepted, "0872", false)
				acceptedRef := accepted.Record.ID + "@" + accepted.Edition
				proposalRef := t01br8PublicExact(t, client, profile.readTool, proposal.Record.ID)
				firstClaim := accepted.Record.Claims[0]
				firstClaim.Text = "First authored patch"
				secondClaim := proposal.Record.Claims[0]
				secondClaim.Text = "Second authored patch"
				id := "chg-20260927-00000871"
				original := t01br7Change(id, []change.SectionPatch{
					{Base: acceptedRef, Extra: carrier.Extra{"x-owner": "ACCEPTED_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &firstClaim, Reason: "Edit accepted"}}},
					{Base: proposalRef, Extra: carrier.Extra{"x-owner": "PROPOSAL_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &secondClaim, Reason: "Edit proposal"}}},
				})
				raw, err := change.Encode(original, []byte("Rationale.\n"))
				if err != nil {
					t.Fatal(err)
				}
				view := t01br6View(t, root)
				created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "two-old-patches", Carrier: string(raw), Snapshots: map[string][]byte{accepted.Edition: view.CurrentSnapshots[accepted.Edition], proposal.Edition: view.CurrentSnapshots[proposal.Edition]}})
				if created.Kind != "written" || created.IsError {
					t.Fatalf("two old exact patches cannot be authored: %+v", created)
				}
				oldBytes := bytes.Clone(t01br6View(t, root).Files["changes/"+id+".md"])
				merged := t01br8Accept(t, client, root, profile.writeTool, profile.readTool, format, accepted, proposal, "spec-20260927-00000873")
				mergedRef := merged.Record.ID + "@" + merged.Edition
				next := merged.Record.Claims[0]
				next.Text = "One merged edit"
				revisionID := "chg-20260927-00000874"
				before := t01br6View(t, root)
				refused := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "many-to-one-owner", Revision: &change.Revision{ID: revisionID, Reason: "Try one merged patch", Patches: []change.SectionPatch{{Base: mergedRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &next, Reason: "Resolve merge"}}}}}})
				if refused.Kind != "conflict" || !refused.IsError {
					t.Fatalf("one merged patch chose one of two owners: %+v", refused)
				}
				t01br4MCPDiagnostic(t, client, profile.readTool, refused, "ambiguous_revision_correspondence", "patches[0]")
				after := t01br6View(t, root)
				if after.Generation != before.Generation || !bytes.Equal(after.Files["changes/"+id+".md"], oldBytes) || len(after.Files["changes/"+revisionID+".md"]) != 0 {
					t.Fatal("many-to-one refusal changed durable history")
				}
			})
		}
	}
}

func TestT01BR8PublicOmittedProposalExamples(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, kind := range []string{"MODIFIED", "ADDED"} {
				t.Run(profile.name+"/"+format+"/"+kind, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)
					base := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000811", "omission", false, false)
					ref := base.Record.ID + "@" + base.Edition
					claim := base.Record.Claims[0]
					if kind == "ADDED" {
						claim = carrier.Claim{ID: "new-rule", Kind: "definition", Text: "Proposed new claim"}
					}
					claim.Examples = []carrier.Example{{ID: "keep", Text: "Keep scenario", Extra: carrier.Extra{"x-owner": "RETAINED_EXAMPLE_OWNER"}}, {ID: "drop", Text: "Other scenario"}}
					operation := change.Operation{Op: kind, Claim: &claim, Reason: "Propose two scenarios"}
					if kind == "MODIFIED" {
						operation.ClaimID = "rule"
					}
					id := "chg-20260927-00000811"
					original := t01br7Change(id, []change.SectionPatch{{Base: ref, Operations: []change.Operation{operation}}})
					t01br6Create(t, client, profile.changeTool, original, base, t01br6View(t, root), "propose-examples")
					if first := t01br6Preview(t, client, profile.changeTool, id); first.Kind != "ready" || first.IsError {
						t.Fatalf("original examples do not preview: %+v", first)
					}
					before := t01br6View(t, root)
					oldBytes := bytes.Clone(before.Files["changes/"+id+".md"])
					omitted := claim
					omitted.Text = "Clarified text only"
					omitted.Examples = nil
					operation.Claim = &omitted
					revisionID := "chg-20260927-00000812"
					patches := []change.SectionPatch{{Base: ref, Operations: []change.Operation{operation}}}
					refused := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "update", Ref: id, RequestID: "omit-scenarios", Revision: &change.Revision{ID: revisionID, Reason: "Only text was supplied", Patches: patches}})
					if refused.Kind != "conflict" || !refused.IsError {
						t.Fatalf("omitted examples withdrew proposal meaning: %+v", refused)
					}
					t01br4MCPDiagnostic(t, client, profile.readTool, refused, "unsupported_yaml_content_conversion", "patches[0].operations[0].claim.examples")
					after := t01br6View(t, root)
					if after.Generation != before.Generation || !bytes.Equal(after.Files["changes/"+id+".md"], oldBytes) || len(after.Files["changes/"+revisionID+".md"]) != 0 {
						t.Fatal("omission refusal changed durable history")
					}
					// An explicit partial list remains an ordinary edit after refusal.
					explicit := omitted
					explicit.Examples = claim.Examples[:1]
					patches[0].Operations[0].Claim = &explicit
					written := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "update", Ref: id, RequestID: "explicit-scenarios", Revision: &change.Revision{ID: revisionID, Reason: "Explicitly retain one scenario", Patches: patches}})
					if written.Kind != "written" || written.IsError {
						t.Fatalf("explicit partial scenario edit refused: %+v", written)
					}
					view := t01br6View(t, root)
					saved := change.Parse(view.Files["changes/"+revisionID+".md"])
					if carrier.HasErrors(saved.Diagnostics) || len(saved.Change.Patches[0].Operations[0].Claim.Examples) != 1 || saved.Change.Patches[0].Operations[0].Claim.Examples[0].Extra["x-owner"] != "RETAINED_EXAMPLE_OWNER" {
						t.Fatalf("saved explicit survivor lost its extension: %+v", saved)
					}
					preview := t01br6Preview(t, client, profile.changeTool, revisionID)
					if preview.Kind != "ready" || preview.IsError {
						t.Fatalf("explicit partial scenario edit cannot preview: %+v", preview)
					}
					t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "publish-partial", preview)
					view = t01br6View(t, root)
					published := t01br6Successor(t, view, ref)
					publishedClaim := t01br7Claim(t, published.Record.Claims, explicit.ID)
					if len(publishedClaim.Examples) != 1 || publishedClaim.Examples[0].ID != "keep" || publishedClaim.Examples[0].Extra["x-owner"] != "RETAINED_EXAMPLE_OWNER" || published.Record.WriteReceipt == nil {
						t.Fatalf("published survivor lost meaning or receipt: %+v", published)
					}
				})
			}
		}
	}
}

func TestT01BR8PublicCurrentBaseExampleRebase(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, scenario := range []string{"removed-current", "added-current"} {
				t.Run(profile.name+"/"+format+"/"+scenario, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)
					base := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000831", "current-scenarios", false, true)
					baseRef := base.Record.ID + "@" + base.Edition
					oldClaim := base.Record.Claims[0]
					oldClaim.Text = "Outstanding original edit"
					originalID := "chg-20260927-00000831"
					original := t01br7Change(originalID, []change.SectionPatch{{Base: baseRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &oldClaim, Reason: "Edit original"}}}})
					t01br6Create(t, client, profile.changeTool, original, base, t01br6View(t, root), "outstanding-scenarios")
					if first := t01br6Preview(t, client, profile.changeTool, originalID); first.Kind != "ready" || first.IsError {
						t.Fatalf("original change does not preview: %+v", first)
					}
					currentClaim := base.Record.Claims[0]
					currentClaim.Text = "Independent current edit"
					currentOp := change.Operation{Op: "MODIFIED", ClaimID: "rule", Claim: &currentClaim, Reason: "Advance current scenarios"}
					if scenario == "removed-current" {
						currentClaim.Examples = []carrier.Example{}
						currentOp.RemoveExamples = []string{"e"}
					}
					if scenario == "added-current" {
						currentClaim.Examples = append(append([]carrier.Example{}, currentClaim.Examples...), carrier.Example{ID: "current-only", Text: "New current scenario"})
					}
					currentID := "chg-20260927-00000832"
					independent := t01br7Change(currentID, []change.SectionPatch{{Base: baseRef, Operations: []change.Operation{currentOp}}})
					t01br6Create(t, client, profile.changeTool, independent, base, t01br6View(t, root), "independent-scenarios")
					preview := t01br6Preview(t, client, profile.changeTool, currentID)
					if preview.Kind != "ready" || preview.IsError {
						t.Fatalf("independent current edition does not preview: %+v", preview)
					}
					t01br6Apply(t, client, profile.changeTool, currentID, "apply", "advance-scenarios", preview)
					view := t01br6View(t, root)
					current := t01br6Successor(t, view, baseRef)
					currentRef := t01br8PublicExact(t, client, profile.readTool, current.Record.ID)
					if currentRef != current.Record.ID+"@"+current.Edition {
						t.Fatalf("current exact ref does not match the published carrier: %s", currentRef)
					}
					oldBaseBytes := bytes.Clone(view.Files["specs/"+base.Record.ID+".md"])
					oldCurrentBytes := bytes.Clone(view.Files["specs/"+current.Record.ID+".md"])
					oldChangeBytes := bytes.Clone(view.Files["changes/"+originalID+".md"])
					selected := current.Record.Claims[0]
					selected.Text = "Resolved against selected current base"
					revisionID := "chg-20260927-00000833"
					revision := change.Revision{ID: revisionID, Reason: "Explicitly reconcile current scenarios", Patches: []change.SectionPatch{{Base: currentRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &selected, Reason: "Resolve current base"}}}}}
					if scenario == "removed-current" {
						selected.Examples = []carrier.Example{}
						revision.Patches[0].Operations[0].Claim = &selected
						withOldRemoval := revision
						withOldRemoval.Patches = []change.SectionPatch{{Base: currentRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &selected, Reason: "Resolve current base", RemoveExamples: []string{"e"}}}}}
						unknown := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: originalID, RequestID: "old-removal", Revision: &withOldRemoval})
						if unknown.Kind != "conflict" || !unknown.IsError {
							t.Fatalf("old example was removed again from current base: %+v", unknown)
						}
						t01br4MCPDiagnostic(t, client, profile.readTool, unknown, "unknown_example_removal", currentRef+".operations[0]")
					}
					if scenario == "added-current" {
						selected.Examples = append([]carrier.Example{}, base.Record.Claims[0].Examples...)
						revision.Patches[0].Operations[0].Claim = &selected
						unnamed := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: originalID, RequestID: "unnamed-current-loss", Revision: &revision})
						if unnamed.Kind != "conflict" || !unnamed.IsError {
							t.Fatalf("new current-base scenario was removed without intent: %+v", unnamed)
						}
						t01br4MCPDiagnostic(t, client, profile.readTool, unnamed, "scenario_loss", currentRef+".operations[0]")
						revision.Patches[0].Operations[0].RemoveExamples = []string{"current-only"}
					}
					before := t01br6View(t, root)
					if before.Generation != view.Generation || !bytes.Equal(before.Files["changes/"+originalID+".md"], oldChangeBytes) || len(before.Files["changes/"+revisionID+".md"]) != 0 {
						t.Fatal("current-base refusal changed durable history")
					}
					written := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: originalID, RequestID: "resolved-current-scenarios", Revision: &revision})
					if written.Kind != "written" || written.IsError {
						t.Fatalf("explicit current-base example rebase refused: %+v", written)
					}
					view = t01br6View(t, root)
					saved := change.Parse(view.Files["changes/"+revisionID+".md"])
					if carrier.HasErrors(saved.Diagnostics) || saved.Change.Patches[0].Base != currentRef || saved.Change.Patches[0].Operations[0].Claim.Examples == nil || !bytes.Equal(view.Files["changes/"+originalID+".md"], oldChangeBytes) {
						t.Fatalf("selected current example list or old history was lost: %+v", saved)
					}
					preview = t01br6Preview(t, client, profile.changeTool, revisionID)
					if preview.Kind != "ready" || preview.IsError {
						t.Fatalf("current-base resolution cannot preview: %+v", preview)
					}
					t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "publish-current-resolution", preview)
					view = t01br6View(t, root)
					published := t01br6Successor(t, view, currentRef)
					want := 0
					if scenario == "added-current" {
						want = 1
					}
					if len(published.Record.Claims[0].Examples) != want || published.Record.Claims[0].Text != "Resolved against selected current base" || published.Record.WriteReceipt == nil || published.Record.WriteReceipt.RequestID != "publish-current-resolution" || !bytes.Equal(view.Files["specs/"+base.Record.ID+".md"], oldBaseBytes) || !bytes.Equal(view.Files["specs/"+current.Record.ID+".md"], oldCurrentBytes) {
						t.Fatalf("current-base rebase published wrong scenarios or changed predecessor bytes: %+v", published)
					}
				})
			}
		}
	}
}

func TestT01BR8PublicUnrelatedReplacementDoesNotCopyOwner(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				old := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000841", "old-section", false, false)
				other := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000842", "other-section", false, false)
				oldRef := old.Record.ID + "@" + old.Edition
				otherRef := other.Record.ID + "@" + other.Edition
				oldClaim := old.Record.Claims[0]
				oldClaim.Text = "Old intended edit"
				id := "chg-20260927-00000841"
				original := t01br7Change(id, []change.SectionPatch{{Base: oldRef, Extra: carrier.Extra{"x-owner": "OLD_PATCH_OWNER"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &oldClaim, Reason: "Edit old"}}}})
				t01br6Create(t, client, profile.changeTool, original, old, t01br6View(t, root), "old-owner")
				before := t01br6View(t, root)
				oldBytes := bytes.Clone(before.Files["changes/"+id+".md"])
				oldSpec := bytes.Clone(before.Files["specs/"+old.Record.ID+".md"])
				otherClaim := other.Record.Claims[0]
				otherClaim.Text = "Unrelated edited rule"
				revisionID := "chg-20260927-00000843"
				shared := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "unproven-context", Revision: &change.Revision{ID: revisionID, Reason: "Try unrelated base", Patches: []change.SectionPatch{{Base: otherRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &otherClaim, Reason: "Edit other"}}}}}})
				if shared.Kind != "conflict" || !shared.IsError {
					t.Fatalf("shared operation context guessed unrelated owner: %+v", shared)
				}
				t01br4MCPDiagnostic(t, client, profile.readTool, shared, "ambiguous_revision_correspondence", "patches[0]")
				after := t01br6View(t, root)
				if after.Generation != before.Generation || !bytes.Equal(after.Files["changes/"+id+".md"], oldBytes) || len(after.Files["changes/"+revisionID+".md"]) != 0 {
					t.Fatal("unproven correspondence refusal changed durable history")
				}
				added := carrier.Claim{ID: "other-rule", Kind: "definition", Text: "Explicit unrelated addition"}
				replacement := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "unrelated-replacement", Revision: &change.Revision{ID: revisionID, Reason: "Replace the old patch", Patches: []change.SectionPatch{{Base: otherRef, Operations: []change.Operation{{Op: "ADDED", Claim: &added, Reason: "Add independent rule"}}}}}})
				if replacement.Kind != "written" || replacement.IsError {
					t.Fatalf("unrelated explicit replacement refused: %+v", replacement)
				}
				view := t01br6View(t, root)
				saved := change.Parse(view.Files["changes/"+revisionID+".md"])
				if carrier.HasErrors(saved.Diagnostics) || len(saved.Change.Patches[0].Extra) != 0 || !bytes.Equal(view.Files["changes/"+id+".md"], oldBytes) {
					t.Fatalf("unrelated replacement copied old owner or changed history: %+v", saved)
				}
				preview := t01br6Preview(t, client, profile.changeTool, revisionID)
				if preview.Kind != "ready" || preview.IsError {
					t.Fatalf("unrelated replacement cannot preview: %+v", preview)
				}
				t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "publish-unrelated", preview)
				view = t01br6View(t, root)
				published := t01br6Successor(t, view, otherRef)
				if t01br7Claim(t, published.Record.Claims, "other-rule").Text != "Explicit unrelated addition" || published.Record.WriteReceipt == nil || published.Record.WriteReceipt.RequestID != "publish-unrelated" || !bytes.Equal(view.Files["specs/"+old.Record.ID+".md"], oldSpec) {
					t.Fatalf("unrelated replacement did not publish or changed old spec: %+v", published)
				}
			})
		}
	}
}

func TestT01BR8PublicMixedDuplicateMoveStillRefuses(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				base := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000861", "mixed-sequence", false, false)
				path := filepath.Join(root, ".haft", "specs", base.Record.ID+".md")
				tagged := bytes.Replace(base.Raw, []byte("receiving_use: Preserve exact scenario intent"), []byte("      x-steps: [!vendor a, !vendor a, b, b]\nreceiving_use: Preserve exact scenario intent"), 1)
				if bytes.Equal(tagged, base.Raw) {
					t.Fatal("tagged duplicate fixture was not installed")
				}
				if err := os.WriteFile(path, tagged, 0600); err != nil {
					t.Fatal(err)
				}
				view := t01br6View(t, root)
				base = t01br6Document(t, view, base.Record.ID)
				if !base.Valid() {
					t.Fatalf("tagged base is invalid: %+v", base.Diagnostics)
				}
				ref := base.Record.ID + "@" + base.Edition
				claim := base.Record.Claims[0]
				claim.Extra["x-steps"] = []any{"x", "b", "b", "a", "a"}
				id := "chg-20260927-00000861"
				proposal := t01br7Change(id, []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &claim, Reason: "Insert and move duplicates"}}}})
				t01br6Create(t, client, profile.changeTool, proposal, base, view, "mixed-move")
				before := t01br6View(t, root)
				preview := t01br6Preview(t, client, profile.changeTool, id)
				if preview.Kind != "conflict" || !preview.IsError {
					t.Fatalf("mixed tagged duplicate move previewed: %+v", preview)
				}
				t01br4MCPDiagnostic(t, client, profile.readTool, preview, "ambiguous_yaml_correspondence", "claims[0].x-steps")
				apply := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "apply", Ref: id, RequestID: "publish-mixed-move", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]})
				if apply.Kind != "conflict" || !apply.IsError {
					t.Fatalf("mixed tagged duplicate move applied: %+v", apply)
				}
				after := t01br6View(t, root)
				if after.Generation != before.Generation || !bytes.Equal(after.Files["specs/"+base.Record.ID+".md"], tagged) || len(after.Documents) != len(before.Documents) {
					t.Fatal("mixed duplicate move refusal changed durable state")
				}
			})
		}
	}
}
