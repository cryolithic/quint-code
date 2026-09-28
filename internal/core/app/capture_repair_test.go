package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/check"
	"github.com/m0n0x41d/haft/internal/core/checkrunner"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

const cacheFilledDuringTest = `package answer
import ("os"; "testing")
func TestAnswer(t *testing.T) {
	if err := os.MkdirAll(".haft/.runtime", 0700); err != nil { t.Fatal(err) }
	f, err := os.OpenFile(".haft/.runtime/capture-runs.marker", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil { t.Fatal(err) }
	if _, err := f.Write([]byte("run\n")); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	if Answer() != 1 { t.Fatal("answer must be one") }
	if err := os.MkdirAll(".haft/.cache/disclosure", 0700); err != nil { t.Fatal(err) }
	pad, err := os.Create(".haft/.cache/disclosure/pad.json")
	if err != nil { t.Fatal(err) }
	if err := pad.Truncate(512 << 20); err != nil { t.Fatal(err) }
	if err := pad.Close(); err != nil { t.Fatal(err) }
}
`

const cacheAndReceiptBlockedDuringTest = `package answer
import ("os"; "testing")
func TestAnswer(t *testing.T) {
	if err := os.MkdirAll(".haft/.runtime", 0700); err != nil { t.Fatal(err) }
	f, err := os.OpenFile(".haft/.runtime/capture-runs.marker", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil { t.Fatal(err) }
	if _, err := f.Write([]byte("run\n")); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	if Answer() != 1 { t.Fatal("answer must be one") }
	if err := os.MkdirAll(".haft/.cache/disclosure", 0700); err != nil { t.Fatal(err) }
	pad, err := os.Create(".haft/.cache/disclosure/pad.json")
	if err != nil { t.Fatal(err) }
	if err := pad.Truncate(512 << 20); err != nil { t.Fatal(err) }
	if err := pad.Close(); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(".haft/.capture-receipts/COMPLETE_NAME", 0700); err != nil { t.Fatal(err) }
}
`

const receiptBlockedDuringTest = `package answer
import ("os"; "testing")
func TestAnswer(t *testing.T) {
	if err := os.MkdirAll(".haft/.runtime", 0700); err != nil { t.Fatal(err) }
	f, err := os.OpenFile(".haft/.runtime/capture-runs.marker", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil { t.Fatal(err) }
	if _, err := f.Write([]byte("run\n")); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	if Answer() != 1 { t.Fatal("answer must be one") }
	if err := os.MkdirAll(".haft/.capture-receipts/COMPLETE_NAME", 0700); err != nil { t.Fatal(err) }
}
`

func requireTerminalSummary(t *testing.T, response delivery.Response, replay bool, assessment, persistence string) map[string]any {
	t.Helper()
	if size := delivery.Size(response); size > delivery.Budget {
		t.Fatalf("terminal reply exceeds %d bytes: %d", delivery.Budget, size)
	}
	if response.Delivery.Catalog != nil || len(response.Delivery.Available) != 0 || response.Delivery.Next != nil {
		t.Fatalf("terminal reply advertised a fabricated continuation: %+v", response.Delivery)
	}
	summary := object(response.Data)
	if summary["application_outcome"] != check.Passed || summary["execution_replay"] != replay || summary["receipt_persistence"] != persistence || summary["continuation_unavailable"] != true {
		t.Fatalf("known terminal facts or continuation state missing: %+v", summary)
	}
	if summary["current_basis"] != assessment || summary["recorded_post_run_basis"] != "same" {
		t.Fatalf("recorded/current basis collapsed: %+v", summary)
	}
	if replay && summary["current_basis_assessment"] != assessment {
		t.Fatalf("non-executing replay assessment missing: %+v", summary)
	}
	basis := response.Basis
	for _, key := range []string{"memory_generation", "code_basis", "implementation_basis", "check_basis", "dependency_basis", "expected_contract_digest", "claim_ref_digest"} {
		if !carrier.ValidDigest(basis[key]) {
			t.Fatalf("exact %s missing in no-ref reply: %+v", key, basis)
		}
	}
	if basis["claim_ref"] == "" || carrier.Digest([]byte(basis["claim_ref"])) != basis["claim_ref_digest"] {
		t.Fatalf("exact claim address was lost or excerpted: %+v", basis)
	}
	process := object(summary["process_accounting"])
	if process["started"] != true || process["process_output_complete"] != true || process["output_exceeded"] != false ||
		process["stdout_drain_complete"] != true || process["stderr_drain_complete"] != true || process["pipe_drain_uncertain"] != false {
		t.Fatalf("process completion facts absent: %+v", process)
	}
	if !carrier.ValidDigest(str(process["stdout_digest"])) || !carrier.ValidDigest(str(process["stderr_digest"])) || process["stdout_bytes"] == nil || process["stderr_bytes"] == nil || process["exit_code"] == nil {
		t.Fatalf("process byte accounting absent: %+v", process)
	}
	return summary
}

func TestCaptureCachePublicationFailureRetainsKnownOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test processes")
	}
	for _, afterLaunch := range []bool{false, true} {
		name := "already-full"
		source := captureMarkerTest
		if afterLaunch {
			name = "filled-after-launch"
			source = cacheFilledDuringTest
		}
		t.Run(name, func(t *testing.T) {
			s, q := captureFixture(t, source)
			if !afterLaunch {
				padPath := filepath.Join(s.Root, ".haft", ".cache", "disclosure", "pad.json")
				if err := os.MkdirAll(filepath.Dir(padPath), 0700); err != nil {
					t.Fatal(err)
				}
				pad, err := os.Create(padPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := pad.Truncate(512 << 20); err != nil {
					t.Fatal(err)
				}
				if err := pad.Close(); err != nil {
					t.Fatal(err)
				}
			}
			first := public(t, s, q)
			if first.Kind != check.Passed || captureRuns(t, s.Root) != 1 || first.Delivery.NoNext == "" {
				t.Fatalf("known outcome disappeared after cache failure: %+v runs=%d", first, captureRuns(t, s.Root))
			}
			firstSummary := requireTerminalSummary(t, first, false, "same", "confirmed")
			retry := public(t, Service{Root: s.Root}, q)
			if retry.Kind != check.Passed || captureRuns(t, s.Root) != 1 {
				t.Fatalf("same ID reran or lost terminal facts after restart: %+v", retry)
			}
			retrySummary := requireTerminalSummary(t, retry, true, "same", "confirmed")
			if !sameJSON(first.Basis, retry.Basis) || !sameJSON(firstSummary["process_accounting"], retrySummary["process_accounting"]) {
				t.Fatal("cacheless replay changed original basis or process accounting")
			}
		})
	}
}

func TestCaptureCacheAndReceiptFailureStillReturnsKnownOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	completeName := strings.TrimPrefix(carrier.Digest([]byte("capture-fixture")), "sha256:") + ".complete.json"
	source := strings.Replace(cacheAndReceiptBlockedDuringTest, "COMPLETE_NAME", completeName, 1)
	s, q := captureFixture(t, source)
	first := public(t, s, q)
	if first.Kind != check.Passed || captureRuns(t, s.Root) != 1 {
		t.Fatalf("both publication paths failed and known outcome vanished: %+v", first)
	}
	requireTerminalSummary(t, first, false, "same", "uncertain")
	retry := public(t, Service{Root: s.Root}, q)
	if retry.Kind != "unavailable" || captureRuns(t, s.Root) != 1 || !strings.Contains(retry.Delivery.NoNext, "Receipt state is unavailable") {
		t.Fatalf("failed receipt path reran or suggested a fresh effect: %+v", retry)
	}
}

func TestCaptureReceiptFailureKeepsCachedResultAndVisibleUncertainty(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	completeName := strings.TrimPrefix(carrier.Digest([]byte("capture-fixture")), "sha256:") + ".complete.json"
	source := strings.Replace(receiptBlockedDuringTest, "COMPLETE_NAME", completeName, 1)
	s, q := captureFixture(t, source)
	first := public(t, s, q)
	if first.Kind != check.Passed || captureRuns(t, s.Root) != 1 {
		t.Fatalf("known cached outcome disappeared when receipt completion failed: %+v", first)
	}
	if captureRef(t, first) == "" || object(first.Data)["receipt_persistence"] != "uncertain" || object(first.Data)["continuation_unavailable"] != false {
		t.Fatalf("real ref or receipt uncertainty missing: %+v", first)
	}
	retry := public(t, Service{Root: s.Root}, q)
	if retry.Kind != "unavailable" || captureRuns(t, s.Root) != 1 {
		t.Fatalf("uncertain receipt permitted same-ID rerun: %+v", retry)
	}
}

func TestCaptureReplaySeparatesHistoricalOutcomeFromCurrentBasis(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	s, q := captureFixture(t, captureMarkerTest)
	first := public(t, s, q)
	if first.Kind != check.Passed {
		t.Fatalf("baseline did not pass: %+v", first)
	}
	ref := captureRef(t, first)
	original, err := s.memory().ReadTransient(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	runnerRegressionWrite(t, s.Root, "answer.go", "package answer\nfunc Answer() int { return 2 }\n")
	changed := public(t, Service{Root: s.Root}, q)
	if changed.Kind != check.Passed || captureRef(t, changed) != ref || captureRuns(t, s.Root) != 1 {
		t.Fatalf("changed-code same ID reran or erased history: %+v", changed)
	}
	codeSummary := object(changed.Data)
	if codeSummary["execution_replay"] != true || codeSummary["recorded_post_run_basis"] != "same" || codeSummary["current_basis_assessment"] != "changed" || codeSummary["current_basis"] != "changed" {
		t.Fatalf("historical pass appeared current after code change: %+v", codeSummary)
	}
	stored, err := s.memory().ReadTransient(context.Background(), ref)
	if err != nil || !bytes.Equal(original, stored) {
		t.Fatalf("replay changed captured bytes: %v", err)
	}
	historicalRead := public(t, s, Request{Operation: "read", Ref: ref})
	if object(historicalRead.Data)["current_basis"] != "same" || object(historicalRead.Data)["basis_comparison_scope"] != "immediately_after_capture" || object(historicalRead.Data)["execution_replay"] == true {
		t.Fatalf("exact historical read was rewritten as replay: %+v", historicalRead.Data)
	}
	runnerRegressionWrite(t, s.Root, "answer.go", "package answer\nfunc Answer() int { return 1 }\n")
	view := readView(t, s)
	var document carrier.Document
	for _, candidate := range view.Documents {
		if candidate.Record.Slug == "answer" {
			document = candidate
		}
	}
	if document.Record.ID == "" {
		t.Fatal("fixture claim missing")
	}
	old := document.Record.ID + "@" + document.Edition
	successor := document.Record
	successor.ID = ""
	successor.Claims[0].Text += " Clarified after the original execution."
	successor.Supersedes = []string{old}
	successor.SupersedeReason = "Clarify the current claim"
	run(t, s, Request{Operation: "remember", RequestID: "capture-claim-successor", Carrier: encode(t, successor, document.Body), ExpectedGeneration: view.Generation, ExpectedHeads: []string{old}}, "written")
	claimChanged := public(t, s, q)
	claimSummary := object(claimChanged.Data)
	if claimChanged.Kind != check.Passed || claimSummary["current_basis"] != "changed" || claimSummary["recorded_post_run_basis"] != "same" || captureRuns(t, s.Root) != 1 {
		t.Fatalf("changed claim inherited a current pass or reran: %+v", claimChanged)
	}
}

func TestCaptureReplayUnknownBasisAndInvalidIDDoNotExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	s, q := captureFixture(t, captureMarkerTest)
	for _, invalid := range []string{"", " leading", strings.Repeat("x", 513)} {
		bad := q
		bad.RequestID = invalid
		response := public(t, s, bad)
		if response.Kind != "invalid_capture" || len(response.Diagnostics) == 0 || !strings.Contains(response.Diagnostics[0].Message, "1–512") || captureRuns(t, s.Root) != 0 {
			t.Fatalf("invalid ID was classified as infrastructure or launched: %+v", response)
		}
	}
	first := public(t, s, q)
	if first.Kind != check.Passed || captureRuns(t, s.Root) != 1 {
		t.Fatalf("baseline not captured: %+v", first)
	}
	if err := os.Remove(filepath.Join(s.Root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	unknown := public(t, Service{Root: s.Root}, q)
	summary := object(unknown.Data)
	if unknown.Kind != check.Passed || summary["execution_replay"] != true || summary["current_basis"] != "unknown" || summary["recorded_post_run_basis"] != "same" || captureRuns(t, s.Root) != 1 {
		t.Fatalf("unavailable current basis appeared as a fresh pass: %+v", unknown)
	}
}

func TestCaptureCancellationPersistsKnownTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	source := strings.Replace(captureMarkerTest, "t.Log(strings.Repeat(\"fixture-output-\", 1600))", "time.Sleep(10 * time.Second)", 1)
	source = strings.Replace(source, "\"strings\"", "\"time\"", 1)
	s, q := captureFixture(t, source)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan delivery.Response, 1)
	go func() {
		q.Format = delivery.Format
		result <- s.Call(ctx, q)
	}()
	deadline := time.Now().Add(15 * time.Second)
	for captureRuns(t, s.Root) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if captureRuns(t, s.Root) != 1 {
		t.Fatal("test process did not start before cancellation")
	}
	cancel()
	first := <-result
	if first.Kind == check.Passed || first.Kind == "capture_pending" || captureRuns(t, s.Root) != 1 {
		t.Fatalf("canceled known run stayed pending or became a pass: %+v", first)
	}
	retry := public(t, Service{Root: s.Root}, q)
	if retry.Kind != first.Kind || object(retry.Data)["execution_replay"] != true || captureRuns(t, s.Root) != 1 {
		t.Fatalf("same-ID retry after cancellation reran or lost terminal receipt: %+v", retry)
	}
}

func TestCaptureLongClaimRefHasExactBoundedReplyOrFailsBeforeLaunch(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated fixture setup")
	}
	maxRefLength := maxAdmittedCaptureClaimRefLength(strings.Repeat("r", 512))
	if maxRefLength < 256 {
		t.Fatalf("256-byte exact claim ref is no longer representable: max=%d", maxRefLength)
	}
	// This fixture's canonical claim address has a 95-byte prefix before its
	// claim ID. The assertion below keeps that relation explicit if it changes.
	boundedClaimLength := maxRefLength - 95
	for _, claimLength := range []int{boundedClaimLength, 3000} {
		t.Run(strconv.Itoa(claimLength), func(t *testing.T) {
			s, q := captureFixture(t, captureMarkerTest)
			view := readView(t, s)
			long := view.Documents[0].Record
			long.ID = ""
			long.Slug = "long-claim"
			long.Title = "Long claim identifier"
			long.Claims[0].ID = strings.Repeat("c", claimLength)
			long.Claims[0].Text = "Answer returns one under a valid long claim identifier."
			run(t, s, Request{Operation: "remember", RequestID: "long-capture-claim", Carrier: encode(t, long, nil)}, "written")
			q.Ref = "spec:long-claim#" + long.Claims[0].ID
			q.RequestID = strings.Repeat("r", 512)
			if claimLength == boundedClaimLength {
				padPath := filepath.Join(s.Root, ".haft", ".cache", "disclosure", "pad.json")
				if err := os.MkdirAll(filepath.Dir(padPath), 0700); err != nil {
					t.Fatal(err)
				}
				pad, err := os.Create(padPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := pad.Truncate(512 << 20); err != nil {
					t.Fatal(err)
				}
				if err := pad.Close(); err != nil {
					t.Fatal(err)
				}
				first := public(t, s, q)
				if first.Kind != check.Passed || len(first.Basis["claim_ref"]) != maxRefLength || captureRuns(t, s.Root) != 1 {
					t.Fatalf("near-limit exact claim ref was lost: length=%d max=%d response=%+v", len(first.Basis["claim_ref"]), maxRefLength, first)
				}
				requireTerminalSummary(t, first, false, "same", "confirmed")
				retry := public(t, Service{Root: s.Root}, q)
				requireTerminalSummary(t, retry, true, "same", "confirmed")
				if captureRuns(t, s.Root) != 1 {
					t.Fatal("near-limit same-ID retry executed twice")
				}
				if err := os.Remove(padPath); err != nil {
					t.Fatal(err)
				}
				for _, variant := range []string{"expired", "corrupt"} {
					q.RequestID = strings.Repeat(map[string]string{"expired": "e", "corrupt": "c"}[variant], 512)
					created := public(t, s, q)
					if created.Kind != check.Passed || captureRuns(t, s.Root) != 2+map[string]int{"expired": 0, "corrupt": 1}[variant] {
						t.Fatalf("near-limit %s fixture did not execute exactly once: %+v", variant, created)
					}
					ref := captureRef(t, created)
					path := filepath.Join(s.Root, ".haft", ".cache", "disclosure", strings.TrimPrefix(ref, "result:sha256:")+".json")
					if variant == "expired" {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					} else if err := os.WriteFile(path, []byte("corrupted cached result"), 0600); err != nil {
						t.Fatal(err)
					}
					lost := public(t, Service{Root: s.Root}, q)
					if lost.Kind != "capture_"+variant || captureRuns(t, s.Root) != 2+map[string]int{"expired": 0, "corrupt": 1}[variant] {
						t.Fatalf("near-limit %s retry reran or misclassified result: %+v", variant, lost)
					}
					requireTerminalSummary(t, lost, true, "same", "confirmed")
					if len(lost.Diagnostics) == 0 || !strings.Contains(lost.Diagnostics[0].Message, "new request_id") {
						t.Fatalf("%s recovery choice missing: %+v", variant, lost.Diagnostics)
					}
				}
				return
			}
			response := public(t, s, q)
			if response.Kind != "capture_capacity_exceeded" || response.Operation != "check" || len(response.Diagnostics) == 0 || response.Diagnostics[0].Code != "capture_capacity_exceeded" || !strings.Contains(response.Diagnostics[0].Message, "capture_terminal_limit") || captureRuns(t, s.Root) != 0 {
				t.Fatalf("valid but unrepresentable exact claim ref reached execution: %+v", response)
			}
			for _, route := range []string{"check/prepare", "bounded external run", "check/observe"} {
				if !strings.Contains(response.Delivery.NoNext, route) || !strings.Contains(response.Diagnostics[0].Message, route) {
					t.Fatalf("capacity refusal omitted %q: %+v", route, response)
				}
			}
			claimPath := filepath.Join(s.Root, ".haft", ".capture-receipts", strings.TrimPrefix(carrier.Digest([]byte(q.RequestID)), "sha256:")+".json")
			if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
				t.Fatalf("capacity refusal consumed request_id: %v", err)
			}
		})
	}
}

func maxAdmittedCaptureClaimRefLength(requestID string) int {
	basis := map[string]string{}
	for _, key := range []string{"memory_generation", "code_basis", "implementation_basis", "check_basis", "dependency_basis", "expected_contract_digest"} {
		basis[key] = carrier.Digest([]byte(key))
	}
	for length := 256; length < 3000; length++ {
		basis["claim_ref"] = strings.Repeat("c", length)
		basis["claim_ref_digest"] = carrier.Digest([]byte(basis["claim_ref"]))
		q := Request{RequestID: requestID}
		if err := captureTerminalCapacity(q, carrier.Digest(nil), basis); err != nil {
			return length - 1
		}
	}
	return 2999
}

func TestCaptureLegacyReceiptReplayKeepsHistoricalPartsAndVisibleAssessment(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	s, q := captureFixture(t, captureMarkerTest)
	created := public(t, s, q)
	if created.Kind != check.Passed || captureRuns(t, s.Root) != 1 {
		t.Fatalf("legacy replay fixture did not complete exactly once: %+v", created)
	}
	ref := captureRef(t, created)
	cachePath := filepath.Join(s.Root, ".haft", ".cache", "disclosure", strings.TrimPrefix(ref, "result:sha256:")+".json")
	storedBefore, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	observationRequest := capturePart(t, s, created, "observation")
	observationBefore := collectPart(t, s, observationRequest)
	completeName := strings.TrimPrefix(carrier.Digest([]byte(q.RequestID)), "sha256:") + ".complete.json"
	completePath := filepath.Join(s.Root, ".haft", ".capture-receipts", completeName)
	completeRaw, err := os.ReadFile(completePath)
	if err != nil {
		t.Fatal(err)
	}
	var completion map[string]json.RawMessage
	if err := json.Unmarshal(completeRaw, &completion); err != nil {
		t.Fatal(err)
	}
	completion["format"] = json.RawMessage(`"haft.capture-complete/1"`)
	delete(completion, "terminal")
	legacyRaw, err := json.Marshal(completion)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(completePath, append(legacyRaw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	answerPath := filepath.Join(s.Root, "answer.go")
	if err := os.WriteFile(answerPath, []byte("package answer\nfunc Answer() int { return 2 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	replayed := public(t, Service{Root: s.Root}, q)
	summary := object(replayed.Data)
	if replayed.Kind != check.Passed || summary["execution_replay"] != true || summary["recorded_post_run_basis"] != "same" ||
		summary["current_basis_assessment"] != "changed" || summary["current_basis"] != "changed" || summary["basis_comparison_scope"] != "non_executing_retry" ||
		captureRuns(t, s.Root) != 1 {
		t.Fatalf("legacy replay lost historical/current distinction or reran: %+v", replayed)
	}
	if size := delivery.Size(replayed); size > delivery.Budget {
		t.Fatalf("legacy replay exceeded delivery budget: %d", size)
	}
	storedAfter, err := os.ReadFile(cachePath)
	if err != nil || !bytes.Equal(storedBefore, storedAfter) {
		t.Fatalf("legacy replay changed cached bytes: %v", err)
	}
	if observationAfter := collectPart(t, Service{Root: s.Root}, observationRequest); !bytes.Equal(observationBefore, observationAfter) {
		t.Fatal("legacy exact-ref observation read changed after replay")
	}
}

func TestCaptureUncertainCleanupCannotCertifyRecordedOrRetryBasis(t *testing.T) {
	s, q := captureFixture(t, captureMarkerTest)
	before := readView(t, s)
	expected, command, index, basisCapture, err := s.prepareCheck(q, before)
	if err != nil {
		t.Fatal(err)
	}
	expectedRaw, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	basis := captureBasis(before.Generation, index.Basis, expected)
	if assessment := s.captureCurrentAssessment(context.Background(), q, basis, carrier.Digest(expectedRaw), false); assessment != "same" {
		t.Fatalf("fixture did not have matching non-executing basis: %s", assessment)
	}
	if assessment := s.captureCurrentAssessment(context.Background(), q, basis, carrier.Digest(expectedRaw), true); assessment != "unknown" {
		t.Fatalf("unsettled descendants certified retry basis: %s", assessment)
	}
	exit := 0
	observed := checkrunner.Result{Started: true, ExitCode: &exit, Failure: "cleanup_failed",
		StdoutDigest: carrier.Digest(nil), StderrDigest: carrier.Digest(nil),
		StdoutDrainComplete: true, StderrDrainComplete: true, CleanupUncertain: true}
	result := s.capturedCheck(context.Background(), q, before, expected, command, index.Basis, basisCapture, s.Root, "fixture", observed, nil)
	data := object(result.Data)
	if result.Kind == check.Passed || data["current_basis"] != "unknown" || data["declared_check_binding"] != false {
		t.Fatalf("unsettled cleanup certified post-run result: %+v", result)
	}
	terminal := captureTerminal(result)
	if terminal.RecordedPostRunBasis != "unknown" || !terminal.CleanupUncertain || terminal.ProcessOutputComplete {
		t.Fatalf("terminal certified unsettled cleanup: %+v", terminal)
	}
	response := captureTerminalResponse(terminal, q.RequestID, true, "confirmed", "unknown", result.Kind, result.Failed())
	if size := delivery.Size(response); size > delivery.Budget {
		t.Fatalf("unsettled terminal reply exceeded delivery budget: %d", size)
	}
	accounting := object(object(response.Data)["process_accounting"])
	if accounting["cleanup_uncertain"] != true || accounting["process_output_complete"] != false || len(response.Diagnostics) < 2 {
		t.Fatalf("unsettled cleanup omitted from bounded reply: %+v", response)
	}
}
