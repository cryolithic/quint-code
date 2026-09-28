package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

func TestT01BR2ReauthorFaultRetryRetainsExactOutput(t *testing.T) {
	for _, stage := range []string{"staged", "published", "before_commit", "committed"} {
		t.Run(stage, func(t *testing.T) {
			s := service(t)
			_, predecessor := seedT01BActiveSpec(t, s)
			preview := public(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: predecessor})
			if preview.Kind != "ready" {
				t.Fatalf("preview: %+v", preview)
			}
			apply := Request{Operation: "change", Action: "reauthor_apply", Ref: predecessor,
				RequestID: "reauthor-fault-" + stage, ExpectedGeneration: preview.Basis["memory_generation"],
				PreviewDigest: preview.Basis["preview_digest"]}
			s.Store = &store.Store{Root: s.Root, Fault: func(point store.Point) error {
				if point.Stage == stage {
					return errors.New("injected " + stage)
				}
				return nil
			}}
			first := public(t, s, apply)
			assertFaultResponseBound(t, first)
			if !first.IsError || !hasFaultDiagnostic(first, "publication_error") {
				t.Fatalf("interrupted publication appeared successful: %+v", first)
			}
			if ref := str(object(first.Data)["published_successor_ref"]); ref != "" {
				t.Fatalf("error reply claimed an unverified output: %s", ref)
			}
			if stage == "committed" {
				guidance := faultDiagnosticMessage(first, "reply_interrupted")
				if first.Kind != "written" || !strings.Contains(guidance, "retry the same request ID") {
					t.Fatalf("committed publication lost reply-interruption guidance: %+v", first)
				}
			}
			if stage != "committed" && first.Kind != "interrupted" {
				t.Fatalf("uncommitted publication appeared written: %+v", first)
			}

			fresh := Service{Root: s.Root, Now: s.Now}
			retry := public(t, fresh, apply)
			assertFaultResponseBound(t, retry)
			if retry.Kind != "replayed" || retry.IsError {
				t.Fatalf("same-ID retry did not recover exact output: %+v", retry)
			}
			ref := str(object(retry.Data)["published_successor_ref"])
			parsed, err := carrier.ParseRef(ref)
			if err != nil || !parsed.Pinned() || parsed.ClaimID != "" {
				t.Fatalf("retry omitted pinned whole-spec output: %q, %v", ref, err)
			}
			readRequest := exactReauthorReadRequest(t, retry, ref)
			read := public(t, fresh, readRequest)
			if read.Kind != "found" || read.IsError || str(object(read.Data)["exact_ref"]) != ref {
				t.Fatalf("returned exact route failed: %+v", read)
			}
			again := public(t, fresh, apply)
			assertFaultResponseBound(t, again)
			if again.Kind != "replayed" || again.IsError || str(object(again.Data)["published_successor_ref"]) != ref {
				t.Fatalf("retry changed original output identity: %+v", again)
			}
			if documents := readView(t, fresh).Documents; len(documents) != 2 {
				t.Fatalf("same-ID retry duplicated successor: %d documents", len(documents))
			}
		})
	}
}

func TestT01BR2ReauthorReplaysActualOldJournalAndRefusesChangedOutput(t *testing.T) {
	fixture := filepath.Join("testdata", "t01br2-old-journal")
	root := t.TempDir()
	copyOldReauthorFixture(t, fixture, root)
	requestBytes, err := os.ReadFile(filepath.Join(fixture, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	var request Request
	if err := json.Unmarshal(requestBytes, &request); err != nil {
		t.Fatal(err)
	}
	expectedBytes, err := os.ReadFile(filepath.Join(fixture, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected struct {
		ProducerHead          string `json:"producer_head"`
		ProducerBinarySHA256  string `json:"producer_binary_sha256"`
		PredecessorRef        string `json:"predecessor_ref"`
		PublishedSuccessorRef string `json:"published_successor_ref"`
	}
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		t.Fatal(err)
	}
	if expected.ProducerHead != "b6874589aa9247f588a3bfd29a6ba22ccee40d63" ||
		expected.ProducerBinarySHA256 != "5341548f53edd67b6512f17cb59d87dfa5f9129ed3842f945439729837684c16" ||
		request.Ref != expected.PredecessorRef {
		t.Fatal("old journal fixture provenance or predecessor changed")
	}
	s := Service{Root: root}
	seedBytes, err := os.ReadFile(filepath.Join(fixture, "seed-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seedRequest Request
	if err := json.Unmarshal(seedBytes, &seedRequest); err != nil {
		t.Fatal(err)
	}
	seedReplay := public(t, s, seedRequest)
	assertFaultResponseBound(t, seedReplay)
	if seedReplay.Kind != "replayed" || seedReplay.IsError {
		t.Fatalf("old seed journal did not replay: %+v", seedReplay)
	}
	replayed := public(t, s, request)
	assertFaultResponseBound(t, replayed)
	if replayed.Kind != "replayed" || replayed.IsError || str(object(replayed.Data)["published_successor_ref"]) != expected.PublishedSuccessorRef {
		t.Fatalf("old journal did not return its original output: %+v", replayed)
	}
	readRequest := exactReauthorReadRequest(t, replayed, expected.PublishedSuccessorRef)
	read := public(t, s, readRequest)
	if read.Kind != "found" || read.IsError || str(object(read.Data)["exact_ref"]) != expected.PublishedSuccessorRef {
		t.Fatalf("old output exact read failed: %+v", read)
	}
	predecessor := public(t, s, Request{Operation: "recall", Ref: expected.PredecessorRef})
	if predecessor.Kind != "found" || predecessor.IsError {
		t.Fatalf("old predecessor history became unreadable: %+v", predecessor)
	}
	verifyOldReauthorFixtureUnchanged(t, fixture, root)

	parsed, err := carrier.ParseRef(expected.PublishedSuccessorRef)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, ".haft", "editions", "sha256", strings.TrimPrefix(parsed.Digest, "sha256:")+".json")
	original, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"corrupt", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			if mutation == "corrupt" {
				changed := append(bytes.Clone(original), '\n')
				if err := os.WriteFile(snapshot, changed, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "delete" {
				if err := os.Remove(snapshot); err != nil {
					t.Fatal(err)
				}
			}
			conflict := public(t, s, request)
			assertFaultResponseBound(t, conflict)
			if conflict.Kind != "replay_conflict" || !conflict.IsError || !hasFaultDiagnostic(conflict, "published_output_changed") {
				t.Fatalf("changed published output gained a receipt: %+v", conflict)
			}
			if ref := str(object(conflict.Data)["published_successor_ref"]); ref != "" {
				t.Fatalf("replay conflict claimed a published ref: %s", ref)
			}
			if err := os.WriteFile(snapshot, original, 0600); err != nil {
				t.Fatal(err)
			}
			restored := public(t, s, request)
			assertFaultResponseBound(t, restored)
			if restored.Kind != "replayed" || restored.IsError || str(object(restored.Data)["published_successor_ref"]) != expected.PublishedSuccessorRef {
				t.Fatalf("restored exact output did not replay: %+v", restored)
			}
		})
	}
	verifyOldReauthorFixtureUnchanged(t, fixture, root)
}

func hasFaultDiagnostic(response delivery.Response, code string) bool {
	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func faultDiagnosticMessage(response delivery.Response, code string) string {
	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Code == code {
			return diagnostic.Message
		}
	}
	return ""
}

func assertFaultResponseBound(t *testing.T, response delivery.Response) {
	t.Helper()
	size := delivery.Size(response)
	if size > delivery.Budget {
		t.Fatalf("public response exceeds budget: %d > %d", size, delivery.Budget)
	}
}

func exactReauthorReadRequest(t *testing.T, response delivery.Response, ref string) Request {
	t.Helper()
	address := object(object(response.Data)["exact_read_request"])
	if address["format"] != delivery.Format || address["operation"] != "recall" || address["ref"] != ref {
		t.Fatalf("returned route is not the exact published ref: %+v", address)
	}
	encoded, err := json.Marshal(address)
	if err != nil {
		t.Fatal(err)
	}
	var request Request
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	return request
}

func copyOldReauthorFixture(t *testing.T, fixture, root string) {
	t.Helper()
	base := filepath.Join(fixture, ".haft")
	err := filepath.WalkDir(base, func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(base, source)
		if err != nil {
			return err
		}
		destination := filepath.Join(root, ".haft", relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		bytes, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, bytes, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func verifyOldReauthorFixtureUnchanged(t *testing.T, fixture, root string) {
	t.Helper()
	base := filepath.Join(fixture, ".haft")
	err := filepath.WalkDir(base, func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(base, source)
		if err != nil {
			return err
		}
		destination := filepath.Join(root, ".haft", relative)
		before, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		after, err := os.ReadFile(destination)
		if err != nil {
			return err
		}
		if !bytes.Equal(before, after) {
			return errors.New("old journal fixture bytes changed at " + relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
