package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/check"
	"github.com/m0n0x41d/haft/internal/core/checkrunner"
)

func TestCapturePreStartPipeFailureRetainsKnownEnvironmentOutcome(t *testing.T) {
	run := checkrunner.Result{
		Failure: "start_failed", StdoutDigest: carrier.Digest(nil), StderrDigest: carrier.Digest(nil),
		PipeDrainUncertain: true,
	}
	s, q, result, payloadDigest := injectedPipeSetupFailure(t, run)
	if result.Kind != check.EnvironmentFailure {
		t.Fatalf("pipe setup failure became an unknown outcome: %+v", result)
	}
	process, ok := object(result.Data)["process"].(CaptureProcess)
	if !ok {
		t.Fatalf("process accounting absent: %+v", result.Data)
	}
	if process.Started || process.ExitCode != nil || process.Failure != "start_failed" || process.StdoutBytes != 0 || process.StderrBytes != 0 || process.StdoutDigest != carrier.Digest(nil) || process.StderrDigest != carrier.Digest(nil) {
		t.Fatalf("pre-start process facts malformed: %+v", process)
	}
	if !process.PipeDrainUncertain || process.StdoutDrainComplete || process.StderrDrainComplete || process.CleanupUncertain || !strings.Contains(process.ProcessDiagnostic, "too many open files") {
		t.Fatalf("pipe setup was confused with completed output or lost its OS diagnostic: %+v", process)
	}
	first := s.completeCapture(context.Background(), q, result, payloadDigest)
	if first.Kind != check.EnvironmentFailure || captureRuns(t, s.Root) != 0 {
		t.Fatalf("known no-start outcome was lost or project test launched: %+v", first)
	}
	diagnosticVisible := false
	for _, diagnostic := range first.Diagnostics {
		diagnosticVisible = diagnosticVisible || strings.Contains(diagnostic.Message, "too many open files")
	}
	if !diagnosticVisible && !strings.Contains(str(object(first.Data)["process_diagnostic"]), "too many open files") {
		t.Fatalf("first response hid the OS pipe diagnostic: diagnostics=%+v data=%+v", first.Diagnostics, first.Data)
	}
	claim, err := s.memory().LookupCapture(context.Background(), q.RequestID, payloadDigest)
	if err != nil || claim.Kind != "completed" || claim.Terminal == nil {
		t.Fatalf("terminal receipt unavailable: %+v err=%v", claim, err)
	}
	terminal := claim.Terminal
	if terminal.ResultKind != check.EnvironmentFailure || terminal.ReasonCode != "start_failed" || terminal.ProcessStarted || terminal.ProcessOutputComplete || !strings.Contains(terminal.ProcessDiagnostic, "too many open files") {
		t.Fatalf("terminal receipt did not retain known OS failure: %+v", terminal)
	}
	retry := public(t, Service{Root: s.Root}, q)
	if retry.Kind != check.EnvironmentFailure || object(retry.Data)["execution_replay"] != true || captureRuns(t, s.Root) != 0 {
		t.Fatalf("same-ID retry did not replay no-start outcome: %+v", retry)
	}
}

func TestCapturePreStartPipeFailureReceiptOnlyReplayKeepsDiagnostic(t *testing.T) {
	s, q, result, payloadDigest := injectedPipeSetupFailure(t, checkrunner.Result{})
	terminal := captureTerminal(result)
	if err := s.memory().CompleteCaptureOutcome(context.Background(), q.RequestID, payloadDigest, "", terminal); err != nil {
		t.Fatalf("valid no-start facts could not be retained without cache: %v", err)
	}
	retry := public(t, Service{Root: s.Root}, q)
	if retry.Kind != check.EnvironmentFailure || object(retry.Data)["execution_replay"] != true || captureRuns(t, s.Root) != 0 {
		t.Fatalf("receipt-only retry lost the environment outcome: %+v", retry)
	}
	process := object(object(retry.Data)["process_accounting"])
	if process["started"] != false || process["process_output_complete"] != false || process["stdout_drain_complete"] != false || process["stderr_drain_complete"] != false || !strings.Contains(str(process["process_diagnostic"]), "too many open files") {
		t.Fatalf("receipt-only replay fabricated output or lost OS diagnostic: %+v", process)
	}
}

func TestCaptureProcessDiagnosticHasBoundedJSONFootprint(t *testing.T) {
	diagnostic := boundedProcessDiagnostic(errors.New(strings.Repeat("\"\\<>&\n", 100)))
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostic) > 256 || len(encoded) != len(diagnostic)+2 || !strings.HasSuffix(diagnostic, "...") {
		t.Fatalf("terminal diagnostic was unbounded or expanded in JSON: raw=%q bytes=%d encoded=%d", diagnostic, len(diagnostic), len(encoded))
	}
}

func TestKnownNoStartFactsPreserveTimeoutAndCancellation(t *testing.T) {
	for _, failure := range []string{"timeout", "canceled"} {
		run := knownNoStartRunnerFacts(checkrunner.Result{Failure: failure}, errors.New("pipe setup failed"))
		if run.Failure != failure || run.Started || run.StdoutDrainComplete || run.StderrDrainComplete || !run.PipeDrainUncertain || run.StdoutDigest != carrier.Digest(nil) || run.StderrDigest != carrier.Digest(nil) {
			t.Fatalf("pre-start %s was relabelled or granted output completeness: %+v", failure, run)
		}
	}
}

func injectedPipeSetupFailure(t *testing.T, run checkrunner.Result) (Service, Request, Result, string) {
	t.Helper()
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
		t.Fatalf("capture claim: %+v err=%v", claim, err)
	}
	runErr := errors.New("stdout pipe: pipe: too many open files")
	result := s.capturedCheck(context.Background(), q, before, expected, command, index.Basis, basisCapture, s.Root, "injected-no-start", run, runErr)
	return s, q, result, digest
}
