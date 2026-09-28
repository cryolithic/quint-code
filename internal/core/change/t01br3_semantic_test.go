package change

import (
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func TestT01BR3ExplicitRemovalDoesNotBlockOnRemovedTag(t *testing.T) {
	d, _, _, c := setup(t)
	raw := strings.Replace(string(d.Raw), "  - id: total-preserved\n", "  - id: total-preserved\n    x-obsolete: !vendor abc\n", 1)
	_, snapshot, hash, err := carrier.NewSnapshot([]byte(raw), carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + hash
	c.Patches[0].Base = ref
	c.Patches[0].Operations = []Operation{{Op: "REMOVED", ClaimID: "total-preserved", Reason: "Explicitly retire this claim"}}
	preview := Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "ready" || len(preview.Losses) != 1 || preview.Losses[0].Kind != "claim_removed" || len(preview.Outputs) != 1 {
		t.Fatalf("explicit removal was not distinguished from retained loss: %+v", preview)
	}
}

func TestT01BR3ClaimRenameAndScenarioReorderPreserveRetainedExtensions(t *testing.T) {
	d, _, _, c := setup(t)
	first := d.Record.Claims[0].Examples[0]
	first.Extra = carrier.Extra{"x-rule": map[string]any{"basis": "first"}}
	second := carrier.Example{ID: "new-order", Given: "A new order", When: "Cancel is invoked", Then: "The amount remains unchanged", Extra: carrier.Extra{"x-rule": map[string]any{"basis": "second"}}}
	d.Record.Claims[0].Examples = []carrier.Example{first, second}
	raw, err := carrier.Encode(d.Record, d.Body)
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, hash, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + hash
	c.Patches[0].Base = ref
	modified := d.Record.Claims[0]
	modified.Text += " The two scenarios are intentionally reordered."
	modified.Examples = []carrier.Example{second, first}
	c.Patches[0].Operations = []Operation{
		{Op: "MODIFIED", ClaimID: modified.ID, Claim: &modified, Reason: "Clarify and reorder scenarios"},
		{Op: "RENAMED", ClaimID: "cancelable", NewID: "eligible", Reason: "Repair stable claim address"},
	}
	preview := Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "ready" || len(preview.Outputs) != 1 || len(preview.Losses) != 1 || preview.Losses[0].Kind != "claim_address_changed" {
		t.Fatalf("intentional claim/scenario delta was called semantic loss: %+v", preview)
	}
	got := preview.Outputs[0].Successor.Claims[0].Examples
	if got[0].ID != "new-order" || got[1].ID != "paid-order" || got[0].Extra["x-rule"] == nil || got[1].Extra["x-rule"] == nil {
		t.Fatalf("scenario identity/extensions did not survive reorder: %+v", got)
	}
}

func TestT01BR3IntermediateEditCannotExemptFinallyRetainedExtension(t *testing.T) {
	d, _, _, c := setup(t)
	raw := strings.Replace(string(d.Raw), "  - id: total-preserved\n", "  - id: total-preserved\n    x-vendor: !vendor abc\n", 1)
	_, snapshot, hash, err := carrier.NewSnapshot([]byte(raw), carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	ref := d.Record.ID + "@" + hash
	c.Patches[0].Base = ref
	base := carrier.Parse([]byte(raw)).Record.Claims[0]
	changed := base
	changed.Extra = carrier.Extra{"x-vendor": "different"}
	changed.Text += " Deliberate edit."
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: base.ID, Claim: &changed, Reason: "Change extension meaning"}}
	preview := Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "ready" {
		t.Fatalf("explicit extension edit was refused: %+v", preview.Diagnostics)
	}
	restored := changed
	restored.Extra = carrier.Extra{"x-vendor": "abc"}
	c.Patches[0].Operations = append(c.Patches[0].Operations, Operation{Op: "MODIFIED", ClaimID: base.ID, Claim: &restored, Reason: "Restore the original typed value"})
	preview = Preview(c, map[string]Basis{ref: {Snapshot: snapshot, CurrentRef: ref}})
	if preview.Kind != "conflict" || !code(preview.Diagnostics, "unsupported_yaml_tag_conversion") || len(preview.Outputs) != 0 {
		t.Fatalf("intermediate edit hid finally retained tag loss: %+v", preview)
	}
}
