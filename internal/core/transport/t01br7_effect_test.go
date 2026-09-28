package transport

import (
	"bytes"
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
)

func t01br7Change(id string, patches []change.SectionPatch) change.Change {
	return change.Change{
		Format: change.Format, ID: id, ChangeKey: id, Title: "Exact revision boundary",
		Intent: "Preserve only the authored meaning", State: "open", CreatedAt: "2026-09-27T00:00:00Z",
		Patches: patches,
	}
}

func t01br7Seed(t *testing.T, client *t01aProtocolClient, root, tool, format, id, key string) carrier.Document {
	t.Helper()
	raw := strings.Replace(t01br5Spec(format), "spec-20260927-00000011", id, 1)
	raw = strings.Replace(raw, "slug: local-rule", "slug: "+key, 1)
	raw = strings.Replace(raw, "about: domain:Billing.Semantic", "about: domain:Billing."+key, 1)
	written := client.mustCall(t, tool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "seed-" + key, Carrier: raw})
	if written.Kind != "written" || written.IsError {
		t.Fatalf("seed %s failed: %+v", key, written)
	}
	return t01br6Document(t, t01br6View(t, root), id)
}

func t01br7Advance(t *testing.T, client *t01aProtocolClient, root, tool string, base carrier.Document, suffix string, rename bool) carrier.Document {
	t.Helper()
	view := t01br6View(t, root)
	ref := base.Record.ID + "@" + base.Edition
	claim := base.Record.Claims[0]
	operation := change.Operation{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Advance the exact edition"}
	if rename {
		operation = change.Operation{Op: "RENAMED", ClaimID: claim.ID, NewID: "rule2", Reason: "Rename the exact claim"}
	} else {
		claim.Text = "Independent edition " + suffix
	}
	id := "chg-20260927-0000" + suffix
	candidate := t01br7Change(id, []change.SectionPatch{{Base: ref, Operations: []change.Operation{operation}}})
	t01br6Create(t, client, tool, candidate, base, view, "create-"+suffix)
	preview := t01br6Preview(t, client, tool, id)
	if preview.Kind != "ready" || preview.IsError {
		t.Fatalf("independent edition preview failed: %+v", preview)
	}
	t01br6Apply(t, client, tool, id, "apply", "apply-"+suffix, preview)
	return t01br6Successor(t, t01br6View(t, root), ref)
}

func TestT01BR7PublicAppliedTransitiveRebase(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, scenario := range []string{"plain", "owner", "renamed-owner"} {
				t.Run(profile.name+"/"+format+"/"+scenario, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)
					base := t01br7Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000101", "lineage-a")
					view := t01br6View(t, root)
					oldRef := base.Record.ID + "@" + base.Edition
					claim := base.Record.Claims[0]
					claim.Text = "Original intended edit"
					patch := change.SectionPatch{Base: oldRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Edit original"}}}
					if scenario != "plain" {
						patch.Extra = carrier.Extra{"x-owner": "ORIGINAL_PATCH_OWNER"}
					}
					original := t01br7Change("chg-20260927-00000101", []change.SectionPatch{patch})
					t01br6Create(t, client, profile.changeTool, original, base, view, "original")
					if first := t01br6Preview(t, client, profile.changeTool, original.ID); first.Kind != "ready" || first.IsError {
						t.Fatalf("original change does not preview: %+v", first)
					}
					oldBytes := bytes.Clone(t01br6View(t, root).Files["changes/"+original.ID+".md"])
					middle := t01br7Advance(t, client, root, profile.changeTool, base, "0102", scenario == "renamed-owner")
					current := t01br7Advance(t, client, root, profile.changeTool, middle, "0103", false)
					middleRef := middle.Record.ID + "@" + middle.Edition
					currentRef := current.Record.ID + "@" + current.Edition
					if middle.Record.ID == base.Record.ID || current.Record.ID == middle.Record.ID || !reflect.DeepEqual(middle.Record.Supersedes, []string{oldRef}) || !reflect.DeepEqual(current.Record.Supersedes, []string{middleRef}) {
						t.Fatalf("independent applied ancestry is not exact: %s -> %s -> %s", oldRef, middleRef, currentRef)
					}
					currentBytes := bytes.Clone(t01br6View(t, root).Files["specs/"+current.Record.ID+".md"])
					nextClaim := current.Record.Claims[0]
					nextClaim.Text = "Explicitly rebased edit"
					revisionID := "chg-20260927-00000104"
					updated := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: original.ID, RequestID: "transitive-rebase", Revision: &change.Revision{ID: revisionID, Reason: "Use the current exact descendant", Patches: []change.SectionPatch{{Base: currentRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: nextClaim.ID, Claim: &nextClaim, Reason: "Edit current"}}}}}})
					if updated.Kind != "written" || updated.IsError || delivery.Size(updated) > delivery.Budget {
						t.Fatalf("exact transitive rebase refused: %+v", updated)
					}
					view = t01br6View(t, root)
					saved := change.Parse(view.Files["changes/"+revisionID+".md"])
					if carrier.HasErrors(saved.Diagnostics) || saved.Change.Patches[0].Base != currentRef || !bytes.Equal(view.Files["changes/"+original.ID+".md"], oldBytes) {
						t.Fatalf("saved transitive rebase lost owner or history: %+v", saved.Change.Patches)
					}
					if scenario == "plain" && len(saved.Change.Patches[0].Extra) != 0 || scenario != "plain" && saved.Change.Patches[0].Extra["x-owner"] != "ORIGINAL_PATCH_OWNER" {
						t.Fatalf("saved transitive patch owner is wrong: %+v", saved.Change.Patches[0].Extra)
					}
					preview := t01br6Preview(t, client, profile.changeTool, revisionID)
					if preview.Kind != "ready" || preview.IsError {
						t.Fatalf("rebased exact descendant does not preview: %+v", preview)
					}
					t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "rebased-apply", preview)
					view = t01br6View(t, root)
					published := t01br6Successor(t, view, currentRef)
					if published.Record.Claims[0].ID != nextClaim.ID || published.Record.Claims[0].Text != "Explicitly rebased edit" || !bytes.Equal(view.Files["specs/"+current.Record.ID+".md"], currentBytes) {
						t.Fatalf("published rebase lost content or predecessor: %+v", published.Record.Claims)
					}
				})
			}
		}
	}
}

func TestT01BR7PublicReorderedAppliedLineages(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				alpha := t01br7Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000201", "lineage-alpha")
				beta := t01br7Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000202", "lineage-beta")
				view := t01br6View(t, root)
				alphaRef := alpha.Record.ID + "@" + alpha.Edition
				betaRef := beta.Record.ID + "@" + beta.Edition
				alphaClaim := alpha.Record.Claims[0]
				alphaClaim.Text = "Original alpha edit"
				betaClaim := beta.Record.Claims[0]
				betaClaim.Text = "Original beta edit"
				id := "chg-20260927-00000201"
				original := t01br7Change(id, []change.SectionPatch{
					{Base: alphaRef, Extra: carrier.Extra{"x-owner": "ALPHA_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &alphaClaim, Reason: "Edit alpha"}}},
					{Base: betaRef, Extra: carrier.Extra{"x-owner": "BETA_ONLY"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaClaim, Reason: "Edit beta"}}},
				})
				raw, err := change.Encode(original, []byte("Rationale.\n"))
				if err != nil {
					t.Fatal(err)
				}
				created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "two-lineages", Carrier: string(raw), Snapshots: map[string][]byte{alpha.Edition: view.CurrentSnapshots[alpha.Edition], beta.Edition: view.CurrentSnapshots[beta.Edition]}})
				if created.Kind != "written" || created.IsError {
					t.Fatalf("two-lineage change creation failed: %+v", created)
				}
				if preview := t01br6Preview(t, client, profile.changeTool, id); preview.Kind != "ready" || preview.IsError {
					t.Fatalf("two-lineage original preview failed: %+v", preview)
				}
				oldBytes := bytes.Clone(t01br6View(t, root).Files["changes/"+id+".md"])
				alphaMiddle := t01br7Advance(t, client, root, profile.changeTool, alpha, "0202", true)
				alphaCurrent := t01br7Advance(t, client, root, profile.changeTool, alphaMiddle, "0203", false)
				betaMiddle := t01br7Advance(t, client, root, profile.changeTool, beta, "0204", false)
				betaCurrent := t01br7Advance(t, client, root, profile.changeTool, betaMiddle, "0205", false)
				alphaCurrentRef := alphaCurrent.Record.ID + "@" + alphaCurrent.Edition
				betaCurrentRef := betaCurrent.Record.ID + "@" + betaCurrent.Edition
				alphaEdited := alphaCurrent.Record.Claims[0]
				alphaEdited.Text = "Rebased alpha rule two"
				betaEdited := betaCurrent.Record.Claims[0]
				betaEdited.Text = "Rebased beta rule"
				revisionID := "chg-20260927-00000206"
				rebased := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: id, RequestID: "reorder-rebase", Revision: &change.Revision{ID: revisionID, Reason: "Rebase two exact chains", Patches: []change.SectionPatch{
					{Base: betaCurrentRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &betaEdited, Reason: "Edit beta"}}},
					{Base: alphaCurrentRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule2", Claim: &alphaEdited, Reason: "Edit alpha"}}},
				}}})
				if rebased.Kind != "written" || rebased.IsError || delivery.Size(rebased) > delivery.Budget {
					t.Fatalf("reordered transitive lineages refused: %+v", rebased)
				}
				view = t01br6View(t, root)
				saved := change.Parse(view.Files["changes/"+revisionID+".md"])
				if carrier.HasErrors(saved.Diagnostics) || saved.Change.Patches[0].Base != betaCurrentRef || saved.Change.Patches[0].Extra["x-owner"] != "BETA_ONLY" || saved.Change.Patches[1].Base != alphaCurrentRef || saved.Change.Patches[1].Extra["x-owner"] != "ALPHA_ONLY" || !bytes.Equal(view.Files["changes/"+id+".md"], oldBytes) {
					t.Fatalf("reordered transitive owners crossed: %+v", saved.Change.Patches)
				}
				preview := t01br6Preview(t, client, profile.changeTool, revisionID)
				if preview.Kind != "ready" || preview.IsError {
					t.Fatalf("reordered transitive proposal cannot publish: %+v", preview)
				}
				t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "reorder-apply", preview)
				view = t01br6View(t, root)
				alphaPublished := t01br6Successor(t, view, alphaCurrentRef).Record.Claims[0]
				betaPublished := t01br6Successor(t, view, betaCurrentRef).Record.Claims[0]
				if alphaPublished.ID != "rule2" || alphaPublished.Text != "Rebased alpha rule two" || betaPublished.ID != "rule" || betaPublished.Text != "Rebased beta rule" {
					t.Fatalf("published reordered owners lost intended content: alpha=%+v beta=%+v", alphaPublished, betaPublished)
				}
			})
		}
	}
}

func t01br7Claim(t *testing.T, claims []carrier.Claim, id string) carrier.Claim {
	t.Helper()
	for _, claim := range claims {
		if claim.ID == id {
			return claim
		}
	}
	t.Fatalf("claim %s is absent from %+v", id, claims)
	return carrier.Claim{}
}

func TestT01BR7PublicProposalOnlyExampleEdits(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, kind := range []string{"MODIFIED", "ADDED"} {
				for _, editKind := range []string{"partial", "clear", "rename", "tagged-survivor"} {
					t.Run(profile.name+"/"+format+"/"+kind+"/"+editKind, func(t *testing.T) {
						root := t.TempDir()
						client := t01aStartClient(t, app.Service{Root: root}, profile.name)
						client.list(t)
						base := t01br7Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000301", "proposal-examples")
						view := t01br6View(t, root)
						ref := base.Record.ID + "@" + base.Edition
						claim := base.Record.Claims[0]
						if kind == "ADDED" {
							claim = carrier.Claim{ID: "new-rule", Kind: "definition", Text: "New proposal claim"}
						}
						proposed := []carrier.Example{{ID: "keep", Text: "Keep proposed scenario", Extra: carrier.Extra{"x-owner": "retained"}}, {ID: "drop", Text: "Withdraw proposed scenario"}}
						claim.Examples = proposed
						operation := change.Operation{Op: kind, Claim: &claim, Reason: "Propose two scenarios"}
						if kind == "MODIFIED" {
							claim.Examples = append(append([]carrier.Example{}, base.Record.Claims[0].Examples...), proposed...)
							operation.ClaimID = claim.ID
						}
						id := "chg-20260927-00000301"
						original := t01br7Change(id, []change.SectionPatch{{Base: ref, Operations: []change.Operation{operation}}})
						t01br6Create(t, client, profile.changeTool, original, base, view, "original-examples")
						if first := t01br6Preview(t, client, profile.changeTool, id); first.Kind != "ready" || first.IsError {
							t.Fatalf("original proposal examples do not preview: %+v", first)
						}
						path := filepath.Join(root, ".haft", "changes", id+".md")
						view = t01br6View(t, root)
						oldBytes := bytes.Clone(view.Files["changes/"+id+".md"])
						if editKind == "tagged-survivor" {
							tagged := bytes.Replace(oldBytes, []byte("x-owner: retained"), []byte("x-owner: !vendor retained"), 1)
							if bytes.Equal(oldBytes, tagged) {
								t.Fatal("tagged survivor fixture was not installed")
							}
							if err := os.WriteFile(path, tagged, 0600); err != nil {
								t.Fatal(err)
							}
							oldBytes = tagged
						}
						parsed := change.Parse(oldBytes)
						if carrier.HasErrors(parsed.Diagnostics) {
							t.Fatal(parsed.Diagnostics)
						}
						revised := parsed.Change.Patches
						claimNext := *revised[0].Operations[0].Claim
						switch editKind {
						case "partial", "tagged-survivor":
							claimNext.Examples = claimNext.Examples[:len(claimNext.Examples)-1]
						case "clear":
							if kind == "MODIFIED" {
								claimNext.Examples = claimNext.Examples[:len(base.Record.Claims[0].Examples)]
							} else {
								claimNext.Examples = []carrier.Example{}
							}
						case "rename":
							claimNext.Examples[len(claimNext.Examples)-1].ID = "renamed"
						}
						revised[0].Operations[0].Claim = &claimNext
						before := t01br6View(t, root)
						revisionID := "chg-20260927-00000302"
						updated := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "update", Ref: id, RequestID: "withdraw-example", Revision: &change.Revision{ID: revisionID, Reason: "Explicit example list edit", Patches: revised}})
						if editKind == "tagged-survivor" {
							if updated.Kind != "conflict" || !updated.IsError {
								t.Fatalf("retained tagged example was normalized: %+v", updated)
							}
							index := 0
							if kind == "MODIFIED" {
								index = len(base.Record.Claims[0].Examples)
							}
							path := fmt.Sprintf("patches[0].operations[0].claim.examples[%d].x-owner", index)
							t01br4MCPDiagnostic(t, client, profile.readTool, updated, "unsupported_yaml_tag_conversion", path)
							if after := t01br6View(t, root); after.Generation != before.Generation || !bytes.Equal(after.Files["changes/"+id+".md"], oldBytes) || len(after.Files["changes/"+revisionID+".md"]) != 0 {
								t.Fatal("tagged survivor conflict changed durable history")
							}
							return
						}
						if updated.Kind != "written" || updated.IsError || delivery.Size(updated) > delivery.Budget {
							t.Fatalf("proposal-only example edit refused: %+v", updated)
						}
						view = t01br6View(t, root)
						saved := change.Parse(view.Files["changes/"+revisionID+".md"])
						if carrier.HasErrors(saved.Diagnostics) || !bytes.Equal(view.Files["changes/"+id+".md"], oldBytes) {
							t.Fatalf("example revision changed historical bytes: %+v", saved.Diagnostics)
						}
						if saved.Change.Patches[0].Operations[0].Claim == nil || !reflect.DeepEqual(saved.Change.Patches[0].Operations[0].Claim.Examples, claimNext.Examples) {
							t.Fatalf("saved explicit example list differs from request: %+v", saved.Change.Patches[0].Operations[0].Claim)
						}
						preview := t01br6Preview(t, client, profile.changeTool, revisionID)
						if preview.Kind != "ready" || preview.IsError {
							t.Fatalf("revised examples do not preview: %+v", preview)
						}
						t01br6Apply(t, client, profile.changeTool, revisionID, "apply", "publish-examples", preview)
						view = t01br6View(t, root)
						if editKind == "clear" && kind == "MODIFIED" {
							if len(view.Documents) != 1 || !bytes.Equal(view.Files["specs/"+base.Record.ID+".md"], base.Raw) {
								t.Fatal("clearing only proposal-added examples changed the unchanged base")
							}
							return
						}
						published := t01br6Successor(t, view, ref)
						got := t01br7Claim(t, published.Record.Claims, claimNext.ID)
						for _, example := range got.Examples {
							if example.ID == "drop" {
								t.Fatalf("withdrawn example was published: %+v", got.Examples)
							}
						}
						if editKind == "partial" && t01br7Example(t, got.Examples, "keep").Extra["x-owner"] != "retained" {
							t.Fatalf("surviving example extension was lost: %+v", got.Examples)
						}
						if editKind == "clear" {
							expected := 0
							if kind == "MODIFIED" {
								expected = len(base.Record.Claims[0].Examples)
							}
							if len(got.Examples) != expected {
								t.Fatalf("explicit clear retained proposal-only examples: %+v", got.Examples)
							}
						}
						if editKind == "rename" && t01br7Example(t, got.Examples, "renamed").ID != "renamed" {
							t.Fatalf("example rename was not published: %+v", got.Examples)
						}
					})
				}
			}
		}
	}
}

func t01br7Example(t *testing.T, examples []carrier.Example, id string) carrier.Example {
	t.Helper()
	for _, example := range examples {
		if example.ID == id {
			return example
		}
	}
	t.Fatalf("example %s is absent from %+v", id, examples)
	return carrier.Example{}
}

func TestT01BR7PublicDuplicateMoveRefusesWithoutMutation(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			t.Run(profile.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				client := t01aStartClient(t, app.Service{Root: root}, profile.name)
				client.list(t)
				base := t01br7Seed(t, client, root, profile.writeTool, format, "spec-20260927-00000401", "duplicate-steps")
				path := filepath.Join(root, ".haft", "specs", base.Record.ID+".md")
				tagged := strings.Replace(string(base.Raw), "receiving_use: Review a local claim", "      x-steps: [!vendor a, !vendor a, b, b]\nreceiving_use: Review a local claim", 1)
				if tagged == string(base.Raw) {
					t.Fatal("tagged base fixture was not installed")
				}
				if err := os.WriteFile(path, []byte(tagged), 0600); err != nil {
					t.Fatal(err)
				}
				view := t01br6View(t, root)
				if len(view.Documents) != 1 || view.Documents[0].Record.ID != base.Record.ID || !view.Documents[0].Valid() {
					t.Fatalf("tagged base did not parse as the selected spec: diagnostics=%+v", view.Diagnostics)
				}
				base = t01br6Document(t, view, base.Record.ID)
				ref := base.Record.ID + "@" + base.Edition
				claim := base.Record.Claims[0]
				claim.Extra["x-steps"] = []any{"b", "b", "a", "a"}
				id := "chg-20260927-00000401"
				candidate := t01br7Change(id, []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Reorder duplicated steps"}}}})
				t01br6Create(t, client, profile.changeTool, candidate, base, view, "duplicate-move")
				before := t01br6View(t, root)
				preview := t01br6Preview(t, client, profile.changeTool, id)
				if preview.Kind != "conflict" || !preview.IsError {
					t.Fatalf("duplicate tagged move preview published a conversion: %+v", preview)
				}
				t01br4MCPDiagnostic(t, client, profile.readTool, preview, "ambiguous_yaml_correspondence", "claims[0].x-steps")
				apply := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "apply", Ref: id, RequestID: "duplicate-apply", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]})
				if apply.Kind != "conflict" || !apply.IsError {
					t.Fatalf("duplicate tagged move applied: %+v", apply)
				}
				after := t01br6View(t, root)
				if after.Generation != before.Generation || !bytes.Equal(after.Files["specs/"+base.Record.ID+".md"], []byte(tagged)) || len(after.Documents) != len(before.Documents) || len(after.Files) != len(before.Files) {
					t.Fatal("duplicate move refusal changed durable bytes or generation")
				}

				// A historical change with a tagged sequence is a raw fixture only;
				// the attempted update remains the advertised MCP effect.
				oldClaim := base.Record.Claims[0]
				oldClaim.Extra["x-steps"] = []any{"a", "a", "b", "b"}
				oldID := "chg-20260927-00000411"
				old := t01br7Change(oldID, []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: oldClaim.ID, Claim: &oldClaim, Reason: "Keep original steps"}}}})
				t01br6Create(t, client, profile.changeTool, old, base, after, "old-steps")
				changePath := filepath.Join(root, ".haft", "changes", oldID+".md")
				oldRaw := t01br6View(t, root).Files["changes/"+oldID+".md"]
				taggedChange := bytes.Replace(oldRaw, []byte("- a\n"), []byte("- !vendor a\n"), 2)
				if bytes.Count(taggedChange, []byte("!vendor a")) != 2 {
					t.Fatal("tagged change fixture was not installed")
				}
				if err := os.WriteFile(changePath, taggedChange, 0600); err != nil {
					t.Fatal(err)
				}
				parsed := change.Parse(taggedChange)
				if carrier.HasErrors(parsed.Diagnostics) {
					t.Fatal(parsed.Diagnostics)
				}
				revised := parsed.Change.Patches
				next := *revised[0].Operations[0].Claim
				next.Extra["x-steps"] = []any{"b", "b", "a", "a"}
				revised[0].Operations[0].Claim = &next
				before = t01br6View(t, root)
				revisionID := "chg-20260927-00000412"
				updated := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "update", Ref: oldID, RequestID: "duplicate-revision", Revision: &change.Revision{ID: revisionID, Reason: "Reorder duplicated values", Patches: revised}})
				if updated.Kind != "conflict" || !updated.IsError {
					t.Fatalf("revision converted tagged duplicate move: %+v", updated)
				}
				t01br4MCPDiagnostic(t, client, profile.readTool, updated, "unsupported_yaml_tag_conversion", "patches[0].operations[0].claim.x-steps[0]")
				after = t01br6View(t, root)
				if after.Generation != before.Generation || !bytes.Equal(after.Files["changes/"+oldID+".md"], taggedChange) || len(after.Files["changes/"+revisionID+".md"]) != 0 {
					t.Fatal("duplicate revision refusal changed durable history")
				}
			})
		}
	}
}
