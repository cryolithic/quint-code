package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
)

func TestT01BRSuccessiveV2ChangePreviewApplyKeepsOneHead(t *testing.T) {
	s := service(t)
	ids := []string{"spec-20260925-00000003", "spec-20260925-00000002", "spec-20260925-00000001"}
	s.NewID = func(kind string) (string, error) {
		if kind != "spec" || len(ids) == 0 {
			t.Fatalf("unexpected generated ID request: %s", kind)
		}
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	_, oldRef := seedT01BActiveSpec(t, s)
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: oldRef}, "ready")
	apply := Request{Operation: "change", Action: "reauthor_apply", Ref: oldRef, RequestID: "sequential-reauthor", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	run(t, s, apply, "written")
	for step := 1; step <= 2; step++ {
		view := readView(t, s)
		head := view.Projection.Resolve("spec:order-cancel")
		if head.Kind != "found" || head.Document == nil {
			t.Fatalf("step %d had no sole current content: %+v", step, head)
		}
		base := head.Document.Record.ID + "@" + head.Document.Edition
		claim := head.Document.Record.Claims[0]
		claim.Text += " Sequential edit."
		id := fmt.Sprintf("chg-20260925-%08d", step)
		patch := change.SectionPatch{Base: base, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Refine current content"}}}
		candidate := change.Change{Format: change.Format, ID: id, ChangeKey: id, Title: "Refine current content", Intent: "Continue the exact content lineage", State: "open", CreatedAt: s.now(), Patches: []change.SectionPatch{patch}}
		raw, err := change.Encode(candidate, nil)
		if err != nil {
			t.Fatal(err)
		}
		run(t, s, Request{Operation: "change", Action: "create", RequestID: "sequential-create-" + id, Carrier: string(raw)}, "written")
		prepared := run(t, s, Request{Operation: "change", Action: "preview", Ref: id}, "ready")
		publication := Request{Operation: "change", Action: "apply", Ref: prepared.Basis["change_ref"], RequestID: "sequential-apply-" + id, ExpectedGeneration: prepared.Basis["memory_generation"], PreviewDigest: prepared.Basis["preview_digest"]}
		run(t, s, publication, "written")
		view = readView(t, s)
		head = view.Projection.Resolve("spec:order-cancel")
		if head.Kind != "found" || head.Document == nil || head.Document.Record.Format != "haft/2" {
			t.Fatalf("step %d did not retain sole current v2 content: %+v", step, head)
		}
		if len(view.Projection.Heads(head.Document.Record.ID)) != 1 {
			t.Fatalf("step %d left competing content heads", step)
		}
	}
	if len(ids) != 0 {
		t.Fatalf("unused descending successor IDs: %v", ids)
	}
}

func TestT01BRMixedFormatHeadsDegradeStructuralResult(t *testing.T) {
	s := service(t)
	s.NewID = func(kind string) (string, error) {
		return kind + "-20260925-00000002", nil
	}
	old, oldRef := seedT01BActiveSpec(t, s)
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: oldRef}, "ready")
	apply := Request{Operation: "change", Action: "reauthor_apply", Ref: oldRef, RequestID: "mixed-branch-reauthor", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	run(t, s, apply, "written")
	run(t, s, Request{Operation: "check", Action: "structural"}, "structurally_valid")

	branch := old.Record
	branch.ID = "spec-20260925-00000003"
	branch.Supersedes = []string{oldRef}
	branch.SupersedeReason = "Independently accepted branch"
	raw := encode(t, branch, old.Body)
	path := filepath.Join(s.Root, ".haft", "specs", branch.ID+".md")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	v := readView(t, s)
	if v.Coverage != "degraded" {
		t.Fatalf("mixed heads coverage = %s", v.Coverage)
	}
	var activeRef, contentRef string
	for _, e := range v.Projection.Entries {
		if e.State == carrier.Contested && e.Document.Record.Format == "haft/1" {
			activeRef = e.Ref
		}
		if e.State == carrier.ContentContested && e.Document.Record.Format == "haft/2" {
			contentRef = e.Ref
		}
	}
	if activeRef == "" || contentRef == "" {
		t.Fatalf("mixed heads were not projected distinctly: %+v", v.Projection.Entries)
	}
	// The separately authored branch also needs a durable exact snapshot for a
	// pinned structural check; an unpersisted pin must remain unresolved.
	branchRef, err := carrier.ParseRef(activeRef)
	if err != nil {
		t.Fatal(err)
	}
	branchSnapshot, ok := v.CurrentSnapshots[branchRef.Digest]
	if !ok {
		t.Fatal("branch has no captured exact snapshot")
	}
	snapshotPath := filepath.Join(s.Root, ".haft", "editions", "sha256", strings.TrimPrefix(branchRef.Digest, "sha256:")+".json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, branchSnapshot, 0600); err != nil {
		t.Fatal(err)
	}
	v = readView(t, s)
	r := run(t, s, Request{Operation: "check", Action: "structural"}, "conflict")
	if r.Coverage != "degraded" || !hasAppDiagnostic(r, "mixed_format_heads") {
		t.Fatalf("structural result hid mixed heads: %+v", r)
	}
	wantRefs := v.Projection.Heads(old.Record.ID)
	if refs := r.Data.(map[string]any)["head_refs"].([]string); !reflect.DeepEqual(refs, wantRefs) {
		t.Fatalf("whole-project structural result omitted typed mixed heads: %v", refs)
	}
	message := ""
	for _, d := range r.Diagnostics {
		if d.Code == "mixed_format_heads" {
			message = d.Message
		}
	}
	if !strings.Contains(message, activeRef) || !strings.Contains(message, contentRef) || !strings.Contains(message, "separate authorized repair") {
		t.Fatalf("structural result lacks exact participants or repair boundary: %+v", r.Diagnostics)
	}
	for _, ref := range wantRefs {
		pinned := run(t, s, Request{Operation: "check", Action: "structural", Ref: ref}, "conflict")
		data, ok := pinned.Data.(map[string]any)
		if !ok {
			t.Fatalf("pinned structural result %s has no typed metadata: %+v", ref, pinned)
		}
		if pinned.Coverage != "degraded" || !reflect.DeepEqual(data["head_refs"], wantRefs) {
			t.Fatalf("pinned structural result %s omitted mixed participants: %+v", ref, pinned)
		}
	}
	alias := run(t, s, Request{Operation: "check", Action: "structural", Ref: "spec:order-cancel"}, "conflict")
	metadata, ok := alias.Data.(map[string]any)
	if !ok || metadata["repair_support"] != "unsupported_mixed_format_merge" {
		t.Fatalf("alias check did not explain unsupported mixed repair: %+v", alias)
	}
	refs := metadata["head_refs"].([]string)
	if !reflect.DeepEqual(refs, wantRefs) || !hasAppDiagnostic(alias, "mixed_format_heads") {
		t.Fatalf("alias check omitted exact mixed heads: %+v", alias)
	}
	for _, target := range append([]string{"", "spec:order-cancel"}, wantRefs...) {
		response := public(t, s, Request{Operation: "check", Action: "structural", Ref: target})
		if response.Kind != "conflict" || response.Coverage != "degraded" {
			t.Fatalf("public structural result for %q hid mixed conflict: %+v", target, response)
		}
		metadata, ok := response.Data.(map[string]any)
		if !ok || metadata["repair_support"] != "unsupported_mixed_format_merge" {
			t.Fatalf("public structural summary for %q lacks repair metadata: %+v", target, response.Data)
		}
		rawRefs, err := json.Marshal(metadata["head_refs"])
		if err != nil {
			t.Fatal(err)
		}
		var publicRefs []string
		if err := json.Unmarshal(rawRefs, &publicRefs); err != nil || !reflect.DeepEqual(publicRefs, wantRefs) {
			t.Fatalf("public structural summary for %q omitted typed participants: %v, %v", target, metadata["head_refs"], err)
		}
	}
}
