package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/check"
	"github.com/m0n0x41d/haft/internal/core/checkrunner"
	"github.com/m0n0x41d/haft/internal/core/code"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

const captureMaxTimeoutMillis = 120000
const captureMaxOutputBytes = 8 << 20

// CaptureProcess records what the bounded effect shell actually observed. The
// digests and byte counts cover all bytes actually observed before pipe
// closure, including bytes beyond the retained aggregate output limit. An
// incomplete or uncertain drain need not include the full emitted stream.
type CaptureProcess struct {
	Command             []string          `json:"command"`
	Executable          string            `json:"executable"`
	CWD                 string            `json:"cwd"`
	Environment         map[string]string `json:"environment"`
	Toolchain           string            `json:"toolchain"`
	Started             bool              `json:"started"`
	ExitCode            *int              `json:"exit_code"`
	Failure             string            `json:"environment_failure,omitempty"`
	ProcessDiagnostic   string            `json:"process_diagnostic,omitempty"`
	TimeoutMillis       int               `json:"timeout_ms"`
	MaxOutputBytes      int               `json:"max_output_bytes"`
	StdoutBytes         int64             `json:"stdout_bytes"`
	StderrBytes         int64             `json:"stderr_bytes"`
	StdoutDigest        string            `json:"stdout_digest"`
	StderrDigest        string            `json:"stderr_digest"`
	OutputExceeded      bool              `json:"output_exceeded"`
	PipeDrainUncertain  bool              `json:"pipe_drain_uncertain"`
	CleanupUncertain    bool              `json:"cleanup_uncertain"`
	StdoutDrainComplete bool              `json:"stdout_drain_complete"`
	StderrDrainComplete bool              `json:"stderr_drain_complete"`
}

func validateCapture(q Request) error {
	if err := store.ValidateCaptureRequestID(q.RequestID); err != nil {
		return err
	}
	if q.Ref == "" || q.CheckRef == "" || q.Scope == "" || q.Capture == nil {
		return fmt.Errorf("capture requires exact claim/check refs, scope and capture limits")
	}
	if q.Capture.TimeoutMillis < 1 || q.Capture.TimeoutMillis > captureMaxTimeoutMillis {
		return fmt.Errorf("timeout_ms must be between 1 and %d", captureMaxTimeoutMillis)
	}
	if q.Capture.MaxOutputBytes < 1 || q.Capture.MaxOutputBytes > captureMaxOutputBytes {
		return fmt.Errorf("max_output_bytes must be between 1 and %d aggregate bytes", captureMaxOutputBytes)
	}
	return nil
}

func captureBasis(generation, codeBasis string, expected check.Contract) map[string]string {
	expectedBytes, _ := json.Marshal(expected)
	claimRef := expected.Basis.Claim
	return map[string]string{"memory_generation": generation, "code_basis": codeBasis,
		"claim_ref": claimRef, "claim_ref_digest": carrier.Digest([]byte(claimRef)),
		"implementation_basis": expected.Basis.Code, "check_basis": expected.Basis.Check,
		"dependency_basis": expected.Basis.Dependencies, "expected_contract_digest": carrier.Digest(expectedBytes)}
}

func captureTerminalCapacity(q Request, payloadDigest string, basis map[string]string) error {
	exit := int(^uint(0) >> 1)
	terminal := store.CaptureTerminal{ResultKind: check.EnvironmentFailure, ReasonCode: strings.Repeat("r", 128),
		Basis: basis, RecordedPostRunBasis: "unknown", DeclaredCheckBinding: false,
		ProcessStarted: false, ExitCode: &exit, ProcessFailure: strings.Repeat("f", 64),
		ProcessDiagnostic: strings.Repeat("d", 256),
		StdoutBytes:       9223372036854775807, StderrBytes: 9223372036854775807,
		StdoutDigest: carrier.Digest(nil), StderrDigest: carrier.Digest(nil),
		ProcessOutputComplete: false}
	cleanup := terminal
	cleanup.CleanupUncertain = true
	for _, candidate := range []store.CaptureTerminal{terminal, cleanup} {
		if err := store.CheckCaptureTerminalCapacity(q.RequestID, payloadDigest, candidate); err != nil {
			return err
		}
		responses := []delivery.Response{
			captureTerminalResponse(candidate, q.RequestID, true, "uncertain", "unknown", "capture_unavailable", true),
			captureTerminalFailureResponse(candidate, q.RequestID, "unknown", "unavailable", strings.Repeat("a", 240)),
		}
		for _, response := range responses {
			if size := delivery.Size(response); size > delivery.Budget {
				return fmt.Errorf("capture_terminal_limit: bounded terminal reply would exceed %d bytes", delivery.Budget)
			}
		}
	}
	return nil
}

func (s Service) captureCall(ctx context.Context, q Request) delivery.Response {
	if err := validateCapture(q); err != nil {
		return delivery.Error("invalid_capture", err.Error())
	}
	payload, err := json.Marshal(q)
	if err != nil {
		return delivery.Error("invalid_capture", err.Error())
	}
	digest := carrier.Digest(payload)
	claim, err := s.memory().LookupCapture(ctx, q.RequestID, digest)
	if err != nil {
		return captureReceiptUnavailable(err.Error())
	}
	if claim.Kind != "absent" {
		return s.captureClaimResponse(ctx, q, claim)
	}
	v, err := s.memory().Read(ctx)
	if err != nil {
		return delivery.Error("unavailable", err.Error())
	}
	if v.Coverage != "complete" {
		return delivery.Error("check_basis_unresolved", "Project memory capture is incomplete; no test was started")
	}
	expected, command, index, basisCapture, err := s.prepareCheck(q, v)
	if err != nil {
		return delivery.Error("check_basis_unresolved", err.Error()+"; no test was started")
	}
	basis := captureBasis(v.Generation, index.Basis, expected)
	if err := captureTerminalCapacity(q, digest, basis); err != nil {
		return captureCapacityRefusal(err)
	}
	environment, err := code.GoTestEnvironment(index.Config)
	if err != nil {
		return delivery.Error("check_basis_unresolved", err.Error()+"; no test was started")
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return delivery.Error("invalid_capture", err.Error())
	}
	version, err := verifyCaptureToolchain(ctx, root, environment, index.Config.Toolchain)
	if err != nil {
		return delivery.Error("environment_failure", err.Error()+"; no project test was started")
	}
	if err := s.captureStartBasis(ctx, q, v.Generation, expected, index.Basis); err != nil {
		return delivery.Error("check_basis_changed", err.Error()+"; no project test was started")
	}
	claim, err = s.memory().BeginCapture(ctx, q.RequestID, digest)
	if err != nil {
		return captureReceiptUnavailable(err.Error() + "; no project test was started by this call")
	}
	if claim.Kind != "started" {
		return s.captureClaimResponse(ctx, q, claim)
	}
	run, runErr := checkrunner.Run(ctx, checkrunner.Request{
		Root: root, Command: command, Environment: environment,
		Timeout:        time.Duration(q.Capture.TimeoutMillis) * time.Millisecond,
		MaxOutputBytes: q.Capture.MaxOutputBytes,
	})
	result := s.capturedCheck(ctx, q, v, expected, command, index.Basis, basisCapture, root, version, run, runErr)
	return s.completeCapture(ctx, q, result, digest)
}

func captureCapacityRefusal(reason error) delivery.Response {
	response := delivery.Error("capture_capacity_exceeded", reason.Error()+"; no request_id was consumed and no project test was started. Use check/prepare, a separately authorized bounded external run, then check/observe with independently captured facts.")
	response.Operation = "check"
	response.Delivery.NoNext = "This exact capture cannot fit its bounded receipt or reply. Use check/prepare, a separately authorized bounded external run, then check/observe; repeating the unchanged capture will be refused."
	return response
}

func captureReceiptUnavailable(message string) delivery.Response {
	response := delivery.Error("unavailable", message)
	response.Operation = "check"
	response.Delivery.NoNext = "Receipt state is unavailable. Restore receipt access and inspect the original attempt; identical retry may recover it. Do not infer permission for a new run."
	return response
}

// A toolchain probe or another local writer can change the project while the
// command is being prepared. Recheck immediately before claiming execution;
// mid-run changes are separately rejected by the post-run basis comparison.
func (s Service) captureStartBasis(ctx context.Context, q Request, generation string, expected check.Contract, basis string) error {
	v, err := s.memory().Read(ctx)
	if err != nil {
		return err
	}
	if v.Coverage != "complete" || v.Generation != generation {
		return fmt.Errorf("project memory generation changed or became incomplete")
	}
	current, _, index, _, err := s.prepareCheck(q, v)
	if err != nil {
		return err
	}
	if index.Basis != basis || !sameJSON(current, expected) {
		return fmt.Errorf("declared test or exact code basis changed during preparation")
	}
	return nil
}

func verifyCaptureToolchain(ctx context.Context, root string, environment map[string]string, expected string) (string, error) {
	probe, err := checkrunner.Run(ctx, checkrunner.Request{
		Root: root, Command: []string{"go", "version"}, Environment: environment,
		Timeout: 5 * time.Second, MaxOutputBytes: 2048,
	})
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(probe.Stdout))
	if probe.Failure != "" || probe.OutputExceeded || probe.ExitCode == nil || *probe.ExitCode != 0 || len(fields) < 3 || fields[2] != expected {
		return "", fmt.Errorf("local Go toolchain does not match declared %s", expected)
	}
	return fields[2], nil
}

func (s Service) capturedCheck(ctx context.Context, q Request, before store.View, expected check.Contract, command []string, codeBasis string, basisCapture CheckBasisCapture, root, toolchain string, run checkrunner.Result, runErr error) Result {
	run = knownNoStartRunnerFacts(run, runErr)
	r := Result{Format: Format, Operation: "check", Kind: check.Unattributable,
		Diagnostics: []carrier.Diagnostic{}, Basis: captureBasis(before.Generation, codeBasis, expected),
		Coverage: before.Coverage,
		Limits:   []string{"Capture runs only the selected Go test; oracle fitness and external module/toolchain bytes remain outside this local attribution", "The process inherits host PATH, HOME and temporary-directory locations; pinned Go variables are recorded, but host cache and executable bytes are not captured", "The recorded post-run basis is historical; a same-ID replay separately assesses current local basis without executing", "The process can have project/environment side effects; no project-memory publication is implied"}}
	process := CaptureProcess{Command: command, Executable: run.Executable, CWD: root, Environment: run.Environment, Toolchain: toolchain,
		Started: run.Started, ExitCode: run.ExitCode, Failure: run.Failure,
		TimeoutMillis: q.Capture.TimeoutMillis, MaxOutputBytes: q.Capture.MaxOutputBytes,
		StdoutBytes: run.StdoutBytes, StderrBytes: run.StderrBytes,
		StdoutDigest: run.StdoutDigest, StderrDigest: run.StderrDigest,
		OutputExceeded: run.OutputExceeded, PipeDrainUncertain: run.PipeDrainUncertain, CleanupUncertain: run.CleanupUncertain,
		StdoutDrainComplete: run.StdoutDrainComplete, StderrDrainComplete: run.StderrDrainComplete}
	if runErr != nil {
		if process.Failure == "" {
			process.Failure = "capture_error"
		}
		process.ProcessDiagnostic = boundedProcessDiagnostic(runErr)
		r.Diagnostics = append(r.Diagnostics, carrier.Diagnostic{Code: "capture_error", Message: runErr.Error(), Severity: "error"})
	}
	input := check.ObservationInput{Expected: expected, Observed: check.Run{
		Started: run.Started, ExitCode: run.ExitCode, Command: command, Selector: expected.Selector,
		Basis: expected.Basis, Stdout: run.Stdout, Stderr: run.Stderr,
		EnvironmentFailure: process.Failure,
	}}
	observation := check.GoTestObservation(input)
	currentBasis := "unknown"
	declared := false
	if after, err := s.memory().Read(ctx); err == nil && after.Coverage == "complete" {
		if current, _, currentIndex, _, prepareErr := s.prepareCheck(q, after); prepareErr == nil {
			declared = check.DeclarationMismatch(expected, current) == ""
			currentBasis = "changed"
			if declared && sameJSON(expected.Basis, current.Basis) && currentIndex.Basis == codeBasis && after.Generation == before.Generation {
				currentBasis = "same"
			}
		} else {
			r.Diagnostics = append(r.Diagnostics, carrier.Diagnostic{Code: "post_capture_basis_unresolved", Message: prepareErr.Error(), Severity: "warning"})
		}
	} else if err != nil {
		r.Diagnostics = append(r.Diagnostics, carrier.Diagnostic{Code: "post_capture_read_unavailable", Message: err.Error(), Severity: "warning"})
	}
	if run.OutputExceeded {
		if observation.Status == check.Passed || observation.Status == check.Unattributable {
			observation.Status = check.Unattributable
			observation.ReasonCode = "output_limit"
		}
		observation.Limits = append(observation.Limits, "Process output exceeded the aggregate capture limit; digests and lengths cover observed drained bytes, retained prefixes are incomplete")
	}
	if process.PipeDrainUncertain {
		observation.Limits = append(observation.Limits, "Pipe drain was not established before completion; observed byte counts and digests do not certify the full emitted stream")
	}
	if process.CleanupUncertain {
		currentBasis = "unknown"
		declared = false
		if observation.Status == check.Passed {
			observation.Status = check.Unattributable
			observation.ReasonCode = "cleanup_uncertain"
		}
		observation.Limits = append(observation.Limits, "Process-group cleanup was not certified; descendants may still change the basis after this response")
	}
	if currentBasis != "same" {
		if observation.Status == check.Passed {
			observation.Status = check.Unattributable
			observation.ReasonCode = "basis_changed_or_unavailable"
		}
		observation.Limits = append(observation.Limits, "Exact project basis was not stable across execution; this run cannot establish a current pass")
	}
	r.Kind = observation.Status
	r.Limits = append(r.Limits, observation.Limits...)
	r.Data = map[string]any{"observation": observation, "expected": expected, "process": process,
		"current_basis": currentBasis, "declared_check_binding": declared, "basis_capture": basisCapture}
	return r
}

// A pre-start adapter error has observed no project output. Preserve this
// narrow known result even if an older runner returned an empty Result with
// the error; no EOF or process cleanup is inferred from zero observed bytes.
func knownNoStartRunnerFacts(run checkrunner.Result, runErr error) checkrunner.Result {
	if runErr == nil || run.Started || run.ExitCode != nil || run.StdoutBytes != 0 || run.StderrBytes != 0 || len(run.Stdout) != 0 || len(run.Stderr) != 0 {
		return run
	}
	if run.Failure == "" {
		run.Failure = "start_failed"
	}
	if run.StdoutDigest == "" {
		run.StdoutDigest = carrier.Digest(nil)
	}
	if run.StderrDigest == "" {
		run.StderrDigest = carrier.Digest(nil)
	}
	run.StdoutDrainComplete = false
	run.StderrDrainComplete = false
	run.PipeDrainUncertain = true
	return run
}

func boundedProcessDiagnostic(err error) string {
	diagnostic := strings.Map(func(value rune) rune {
		if value == '"' || value == '\\' || value == '<' || value == '>' || value == '&' || value == '\u2028' || value == '\u2029' || unicode.IsControl(value) {
			return '?'
		}
		return value
	}, err.Error())
	if len(diagnostic) <= store.CaptureProcessDiagnosticLimit {
		return diagnostic
	}
	limit := store.CaptureProcessDiagnosticLimit - len("...")
	for !utf8.RuneStart(diagnostic[limit]) {
		limit--
	}
	return diagnostic[:limit] + "..."
}

func (s Service) completeCapture(ctx context.Context, q Request, r Result, payloadDigest string) delivery.Response {
	d := makeDelivery(q, r)
	terminal := captureTerminal(r)
	metadata := r
	metadata.Data = nil
	raw, encodeErr := json.Marshal(capturedResult{Format: "haft.transient-result/1", Request: q, Result: metadata, Document: &d})
	ref := ""
	cacheErr := encodeErr
	cacheContext, cancelCache := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	if cacheErr == nil {
		ref, cacheErr = s.memory().PutTransient(cacheContext, raw)
	}
	cancelCache()
	receiptContext, cancelReceipt := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelReceipt()
	receiptErr := s.memory().CompleteCaptureOutcome(receiptContext, q.RequestID, payloadDigest, ref, terminal)
	persistence := "confirmed"
	if receiptErr != nil {
		persistence = "uncertain"
		d.Diagnostics = append(d.Diagnostics, carrier.Diagnostic{Code: "capture_receipt_persistence_uncertain", Message: "The known outcome was returned, but terminal receipt publication could not be confirmed: " + receiptErr.Error(), Severity: "warning"})
	}
	if cacheErr != nil {
		return captureTerminalResponse(terminal, q.RequestID, false, persistence, "", r.Kind, r.Failed())
	}
	d.Ref = ref
	if receiptErr != nil && cacheErr == nil {
		summary := captureTerminalSummary(terminal, q.RequestID, false, persistence, "")
		summary["continuation_unavailable"] = false
		d.Summary = rawJSON(summary)
	}
	return delivery.Present(d, delivery.Request{Format: delivery.Format, Operation: "read", Ref: ref, View: "summary"})
}

func captureTerminal(r Result) store.CaptureTerminal {
	data := object(r.Data)
	observation, _ := data["observation"].(check.Observation)
	process, _ := data["process"].(CaptureProcess)
	basis := map[string]string{}
	for key, value := range r.Basis {
		basis[key] = value
	}
	basis["claim_ref_digest"] = carrier.Digest([]byte(basis["claim_ref"]))
	return store.CaptureTerminal{ResultKind: r.Kind, ReasonCode: observation.ReasonCode, Basis: basis,
		RecordedPostRunBasis: str(data["current_basis"]), DeclaredCheckBinding: data["declared_check_binding"] == true,
		ProcessStarted: process.Started, ExitCode: process.ExitCode, ProcessFailure: process.Failure,
		ProcessDiagnostic: process.ProcessDiagnostic,
		StdoutBytes:       process.StdoutBytes, StderrBytes: process.StderrBytes,
		StdoutDigest: process.StdoutDigest, StderrDigest: process.StderrDigest,
		OutputExceeded: process.OutputExceeded, PipeDrainUncertain: process.PipeDrainUncertain, CleanupUncertain: process.CleanupUncertain,
		StdoutDrainComplete: process.StdoutDrainComplete, StderrDrainComplete: process.StderrDrainComplete,
		ProcessOutputComplete: process.Failure == "" && !process.OutputExceeded && !process.PipeDrainUncertain && !process.CleanupUncertain && process.StdoutDrainComplete && process.StderrDrainComplete}
}

func captureTerminalSummary(terminal store.CaptureTerminal, requestID string, replay bool, persistence, assessment string) map[string]any {
	current := terminal.RecordedPostRunBasis
	scope := "immediately_after_capture"
	if replay {
		current = assessment
		scope = "non_executing_retry"
	}
	process := map[string]any{"started": terminal.ProcessStarted, "exit_code": terminal.ExitCode,
		"environment_failure": terminal.ProcessFailure, "stdout_bytes": terminal.StdoutBytes,
		"stderr_bytes": terminal.StderrBytes, "stdout_digest": terminal.StdoutDigest,
		"stderr_digest": terminal.StderrDigest, "output_exceeded": terminal.OutputExceeded,
		"pipe_drain_uncertain":    terminal.PipeDrainUncertain,
		"cleanup_uncertain":       terminal.CleanupUncertain,
		"stdout_drain_complete":   terminal.StdoutDrainComplete,
		"stderr_drain_complete":   terminal.StderrDrainComplete,
		"process_output_complete": terminal.ProcessOutputComplete}
	if terminal.ProcessDiagnostic != "" {
		process["process_diagnostic"] = terminal.ProcessDiagnostic
	}
	summary := map[string]any{"application_outcome": terminal.ResultKind, "reason_code": terminal.ReasonCode,
		"request_id_digest": carrier.Digest([]byte(requestID)), "process_accounting": process,
		"current_basis": current, "basis_comparison_scope": scope,
		"recorded_post_run_basis":       terminal.RecordedPostRunBasis,
		"recorded_post_run_basis_scope": "immediately_after_capture",
		"declared_check_binding":        terminal.DeclaredCheckBinding,
		"execution_replay":              replay, "receipt_persistence": persistence}
	if len(requestID) <= 80 {
		summary["request_id"] = requestID
	}
	if replay {
		summary["current_basis_assessment"] = assessment
		summary["current_basis_assessment_scope"] = "non_executing_retry"
	}
	return summary
}

func captureTerminalResponse(terminal store.CaptureTerminal, requestID string, replay bool, persistence, assessment, kind string, isError bool) delivery.Response {
	summary := captureTerminalSummary(terminal, requestID, replay, persistence, assessment)
	summary["continuation_unavailable"] = true
	note := "Captured output continuation is unavailable; no result ref was published. The same request_id cannot execute again."
	lifetime := "receipt_only"
	if persistence == "uncertain" {
		note = "The terminal receipt is uncertain. Inspect the original attempt; the same request_id must not start another test."
		lifetime = "in_memory_only"
	}
	diagnostics := []carrier.Diagnostic{{Code: "continuation_unavailable", Message: "Result bytes were not published; only bounded terminal facts are available.", Severity: "warning"}}
	limits := []string{"Oracle fitness, host executable bytes and external dependencies remain outside this local attribution", "This bounded safety receipt does not retain full process output or create evidence"}
	if persistence == "uncertain" {
		diagnostics = append(diagnostics, carrier.Diagnostic{Code: "capture_receipt_persistence_uncertain", Message: "Terminal receipt publication could not be confirmed.", Severity: "warning"})
	}
	if terminal.CleanupUncertain {
		diagnostics = append(diagnostics, carrier.Diagnostic{Code: "cleanup_uncertain", Message: "Cleanup unconfirmed; descendants may still change basis.", Severity: "warning"})
	}
	return delivery.Response{Format: delivery.Format, Operation: "check", Kind: kind, Data: summary,
		Diagnostics: diagnostics, Basis: terminal.Basis, Coverage: "complete",
		Limits:  limits,
		IsError: isError, Delivery: delivery.State{View: "summary", Lifetime: lifetime, Complete: true,
			NoNext: note, Budget: delivery.Budget,
			Omissions: delivery.Omission{Note: "Full output parts are unavailable; terminal facts are complete only within this bounded receipt"}}}
}

func (s Service) captureClaimResponse(ctx context.Context, q Request, claim store.CaptureClaim) delivery.Response {
	if claim.Kind == "completed" {
		d, err := s.loadTransient(ctx, claim.Ref, false, "")
		if err != nil {
			kind := readError(err)
			actions := map[string]string{
				"expired": "The disposable result bytes are missing. This request_id cannot execute again; use a new request_id only after deciding to execute again on the current basis.",
				"corrupt": "The cached result bytes failed digest verification. Do not trust or rerun this request_id; use a new request_id only after deciding to execute again on the current basis.",
			}
			action := actions[kind]
			if action == "" {
				action = "The cached result cannot be read. This request_id cannot execute again; use a new request_id only after a deliberate new-run decision."
			}
			reclassified := store.CaptureClaim{Kind: kind, Ref: claim.Ref, Terminal: claim.Terminal, Action: action}
			return s.captureClaimFailure(ctx, q, reclassified)
		}
		return s.captureReplay(ctx, q, d, claim.Terminal)
	}
	if claim.Kind == "completed_unavailable" && claim.Terminal != nil {
		terminal := *claim.Terminal
		assessment := s.captureCurrentAssessment(ctx, q, terminal.Basis, terminal.Basis["expected_contract_digest"], terminal.CleanupUncertain)
		return captureTerminalResponse(terminal, q.RequestID, true, "confirmed", assessment, terminal.ResultKind, terminal.ResultKind != check.Passed)
	}
	return s.captureClaimFailure(ctx, q, claim)
}

func (s Service) captureReplay(ctx context.Context, q Request, d delivery.Document, terminal *store.CaptureTerminal) delivery.Response {
	basis := d.Basis
	if terminal != nil {
		basis = terminal.Basis
	}
	expectedDigest := basis["expected_contract_digest"]
	if expectedDigest == "" {
		part, err := d.Member("expected")
		if err == nil && part.Media == "json" {
			expectedDigest = legacyExpectedContractDigest(part.Raw)
		}
	}
	cleanupUncertain := terminal != nil && terminal.CleanupUncertain
	assessment := s.captureCurrentAssessment(ctx, q, basis, expectedDigest, cleanupUncertain)
	var summary map[string]any
	if terminal != nil {
		summary = captureTerminalSummary(*terminal, q.RequestID, true, "confirmed", assessment)
	} else {
		summary = legacyCaptureReplaySummary(d.Summary, q.RequestID, assessment)
	}
	summary["continuation_unavailable"] = d.Ref == ""
	d.Summary = rawJSON(summary)
	d.Limits = append(d.Limits, "Replay did not execute a project test; current_basis is a non-executing local assessment and the original stored result remains historical")
	if cleanupUncertain {
		d.Diagnostics = append([]carrier.Diagnostic{{Code: "cleanup_uncertain", Message: "Process-group cleanup was not certified; descendants may still change the basis, so retry currentness remains unknown.", Severity: "warning"}}, d.Diagnostics...)
	}
	return delivery.Present(d, delivery.Request{Format: delivery.Format, Operation: "read", Ref: d.Ref, View: "summary"})
}

// Old completion receipts still point to immutable cached documents. Their
// flat summary exceeds delivery's 16-key view, so select the replay facts in
// a bounded presentation without rewriting the cached bytes or parts.
func legacyCaptureReplaySummary(raw []byte, requestID, assessment string) map[string]any {
	original := object(delivery.Value(raw))
	process := map[string]any{}
	for _, key := range []string{"started", "exit_code", "environment_failure", "stdout_bytes", "stderr_bytes", "stdout_digest", "stderr_digest", "output_exceeded", "process_output_complete"} {
		if value, present := original[key]; present {
			process[key] = value
		}
	}
	return map[string]any{
		"application_outcome":            original["application_outcome"],
		"reason_code":                    original["reason_code"],
		"selector":                       original["selector"],
		"scope":                          original["scope"],
		"declared_check_binding":         original["declared_check_binding"],
		"request_id_digest":              carrier.Digest([]byte(requestID)),
		"process_accounting":             process,
		"recorded_post_run_basis":        original["current_basis"],
		"recorded_post_run_basis_scope":  "immediately_after_capture",
		"current_basis":                  assessment,
		"basis_comparison_scope":         "non_executing_retry",
		"current_basis_assessment":       assessment,
		"current_basis_assessment_scope": "non_executing_retry",
		"execution_replay":               true,
	}
}

func (s Service) captureCurrentAssessment(ctx context.Context, q Request, basis map[string]string, expectedDigest string, cleanupUncertain bool) string {
	if cleanupUncertain {
		return "unknown"
	}
	if basis["memory_generation"] == "" || basis["code_basis"] == "" || expectedDigest == "" {
		return "unknown"
	}
	view, err := s.memory().Read(ctx)
	if err != nil || view.Coverage != "complete" {
		return "unknown"
	}
	if view.Generation != basis["memory_generation"] {
		return "changed"
	}
	current, _, index, _, err := s.prepareCheck(q, view)
	if err != nil {
		return "unknown"
	}
	if index.Basis != basis["code_basis"] {
		return "changed"
	}
	currentBytes, err := json.Marshal(current)
	if err != nil {
		return "unknown"
	}
	if carrier.Digest(currentBytes) != expectedDigest {
		return "changed"
	}
	return "same"
}

func (s Service) captureClaimFailure(ctx context.Context, q Request, claim store.CaptureClaim) delivery.Response {
	r := delivery.Error("capture_"+claim.Kind, claim.Action)
	r.Operation = "check"
	r.Delivery.NoNext = "This request_id cannot start another test. Inspect the original attempt; choose a new ID only for a deliberate new run."
	if claim.Kind == "pending" {
		r.Delivery.NoNext = "The original may still run. Retry the identical request_id later to recover completion; a new ID is only a deliberate new run after inspection."
	}
	if claim.Terminal == nil {
		return r
	}
	terminal := *claim.Terminal
	assessment := s.captureCurrentAssessment(ctx, q, terminal.Basis, terminal.Basis["expected_contract_digest"], terminal.CleanupUncertain)
	return captureTerminalFailureResponse(terminal, q.RequestID, assessment, claim.Kind, claim.Action)
}

func captureTerminalFailureResponse(terminal store.CaptureTerminal, requestID, assessment, kind, action string) delivery.Response {
	response := captureTerminalResponse(terminal, requestID, true, "confirmed", assessment, "capture_"+kind, true)
	response.Diagnostics[0] = carrier.Diagnostic{Code: "capture_" + kind, Message: boundedCaptureAction(action), Severity: "error"}
	response.Delivery.NoNext = "This request_id cannot start another test. Inspect the original attempt; choose a new ID only for a deliberate new run."
	return response
}

func boundedCaptureAction(action string) string {
	if len(action) <= 240 {
		return action
	}
	return "The cached result cannot be read. This request_id cannot execute again; use a new request_id only after a deliberate new-run decision."
}
