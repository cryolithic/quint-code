package transport

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01BR10PublicAncestorOwnersCannotCollapse(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		for _, format := range []string{"haft/1", "haft/2"} {
			for _, reverse := range []bool{false, true} {
				name := profile.name + "/" + format + "/forward"
				if reverse {
					name = profile.name + "/" + format + "/reverse"
				}
				t.Run(name, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)

					a := t01br8Seed(t, client, root, profile.writeTool, format, "spec-20260927-00001001", "br10-a", false, false)
					aRef := t01br8PublicExact(t, client, profile.readTool, a.Record.ID)
					if aRef != a.Record.ID+"@"+a.Edition {
						t.Fatalf("public A ref differs from its captured edition: %s", aRef)
					}
					b := t01br7Advance(t, client, root, profile.changeTool, a, "1011", false)
					bRef := t01br8PublicExact(t, client, profile.readTool, b.Record.ID)
					if bRef != b.Record.ID+"@"+b.Edition || !reflect.DeepEqual(b.Record.Supersedes, []string{aRef}) {
						t.Fatalf("public B is not the exact successor of A: %s", bRef)
					}

					aClaim := a.Record.Claims[0]
					aClaim.Text = "Outstanding A edit"
					bClaim := b.Record.Claims[0]
					bClaim.Text = "Outstanding B edit"
					oldA := change.SectionPatch{Base: aRef, Extra: carrier.Extra{"x-owner": "A_OWNER"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &aClaim, Reason: "Edit A"}}}
					oldB := change.SectionPatch{Base: bRef, Extra: carrier.Extra{"x-owner": "B_OWNER"}, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &bClaim, Reason: "Edit B"}}}
					patches := []change.SectionPatch{oldA, oldB}
					if reverse {
						patches = []change.SectionPatch{oldB, oldA}
					}
					originalID := "chg-20260927-00001001"
					original := t01br7Change(originalID, patches)
					raw, err := change.Encode(original, []byte("Two distinct old owners.\n"))
					if err != nil {
						t.Fatal(err)
					}
					created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "br10-original", Carrier: string(raw)})
					if created.Kind != "written" || created.IsError || delivery.Size(created) > delivery.Budget {
						t.Fatalf("public two-owner change creation failed: %+v", created)
					}
					beforeC := t01br6View(t, root)
					originalBytes := bytes.Clone(beforeC.Files["changes/"+originalID+".md"])
					aBytes := bytes.Clone(beforeC.Files["specs/"+a.Record.ID+".md"])
					bBytes := bytes.Clone(beforeC.Files["specs/"+b.Record.ID+".md"])

					c := t01br7Advance(t, client, root, profile.changeTool, b, "1012", false)
					cRef := t01br8PublicExact(t, client, profile.readTool, c.Record.ID)
					if cRef != c.Record.ID+"@"+c.Edition || !reflect.DeepEqual(c.Record.Supersedes, []string{bRef}) {
						t.Fatalf("public C is not the exact successor of B: %s", cRef)
					}
					for _, ref := range []string{aRef, bRef, cRef} {
						found := client.mustCall(t, profile.readTool, app.Request{Format: delivery.Format, Operation: "recall", Ref: ref})
						if found.Kind != "found" || found.IsError || found.Data.(map[string]any)["exact_ref"] != ref {
							t.Fatalf("historical exact ref was not publicly recoverable: %+v", found)
						}
					}

					cClaim := c.Record.Claims[0]
					cClaim.Text = "Resolved current edit"
					revisionID := "chg-20260927-00001002"
					selected := change.SectionPatch{Base: cRef, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: "rule", Claim: &cClaim, Reason: "Edit C"}}}
					revision := change.Revision{ID: revisionID, Reason: "Rebase to C without selecting either owner", Patches: []change.SectionPatch{selected}}
					before := t01br6View(t, root)
					refused := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "rebase", Ref: originalID, RequestID: "br10-ambiguous", Revision: &revision})
					if refused.Kind != "conflict" || !refused.IsError {
						t.Fatalf("two old owners were silently assigned to one new patch: %+v", refused)
					}
					t01br4MCPDiagnostic(t, client, profile.readTool, refused, "ambiguous_revision_correspondence", "patches[0]")
					t01br4MCPDiagnostic(t, client, profile.readTool, refused, "ambiguous_revision_correspondence", "patches[1]")
					after := t01br6View(t, root)
					if after.Generation != before.Generation || !reflect.DeepEqual(after.Files, before.Files) || len(after.Files["changes/"+revisionID+".md"]) != 0 {
						t.Fatal("ambiguous revision changed durable state or saved a successor")
					}
					if !bytes.Equal(after.Files["changes/"+originalID+".md"], originalBytes) || !bytes.Equal(after.Files["specs/"+a.Record.ID+".md"], aBytes) || !bytes.Equal(after.Files["specs/"+b.Record.ID+".md"], bBytes) {
						t.Fatal("ambiguous revision changed exact predecessor bytes")
					}
				})
			}
		}
	}
}
