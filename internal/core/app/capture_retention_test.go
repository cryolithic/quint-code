package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/core/check"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

const captureOwnedChildTest = `package answer
import (
	"os"
	"os/exec"
	"testing"
	"time"
)
func TestAnswer(t *testing.T) {
	if os.Getenv("HAFT_CAPTURE_CHILD") == "1" {
		time.Sleep(2 * time.Second)
		_ = os.WriteFile("late-marker", []byte("child survived"), 0600)
		return
	}
	child := exec.Command(os.Args[0], "-test.run=^TestAnswer$")
	child.Env = append(os.Environ(), "HAFT_CAPTURE_CHILD=1")
	if err := child.Start(); err != nil { t.Fatal(err) }
	_ = child.Process.Release()
	if Answer() != 1 { t.Fatal("answer must be one") }
}
`

func TestCaptureCleansOrdinaryChildBeforeReportingCurrentPass(t *testing.T) {
	if testing.Short() || runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("actual Go test process group on darwin/linux")
	}
	s, q := captureFixture(t, captureOwnedChildTest)
	start := time.Now()
	captured := public(t, s, q)
	if captured.Kind != check.Passed || time.Since(start) > 8*time.Second {
		t.Fatalf("parent capture did not return promptly with a pass: %+v elapsed=%s", captured, time.Since(start))
	}
	var summary map[string]any
	raw, err := json.Marshal(captured.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &summary); err != nil || summary["current_basis"] != "same" {
		t.Fatalf("post-run basis was not compared after child cleanup: summary=%+v err=%v", summary, err)
	}
	// The ordinary child writes two seconds after start if cleanup missed it.
	time.Sleep(2300 * time.Millisecond)
	_, err = os.Stat(filepath.Join(s.Root, "late-marker"))
	if !os.IsNotExist(err) {
		t.Fatalf("ordinary owned child changed the project after capture: %v", err)
	}
}

func TestCaptureIncompleteStreamRetentionRequiresResult(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	s, q := captureFixture(t, captureMarkerTest)
	q.Capture.MaxOutputBytes = 512
	captured := public(t, s, q)
	if captured.Kind == check.Passed || captureRuns(t, s.Root) != 1 {
		t.Fatalf("bounded output unexpectedly passed or reran: %+v", captured)
	}
	ref := captureRef(t, captured)
	result := collectPart(t, s, capturePart(t, s, captured, "result"))
	if !strings.Contains(string(result), `"output_exceeded":true`) {
		t.Fatal("complete result omitted overflow context")
	}
	before, err := s.memory().Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	remember := Request{Operation: "remember", RequestID: "retain-incomplete-capture", Carrier: "---\nkind: note\ntitle: Incomplete capture context\nabout: domain:Capture.Answer\n---\nThe result preserves the stream accounting.\n", Retain: []Retention{{Ref: ref, Part: "stdout"}}}
	refused := public(t, s, remember)
	if refused.Kind != "invalid_retention" || !refused.Delivery.Complete || len(refused.Diagnostics) == 0 || !strings.Contains(refused.Diagnostics[0].Message, `"part":"result"`) || !strings.Contains(refused.Diagnostics[0].Message, ref) {
		t.Fatalf("incomplete stdout was not rejected with an exact result action: %+v", refused)
	}
	after, err := s.memory().Read(context.Background())
	if err != nil || after.Generation != before.Generation || captureRuns(t, s.Root) != 1 {
		t.Fatalf("rejected retention changed memory or reran capture: read=%v before=%s after=%s", err, before.Generation, after.Generation)
	}
	remember.Retain[0].Part = "result"
	saved := public(t, s, remember)
	if saved.Kind != "written" || captureRuns(t, s.Root) != 1 {
		t.Fatalf("retaining complete result failed or reran capture: %+v", saved)
	}
	found := false
	for _, doc := range readView(t, s).Documents {
		if doc.Record.Title != "Incomplete capture context" {
			continue
		}
		retained := public(t, s, Request{Operation: "recall", Ref: doc.Record.ID})
		actual := collectPart(t, s, deliveredPart(t, s, retained, "report_1_content"))
		if string(actual) != string(result) {
			t.Fatal("retained result changed the captured accounting")
		}
		found = true
	}
	if !found {
		t.Fatal("result attachment was not durably written")
	}
}

func TestCaptureStreamCompletenessUsesEachStreamAndPipeStatus(t *testing.T) {
	stdout := []byte("complete stdout")
	stderr := []byte("prefix")
	process := CaptureProcess{StdoutBytes: int64(len(stdout)), StderrBytes: 100, OutputExceeded: true, StdoutDrainComplete: true, StderrDrainComplete: true}
	d := delivery.Document{Operation: "check", Parts: []delivery.Part{
		delivery.Text("stdout", stdout),
		delivery.Text("stderr", stderr),
		delivery.JSON("process", process, false),
	}}
	for _, tc := range []struct {
		name       string
		incomplete bool
	}{{"stdout", false}, {"stderr", true}} {
		part, err := d.Member(tc.name)
		if err != nil {
			t.Fatal(err)
		}
		incomplete, err := incompleteCaptureRetention(d, part)
		if err != nil || incomplete != tc.incomplete {
			t.Fatalf("per-stream completeness for %s: incomplete=%t err=%v", tc.name, incomplete, err)
		}
	}
	s, _ := runnerRegressionFixture(t, map[string]string{
		"answer.go":      "package answer\nfunc Answer() int { return 1 }\n",
		"answer_test.go": runnerAnswerTest,
	})
	envelope := capturedResult{Format: "haft.transient-result/1", Request: Request{Operation: "check", Action: "capture"}, Result: Result{Format: Format, Operation: "check", Kind: check.Unattributable}, Document: &d}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := s.memory().PutTransient(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	keep := Request{Operation: "remember", RequestID: "retain-complete-stream-with-other-overflow", Carrier: "---\nkind: note\ntitle: Complete stream amid other overflow\nabout: domain:Capture.Answer\n---\nThe stdout stream completed.\n", Retain: []Retention{{Ref: ref, Part: "stdout"}}}
	if saved := public(t, s, keep); saved.Kind != "written" {
		t.Fatalf("complete stdout was refused because stderr overflowed: %+v", saved)
	}
	process.Failure = "capture_incomplete"
	process.OutputExceeded = false
	process.StderrBytes = int64(len(stderr))
	process.StderrDrainComplete = false
	process.PipeDrainUncertain = true
	d.Parts[2] = delivery.JSON("process", process, false)
	part, err := d.Member("stdout")
	if err != nil {
		t.Fatal(err)
	}
	incomplete, err := incompleteCaptureRetention(d, part)
	if err != nil || incomplete {
		t.Fatalf("complete stdout was blocked by held stderr pipe: incomplete=%t err=%v", incomplete, err)
	}
	part, err = d.Member("stderr")
	if err != nil {
		t.Fatal(err)
	}
	incomplete, err = incompleteCaptureRetention(d, part)
	if err != nil || !incomplete {
		t.Fatalf("held stderr pipe was labelled complete below the cap: incomplete=%t err=%v", incomplete, err)
	}
	pipeEnvelope := capturedResult{Format: "haft.transient-result/1", Request: Request{Operation: "check", Action: "capture"}, Result: Result{Format: Format, Operation: "check", Kind: check.Unattributable}, Document: &d}
	pipeRaw, err := json.Marshal(pipeEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	pipeRef, err := s.memory().PutTransient(context.Background(), pipeRaw)
	if err != nil {
		t.Fatal(err)
	}
	keep.RequestID = "retain-complete-stdout-with-held-stderr"
	keep.Carrier = "---\nkind: note\ntitle: Complete stdout with held stderr\nabout: domain:Capture.Answer\n---\nThe stdout pipe reached EOF.\n"
	keep.Retain[0] = Retention{Ref: pipeRef, Part: "stdout"}
	if saved := public(t, s, keep); saved.Kind != "written" {
		t.Fatalf("complete sibling stream was refused: %+v", saved)
	}
	keep.RequestID = "reject-held-stderr"
	keep.Retain[0].Part = "stderr"
	if refused := public(t, s, keep); refused.Kind != "invalid_retention" {
		t.Fatalf("held stderr stream was saved alone: %+v", refused)
	}
	observation := check.Observation{Input: check.ObservationInput{Observed: check.Run{Stdout: stdout, Stderr: stderr}}}
	r := Result{Format: Format, Operation: "check", Kind: check.Unattributable, Data: map[string]any{"observation": observation, "process": process}}
	view := makeDelivery(Request{Operation: "check", Action: "capture", RequestID: "pipe-capture"}, r)
	var summary map[string]any
	if err := json.Unmarshal(view.Summary, &summary); err != nil {
		t.Fatal(err)
	}
	if summary["process_output_complete"] != false {
		t.Fatalf("pipe capture summary claimed complete output: %+v", summary)
	}
	stream, err := view.Member("stdout")
	if err != nil || strings.Contains(stream.Label, "uncertain") {
		t.Fatalf("complete stdout inherited held stderr label: part=%+v err=%v", stream, err)
	}
	stream, err = view.Member("stderr")
	if err != nil || !strings.Contains(stream.Label, "uncertain") {
		t.Fatalf("held stderr pipe lost incompleteness label: part=%+v err=%v", stream, err)
	}
	process.CleanupUncertain = true
	d.Parts[2] = delivery.JSON("process", process, false)
	cleanupEnvelope := capturedResult{Format: "haft.transient-result/1", Request: Request{Operation: "check", Action: "capture"}, Result: Result{Format: Format, Operation: "check", Kind: check.Unattributable}, Document: &d}
	cleanupRaw, err := json.Marshal(cleanupEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	cleanupRef, err := s.memory().PutTransient(context.Background(), cleanupRaw)
	if err != nil {
		t.Fatal(err)
	}
	keep.RequestID = "reject-stdout-after-uncertain-cleanup"
	keep.Retain[0] = Retention{Ref: cleanupRef, Part: "stdout"}
	if refused := public(t, s, keep); refused.Kind != "invalid_retention" {
		t.Fatalf("uncertain cleanup allowed standalone sibling stream retention: %+v", refused)
	}
	r.Data = map[string]any{"observation": observation, "process": process}
	view = makeDelivery(Request{Operation: "check", Action: "capture", RequestID: "cleanup-capture"}, r)
	if err := json.Unmarshal(view.Summary, &summary); err != nil || summary["process_output_complete"] != false || summary["cleanup_uncertain"] != true {
		t.Fatalf("uncertain cleanup was hidden in process summary: summary=%+v err=%v", summary, err)
	}
	process.CleanupUncertain = false
	for _, failure := range []string{"timeout", "canceled", "cleanup_failed", "wait_failed"} {
		process.Failure = failure
		process.StdoutDrainComplete = true
		process.StderrDrainComplete = true
		process.PipeDrainUncertain = false
		r.Data = map[string]any{"observation": observation, "process": process}
		view = makeDelivery(Request{Operation: "check", Action: "capture", RequestID: "uncertain-capture"}, r)
		if err := json.Unmarshal(view.Summary, &summary); err != nil || summary["process_output_complete"] != false {
			t.Fatalf("%s claimed complete output despite uncertain process: summary=%+v err=%v", failure, summary, err)
		}
		stream, err = view.Member("stdout")
		if err != nil {
			t.Fatal(err)
		}
		incomplete, err = incompleteCaptureRetention(view, stream)
		if err != nil || !incomplete {
			t.Fatalf("%s allowed standalone stream retention: incomplete=%t err=%v", failure, incomplete, err)
		}
	}
}

func TestTransientFailureGuidesNonexecutingAndCaptureRecovery(t *testing.T) {
	for _, kind := range []string{"expired", "corrupt"} {
		response := transientFailure(fmt.Errorf("%s: test cache", kind))
		if response.Kind != kind || len(response.Diagnostics) == 0 {
			t.Fatalf("missing %s result guidance: %+v", kind, response)
		}
		message := response.Diagnostics[0].Message
		if !response.Delivery.Complete || strings.Contains(response.Delivery.NoNext, "repeat the original operation") {
			t.Fatalf("%s recovery was truncated or gave unsafe blanket advice: %+v", kind, response)
		}
		for _, fragment := range []string{"recall", "context", "check/prepare", "check/observe", "same request_id", "deliberate new run", "other effectful receipt"} {
			if !strings.Contains(message, fragment) {
				t.Fatalf("%s guidance omitted %q: %s", kind, fragment, message)
			}
		}
	}
}
