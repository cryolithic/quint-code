package transport

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

func TestT01BR3SemanticRefusalIsReadableAcrossMCPProfiles(t *testing.T) {
	root := t.TempDir()
	service := app.Service{Root: root}
	defaultClient := t01aStartClient(t, service, ProfileDefault)
	legacyClient := t01aStartClient(t, service, ProfileLegacy)
	defaultClient.list(t)
	legacyClient.list(t)
	seed := defaultClient.mustCall(t, "haft_write", app.Request{
		Format: delivery.Format, Operation: "remember", RequestID: "t01br3-mcp-seed",
		Carrier: "---\nformat: haft/1\nid: spec-20260925-0150a001\nkind: spec\ntitle: Historic semantic rule\nstatus: proposed\norigin: agent_proposal\nabout: domain:Billing.Semantic\nslug: semantic-rule\nreceiving_use: Reauthor one exact edition\nclaims:\n  - id: L1\n    kind: definition\n    text: Preserve authored values\n---\nHistorical body.\n",
	})
	if seed.Kind != "written" || seed.IsError {
		t.Fatalf("seed v1 spec: %+v", seed)
	}
	view, err := (store.Store{Root: root}).Read(context.Background())
	if err != nil || len(view.Documents) != 1 {
		t.Fatalf("read seeded spec: %v %+v", err, view)
	}
	old := view.Documents[0]
	var tags strings.Builder
	for index := 0; index < 24; index++ {
		fmt.Fprintf(&tags, "x-tag-%02d: !currency %d\n", index, index)
	}
	modified := strings.Replace(string(old.Raw), "claims:\n", tags.String()+"claims:\n", 1)
	if modified == string(old.Raw) {
		t.Fatal("tag fixture did not modify the v1 spec")
	}
	path := filepath.Join(root, ".haft", "specs", old.Record.ID+".md")
	if err := os.WriteFile(path, []byte(modified), 0600); err != nil {
		t.Fatal(err)
	}
	view, err = (store.Store{Root: root}).Read(context.Background())
	if err != nil || len(view.Documents) != 1 || !view.Documents[0].Valid() {
		t.Fatalf("tagged predecessor is not readable: %v %+v", err, view)
	}
	old = view.Documents[0]
	ref := old.Record.ID + "@" + old.Edition
	for _, profile := range []struct {
		name   string
		client *t01aProtocolClient
		tool   string
		read   string
	}{
		{"default", defaultClient, "haft_change", "haft_read"},
		{"legacy", legacyClient, "haft", "haft"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			preview := profile.client.mustCall(t, profile.tool, app.Request{
				Format: delivery.Format, Operation: "change", Action: "reauthor_preview", Ref: ref,
			})
			if preview.Kind != "conflict" || !preview.IsError || preview.Delivery.ReadTool != profile.read || preview.Delivery.Catalog == nil {
				t.Fatalf("unsupported conversion lost public route: %+v", preview)
			}
			parts := t01aParts(t, profile.client, preview)
			if _, found := parts["diagnostics"]; !found {
				t.Fatal("returned parts omit diagnostics")
			}
			if _, found := parts["old"]; !found {
				t.Fatal("returned parts omit original carrier")
			}
			page := t01aRead(t, profile.client, profile.read, delivery.Request{
				Format: delivery.Format, Operation: "read", Ref: preview.Delivery.Catalog.Ref,
				View: "detail", Part: "diagnostics/23", ExpectedGeneration: preview.Basis["memory_generation"],
			})
			raw, err := json.Marshal(page.Data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte("x-tag-23")) || !bytes.Contains(raw, []byte("unsupported_yaml_tag_conversion")) {
				t.Fatalf("exact path/code unavailable through MCP detail: %s", raw)
			}
			oldRequest := delivery.Request{
				Format: delivery.Format, Operation: "read", Ref: preview.Delivery.Catalog.Ref,
				View: "bytes", Part: "old/0", ExpectedGeneration: preview.Basis["memory_generation"],
			}
			var recovered []byte
			for pageNumber := 0; pageNumber < 10; pageNumber++ {
				page := t01aRead(t, profile.client, profile.read, oldRequest)
				data, ok := page.Data.(map[string]any)
				if !ok {
					t.Fatalf("old byte page has wrong shape: %+v", page)
				}
				encoded, ok := data["bytes_base64"].(string)
				if !ok {
					t.Fatalf("old byte page lacks content: %+v", page)
				}
				fragment, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					t.Fatal(err)
				}
				recovered = append(recovered, fragment...)
				if page.Delivery.Next == nil {
					break
				}
				oldRequest = *page.Delivery.Next
			}
			if !bytes.Equal(recovered, old.Raw) {
				t.Fatal("returned original carrier bytes differ from predecessor")
			}
			refused := profile.client.mustCall(t, profile.tool, app.Request{
				Format: delivery.Format, Operation: "change", Action: "reauthor_apply", Ref: ref,
				RequestID:          "t01br3-apply-" + profile.name,
				ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"],
			})
			if refused.Kind != "conflict" || !refused.IsError {
				t.Fatalf("unsupported conversion was published: %+v", refused)
			}
		})
	}
	after, err := (store.Store{Root: root}).Read(context.Background())
	if err != nil || len(after.Documents) != 1 || after.Generation != view.Generation || !bytes.Equal(after.Documents[0].Raw, old.Raw) {
		t.Fatalf("refused MCP calls changed predecessor or generation: %v %+v", err, after)
	}
}

func TestT01BR3ChangeCreateRejectsSuppliedTagAcrossMCPProfiles(t *testing.T) {
	root := t.TempDir()
	service := app.Service{Root: root}
	defaultClient := t01aStartClient(t, service, ProfileDefault)
	legacyClient := t01aStartClient(t, service, ProfileLegacy)
	defaultClient.list(t)
	legacyClient.list(t)
	seed := defaultClient.mustCall(t, "haft_write", app.Request{
		Format: delivery.Format, Operation: "remember", RequestID: "t01br3-create-base",
		Carrier: "---\nformat: haft/1\nid: spec-20260925-0150a002\nkind: spec\ntitle: Change base\nstatus: proposed\norigin: agent_proposal\nabout: domain:Billing.Change\nslug: change-base\nreceiving_use: Test one supplied claim\nclaims:\n  - id: L1\n    kind: definition\n    text: Base meaning\n---\n",
	})
	if seed.Kind != "written" || seed.IsError {
		t.Fatalf("seed change base: %+v", seed)
	}
	before, err := (store.Store{Root: root}).Read(context.Background())
	if err != nil || len(before.Documents) != 1 {
		t.Fatalf("read change base: %v %+v", err, before)
	}
	base := before.Documents[0].Record.ID + "@" + before.Documents[0].Edition
	changeText := fmt.Sprintf("---\nformat: haft.change/1\nid: chg-20260925-0150a001\nchange_key: chg-20260925-0150a001\ntitle: Preserve supplied claim\nintent: Add one definition\nstate: open\ncreated_at: 2026-09-25T00:00:00Z\npatches:\n  - base: %s\n    operations:\n      - op: ADDED\n        claim:\n          id: L2\n          kind: definition\n          text: Added meaning\n          x-unit: !currency 12\n---\nAuthored change prose.\n", base)
	for _, profile := range []struct {
		name   string
		client *t01aProtocolClient
		tool   string
	}{
		{"default", defaultClient, "haft_change"},
		{"legacy", legacyClient, "haft"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			refused := profile.client.mustCall(t, profile.tool, app.Request{
				Format: delivery.Format, Operation: "change", Action: "create",
				RequestID: "t01br3-create-tag-" + profile.name, Carrier: changeText,
			})
			if refused.Kind != "invalid" || !refused.IsError {
				t.Fatalf("supplied custom tag was admitted: %+v", refused)
			}
			found := false
			for _, diagnostic := range refused.Diagnostics {
				found = found || diagnostic.Code == "unsupported_yaml_tag_conversion" && diagnostic.Path == "patches[0].operations[0].claim.x-unit"
			}
			if !found {
				t.Fatalf("exact supplied-value diagnostic missing: %+v", refused.Diagnostics)
			}
			after, err := (store.Store{Root: root}).Read(context.Background())
			if err != nil || after.Generation != before.Generation || !reflect.DeepEqual(after.Files, before.Files) {
				t.Fatalf("refused change/create wrote project memory: %v %+v", err, after)
			}
		})
	}
}
