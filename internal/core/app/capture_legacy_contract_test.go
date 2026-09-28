package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/check"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func legacyExpectedDocument(t *testing.T, original delivery.Document) delivery.Document {
	t.Helper()
	old := original
	old.Basis = map[string]string{}
	for key, value := range original.Basis {
		if key != "expected_contract_digest" {
			old.Basis[key] = value
		}
	}
	old.Parts = append([]delivery.Part(nil), original.Parts...)
	return old
}

func changeLegacyExpectedPart(t *testing.T, original delivery.Document, change func(map[string]any)) delivery.Document {
	t.Helper()
	d := legacyExpectedDocument(t, original)
	for i, part := range d.Parts {
		if part.Name != "expected" {
			continue
		}
		var expected map[string]any
		if err := json.Unmarshal(part.Raw, &expected); err != nil {
			t.Fatal(err)
		}
		change(expected)
		raw, err := json.Marshal(expected)
		if err != nil {
			t.Fatal(err)
		}
		d.Parts[i].Raw = raw
		return d
	}
	t.Fatal("actual captured document has no expected part")
	return d
}

func legacyReplayAssessment(t *testing.T, s Service, q Request, d delivery.Document) string {
	t.Helper()
	response := s.captureReplay(context.Background(), q, d, nil)
	if response.Kind != check.Passed || object(response.Data)["execution_replay"] != true {
		t.Fatalf("legacy replay altered historical outcome: %+v", response)
	}
	if size := delivery.Size(response); size > delivery.Budget {
		t.Fatalf("legacy replay exceeded delivery budget: %d", size)
	}
	return str(object(response.Data)["current_basis_assessment"])
}

func TestCaptureLegacyExpectedFallbackComparesSemanticContract(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	s, q := captureFixture(t, captureMarkerTest)
	first := public(t, s, q)
	if first.Kind != check.Passed || captureRuns(t, s.Root) != 1 {
		t.Fatalf("fixture did not capture one pass: %+v", first)
	}
	ref := captureRef(t, first)
	storedPath := filepath.Join(s.Root, ".haft", ".cache", "disclosure", strings.TrimPrefix(ref, "result:sha256:")+".json")
	storedBefore, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.loadTransient(context.Background(), ref, false, "")
	if err != nil {
		t.Fatal(err)
	}
	part, err := d.Member("expected")
	if err != nil {
		t.Fatal(err)
	}
	if d.Basis["expected_contract_digest"] == carrier.Digest(part.Raw) {
		t.Fatal("fixture did not exercise the map-versus-struct digest difference")
	}
	legacy := legacyExpectedDocument(t, d)
	if got := legacyReplayAssessment(t, s, q, legacy); got != "same" {
		t.Fatalf("unchanged legacy expected contract = %q, want same", got)
	}

	oldGoEnvironment := changeLegacyExpectedPart(t, d, func(expected map[string]any) {
		environment := object(object(expected["basis"])["environment"])
		if environment["goproxy"] == nil || environment["gosumdb"] == nil {
			t.Fatal("fixture has no pinned current Go environment")
		}
		delete(environment, "goproxy")
		delete(environment, "gosumdb")
	})
	if got := legacyReplayAssessment(t, s, q, oldGoEnvironment); got != "changed" {
		t.Fatalf("genuine older Go environment = %q, want changed", got)
	}

	unsupported := changeLegacyExpectedPart(t, d, func(expected map[string]any) {
		expected["future_semantics"] = "cannot silently discard"
	})
	if got := legacyReplayAssessment(t, s, q, unsupported); got != "unknown" {
		t.Fatalf("unsupported legacy contract = %q, want unknown", got)
	}
	underdetermined := changeLegacyExpectedPart(t, d, func(expected map[string]any) {
		delete(object(expected["basis"]), "environment")
	})
	if got := legacyReplayAssessment(t, s, q, underdetermined); got != "unknown" {
		t.Fatalf("underdetermined legacy contract = %q, want unknown", got)
	}
	unreadable := legacyExpectedDocument(t, d)
	for i, candidate := range unreadable.Parts {
		if candidate.Name == "expected" {
			unreadable.Parts = append(unreadable.Parts[:i], unreadable.Parts[i+1:]...)
			break
		}
	}
	if got := legacyReplayAssessment(t, s, q, unreadable); got != "unknown" {
		t.Fatalf("unreadable expected part = %q, want unknown", got)
	}

	if err := os.WriteFile(filepath.Join(s.Root, "answer.go"), []byte("package answer\nfunc Answer() int { return 2 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := legacyReplayAssessment(t, s, q, legacy); got != "changed" {
		t.Fatalf("real code change = %q, want changed", got)
	}
	if captureRuns(t, s.Root) != 1 {
		t.Fatalf("legacy replay reran project test: %d", captureRuns(t, s.Root))
	}
	storedAfter, err := os.ReadFile(storedPath)
	if err != nil || !bytes.Equal(storedBefore, storedAfter) {
		t.Fatalf("legacy replay changed cached bytes: %v", err)
	}
}

func TestLegacyExpectedDigestRejectsMissingOrUnknownSemantics(t *testing.T) {
	s, q := captureFixture(t, captureMarkerTest)
	view := readView(t, s)
	expected, _, _, _, err := s.prepareCheck(q, view)
	if err != nil {
		t.Fatal(err)
	}
	currentRaw, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := json.Marshal(delivery.Value(currentRaw))
	if err != nil {
		t.Fatal(err)
	}
	if got := legacyExpectedContractDigest(complete); got != carrier.Digest(currentRaw) {
		t.Fatalf("supported map-encoded contract digest = %q, want %q", got, carrier.Digest(currentRaw))
	}
	changed := func(edit func(map[string]any)) []byte {
		var object map[string]any
		if err := json.Unmarshal(complete, &object); err != nil {
			t.Fatal(err)
		}
		edit(object)
		raw, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for name, raw := range map[string][]byte{
		"unknown nested": changed(func(contract map[string]any) {
			object(contract["basis"])["future_semantics"] = true
		}),
		"missing selector": changed(func(contract map[string]any) {
			delete(object(contract["selector"]), "test")
		}),
		"null environment": changed(func(contract map[string]any) {
			object(contract["basis"])["environment"] = nil
		}),
		"trailing data": append(bytes.Clone(complete), []byte(`{}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if got := legacyExpectedContractDigest(raw); got != "" {
				t.Fatalf("unsupported contract produced comparable digest %q", got)
			}
		})
	}
}
