package transport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01AREffectHintsAndDisposableIO(t *testing.T) {
	root := t.TempDir()
	service := app.Service{Root: root, SourceRoot: "../source/testdata/pin-a", SourceRepository: "fixture://t01ar"}
	defaultClient := t01aStartClient(t, service, ProfileDefault)
	legacyClient := t01aStartClient(t, service, ProfileLegacy)
	for _, profile := range []struct {
		client *t01aProtocolClient
		want   map[string]bool
	}{
		{defaultClient, map[string]bool{"haft_read": false, "haft_write": true, "haft_change": true, "haft_check": true, "haft_fpf": false}},
		{legacyClient, map[string]bool{"haft": true}},
	} {
		for _, tool := range profile.client.list(t) {
			mayChangeProject, found := profile.want[tool.Name]
			if !found {
				t.Fatalf("unexpected advertised tool %s", tool.Name)
			}
			annotations := tool.Annotations
			if annotations["readOnlyHint"] != false || annotations["destructiveHint"] != mayChangeProject || annotations["idempotentHint"] != false || annotations["openWorldHint"] != false {
				t.Fatalf("%s hides disposable IO or possible project-test effects: %+v", tool.Name, annotations)
			}
			for _, detail := range []string{".runtime/writer.lock", ".cache/disclosure", "read-only root"} {
				if !strings.Contains(tool.Description, detail) {
					t.Fatalf("%s description omits %s", tool.Name, detail)
				}
			}
		}
	}

	// Source status is an application read. Its delivery nevertheless creates
	// the shared lock and a disposable result, never an authored project record.
	status := defaultClient.mustCall(t, "haft_fpf", app.Request{Format: delivery.Format, Operation: "fpf", Action: "status"})
	if status.IsError || status.Kind != "available" || status.Delivery.Catalog == nil {
		t.Fatalf("source status did not produce a readable result: %+v", status)
	}
	lock := filepath.Join(root, ".haft", ".runtime", "writer.lock")
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("read did not create the shared lock: %v", err)
	}
	cache, err := filepath.Glob(filepath.Join(root, ".haft", ".cache", "disclosure", "*.json"))
	if err != nil || len(cache) == 0 {
		t.Fatalf("read did not create a disposable result: %v / %v", cache, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".haft"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != ".runtime" && entry.Name() != ".cache" {
			t.Fatalf("application read published %s", entry.Name())
		}
	}

	// A read-only lock is one concrete read-only-root failure: the application
	// can classify pinned source, but it must disclose that continuation is
	// unavailable instead of promising a readable transient result.
	if err := os.Chmod(lock, 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lock, 0600) })
	probe, err := os.OpenFile(lock, os.O_RDWR, 0)
	if err == nil {
		_ = probe.Close()
		t.Skip("current process can still write a read-only lock")
	}
	for _, profile := range []struct {
		client *t01aProtocolClient
		tool   string
	}{
		{defaultClient, "haft_fpf"},
		{legacyClient, "haft"},
	} {
		result := profile.client.mustCall(t, profile.tool, app.Request{Format: delivery.Format, Operation: "fpf", Action: "status"})
		if result.Kind != "available" || result.Delivery.Catalog != nil || !strings.Contains(result.Delivery.NoNext, "Transient continuation unavailable") {
			t.Fatalf("%s hid read-only-root delivery failure: %+v", profile.tool, result)
		}
	}
}
