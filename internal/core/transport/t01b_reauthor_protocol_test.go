package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

func t01brPublishedRoute(t *testing.T, result delivery.Response) (string, app.Request) {
	t.Helper()
	if result.IsError || delivery.Size(result) > delivery.Budget {
		t.Fatalf("reauthor receipt is an error or exceeds the public envelope: %+v", result)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("reauthor receipt has no public summary: %+v", result)
	}
	ref, ok := data["published_successor_ref"].(string)
	if !ok || ref == "" {
		t.Fatalf("reauthor receipt omitted exact published successor identity: %+v", data)
	}
	if _, advertised := data["new_ref"]; advertised {
		t.Fatalf("bounded receipt advertised a field excluded from its documented summary: %+v", data)
	}
	content, ok := data["published_content"].(map[string]any)
	if !ok || content["state_at_publication"] != carrier.Current || content["live_currentness"] != "not_reassessed" {
		t.Fatalf("receipt misstates publication and live currentness: %+v", content)
	}
	authority, ok := data["decision_authority"].(map[string]any)
	if !ok || authority["assessment"] != "not_assessed" {
		t.Fatalf("receipt promoted content publication to decision authority: %+v", authority)
	}
	raw, err := json.Marshal(data["exact_read_request"])
	if err != nil {
		t.Fatal(err)
	}
	var route app.Request
	if err := json.Unmarshal(raw, &route); err != nil {
		t.Fatal(err)
	}
	if route.Format != delivery.Format || route.Operation != "recall" || route.Ref != ref || !strings.Contains(ref, "@sha256:") {
		t.Fatalf("receipt route is not executable against the exact successor: %+v", route)
	}
	return ref, route
}

func TestT01BSelectedReauthorLargePreviewAcrossCLIAndMCP(t *testing.T) {
	root := t.TempDir()
	service := app.Service{Root: root}
	defaultClient := t01aStartClient(t, service, ProfileDefault)
	legacyClient := t01aStartClient(t, service, ProfileLegacy)
	defaultClient.list(t)
	legacyClient.list(t)
	body := strings.Repeat("Historical specification rationale survives this exact edition.\n", 400) + "T01B_END_MARKER\n"
	carrierText := "---\nformat: haft/1\nid: spec-20260925-010b0001\nkind: spec\ntitle: Historic cancellation rule\nstatus: proposed\norigin: agent_proposal\nabout: domain:Billing.Cancel\nslug: cancel-rule\nreceiving_use: Explain a chosen v1 reauthor\nclaims:\n  - id: L1\n    kind: definition\n    text: Cancellation preserves the total\n    x-future: exact extension\n---\n" + body
	seed := defaultClient.mustCall(t, "haft_write", app.Request{
		Format: delivery.Format, Operation: "remember", RequestID: "t01b-large-v1-seed", Carrier: carrierText,
	})
	if seed.Kind != "written" || seed.IsError {
		t.Fatalf("seed v1 spec: %+v", seed)
	}
	v, err := (store.Store{Root: root}).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var pinned string
	var oldBytes []byte
	for _, document := range v.Documents {
		if document.Record.ID == "spec-20260925-010b0001" {
			pinned = document.Record.ID + "@" + document.Edition
			oldBytes = append([]byte(nil), document.Raw...)
		}
	}
	if pinned == "" {
		t.Fatal("seeded v1 spec not found")
	}
	request := app.Request{Format: delivery.Format, Operation: "change", Action: "reauthor_preview", Ref: pinned}
	var digest, generation string
	for _, profile := range []struct {
		client *t01aProtocolClient
		tool   string
		read   string
	}{{defaultClient, "haft_change", "haft_read"}, {legacyClient, "haft", "haft"}} {
		preview := profile.client.mustCall(t, profile.tool, request)
		if preview.Kind != "ready" || preview.IsError || preview.Delivery.ReadTool != profile.read {
			t.Fatalf("%s preview: %+v", profile.tool, preview)
		}
		if preview.Basis["preview_digest"] == "" || preview.Basis["memory_generation"] == "" {
			t.Fatalf("%s omitted CAS basis: %+v", profile.tool, preview.Basis)
		}
		if digest == "" {
			digest = preview.Basis["preview_digest"]
			generation = preview.Basis["memory_generation"]
		} else if preview.Basis["preview_digest"] != digest || preview.Basis["memory_generation"] != generation {
			t.Fatalf("%s preview changed with no content write", profile.tool)
		}
		parts := t01aParts(t, profile.client, preview)
		prototype, ok := parts["new_content"]
		if !ok {
			t.Fatalf("%s cannot progressively read the v2 prototype: %+v", profile.tool, parts)
		}
		full := t01aBytesPart(t, profile.client, profile.read, prototype)
		if !bytes.Contains(full, []byte("T01B_END_MARKER")) || !bytes.Contains(full, []byte("x-future")) {
			t.Fatalf("%s prototype lost body or extension", profile.tool)
		}
	}
	binary := t01arCLI(t)
	help, err := exec.Command(binary, "help").Output()
	if err != nil || !bytes.Contains(help, []byte("reauthor_preview")) || !bytes.Contains(help, []byte("reauthor_apply")) || !bytes.Contains(help, []byte("haft/2 spec writing publishes current content")) || !bytes.Contains(help, []byte("published_successor_ref")) || !bytes.Contains(help, []byte("exact_read_request")) {
		t.Fatalf("actual CLI help omits selected spec route: %v", err)
	}
	cliPreview := t01arCallCLI(t, binary, root, request)
	if cliPreview.Kind != "ready" || cliPreview.Basis["preview_digest"] != digest || cliPreview.Basis["memory_generation"] != generation {
		t.Fatalf("actual CLI preview differs from MCP: %+v", cliPreview)
	}
	apply := app.Request{Format: delivery.Format, Operation: "change", Action: "reauthor_apply", Ref: pinned,
		RequestID: "t01b-re-author-once", ExpectedGeneration: generation, PreviewDigest: digest}
	written := legacyClient.mustCall(t, "haft", apply)
	if written.Kind != "written" || written.IsError {
		t.Fatalf("legacy apply: %+v", written)
	}
	replayed := defaultClient.mustCall(t, "haft_change", apply)
	if replayed.Kind != "replayed" || replayed.IsError {
		t.Fatalf("same-ID cross-profile replay: %+v", replayed)
	}
	writtenRef, route := t01brPublishedRoute(t, written)
	replayedRef, replayRoute := t01brPublishedRoute(t, replayed)
	if replayedRef != writtenRef || replayRoute.Ref != route.Ref {
		t.Fatal("cross-profile replay changed the exact published successor route")
	}
	for _, surface := range []struct {
		name   string
		result delivery.Response
	}{{"default MCP", defaultClient.mustCall(t, "haft_read", route)}, {"legacy MCP", legacyClient.mustCall(t, "haft", route)}, {"CLI", t01arCallCLI(t, binary, service.Root, route)}} {
		if surface.result.Kind != "found" || surface.result.IsError || surface.result.Data.(map[string]any)["exact_ref"] != writtenRef {
			t.Fatalf("%s could not execute receipt route: %+v", surface.name, surface.result)
		}
	}
	cliReplay := t01arCallCLI(t, binary, service.Root, apply)
	cliRef, cliRoute := t01brPublishedRoute(t, cliReplay)
	if cliReplay.Kind != "replayed" || cliRef != writtenRef || cliRoute.Ref != route.Ref {
		t.Fatalf("CLI replay changed published identity: %+v", cliReplay)
	}
	var full app.Result
	t01arPart(t, defaultClient, replayed, "result", &full)
	fullData, ok := full.Data.(map[string]any)
	if !ok || fullData["published_successor_ref"] != writtenRef || fullData["new_ref"] != writtenRef {
		t.Fatalf("full replay metadata differs from summary: %+v", full.Data)
	}
	current, err := (store.Store{Root: root}).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var successor carrier.Document
	for _, document := range current.Documents {
		if document.Record.ID+"@"+document.Edition == writtenRef {
			successor = document
		}
	}
	if successor.Record.ID == "" {
		t.Fatal("published successor is absent from current owned project")
	}
	later := successor.Record
	later.ID = "spec-20260925-010b0003"
	later.Supersedes = []string{writtenRef}
	later.SupersedeReason = "Revise current content after the selected reauthor"
	later.WriteReceipt = nil
	later.Claims[0].Text += " Later content revision."
	laterBytes, err := carrier.Encode(later, successor.Body)
	if err != nil {
		t.Fatal(err)
	}
	laterWrite := defaultClient.mustCall(t, "haft_write", app.Request{Format: delivery.Format, Operation: "remember", RequestID: "t01br-later-v2", Carrier: string(laterBytes), ExpectedHeads: []string{writtenRef}})
	if laterWrite.Kind != "written" || laterWrite.IsError {
		t.Fatalf("later current-content successor was not published: %+v", laterWrite)
	}
	laterReplay := legacyClient.mustCall(t, "haft", apply)
	laterRef, laterRoute := t01brPublishedRoute(t, laterReplay)
	if laterReplay.Kind != "replayed" || laterRef != writtenRef || laterRoute.Ref != route.Ref {
		t.Fatalf("replay inferred a later live head rather than original output: %+v", laterReplay)
	}
	oldPath := filepath.Join(root, ".haft", "specs", "spec-20260925-010b0001.md")
	currentOld, err := os.ReadFile(oldPath)
	if err != nil || !bytes.Equal(currentOld, oldBytes) {
		t.Fatalf("reauthor changed predecessor bytes: %v", err)
	}
	stale := apply
	stale.RequestID = "t01b-re-author-stale"
	rejected := defaultClient.mustCall(t, "haft_change", stale)
	if rejected.Kind != "conflict" || !rejected.IsError {
		t.Fatalf("stale CAS was not rejected: %+v", rejected)
	}
}
