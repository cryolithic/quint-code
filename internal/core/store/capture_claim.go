package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

// Execution receipts survive disposal of .cache/disclosure. They are neither
// canonical project memory nor runtime scratch: deleting them loses the guard
// against executing an already attempted request_id.
const captureClaimDir = ".capture-receipts"
const captureClaimFileLimit = 4096
const CaptureProcessDiagnosticLimit = 256

var errCaptureResultCorrupt = errors.New("capture_result_corrupt: cached result digest mismatch")

// CaptureClaim describes a retained execution receipt. A pending claim may
// represent a process that ran before its result was recorded. Retrying it must
// never execute the project command again. An expired completed claim retains
// that prohibition even when its separately cached result bytes are gone.
type CaptureClaim struct {
	Kind     string           `json:"kind"` // absent, started, pending, completed, completed_unavailable, conflict, expired, corrupt
	Ref      string           `json:"ref,omitempty"`
	Terminal *CaptureTerminal `json:"terminal,omitempty"`
	Action   string           `json:"action,omitempty"`
}

// CaptureTerminal is the bounded outcome retained after a claimed execution.
// It contains no output bytes or assertion text. Exact full output still needs
// the disposable result ref or an explicit retained attachment.
type CaptureTerminal struct {
	ResultKind            string            `json:"result_kind"`
	ReasonCode            string            `json:"reason_code"`
	Basis                 map[string]string `json:"basis"`
	RecordedPostRunBasis  string            `json:"recorded_post_run_basis"`
	DeclaredCheckBinding  bool              `json:"declared_check_binding"`
	ProcessStarted        bool              `json:"process_started"`
	ExitCode              *int              `json:"exit_code"`
	ProcessFailure        string            `json:"process_failure,omitempty"`
	ProcessDiagnostic     string            `json:"process_diagnostic,omitempty"`
	StdoutBytes           int64             `json:"stdout_bytes"`
	StderrBytes           int64             `json:"stderr_bytes"`
	StdoutDigest          string            `json:"stdout_digest"`
	StderrDigest          string            `json:"stderr_digest"`
	OutputExceeded        bool              `json:"output_exceeded"`
	PipeDrainUncertain    bool              `json:"pipe_drain_uncertain"`
	CleanupUncertain      bool              `json:"cleanup_uncertain"`
	StdoutDrainComplete   bool              `json:"stdout_drain_complete"`
	StderrDrainComplete   bool              `json:"stderr_drain_complete"`
	ProcessOutputComplete bool              `json:"process_output_complete"`
}

type captureClaimRecord struct {
	Format        string `json:"format"`
	RequestID     string `json:"request_id"`
	PayloadDigest string `json:"payload_digest"`
}

type captureCompleteRecord struct {
	Format        string           `json:"format"`
	RequestID     string           `json:"request_id"`
	PayloadDigest string           `json:"payload_digest"`
	Ref           string           `json:"ref,omitempty"`
	Terminal      *CaptureTerminal `json:"terminal,omitempty"`
}

// LookupCapture checks an earlier attempt before the application prepares a
// new execution. An absent project store is an absent claim without effects.
func (s Store) LookupCapture(ctx context.Context, requestID, payloadDigest string) (CaptureClaim, error) {
	if err := validateCaptureKey(requestID, payloadDigest); err != nil {
		return CaptureClaim{}, err
	}
	h, err := s.open(ctx, false)
	if errors.Is(err, fs.ErrNotExist) {
		_, absentErr := os.Lstat(filepath.Join(s.Root, ".haft"))
		info, rootErr := os.Stat(s.Root)
		if rootErr == nil && info.IsDir() && errors.Is(absentErr, fs.ErrNotExist) {
			return CaptureClaim{Kind: "absent"}, nil
		}
	}
	if err != nil {
		return CaptureClaim{}, err
	}
	defer h.close()
	claim, err := readCaptureClaim(h, requestID, payloadDigest)
	if err != nil {
		return CaptureClaim{}, err
	}
	return claim, h.checkIdentity()
}

// BeginCapture atomically claims one request before any project process is
// started. The writer lock is released on return and is never held during the
// process run. One request ID can be used only with one exact payload digest.
func (s Store) BeginCapture(ctx context.Context, requestID, payloadDigest string) (CaptureClaim, error) {
	if err := validateCaptureKey(requestID, payloadDigest); err != nil {
		return CaptureClaim{}, err
	}
	h, err := s.open(ctx, true)
	if err != nil {
		return CaptureClaim{}, err
	}
	defer h.close()
	if err := ensureDirs(h.root, captureClaimDir, 0700); err != nil {
		return CaptureClaim{}, err
	}
	claim, err := readCaptureClaim(h, requestID, payloadDigest)
	if err != nil || claim.Kind != "absent" {
		return claim, err
	}
	if err := ctx.Err(); err != nil {
		return CaptureClaim{}, err
	}
	record := captureClaimRecord{"haft.capture-claim/1", requestID, payloadDigest}
	raw, err := json.Marshal(record)
	if err != nil {
		return CaptureClaim{}, err
	}
	raw = append(raw, '\n')
	if err := publishBytes(h.root, captureClaimPath(requestID), raw); err != nil {
		return CaptureClaim{}, err
	}
	if err := h.checkIdentity(); err != nil {
		return CaptureClaim{}, err
	}
	return CaptureClaim{Kind: "started"}, nil
}

// CompleteCapture records a result only after its exact transient bytes have
// been published. It is safe to retry after a lost completion reply. A missing
// result cache never turns the same request into a fresh execution claim.
func (s Store) CompleteCapture(ctx context.Context, requestID, payloadDigest, ref string) error {
	return s.completeCapture(ctx, requestID, payloadDigest, ref, nil)
}

// CompleteCaptureOutcome records bounded facts even if the disposable result
// could not be published. An empty ref means continuation is unavailable;
// neither this receipt nor a retry grants another execution.
func (s Store) CompleteCaptureOutcome(ctx context.Context, requestID, payloadDigest, ref string, terminal CaptureTerminal) error {
	if err := CheckCaptureTerminalCapacity(requestID, payloadDigest, terminal); err != nil {
		return err
	}
	return s.completeCapture(ctx, requestID, payloadDigest, ref, &terminal)
}

// CheckCaptureTerminalCapacity is the pure pre-launch admission check for the
// worst-case bounded safety receipt. It prevents a valid but oversized claim
// address from consuming a request ID without any representable completion.
func CheckCaptureTerminalCapacity(requestID, payloadDigest string, terminal CaptureTerminal) error {
	if err := validateCaptureKey(requestID, payloadDigest); err != nil {
		return err
	}
	if err := validateCaptureTerminal(terminal); err != nil {
		return err
	}
	reservedRef := "result:" + carrier.Digest(nil)
	record := captureCompleteRecord{Format: "haft.capture-complete/2", RequestID: requestID, PayloadDigest: payloadDigest, Ref: reservedRef, Terminal: &terminal}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(raw)+1 > captureClaimFileLimit {
		return fmt.Errorf("capture_terminal_limit: bounded completion receipt exceeds %d bytes", captureClaimFileLimit)
	}
	return nil
}

func (s Store) completeCapture(ctx context.Context, requestID, payloadDigest, ref string, terminal *CaptureTerminal) error {
	if err := validateCaptureKey(requestID, payloadDigest); err != nil {
		return err
	}
	if ref != "" {
		if err := validateCaptureRef(ref); err != nil {
			return err
		}
	}
	if ref == "" && terminal == nil {
		return fmt.Errorf("invalid_capture_result_ref: result ref or terminal facts required")
	}
	h, err := s.open(ctx, true)
	if err != nil {
		return err
	}
	defer h.close()
	claim, err := readCaptureClaim(h, requestID, payloadDigest)
	if err != nil {
		return err
	}
	if claim.Kind == "conflict" {
		return fmt.Errorf("capture_request_conflict: request_id already has a different payload")
	}
	if claim.Kind == "absent" {
		return fmt.Errorf("capture_claim_missing: begin the exact request before executing")
	}
	if claim.Kind == "completed" || claim.Kind == "completed_unavailable" || claim.Kind == "expired" || claim.Kind == "corrupt" {
		if claim.Ref != ref || !reflect.DeepEqual(claim.Terminal, terminal) {
			return fmt.Errorf("capture_result_conflict: request_id already has a different result")
		}
		if claim.Kind == "expired" {
			return fmt.Errorf("capture_result_expired: cached result is unavailable; do not rerun with the same request_id")
		}
		if claim.Kind == "corrupt" {
			return errCaptureResultCorrupt
		}
		return nil
	}
	if ref != "" {
		if err := verifyCaptureRef(h, ref); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	format := "haft.capture-complete/1"
	if terminal != nil {
		format = "haft.capture-complete/2"
	}
	record := captureCompleteRecord{Format: format, RequestID: requestID, PayloadDigest: payloadDigest, Ref: ref, Terminal: terminal}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if len(raw) > captureClaimFileLimit {
		return fmt.Errorf("capture_terminal_limit: bounded completion receipt exceeds %d bytes", captureClaimFileLimit)
	}
	if err := publishBytes(h.root, captureCompletePath(requestID), raw); err != nil {
		return err
	}
	return h.checkIdentity()
}

func readCaptureClaim(h *handle, requestID, payloadDigest string) (CaptureClaim, error) {
	claimPath := captureClaimPath(requestID)
	raw, err := regularBytes(h.root, claimPath, captureClaimFileLimit)
	if errors.Is(err, fs.ErrNotExist) {
		_, completeErr := regularBytes(h.root, captureCompletePath(requestID), captureClaimFileLimit)
		if errors.Is(completeErr, fs.ErrNotExist) {
			return CaptureClaim{Kind: "absent"}, nil
		}
		if completeErr == nil {
			return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: completion without claim")
		}
		return CaptureClaim{}, completeErr
	}
	if err != nil {
		return CaptureClaim{}, err
	}
	var record captureClaimRecord
	if err := decodeJSON(raw, &record); err != nil {
		return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: %w", err)
	}
	if record.Format != "haft.capture-claim/1" || record.RequestID != requestID || !carrier.ValidDigest(record.PayloadDigest) {
		return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: identity or format mismatch")
	}
	if record.PayloadDigest != payloadDigest {
		return CaptureClaim{Kind: "conflict", Action: "Use the original payload with this request_id or choose a new request_id for a new execution."}, nil
	}
	complete, err := regularBytes(h.root, captureCompletePath(requestID), captureClaimFileLimit)
	if errors.Is(err, fs.ErrNotExist) {
		return CaptureClaim{Kind: "pending", Action: "The original execution may still be running or its outcome may be unknown. Retry the identical request_id later to recover completion; choose a new ID only for a deliberate new run after inspecting the original."}, nil
	}
	if err != nil {
		return CaptureClaim{}, err
	}
	var finished captureCompleteRecord
	if err := decodeJSON(complete, &finished); err != nil {
		return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: %w", err)
	}
	if finished.RequestID != requestID || finished.PayloadDigest != payloadDigest {
		return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: completion identity mismatch")
	}
	if finished.Format != "haft.capture-complete/1" && finished.Format != "haft.capture-complete/2" {
		return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: completion format mismatch")
	}
	if finished.Format == "haft.capture-complete/1" && finished.Terminal != nil {
		return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: legacy completion has terminal facts")
	}
	if finished.Format == "haft.capture-complete/2" {
		if finished.Terminal == nil {
			return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: terminal facts missing")
		}
		if err := validateCaptureTerminal(*finished.Terminal); err != nil {
			return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: %w", err)
		}
	}
	if finished.Ref == "" && finished.Terminal != nil {
		return CaptureClaim{Kind: "completed_unavailable", Terminal: finished.Terminal, Action: "The original outcome is recorded, but its disposable output continuation was not published. This request_id cannot run again."}, nil
	}
	if err := validateCaptureRef(finished.Ref); err != nil {
		return CaptureClaim{}, fmt.Errorf("capture_claim_corrupt: %w", err)
	}
	err = verifyCaptureRef(h, finished.Ref)
	if errors.Is(err, fs.ErrNotExist) {
		return CaptureClaim{Kind: "expired", Ref: finished.Ref, Terminal: finished.Terminal, Action: "The disposable result bytes are missing. This request_id cannot execute again; use a new request_id only after deciding to execute again on the current basis."}, nil
	}
	if errors.Is(err, errCaptureResultCorrupt) {
		return CaptureClaim{Kind: "corrupt", Ref: finished.Ref, Terminal: finished.Terminal, Action: "The cached result bytes failed digest verification. Do not trust or rerun this request_id; use a new request_id only after deciding to execute again on the current basis."}, nil
	}
	if err != nil {
		return CaptureClaim{}, err
	}
	return CaptureClaim{Kind: "completed", Ref: finished.Ref, Terminal: finished.Terminal}, nil
}

func validateCaptureTerminal(terminal CaptureTerminal) error {
	kinds := map[string]bool{"passed": true, "assertion_failure": true, "skipped": true, "not_run": true, "environment_failure": true, "unattributable": true}
	if !kinds[terminal.ResultKind] || len(terminal.ReasonCode) > 128 || len(terminal.ProcessFailure) > 64 || len(terminal.ProcessDiagnostic) > CaptureProcessDiagnosticLimit || !utf8.ValidString(terminal.ProcessDiagnostic) || !captureDiagnosticSafe(terminal.ProcessDiagnostic) {
		return fmt.Errorf("invalid_capture_terminal: outcome facts are unbounded or unknown")
	}
	if terminal.ProcessDiagnostic != "" && terminal.ProcessFailure == "" {
		return fmt.Errorf("invalid_capture_terminal: process diagnostic requires process failure")
	}
	if terminal.RecordedPostRunBasis != "same" && terminal.RecordedPostRunBasis != "changed" && terminal.RecordedPostRunBasis != "unknown" {
		return fmt.Errorf("invalid_capture_terminal: post-run basis must be same, changed or unknown")
	}
	if terminal.CleanupUncertain && terminal.RecordedPostRunBasis != "unknown" {
		return fmt.Errorf("invalid_capture_terminal: unsettled process cleanup requires unknown post-run basis")
	}
	if terminal.StdoutBytes < 0 || terminal.StderrBytes < 0 || !carrier.ValidDigest(terminal.StdoutDigest) || !carrier.ValidDigest(terminal.StderrDigest) {
		return fmt.Errorf("invalid_capture_terminal: process accounting is malformed")
	}
	if terminal.ProcessOutputComplete && (terminal.ProcessFailure != "" || terminal.OutputExceeded || terminal.PipeDrainUncertain || terminal.CleanupUncertain || !terminal.StdoutDrainComplete || !terminal.StderrDrainComplete) {
		return fmt.Errorf("invalid_capture_terminal: complete process output contradicts observed pipe drains")
	}
	keys := []string{"memory_generation", "code_basis", "implementation_basis", "check_basis", "dependency_basis", "expected_contract_digest", "claim_ref_digest"}
	claimRef := terminal.Basis["claim_ref"]
	if len(terminal.Basis) != len(keys)+1 || len(claimRef) == 0 {
		return fmt.Errorf("invalid_capture_terminal: exact basis is missing or unbounded")
	}
	if carrier.Digest([]byte(claimRef)) != terminal.Basis["claim_ref_digest"] {
		return fmt.Errorf("invalid_capture_terminal: exact claim ref digest differs")
	}
	for _, key := range keys {
		if !carrier.ValidDigest(terminal.Basis[key]) {
			return fmt.Errorf("invalid_capture_terminal: %s digest is malformed", key)
		}
	}
	return nil
}

func captureDiagnosticSafe(diagnostic string) bool {
	for _, value := range diagnostic {
		if value == '"' || value == '\\' || value == '<' || value == '>' || value == '&' || value == '\u2028' || value == '\u2029' || unicode.IsControl(value) {
			return false
		}
	}
	return true
}

// ValidateCaptureRequestID is shared by application admission and the receipt
// store, so a malformed identity is rejected as caller input before any IO.
func ValidateCaptureRequestID(requestID string) error {
	if len(requestID) == 0 || len(requestID) > 512 || !utf8.ValidString(requestID) || strings.TrimSpace(requestID) != requestID || strings.ContainsFunc(requestID, unicode.IsControl) {
		return fmt.Errorf("invalid_capture_request_id: request_id must be 1–512 non-control UTF-8 bytes without surrounding whitespace")
	}
	return nil
}

func validateCaptureKey(requestID, payloadDigest string) error {
	if err := ValidateCaptureRequestID(requestID); err != nil {
		return err
	}
	if !carrier.ValidDigest(payloadDigest) {
		return fmt.Errorf("invalid_capture_payload_digest: full sha256 digest required")
	}
	return nil
}

func validateCaptureRef(ref string) error {
	if !strings.HasPrefix(ref, "result:") || !carrier.ValidDigest(strings.TrimPrefix(ref, "result:")) {
		return fmt.Errorf("invalid_capture_result_ref: result:sha256 digest required")
	}
	return nil
}

func captureClaimPath(requestID string) string {
	digest := carrier.Digest([]byte(requestID))
	return captureClaimDir + "/" + strings.TrimPrefix(digest, "sha256:") + ".json"
}

func captureCompletePath(requestID string) string {
	return strings.TrimSuffix(captureClaimPath(requestID), ".json") + ".complete.json"
}

func verifyCaptureRef(h *handle, ref string) error {
	digest := strings.TrimPrefix(ref, "result:")
	p := ".cache/disclosure/" + strings.TrimPrefix(digest, "sha256:") + ".json"
	raw, err := regularBytes(h.root, p, transientLimit)
	if err != nil {
		return err
	}
	if carrier.Digest(raw) != digest {
		return errCaptureResultCorrupt
	}
	return nil
}
