package app

import (
	"encoding/json"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/check"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

// burdenProbe measures two public API paths on the same small, test-owned Go
// source. Fixture setup and test assertions are excluded from the counters.
// It is a controlled protocol comparison, not a native agent usefulness test.
type burdenProbe struct {
	t           *testing.T
	s           Service
	requests    int
	requestJSON int
	reads       int
}

func (p *burdenProbe) call(q Request) delivery.Response {
	p.t.Helper()
	q.Format = delivery.Format
	raw, err := json.Marshal(q)
	if err != nil {
		p.t.Fatal(err)
	}
	p.requests++
	p.requestJSON += len(raw)
	if q.Operation == "read" {
		p.reads++
	}
	return public(p.t, p.s, q)
}

func (p *burdenProbe) parts(r delivery.Response, wanted ...string) map[string]delivery.Descriptor {
	p.t.Helper()
	parts := map[string]delivery.Descriptor{}
	for _, part := range r.Delivery.Available {
		parts[part.Name] = part
	}
	hasWanted := func() bool {
		for _, name := range wanted {
			if _, ok := parts[name]; !ok {
				return false
			}
		}
		return true
	}
	if hasWanted() {
		return parts
	}
	if r.Delivery.Catalog == nil {
		p.t.Fatalf("no part catalog for %s", r.Kind)
	}
	q := *r.Delivery.Catalog
	for {
		page := p.call(nextApp(q))
		var data struct {
			Parts []delivery.Descriptor `json:"parts"`
		}
		raw := rawJSON(page.Data)
		if err := json.Unmarshal(raw, &data); err != nil {
			p.t.Fatal(err)
		}
		for _, part := range data.Parts {
			parts[part.Name] = part
		}
		if hasWanted() {
			return parts
		}
		if page.Delivery.Next == nil {
			p.t.Fatalf("catalog omitted required parts %v", wanted)
		}
		q = *page.Delivery.Next
	}
}

func (p *burdenProbe) bytes(part delivery.Descriptor) []byte {
	p.t.Helper()
	q := part.Request
	q.View = "bytes"
	var all []byte
	for {
		response := p.call(nextApp(q))
		if response.Delivery.Encoding != "base64" || response.Delivery.Offset != len(all) {
			p.t.Fatalf("incomplete part read: %+v", response.Delivery)
		}
		var data struct {
			Raw []byte `json:"bytes_base64"`
		}
		raw := rawJSON(response.Data)
		if err := json.Unmarshal(raw, &data); err != nil {
			p.t.Fatal(err)
		}
		all = append(all, data.Raw...)
		if response.Delivery.Next == nil {
			if !response.Delivery.Complete || carrier.Digest(all) != response.Delivery.Digest {
				p.t.Fatal("part read did not match full digest")
			}
			return all
		}
		q = *response.Delivery.Next
	}
}

func burdenFixture(t *testing.T) (Service, Request) {
	t.Helper()
	return runnerRegressionFixture(t, map[string]string{
		"answer.go":      "package answer\nfunc Answer() int { return 1 }\n",
		"answer_test.go": runnerAnswerTest,
	})
}

func probeRetain(t *testing.T, p *burdenProbe, result delivery.Response, requestID string) {
	t.Helper()
	parts := p.parts(result, "stdout")
	stdout, ok := parts["stdout"]
	if !ok || stdout.Digest == "" {
		t.Fatal("stdout retention part absent")
	}
	keep := Request{
		Operation: "remember", RequestID: requestID,
		Carrier: "---\nkind: note\ntitle: Retained answer output\nabout: domain:Burden.Answer\n---\nThe bounded answer test was run.\n",
		Retain:  []Retention{{Ref: stdout.Request.Ref, Part: stdout.Name}},
	}
	if result := p.call(keep); result.Kind != "written" {
		t.Fatalf("retention failed: %+v", result)
	}
}

func TestCaptureControlledBurdenComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("executes two isolated Go test processes")
	}

	beforeService, beforeQuery := burdenFixture(t)
	before := &burdenProbe{t: t, s: beforeService}
	prepared := before.call(beforeQuery)
	if prepared.Kind != "prepared" {
		t.Fatalf("manual path preparation failed: %+v", prepared)
	}
	preparedParts := before.parts(prepared, "expected", "basis_capture", "command", "run_environment")
	var expected check.Contract
	var basis CheckBasisCapture
	var command []string
	var environment map[string]string
	for _, target := range []struct {
		name string
		into any
	}{
		{"expected", &expected},
		{"basis_capture", &basis},
		{"command", &command},
		{"run_environment", &environment},
	} {
		part, ok := preparedParts[target.name]
		if !ok {
			t.Fatalf("required prepare member %s absent", target.name)
		}
		raw := before.bytes(part)
		if err := json.Unmarshal(raw, target.into); err != nil {
			t.Fatal(err)
		}
	}
	beforeReadsToAnswer := before.reads
	preparedData := map[string]any{"expected": expected, "basis_capture": basis, "command": command, "run_environment": environment}
	observedInput := runnerRegressionRun(t, beforeService, preparedData, 0)
	manualObservation := Request{Operation: "check", Action: "observe", Observation: &observedInput}
	manualJSON, err := json.Marshal(manualObservation)
	if err != nil {
		t.Fatal(err)
	}
	observed := before.call(manualObservation)
	if observed.Kind != check.Passed || object(observed.Data)["current_basis"] != "same" {
		t.Fatalf("manual observation did not establish current pass: %+v", observed)
	}
	probeRetain(t, before, observed, "retain-manual-burden")

	afterService, afterQuery := burdenFixture(t)
	after := &burdenProbe{t: t, s: afterService}
	afterQuery.Action = "capture"
	afterQuery.RequestID = "capture-burden"
	afterQuery.Capture = &CaptureOptions{TimeoutMillis: 30000, MaxOutputBytes: 1 << 20}
	captured := after.call(afterQuery)
	if captured.Kind != check.Passed || object(captured.Data)["current_basis"] != "same" {
		t.Fatalf("bounded capture did not establish current pass: %+v", captured)
	}
	afterReadsToAnswer := after.reads
	probeRetain(t, after, captured, "retain-capture-burden")

	t.Logf("controlled burden: before application_requests=%d request_json_bytes=%d reads_to_answer=%d all_reads=%d observation_json_bytes=%d preimage_bytes=%d; after application_requests=%d request_json_bytes=%d reads_to_answer=%d all_reads=%d", before.requests, before.requestJSON, beforeReadsToAnswer, before.reads, len(manualJSON), len(basis.ImplementationPreimage)+len(basis.DependencyPreimage), after.requests, after.requestJSON, afterReadsToAnswer, after.reads)
}
