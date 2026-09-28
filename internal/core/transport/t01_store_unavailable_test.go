package transport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

// A directory at the lock path cannot be opened for read/write even by a
// privileged test process. Exercise the real store and delivery paths rather
// than depending on mode-bit enforcement or skipping under root.
func TestT01StoreReadAndCheckFailClosedAtUnopenableLock(t *testing.T) {
	root := t.TempDir()
	service := app.Service{Root: root}
	defaultClient := t01aStartClient(t, service, ProfileDefault)
	legacyClient := t01aStartClient(t, service, ProfileLegacy)
	defaultClient.list(t)
	legacyClient.list(t)

	recall := app.Request{Format: delivery.Format, Operation: "recall", Query: "no fixture records"}
	check := app.Request{Format: delivery.Format, Operation: "check", Action: "structural"}
	baseline := defaultClient.mustCall(t, "haft_read", recall)
	structural := defaultClient.mustCall(t, "haft_check", check)
	if baseline.Kind != "results" || structural.Kind != "structurally_valid" || baseline.Delivery.Catalog == nil {
		t.Fatalf("control project did not provide a usable read/check basis: %+v / %+v", baseline, structural)
	}
	continuation := *baseline.Delivery.Catalog
	lock := filepath.Join(root, ".haft", ".runtime", "writer.lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	if handle, err := os.OpenFile(lock, os.O_RDWR, 0); err == nil {
		_ = handle.Close()
		t.Fatal("lock directory unexpectedly opened for read/write")
	}

	for _, profile := range []struct {
		client    *t01aProtocolClient
		readTool  string
		checkTool string
	}{
		{defaultClient, "haft_read", "haft_check"},
		{legacyClient, "haft", "haft"},
	} {
		for _, action := range []struct {
			tool    string
			request any
		}{
			{profile.readTool, recall},
			{profile.checkTool, check},
			{profile.readTool, continuation},
		} {
			result := profile.client.mustCall(t, action.tool, action.request)
			if result.Kind != "unavailable" || !result.IsError || result.Coverage != "unavailable" {
				t.Fatalf("%s returned false read/check success under unavailable lock: %+v", action.tool, result)
			}
			if result.Delivery.Catalog != nil || result.Delivery.Next != nil || len(result.Delivery.Available) != 0 {
				t.Fatalf("%s offered an unusable continuation: %+v", action.tool, result.Delivery)
			}
			if len(result.Diagnostics) == 0 || !strings.Contains(result.Diagnostics[0].Message, "writer.lock") {
				t.Fatalf("%s hid the lock failure: %+v", action.tool, result.Diagnostics)
			}
		}
	}
	info, err := os.Stat(lock)
	if err != nil || !info.IsDir() {
		t.Fatalf("unavailable lock path was replaced: %+v / %v", info, err)
	}
}
