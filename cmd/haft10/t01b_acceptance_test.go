package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

const t01bNewSpec = `---
format: haft/2
kind: spec
title: Cancellation amount
about: domain:Billing.OrderCancellation
slug: cancellation-amount
receiving_use: Review a bounded cancellation change
claims:
  - id: total-preserved
    kind: law
    text: Cancellation preserves the amount
    unchecked: A runtime check has not been run
---
This is current authored content, not an accepted choice.
`

const t01bOldSpec = `---
format: haft/1
id: spec-20260925-aabbccdd
kind: spec
title: Historic cancellation amount
status: proposed
origin: agent_proposal
about: domain:Billing.OrderCancellation
created_at: 2026-09-25T10:00:00Z
slug: historic-cancellation
receiving_use: Review a bounded cancellation change
claims:
  - id: total-preserved
    kind: law
    text: Cancellation preserves the amount
    unchecked: A runtime check has not been run
    implemented_by:
      - ref: file:order.go
        covers: Direct domain operation
    checks:
      - ref: test:order_test.go::TestOrder
        covers: Declared check only; no run
    x-claim: {nested: [one, {two: three}]}
x-record: {owner: Ivan, values: [1, 2]}
---
Preserve this exact historical body and its **formatting**.
`

func t01bAPI(t *testing.T, root string, request map[string]any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	code, response, stderr := invoke(t, []string{"api", "--root", root, "--input", "-"}, string(raw))
	if stderr != "" {
		t.Fatalf("unexpected CLI stderr: %q", stderr)
	}
	return code, response
}

func t01bRequest(operation, action string) map[string]any {
	request := map[string]any{"format": "haft.api/2", "operation": operation}
	if action != "" {
		request["action"] = action
	}
	return request
}

func t01bWritten(t *testing.T, code int, response map[string]any) {
	t.Helper()
	if code != 0 || response["result_kind"] != "written" || response["is_error"] != false {
		t.Fatalf("write failed: exit=%d response=%+v", code, response)
	}
}

func t01bDiagnostic(response map[string]any, code string) bool {
	items, _ := response["diagnostics"].([]any)
	for _, item := range items {
		entry, _ := item.(map[string]any)
		if entry["code"] == code {
			return true
		}
	}
	return false
}

func t01bBasis(t *testing.T, response map[string]any, key string) string {
	t.Helper()
	basis, ok := response["basis"].(map[string]any)
	if !ok {
		t.Fatalf("missing result basis: %+v", response)
	}
	value, ok := basis[key].(string)
	if !ok || value == "" {
		t.Fatalf("missing basis %q: %+v", key, basis)
	}
	return value
}

func t01bData(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing response data: %+v", response)
	}
	return data
}

func t01bSpecPaths(t *testing.T, root string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, ".haft", "specs", "spec-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestT01BCLINewSpecHasNoAuthoringStatusOrDecisionAuthority(t *testing.T) {
	root := t.TempDir()
	request := t01bRequest("remember", "")
	request["request_id"] = "t01b-new-spec"
	request["carrier"] = t01bNewSpec
	code, result := t01bAPI(t, root, request)
	t01bWritten(t, code, result)
	paths := t01bSpecPaths(t, root)
	if len(paths) != 1 {
		t.Fatalf("one spec should be published: %v", paths)
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	doc := carrier.Parse(raw)
	if !doc.Valid() || doc.Record.Format != "haft/2" || doc.Record.Origin != "agent_edit" || doc.Record.Status != "" || doc.Record.OperatorConfirmed || doc.Record.LegacyStatus != "" {
		t.Fatalf("v2 authored content manufactured authority: %+v", doc)
	}
	recall := t01bRequest("recall", "")
	recall["ref"] = doc.Record.ID
	code, result = t01bAPI(t, root, recall)
	if code != 0 || result["result_kind"] != "found" || t01bData(t, result)["exact_ref"] == "" {
		t.Fatalf("new v2 spec is not addressable: exit=%d result=%+v", code, result)
	}

	for _, test := range []struct {
		name, carrier, diagnostic string
	}{
		{"empty status presence", strings.Replace(t01bNewSpec, "kind: spec\n", "kind: spec\nstatus: ''\n", 1), "invalid_carrier"},
		{"false confirmation presence", strings.Replace(t01bNewSpec, "kind: spec\n", "kind: spec\noperator_confirmed: false\n", 1), "invalid_carrier"},
		{"empty legacy status presence", strings.Replace(t01bNewSpec, "kind: spec\n", "kind: spec\nlegacy_status: ''\n", 1), "invalid_carrier"},
		{"unsupported record format", strings.Replace(t01bNewSpec, "haft/2", "haft/3", 1), "unsupported_format"},
		{"unsupported v2 decision", strings.Replace(t01bNewSpec, "kind: spec", "kind: decision", 1), "unsupported_kind_format"},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := t01bRequest("remember", "")
			bad["request_id"] = "t01b-negative-" + test.name
			bad["carrier"] = test.carrier
			code, result := t01bAPI(t, root, bad)
			if code != 1 || !t01bDiagnostic(result, test.diagnostic) {
				t.Fatalf("unsupported authoring accepted or wrong diagnostic: exit=%d response=%+v", code, result)
			}
		})
	}
	if len(t01bSpecPaths(t, root)) != 1 {
		t.Fatal("invalid formats or authority fields published a record")
	}

	decision := t01bRequest("remember", "")
	decision["request_id"] = "t01b-unapproved-decision"
	decision["carrier"] = `---
format: haft/1
id: dec-20260925-aabbccdd
kind: decision
title: Choose the cancellation boundary
status: active
origin: agent_proposal
operator_confirmed: false
about: domain:Billing.OrderCancellation
created_at: 2026-09-25T10:00:00Z
object: Cancellation
question: Which boundary is selected?
disposition: choose_now
chosen: direct-domain
why: Bound the direct operation
no_alternative_reason: No other option is currently admitted
---
Agent proposal is not an operator request.
`
	code, result = t01bAPI(t, root, decision)
	if code != 1 || !t01bDiagnostic(result, "operator_confirmation_required") {
		t.Fatalf("unapproved decision bound: exit=%d response=%+v", code, result)
	}
	if _, err := os.Stat(filepath.Join(root, ".haft", "decisions", "dec-20260925-aabbccdd.md")); !os.IsNotExist(err) {
		t.Fatalf("unapproved decision was published: %v", err)
	}
}

func TestT01BCLISelectedReauthorUsesPreviewCASAndPreservesHistory(t *testing.T) {
	root := t.TempDir()
	seed := t01bRequest("remember", "")
	seed["request_id"] = "t01b-seed-v1"
	seed["carrier"] = t01bOldSpec
	code, result := t01bAPI(t, root, seed)
	t01bWritten(t, code, result)
	oldPath := filepath.Join(root, ".haft", "specs", "spec-20260925-aabbccdd.md")
	oldBytes, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	recall := t01bRequest("recall", "")
	recall["ref"] = "spec-20260925-aabbccdd"
	code, result = t01bAPI(t, root, recall)
	if code != 0 || result["result_kind"] != "found" {
		t.Fatalf("old spec unavailable: %d %+v", code, result)
	}
	oldRef, ok := t01bData(t, result)["exact_ref"].(string)
	if !ok || !strings.HasPrefix(oldRef, "spec-20260925-aabbccdd@sha256:") {
		t.Fatalf("old exact ref absent: %+v", result)
	}
	oldDigest := strings.TrimPrefix(oldRef, "spec-20260925-aabbccdd@sha256:")
	snapshotPath := filepath.Join(root, ".haft", "editions", "sha256", oldDigest+".json")
	oldSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}

	preview := t01bRequest("change", "reauthor_preview")
	preview["ref"] = oldRef
	code, ready := t01bAPI(t, root, preview)
	if code != 0 || ready["result_kind"] != "ready" {
		t.Fatalf("selected preview failed: %d %+v", code, ready)
	}
	firstDigest := t01bBasis(t, ready, "preview_digest")
	firstGeneration := t01bBasis(t, ready, "memory_generation")
	oldDetail := t01bRequest("change", "reauthor_preview")
	oldDetail["ref"] = oldRef
	oldDetail["view"] = "detail"
	oldDetail["part"] = "old"
	code, oldPreview := t01bAPI(t, root, oldDetail)
	oldPreviewData := t01bData(t, oldPreview)
	if code != 0 || oldPreviewData["carrier"] != string(oldBytes) || oldPreviewData["status"] != "proposed" || oldPreviewData["origin"] != "agent_proposal" {
		t.Fatalf("preview hid selected bytes or historic provenance: %d %+v", code, oldPreview)
	}
	newDetail := t01bRequest("change", "reauthor_preview")
	newDetail["ref"] = oldRef
	newDetail["view"] = "detail"
	newDetail["part"] = "new_content"
	code, newPreview := t01bAPI(t, root, newDetail)
	newPreviewData := t01bData(t, newPreview)
	prototype, ok := newPreviewData["prototype_carrier"].(string)
	if code != 0 || !ok || !strings.Contains(prototype, "x-record:") || !strings.Contains(prototype, "x-claim:") || newPreviewData["id_created_at_receipt"] != "allocated only by apply" {
		t.Fatalf("preview dropped extensions or allocated write metadata: %d %+v", code, newPreview)
	}
	prototypeDoc := carrier.Parse([]byte(prototype))
	if prototypeDoc.Record.ID != "" || prototypeDoc.Record.CreatedAt != "" || prototypeDoc.Record.WriteReceipt != nil || !bytes.Equal(prototypeDoc.Body, carrier.Parse(oldBytes).Body) {
		t.Fatalf("preview allocated ID/time/receipt or changed body: %+v", prototypeDoc)
	}
	code, repeated := t01bAPI(t, root, preview)
	if code != 0 || repeated["result_kind"] != "ready" || t01bBasis(t, repeated, "preview_digest") != firstDigest || t01bBasis(t, repeated, "memory_generation") != firstGeneration {
		t.Fatalf("preview changed without an authored write: %d %+v", code, repeated)
	}
	if len(t01bSpecPaths(t, root)) != 1 {
		t.Fatal("preview published a spec")
	}
	unpinned := t01bRequest("change", "reauthor_preview")
	unpinned["ref"] = "spec-20260925-aabbccdd"
	code, rejected := t01bAPI(t, root, unpinned)
	if code == 0 || rejected["result_kind"] == "ready" {
		t.Fatalf("unpinned conversion accepted: %d %+v", code, rejected)
	}

	apply := t01bRequest("change", "reauthor_apply")
	apply["ref"] = oldRef
	apply["request_id"] = "t01b-reauthor"
	apply["preview_digest"] = firstDigest
	apply["expected_generation"] = firstGeneration
	advance := t01bRequest("remember", "")
	advance["request_id"] = "t01b-generation-advance"
	advance["carrier"] = "---\nkind: note\ntitle: Unrelated write\nabout: domain:Billing.OrderCancellation\n---\nAdvance generation.\n"
	code, result = t01bAPI(t, root, advance)
	t01bWritten(t, code, result)
	code, stale := t01bAPI(t, root, apply)
	if code != 1 || stale["result_kind"] != "conflict" || !t01bDiagnostic(stale, "concurrent_write") || len(t01bSpecPaths(t, root)) != 1 {
		t.Fatalf("stale generation did not reject before publication: %d %+v", code, stale)
	}
	code, ready = t01bAPI(t, root, preview)
	if code != 0 || ready["result_kind"] != "ready" {
		t.Fatalf("fresh preview failed: %d %+v", code, ready)
	}
	apply["preview_digest"] = t01bBasis(t, ready, "preview_digest")
	apply["expected_generation"] = t01bBasis(t, ready, "memory_generation")
	wrongDigest := make(map[string]any, len(apply))
	for key, value := range apply {
		wrongDigest[key] = value
	}
	wrongDigest["request_id"] = "t01b-wrong-preview"
	wrongDigest["preview_digest"] = "sha256:" + strings.Repeat("0", 64)
	code, rejected = t01bAPI(t, root, wrongDigest)
	if code != 1 || rejected["result_kind"] != "conflict" || !t01bDiagnostic(rejected, "preview_changed") || len(t01bSpecPaths(t, root)) != 1 {
		t.Fatalf("wrong preview digest published: %d %+v", code, rejected)
	}
	code, result = t01bAPI(t, root, apply)
	t01bWritten(t, code, result)
	code, replay := t01bAPI(t, root, apply)
	if code != 0 || replay["result_kind"] != "replayed" {
		t.Fatalf("same request did not replay: %d %+v", code, replay)
	}
	changed := make(map[string]any, len(apply))
	for key, value := range apply {
		changed[key] = value
	}
	changed["preview_digest"] = "sha256:" + strings.Repeat("0", 64)
	code, conflict := t01bAPI(t, root, changed)
	if code != 1 || conflict["result_kind"] != "request_conflict" {
		t.Fatalf("changed same-ID payload replayed: %d %+v", code, conflict)
	}

	paths := t01bSpecPaths(t, root)
	if len(paths) != 2 {
		t.Fatalf("reauthor published %d specs, want one successor: %v", len(paths), paths)
	}
	var successor carrier.Document
	for _, path := range paths {
		if path == oldPath {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		successor = carrier.Parse(raw)
	}
	old := carrier.Parse(oldBytes)
	if !successor.Valid() || successor.Record.Format != "haft/2" || successor.Record.Status != "" || successor.Record.OperatorConfirmed || successor.Record.Origin != "agent_edit" || !reflect.DeepEqual(successor.Record.Supersedes, []string{oldRef}) || successor.Record.WriteReceipt == nil {
		t.Fatalf("new edition misstates current content or provenance: %+v", successor)
	}
	if successor.Record.ID == old.Record.ID || successor.Record.Claims[0].ID != old.Record.Claims[0].ID || !reflect.DeepEqual(successor.Record.Claims[0].Extra, old.Record.Claims[0].Extra) || !reflect.DeepEqual(successor.Record.Extra, old.Record.Extra) || !bytes.Equal(successor.Body, old.Body) || !reflect.DeepEqual(successor.Record.Claims[0].ImplementedBy, old.Record.Claims[0].ImplementedBy) || !reflect.DeepEqual(successor.Record.Claims[0].Checks, old.Record.Claims[0].Checks) {
		t.Fatalf("reauthor lost selected v1 content: old=%+v new=%+v", old, successor)
	}
	currentOldBytes, err := os.ReadFile(oldPath)
	if err != nil || !bytes.Equal(currentOldBytes, oldBytes) {
		t.Fatalf("old carrier bytes changed: %v", err)
	}
	currentSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil || !bytes.Equal(currentSnapshot, oldSnapshot) {
		t.Fatalf("old snapshot bytes changed: %v", err)
	}
	recall["ref"] = oldRef
	code, historical := t01bAPI(t, root, recall)
	if code != 0 || historical["result_kind"] != "found" || t01bData(t, historical)["status"] != "proposed" || t01bData(t, historical)["origin"] != "agent_proposal" {
		t.Fatalf("old exact interpretation changed: %d %+v", code, historical)
	}
}
