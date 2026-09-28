package store

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func TestCaptureClaimConcurrentStartAndLostReplyReplay(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: root}
	ctx := context.Background()
	id := "capture-request-concurrent"
	digest := carrier.Digest([]byte("one exact capture request"))
	before, err := s.LookupCapture(ctx, id, digest)
	if err != nil || before.Kind != "absent" {
		t.Fatalf("fresh lookup: %+v %v", before, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".haft")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("absent lookup mutated project store: %v", err)
	}
	baseView, err := s.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const callers = 12
	results := make(chan CaptureClaim, callers)
	errors := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := (Store{Root: root}).BeginCapture(ctx, id, digest)
			results <- claim
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	started := 0
	pending := 0
	for claim := range results {
		switch claim.Kind {
		case "started":
			started++
		case "pending":
			pending++
			if claim.Action == "" {
				t.Fatal("pending claim lacks recovery action")
			}
		default:
			t.Fatalf("unexpected claim %+v", claim)
		}
	}
	if started != 1 || pending != callers-1 {
		t.Fatalf("one project execution claim required: started=%d pending=%d", started, pending)
	}
	if _, err := s.Read(ctx); err != nil {
		t.Fatalf("claim retained writer lock after begin: %v", err)
	}
	ref, err := s.PutTransient(ctx, []byte(`{"actual_exit_code":0,"stdout":"PASS"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteCapture(ctx, id, digest, ref); err != nil {
		t.Fatal(err)
	}
	view, err := s.Read(ctx)
	if err != nil || view.Coverage != "complete" || view.Generation != baseView.Generation || len(view.Files) != 0 {
		t.Fatalf("capture receipt changed canonical memory: %+v %v", view, err)
	}
	if err := safePath(".capture-receipts/foreign.json"); err == nil {
		t.Fatal("canonical publication could overwrite capture receipt")
	}
	// Simulate a server restart and a lost first response. The independent
	// instance must return the exact saved ref without granting a second run.
	restarted := Store{Root: root}
	replayed, err := restarted.LookupCapture(ctx, id, digest)
	if err != nil || replayed.Kind != "completed" || replayed.Ref != ref {
		t.Fatalf("lost-reply lookup: %+v %v", replayed, err)
	}
	replayed, err = restarted.BeginCapture(ctx, id, digest)
	if err != nil || replayed.Kind != "completed" || replayed.Ref != ref {
		t.Fatalf("lost-reply begin: %+v %v", replayed, err)
	}
	if err := restarted.CompleteCapture(ctx, id, digest, ref); err != nil {
		t.Fatalf("idempotent completion: %v", err)
	}
	conflict, err := restarted.BeginCapture(ctx, id, carrier.Digest([]byte("different request")))
	if err != nil || conflict.Kind != "conflict" {
		t.Fatalf("different payload was admitted: %+v %v", conflict, err)
	}
	if err := restarted.CompleteCapture(ctx, id, digest, "result:"+carrier.Digest([]byte("other"))); err == nil || !strings.Contains(err.Error(), "capture_result_conflict") {
		t.Fatalf("different completion was accepted: %v", err)
	}
	if err := os.Remove(filepath.Join(root, ".haft", ".cache", "disclosure", strings.TrimPrefix(ref, "result:sha256:")+".json")); err != nil {
		t.Fatal(err)
	}
	expired, err := restarted.BeginCapture(ctx, id, digest)
	if err != nil || expired.Kind != "expired" || expired.Ref != ref || expired.Action == "" {
		t.Fatalf("missing result cache granted another run: %+v %v", expired, err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".haft", ".cache")); err != nil {
		t.Fatal(err)
	}
	expired, err = restarted.BeginCapture(ctx, id, digest)
	if err != nil || expired.Kind != "expired" || expired.Ref != ref || expired.Action == "" {
		t.Fatalf("whole-cache disposal erased execution history: %+v %v", expired, err)
	}
	view, err = restarted.Read(ctx)
	if err != nil || view.Coverage != "complete" || view.Generation != baseView.Generation || len(view.Files) != 0 {
		t.Fatalf("cache disposal or receipt changed canonical memory: %+v %v", view, err)
	}
}

func TestCaptureClaimCorruptResultRequiresNewRequestID(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: root}
	ctx := context.Background()
	id := "capture-result-corrupt"
	digest := carrier.Digest([]byte("exact request"))
	claim, err := s.BeginCapture(ctx, id, digest)
	if err != nil || claim.Kind != "started" {
		t.Fatalf("begin: %+v %v", claim, err)
	}
	ref, err := s.PutTransient(ctx, []byte(`{"result":"captured"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteCapture(ctx, id, digest, ref); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".haft", ".cache", "disclosure", strings.TrimPrefix(ref, "result:sha256:")+".json")
	if err := os.WriteFile(p, []byte(`{"result":"altered"}`), 0600); err != nil {
		t.Fatal(err)
	}
	restarted := Store{Root: root}
	for _, lookup := range []func(context.Context, string, string) (CaptureClaim, error){restarted.LookupCapture, restarted.BeginCapture} {
		state, err := lookup(ctx, id, digest)
		if err != nil || state.Kind != "corrupt" || state.Ref != ref || !strings.Contains(state.Action, "new request_id") {
			t.Fatalf("corrupt cache was reused or reexecuted: %+v %v", state, err)
		}
	}
	if err := restarted.CompleteCapture(ctx, id, digest, ref); !errors.Is(err, errCaptureResultCorrupt) {
		t.Fatalf("corrupt completion was accepted: %v", err)
	}
}

func TestCaptureClaimUnknownProcessAndMissingResultStayPending(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: root}
	ctx := context.Background()
	id := "capture-request-pending"
	digest := carrier.Digest([]byte("pending payload"))
	claim, err := s.BeginCapture(ctx, id, digest)
	if err != nil || claim.Kind != "started" {
		t.Fatalf("first begin: %+v %v", claim, err)
	}
	if err := s.CompleteCapture(ctx, id, digest, "result:"+carrier.Digest([]byte("unpublished"))); err == nil {
		t.Fatal("unpublished result ref completed execution")
	}
	restarted := Store{Root: root}
	pending, err := restarted.LookupCapture(ctx, id, digest)
	if err != nil || pending.Kind != "pending" || pending.Action == "" {
		t.Fatalf("unknown process outcome: %+v %v", pending, err)
	}
	pending, err = restarted.BeginCapture(ctx, id, digest)
	if err != nil || pending.Kind != "pending" {
		t.Fatalf("unknown process reexecuted: %+v %v", pending, err)
	}
	if err := s.CompleteCapture(ctx, "never-started", digest, "result:"+carrier.Digest([]byte("x"))); err == nil || !strings.Contains(err.Error(), "capture_claim_missing") {
		t.Fatalf("completion without claim: %v", err)
	}
}

func TestCaptureClaimLockAndReceiptPathFailuresPreventStart(t *testing.T) {
	ctx := context.Background()
	digest := carrier.Digest([]byte("fixture"))
	root := t.TempDir()
	lockPath := filepath.Join(root, ".haft", ".runtime", "writer.lock")
	if err := os.MkdirAll(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: root}).BeginCapture(ctx, "unwritable-lock", digest); err == nil {
		t.Fatal("unopenable writer lock admitted project execution")
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	s := Store{Root: root, LockTimeout: 30 * time.Millisecond}
	h, err := s.open(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Store{Root: root, LockTimeout: 30 * time.Millisecond}).BeginCapture(ctx, "locked", digest)
	h.close()
	if err == nil || !strings.Contains(err.Error(), "lock_timeout") {
		t.Fatalf("unavailable lock admitted project execution: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".haft", ".capture-receipts")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: root}).BeginCapture(ctx, "symlink-receipt", digest); err == nil || !strings.Contains(err.Error(), "symlink_not_allowed") {
		t.Fatalf("symlinked claim receipt path admitted project execution: %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("symlink target changed: %v %+v", err, entries)
	}
}

func TestCaptureClaimRejectsUnboundedOrMalformedIdentity(t *testing.T) {
	s := Store{Root: t.TempDir()}
	ctx := context.Background()
	digest := carrier.Digest([]byte("fixture"))
	for _, id := range []string{"", " ", "bad\nkey", strings.Repeat("x", 513)} {
		if _, err := s.BeginCapture(ctx, id, digest); err == nil {
			t.Fatalf("invalid key admitted: %q", id)
		}
	}
	if _, err := s.BeginCapture(ctx, "valid", "sha256:short"); err == nil {
		t.Fatal("invalid payload digest admitted")
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".haft")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("invalid identity mutated store: %v", err)
	}
}

func TestCaptureTerminalReceiptPreservesKnownOutcomeWithoutCache(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: root}
	ctx := context.Background()
	id := strings.Repeat("x", 512)
	digest := carrier.Digest([]byte("exact request"))
	claimRef := "spec-20260925-12345678@" + carrier.Digest([]byte("edition")) + "#" + strings.Repeat("c", 417)
	if len(claimRef) != 512 {
		t.Fatalf("fixture exact claim ref has %d bytes", len(claimRef))
	}
	basis := map[string]string{"claim_ref": claimRef, "claim_ref_digest": carrier.Digest([]byte(claimRef)),
		"memory_generation": carrier.Digest([]byte("memory")), "code_basis": carrier.Digest([]byte("code")),
		"implementation_basis": carrier.Digest([]byte("implementation")), "check_basis": carrier.Digest([]byte("check")),
		"dependency_basis": carrier.Digest([]byte("dependencies")), "expected_contract_digest": carrier.Digest([]byte("contract"))}
	exit := 0
	terminal := CaptureTerminal{ResultKind: "passed", ReasonCode: "selected_test_passed", Basis: basis,
		RecordedPostRunBasis: "same", DeclaredCheckBinding: true, ProcessStarted: true, ExitCode: &exit,
		StdoutBytes: 16, StderrBytes: 0, StdoutDigest: carrier.Digest([]byte("observed stdout")),
		StderrDigest: carrier.Digest(nil), StdoutDrainComplete: true, StderrDrainComplete: true,
		ProcessOutputComplete: true}
	started, err := s.BeginCapture(ctx, id, digest)
	if err != nil || started.Kind != "started" {
		t.Fatalf("start: %+v %v", started, err)
	}
	if err := s.CompleteCaptureOutcome(ctx, id, digest, "", terminal); err != nil {
		t.Fatal(err)
	}
	completion, err := os.ReadFile(filepath.Join(root, ".haft", filepath.FromSlash(captureCompletePath(id))))
	if err != nil || len(completion) > captureClaimFileLimit {
		t.Fatalf("terminal receipt is not bounded: size=%d err=%v", len(completion), err)
	}
	replayed, err := (Store{Root: root}).LookupCapture(ctx, id, digest)
	if err != nil || replayed.Kind != "completed_unavailable" || replayed.Ref != "" || !reflect.DeepEqual(replayed.Terminal, &terminal) {
		t.Fatalf("known terminal was lost after restart: %+v %v", replayed, err)
	}
	if err := s.CompleteCaptureOutcome(ctx, id, digest, "", terminal); err != nil {
		t.Fatalf("idempotent terminal completion failed: %v", err)
	}
	changed := terminal
	changed.ReasonCode = "other_result"
	if err := s.CompleteCaptureOutcome(ctx, id, digest, "", changed); err == nil || !strings.Contains(err.Error(), "capture_result_conflict") {
		t.Fatalf("conflicting terminal replaced known execution: %v", err)
	}
	badBasis := map[string]string{}
	for key, value := range terminal.Basis {
		badBasis[key] = value
	}
	badBasis["claim_ref_digest"] = carrier.Digest([]byte("another claim"))
	malformed := terminal
	malformed.Basis = badBasis
	if err := s.CompleteCaptureOutcome(ctx, id, digest, "", malformed); err == nil || !strings.Contains(err.Error(), "exact claim ref digest differs") {
		t.Fatalf("mismatched exact claim locator admitted: %v", err)
	}
	incomplete := terminal
	incomplete.StderrDrainComplete = false
	if err := s.CompleteCaptureOutcome(ctx, id, digest, "", incomplete); err == nil || !strings.Contains(err.Error(), "contradicts observed pipe drains") {
		t.Fatalf("contradictory process completeness admitted: %v", err)
	}
	cleanup := terminal
	cleanup.CleanupUncertain = true
	if err := s.CompleteCaptureOutcome(ctx, id, digest, "", cleanup); err == nil || !strings.Contains(err.Error(), "requires unknown post-run basis") {
		t.Fatalf("unsettled cleanup certified same post-run basis: %v", err)
	}
	conflict, err := s.BeginCapture(ctx, id, carrier.Digest([]byte("different request")))
	if err != nil || conflict.Kind != "conflict" {
		t.Fatalf("different request reused the terminal claim: %+v %v", conflict, err)
	}
}
