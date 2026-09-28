package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

func t01br4MCPProfiles() []struct{ name, changeTool, writeTool, readTool string } {
	return []struct{ name, changeTool, writeTool, readTool string }{
		{ProfileDefault, "haft_change", "haft_write", "haft_read"},
		{ProfileLegacy, "haft", "haft", "haft"},
	}
}

func t01br4MCPDiagnostic(t *testing.T, client *t01aProtocolClient, readTool string, response delivery.Response, code, path string) {
	t.Helper()
	if delivery.Size(response) > delivery.Budget {
		t.Fatalf("MCP refusal exceeded 8192 bytes: %+v", response)
	}
	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Code == code && diagnostic.Path == path {
			return
		}
	}
	if response.Delivery.Catalog == nil {
		t.Fatalf("MCP refusal omitted exact diagnostic and continuation: %+v", response)
	}
	parts := t01aParts(t, client, response)
	name := "diagnostics"
	if _, found := parts[name]; !found {
		name = "data_diagnostics"
	}
	if _, found := parts[name]; !found {
		t.Fatalf("MCP refusal omitted diagnostic continuation: %+v", parts)
	}
	page := t01aRead(t, client, readTool, delivery.Request{
		Format: delivery.Format, Operation: "read", Ref: response.Delivery.Catalog.Ref,
		View: "detail", Part: name, ExpectedGeneration: response.Basis["memory_generation"],
	})
	raw, err := json.Marshal(page.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(code)) || !bytes.Contains(raw, []byte(path)) {
		t.Fatalf("MCP continuation lost %s at %s: %s", code, path, raw)
	}
}

func TestT01BR4IgnoredRevisionTagAcrossMCPProfiles(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := t.TempDir()
			client := t01aStartClient(t, app.Service{Root: root}, profile.name)
			client.list(t)
			id := "chg-20260927-00000011"
			raw := "---\nformat: haft.change/1\nid: " + id + "\nchange_key: " + id + "\ntitle: Archive a local change\nintent: Preserve task context\nstate: open\ncreated_at: 2026-09-27T00:00:00Z\nno_spec_change_reason: Administrative fixture\ntasks:\n  - id: inspect\n    text: Inspect source\n    done: false\n    x-owner: abc\n---\nRationale.\n"
			created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "t01br4-mcp-create", Carrier: raw})
			if created.Kind != "written" || created.IsError {
				t.Fatalf("change create failed: %+v", created)
			}
			path := filepath.Join(root, ".haft", "changes", id+".md")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			tagged := bytes.Replace(original, []byte("x-owner: abc"), []byte("x-owner: !vendor abc"), 1)
			if bytes.Equal(tagged, original) {
				t.Fatal("task tag fixture was not inserted")
			}
			if err := os.WriteFile(path, tagged, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := (store.Store{Root: root}).Read(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			empty := []change.Task{}
			refused := client.mustCall(t, profile.changeTool, app.Request{
				Format: delivery.Format, Operation: "change", Action: "archive", Ref: id,
				RequestID: "t01br4-mcp-archive", Revision: &change.Revision{ID: "chg-20260927-00000012", Reason: "Record completion", Tasks: &empty},
			})
			if refused.Kind != "conflict" || !refused.IsError {
				t.Fatalf("ignored task waived a retained tag: %+v", refused)
			}
			t01br4MCPDiagnostic(t, client, profile.readTool, refused, "unsupported_yaml_tag_conversion", "tasks[0].x-owner")
			after, err := (store.Store{Root: root}).Read(context.Background())
			if err != nil || after.Generation != before.Generation || !bytes.Equal(after.Files["changes/"+id+".md"], tagged) {
				t.Fatalf("refused archive changed predecessor: %v %+v", err, after)
			}
		})
	}
}

func TestT01BR4SameRefBindingGuardAcrossMCPProfiles(t *testing.T) {
	for _, profile := range t01br4MCPProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := t.TempDir()
			client := t01aStartClient(t, app.Service{Root: root}, profile.name)
			client.list(t)
			raw := "---\nformat: haft/1\nid: spec-20260927-00000011\nkind: spec\ntitle: Local binding basis\nstatus: proposed\norigin: agent_proposal\nabout: domain:Billing.Semantic\nslug: local-binding\nreceiving_use: Review one check binding\nclaims:\n  - id: rule\n    kind: definition\n    text: Preserve selected binding meaning\n    checks:\n      - ref: test:binding_test.go::TestRule\n        covers: Original coverage\n---\nBody.\n"
			seeded := client.mustCall(t, profile.writeTool, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br4-mcp-spec", Carrier: raw})
			if seeded.Kind != "written" || seeded.IsError {
				t.Fatalf("spec seed failed: %+v", seeded)
			}
			path := filepath.Join(root, ".haft", "specs", "spec-20260927-00000011.md")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			line := "covers: Original coverage"
			index := strings.Index(string(original), line)
			if index < 0 {
				t.Fatal("binding fixture was not found")
			}
			start := strings.LastIndex(string(original[:index]), "\n") + 1
			indent := string(original[start:index])
			tagged := strings.Replace(string(original), line, line+"\n"+indent+"x-owner: !vendor abc", 1)
			if err := os.WriteFile(path, []byte(tagged), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := (store.Store{Root: root}).Read(context.Background())
			if err != nil || len(before.Documents) != 1 || !before.Documents[0].Valid() {
				t.Fatalf("tagged source is not readable: %v %+v", err, before)
			}
			base := before.Documents[0]
			ref := base.Record.ID + "@" + base.Edition
			claim := base.Record.Claims[0]
			claim.Checks[0].Covers = "Revised coverage"
			id := "chg-20260927-00000013"
			c := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Clarify check", Intent: "Keep its extension", State: "open", CreatedAt: "2026-09-27T00:00:00Z", Patches: []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify coverage"}}}}}
			changeRaw, err := change.Encode(c, []byte("Rationale.\n"))
			if err != nil {
				t.Fatal(err)
			}
			created := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "create", RequestID: "t01br4-mcp-binding-create", Carrier: string(changeRaw), Snapshots: map[string][]byte{base.Edition: before.CurrentSnapshots[base.Edition]}})
			if created.Kind != "written" || created.IsError {
				t.Fatalf("binding change create failed: %+v", created)
			}
			preApply, err := (store.Store{Root: root}).Read(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			preview := client.mustCall(t, profile.changeTool, app.Request{Format: delivery.Format, Operation: "change", Action: "preview", Ref: id})
			if preview.Kind != "conflict" || !preview.IsError {
				t.Fatalf("preview waived a retained binding tag: %+v", preview)
			}
			t01br4MCPDiagnostic(t, client, profile.readTool, preview, "unsupported_yaml_tag_conversion", "claims[0].checks[0].x-owner")
			for _, action := range []string{"apply", "sync"} {
				refused := client.mustCall(t, profile.changeTool, app.Request{
					Format: delivery.Format, Operation: "change", Action: action, Ref: id,
					RequestID: "t01br4-mcp-" + action, ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"],
				})
				if refused.Kind != "conflict" || !refused.IsError || delivery.Size(refused) > delivery.Budget {
					t.Fatalf("%s published retained binding tag: %+v", action, refused)
				}
			}
			after, err := (store.Store{Root: root}).Read(context.Background())
			if err != nil || after.Generation != preApply.Generation || !bytes.Equal(after.Documents[0].Raw, []byte(tagged)) {
				t.Fatalf("refused preview/apply/sync changed predecessor: %v %+v", err, after)
			}
		})
	}
}
