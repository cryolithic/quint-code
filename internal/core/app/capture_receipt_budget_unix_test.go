//go:build darwin || linux

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/check"
	"github.com/m0n0x41d/haft/internal/core/checkrunner"
)

// The cache can exhaust its lock wait while the receipt store becomes writable
// shortly afterward. The known terminal facts still need an independent attempt.
func TestCaptureReceiptGetsFreshBudgetAfterCacheLockTimeout(t *testing.T) {
	s, q := captureFixture(t, captureMarkerTest)
	q.Format = Format
	before := readView(t, s)
	expected, command, index, basisCapture, err := s.prepareCheck(q, before)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	digest := carrier.Digest(payload)
	claim, err := s.memory().BeginCapture(context.Background(), q.RequestID, digest)
	if err != nil || claim.Kind != "started" {
		t.Fatalf("begin capture: claim=%+v err=%v", claim, err)
	}
	run := checkrunner.Result{Failure: "start_failed", StdoutDigest: carrier.Digest(nil), StderrDigest: carrier.Digest(nil)}
	result := s.capturedCheck(context.Background(), q, before, expected, command, index.Basis, basisCapture, s.Root, "injected", run, nil)
	if result.Kind != check.EnvironmentFailure {
		t.Fatalf("injected known outcome = %s", result.Kind)
	}

	lock, err := os.OpenFile(filepath.Join(s.Root, ".haft", ".runtime", "writer.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(5300 * time.Millisecond)
		released <- syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	}()

	first := s.completeCapture(context.Background(), q, result, digest)
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	firstSummary := object(first.Data)
	if first.Kind != check.EnvironmentFailure || firstSummary["receipt_persistence"] != "confirmed" || firstSummary["continuation_unavailable"] != true || first.Delivery.NoNext == "" {
		t.Fatalf("cache timeout lost known terminal facts or receipt: %+v", first)
	}
	retry := public(t, Service{Root: s.Root}, q)
	retrySummary := object(retry.Data)
	if retry.Kind != first.Kind || retrySummary["execution_replay"] != true || retrySummary["receipt_persistence"] != "confirmed" || captureRuns(t, s.Root) != 0 {
		t.Fatalf("same ID failed to replay without executing: %+v", retry)
	}
}
