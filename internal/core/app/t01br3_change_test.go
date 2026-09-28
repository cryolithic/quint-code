package app

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01BR3ChangeCreateRefusesSuppliedClaimMeaningLoss(t *testing.T) {
	s := service(t)
	_, base := seed(t, s)
	for _, tc := range []struct {
		name string
		yaml string
		path string
	}{
		{"custom-tag", "x-unit: !currency 12", "patches[0].operations[0].claim.x-unit"},
		{"large-number", "x-unit: 123456789012345678901234", "patches[0].operations[0].claim.x-unit"},
		{"float-one", "x-unit: 1.0", "patches[0].operations[0].claim.x-unit"},
		{"scenario", "examples:\n              - id: payment\n                given: A pending order\n                x-unit: !currency 12", "patches[0].operations[0].claim.examples[0].x-unit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := "chg-20260925-00000001"
			raw := "---\nformat: haft.change/1\nid: " + id + "\nchange_key: " + id + "\ntitle: Preserve supplied claim\nintent: Add one authored definition\nstate: open\ncreated_at: 2026-09-25T00:00:00Z\npatches:\n  - base: " + base + "\n    operations:\n      - op: ADDED\n        claim:\n          id: payment\n          kind: definition\n          text: A payment remains inspectable.\n          " + tc.yaml + "\n---\nAuthored change prose.\n"
			before := readView(t, s)
			q := Request{Operation: "change", Action: "create", RequestID: "reject-" + tc.name, Carrier: raw}
			refused := public(t, s, q)
			if refused.Kind != "invalid" || !refused.IsError || !hasAppDiagnostic(Result{Diagnostics: refused.Diagnostics}, "unsupported_yaml_tag_conversion") && !hasAppDiagnostic(Result{Diagnostics: refused.Diagnostics}, "unsupported_yaml_value_conversion") {
				t.Fatalf("create did not refuse semantic conversion: %+v", refused)
			}
			foundPath := false
			for _, diagnostic := range refused.Diagnostics {
				foundPath = foundPath || diagnostic.Path == tc.path
			}
			if !foundPath || delivery.Size(refused) > delivery.Budget {
				t.Fatalf("exact loss path or bounded disclosure missing: %+v", refused)
			}
			after := readView(t, s)
			if after.Generation != before.Generation || !reflect.DeepEqual(after.Files, before.Files) || len(changeDocuments(after)) != 0 {
				t.Fatal("refused create mutated durable history")
			}
		})
	}
}

func TestT01BR3ChangePreviewApplyRefuseRetainedBaseLoss(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		path string
	}{
		{"tagged-root", "x-vendor: !vendor abc", "x-vendor"},
		{"tagged-v2-root", "x-vendor: !vendor abc", "x-vendor"},
		{"large-number", "x-vendor: 123456789012345678901234", "x-vendor"},
		{"tagged-claim", "x-vendor: !vendor abc", "claims[0].x-vendor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := service(t)
			old, _ := seed(t, s)
			raw := string(old.Raw)
			if tc.name == "tagged-v2-root" {
				raw = strings.Replace(raw, "format: haft/1", "format: haft/2", 1)
				raw = strings.Replace(raw, "status: proposed\n", "", 1)
				raw = strings.Replace(raw, "origin: agent_proposal", "origin: agent_edit", 1)
			}
			if tc.name == "tagged-claim" {
				raw = strings.Replace(raw, "      text:", "      "+tc.yaml+"\n      text:", 1)
			} else {
				raw = strings.Replace(raw, "kind: spec\n", "kind: spec\n"+tc.yaml+"\n", 1)
			}
			if raw == string(old.Raw) {
				t.Fatal("tag fixture did not alter exact base")
			}
			path := filepath.Join(s.Root, ".haft", "specs", old.Record.ID+".md")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			v := readView(t, s)
			baseDoc := v.Documents[0]
			if !baseDoc.Valid() {
				t.Fatalf("historical base became unreadable: %+v", baseDoc.Diagnostics)
			}
			base := baseDoc.Record.ID + "@" + baseDoc.Edition
			claim := baseDoc.Record.Claims[0]
			claim.Text += " This delta changes known text."
			changeID := "chg-20260925-00000001"
			c := change.Change{Format: change.Format, ID: changeID, ChangeKey: changeID, Title: "Retained base", Intent: "Change known text while retaining extensions", State: "open", CreatedAt: s.now(), Patches: []change.SectionPatch{{Base: base, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify text"}}}}}
			changeRaw, err := change.Encode(c, []byte("Keep this change prose.\n"))
			if err != nil {
				t.Fatal(err)
			}
			created := public(t, s, Request{Operation: "change", Action: "create", RequestID: "create-change", Carrier: string(changeRaw), Snapshots: map[string][]byte{baseDoc.Edition: v.CurrentSnapshots[baseDoc.Edition]}})
			if created.Kind != "written" {
				t.Fatalf("change creation: %+v", created)
			}
			before := readView(t, s)
			preview := public(t, s, Request{Operation: "change", Action: "preview", Ref: changeID})
			if preview.Kind != "conflict" || !preview.IsError || delivery.Size(preview) > delivery.Budget {
				t.Fatalf("preview accepted base semantic loss: %+v", preview)
			}
			found := false
			for _, diagnostic := range preview.Diagnostics {
				found = found || diagnostic.Path == tc.path
			}
			if !found {
				t.Fatalf("preview omitted retained source path %s: %+v", tc.path, preview.Diagnostics)
			}
			if tc.name == "tagged-root" {
				read := public(t, s, Request{Operation: "recall", Ref: base})
				if read.Kind != "found" || carrier.Digest(baseDoc.Raw) != carrier.Digest([]byte(raw)) {
					t.Fatalf("exact source ref or digest unavailable: %+v", read)
				}
				carrierBytes := collectPart(t, s, deliveredPart(t, s, read, "carrier"))
				if !bytes.Equal(carrierBytes, []byte(raw)) {
					t.Fatal("public exact read lost the unsupported source bytes")
				}
			}
			apply := Request{Operation: "change", Action: "apply", Ref: changeID, RequestID: "refused-apply", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
			for _, action := range []string{"apply", "sync"} {
				apply.Action = action
				refused := public(t, s, apply)
				if refused.Kind != "conflict" || !refused.IsError {
					t.Fatalf("%s accepted retained base loss: %+v", action, refused)
				}
			}
			after := readView(t, s)
			if after.Generation != before.Generation || !reflect.DeepEqual(after.Files, before.Files) || !bytes.Equal(after.CurrentSnapshots[baseDoc.Edition], before.CurrentSnapshots[baseDoc.Edition]) {
				t.Fatal("refused preview/apply/sync changed durable source")
			}
		})
	}
}

func TestT01BR3ChangeRevisionRefusesRetainedCarrierLoss(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		code string
	}{
		{"tagged-extension", "x-vendor: !vendor abc", "unsupported_yaml_tag_conversion"},
		{"large-number", "x-vendor: 123456789012345678901234", "unsupported_yaml_value_conversion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := service(t)
			id := "chg-20260925-00000001"
			c := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Retained authoring", Intent: "Keep authored context", State: "open", CreatedAt: s.now(), NoSpecChangeReason: "Document-only fixture"}
			raw, err := change.Encode(c, []byte("Exact change prose.\n"))
			if err != nil {
				t.Fatal(err)
			}
			created := public(t, s, Request{Operation: "change", Action: "create", RequestID: "create-old", Carrier: string(raw)})
			if created.Kind != "written" {
				t.Fatalf("change create: %+v", created)
			}
			oldPath := filepath.Join(s.Root, ".haft", "changes", id+".md")
			modified := strings.Replace(string(raw), "title: Retained authoring\n", "title: Retained authoring\n"+tc.line+"\n", 1)
			if err := os.WriteFile(oldPath, []byte(modified), 0600); err != nil {
				t.Fatal(err)
			}
			before := readView(t, s)
			q := Request{Operation: "change", Action: "archive", Ref: id, RequestID: "reject-revision", Revision: &change.Revision{ID: "chg-20260925-00000002", Reason: "Archive exact history"}}
			refused := public(t, s, q)
			if refused.Kind != "conflict" || !refused.IsError {
				t.Fatalf("revision accepted retained content loss: %+v", refused)
			}
			pathFound := false
			for _, diagnostic := range refused.Diagnostics {
				pathFound = pathFound || diagnostic.Path == "x-vendor" && diagnostic.Code == tc.code
			}
			if !pathFound {
				t.Fatalf("revision omitted exact source path: %+v", refused.Diagnostics)
			}
			after := readView(t, s)
			if after.Generation != before.Generation || !reflect.DeepEqual(after.Files, before.Files) || len(changeDocuments(after)) != 1 {
				t.Fatal("refused revision mutated history")
			}
		})
	}
}

func TestT01BR3ChangeArchiveAllowsIntentionalControlRewriteAndReplay(t *testing.T) {
	s := service(t)
	id := "chg-20260925-00000001"
	c := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Archive known state", Intent: "Keep authored context", State: "open", CreatedAt: s.now(), NoSpecChangeReason: "Document-only fixture"}
	raw, err := change.Encode(c, []byte("Exact change prose.\n"))
	if err != nil {
		t.Fatal(err)
	}
	created := public(t, s, Request{Operation: "change", Action: "create", RequestID: "create-old", Carrier: string(raw)})
	if created.Kind != "written" {
		t.Fatalf("change create: %+v", created)
	}
	oldPath := filepath.Join(s.Root, ".haft", "changes", id+".md")
	modified := strings.Replace(string(raw), "state: open\n", "state: !!str open\n", 1)
	if err := os.WriteFile(oldPath, []byte(modified), 0600); err != nil {
		t.Fatal(err)
	}
	q := Request{Operation: "change", Action: "archive", Ref: id, RequestID: "archive-known-state", Revision: &change.Revision{ID: "chg-20260925-00000002", Reason: "Archive authored change"}}
	written := public(t, s, q)
	if written.Kind != "written" || written.IsError {
		t.Fatalf("intentional state rewrite refused: %+v", written)
	}
	if oldBytes, err := os.ReadFile(oldPath); err != nil || !bytes.Equal(oldBytes, []byte(modified)) {
		t.Fatal("archive changed exact predecessor bytes")
	}
	fresh := Service{Root: s.Root, Now: s.Now}
	replayed := public(t, fresh, q)
	if replayed.Kind != "replayed" || replayed.IsError || delivery.Size(replayed) > delivery.Budget || len(changeDocuments(readView(t, fresh))) != 2 {
		t.Fatalf("identical restart replay changed publication: %+v", replayed)
	}
}
