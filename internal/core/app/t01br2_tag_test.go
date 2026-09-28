package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01BR2SelectedReauthorRefusesSemanticTagLoss(t *testing.T) {
	cases := []struct {
		name   string
		before string
		after  string
		path   string
		tag    string
	}{
		{"root", "extension:\n", "x-vendor: !vendor abc\nextension:\n", "x-vendor", "!vendor"},
		{"nested", "\n    owner: Billing\n", "\n    owner: Billing\n    meaning: !currency 12\n", "extension.meaning", "!currency"},
		{"claim", "custom_claim:", "x-unit: !currency 12\n", "claims[0].x-unit", "!currency"},
		{"binary", "extension:\n", "x-vendor: !!binary aGVsbG8=\nextension:\n", "x-vendor", "!!binary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := service(t)
			old, originalRef := seedT01BActiveSpec(t, s)
			original := string(old.Raw)
			modified := strings.Replace(original, tc.before, tc.after, 1)
			if tc.name == "claim" {
				index := strings.Index(original, tc.before)
				if index < 0 {
					t.Fatal("claim extension marker unavailable")
				}
				lineStart := strings.LastIndex(original[:index], "\n") + 1
				indent := original[lineStart:index]
				modified = strings.Replace(original, tc.before, tc.after+indent+tc.before, 1)
			}
			if modified == original {
				t.Fatal("fixture injection did not match")
			}
			path := filepath.Join(s.Root, ".haft", "specs", old.Record.ID+".md")
			if err := os.WriteFile(path, []byte(modified), 0600); err != nil {
				t.Fatal(err)
			}
			before := readView(t, s)
			old = before.Documents[0]
			if !old.Valid() {
				t.Fatalf("externally authored v1 carrier became unreadable: %+v", old.Diagnostics)
			}
			ref := old.Record.ID + "@" + old.Edition
			oldSnapshot := bytes.Clone(before.CurrentSnapshots[old.Edition])
			preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "conflict")
			assertTagRefusal(t, preview.Diagnostics, tc.path, tc.tag)
			data := preview.Data.(map[string]any)
			oldData := data["old"].(map[string]any)
			losses := data["material_losses"].([]string)
			if data["selected_ref"] != ref || data["memory_generation"] == "" || oldData["carrier_digest"] != carrier.Digest(old.Raw) || oldData["snapshot_digest"] != old.Edition || oldData["carrier"] != string(old.Raw) || len(losses) != 1 || !strings.Contains(losses[0], tc.path) {
				t.Fatalf("conflict lost exact predecessor basis or loss: %+v", data)
			}
			apply := Request{Operation: "change", Action: "reauthor_apply", Ref: ref, RequestID: "reject-tagged-" + tc.name,
				ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
			refused := run(t, s, apply, "conflict")
			assertTagRefusal(t, refused.Diagnostics, tc.path, tc.tag)
			after := readView(t, s)
			if len(after.Documents) != len(before.Documents) || !bytes.Equal(after.Documents[0].Raw, old.Raw) || !bytes.Equal(after.CurrentSnapshots[old.Edition], oldSnapshot) || after.Generation != before.Generation {
				t.Fatal("refused preview/apply mutated carrier, snapshot or memory generation")
			}
			run(t, s, Request{Operation: "recall", Ref: originalRef}, "found")
			run(t, s, Request{Operation: "recall", Ref: "spec:" + old.Record.Slug}, "found")
		})
	}
}

func TestT01BR2MultipleTagLossesRemainReadableWithinDisclosureBudget(t *testing.T) {
	s := service(t)
	old, _ := seedT01BActiveSpec(t, s)
	var extensions strings.Builder
	for i := 0; i < 24; i++ {
		fmt.Fprintf(&extensions, "    tag_%02d: !currency %d\n", i, i)
	}
	modified := strings.Replace(string(old.Raw), "\n    owner: Billing\n", "\n    owner: Billing\n"+extensions.String(), 1)
	path := filepath.Join(s.Root, ".haft", "specs", old.Record.ID+".md")
	if err := os.WriteFile(path, []byte(modified), 0600); err != nil {
		t.Fatal(err)
	}
	old = readView(t, s).Documents[0]
	ref := old.Record.ID + "@" + old.Edition
	response := public(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref})
	if response.Kind != "conflict" || !response.IsError || response.Delivery.Catalog == nil || delivery.Size(response) > delivery.Budget {
		t.Fatalf("unsupported tags did not yield bounded readable conflict: %+v", response)
	}
	diagnostics := public(t, s, Request{Operation: "read", Ref: response.Delivery.Catalog.Ref, View: "detail", Part: "diagnostics/23", ExpectedGeneration: response.Basis["memory_generation"]})
	if !bytes.Contains(rawJSON(diagnostics.Data), []byte("extension.tag_23")) {
		t.Fatalf("full path/type diagnostics unavailable through disclosure: %+v", diagnostics)
	}
	oldQuery := Request{Operation: "read", Ref: response.Delivery.Catalog.Ref, View: "bytes", Part: "old/0", ExpectedGeneration: response.Basis["memory_generation"]}
	var reconstructed []byte
	for i := 0; i < 10; i++ {
		chunk := public(t, s, oldQuery)
		fragment, ok := object(chunk.Data)["bytes_base64"].([]byte)
		if !ok || len(fragment) == 0 {
			t.Fatalf("source byte chunk unavailable at %d: %+v", i, chunk)
		}
		reconstructed = append(reconstructed, fragment...)
		if chunk.Delivery.Next == nil {
			break
		}
		oldQuery = nextApp(*chunk.Delivery.Next)
	}
	if !bytes.Equal(reconstructed, old.Raw) {
		t.Fatalf("exact source basis unavailable through disclosure: got %d of %d bytes", len(reconstructed), len(old.Raw))
	}
}

func TestT01BR2SharedRememberWriterRefusesTagLossButAcceptsLexicalNormalization(t *testing.T) {
	s := service(t)
	tagged := "---\nkind: note\ntitle: Vendor field\nabout: domain:Billing.OrderCancellation\nx-vendor: !vendor abc\n---\nOriginal body.\n"
	refused := run(t, s, Request{Operation: "remember", Carrier: tagged, RequestID: "tagged-note"}, "invalid")
	assertTagRefusal(t, refused.Diagnostics, "x-vendor", "!vendor")
	if len(readView(t, s).Documents) != 0 {
		t.Fatal("tagged carrier was published by the shared writer")
	}
	ordinary := strings.Replace(tagged, "x-vendor: !vendor abc", "# lexical comment\nx-vendor: 0x1F", 1)
	run(t, s, Request{Operation: "remember", Carrier: ordinary, RequestID: "ordinary-note"}, "written")
	if len(readView(t, s).Documents) != 1 {
		t.Fatal("ordinary lexical normalization was refused")
	}
}

func TestT01BR2ReauthorAllowsOrdinaryNullSupersedesNormalization(t *testing.T) {
	s := service(t)
	old, _ := seedT01BActiveSpec(t, s)
	modified := strings.Replace(string(old.Raw), "status: active\n", "status: active\nsupersedes: null\n", 1)
	path := filepath.Join(s.Root, ".haft", "specs", old.Record.ID+".md")
	if err := os.WriteFile(path, []byte(modified), 0600); err != nil {
		t.Fatal(err)
	}
	old = readView(t, s).Documents[0]
	if !old.Valid() {
		t.Fatalf("ordinary v1 null field is invalid: %+v", old.Diagnostics)
	}
	ref := old.Record.ID + "@" + old.Edition
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "ready")
	if losses := preview.Data.(map[string]any)["material_losses"].([]string); len(losses) != 0 {
		t.Fatalf("intentional supersedes rewrite was called tag loss: %+v", losses)
	}
	run(t, s, Request{Operation: "change", Action: "reauthor_apply", Ref: ref, RequestID: "null-supersedes",
		ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}, "written")
}

func assertTagRefusal(t *testing.T, diagnostics []carrier.Diagnostic, path, tag string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "unsupported_yaml_tag_conversion" && diagnostic.Path == path && strings.Contains(diagnostic.Message, tag) && strings.Contains(diagnostic.Message, "would become") {
			return
		}
	}
	t.Fatalf("missing exact %s tag refusal at %s: %+v", tag, path, diagnostics)
}

func TestT01BR2ReauthorTagConflictPublicApplyDoesNotPublish(t *testing.T) {
	s := service(t)
	old, originalRef := seedT01BActiveSpec(t, s)
	modified := strings.Replace(string(old.Raw), "extension:\n", "x-vendor: !vendor abc\nextension:\n", 1)
	path := filepath.Join(s.Root, ".haft", "specs", old.Record.ID+".md")
	if err := os.WriteFile(path, []byte(modified), 0600); err != nil {
		t.Fatal(err)
	}
	old = readView(t, s).Documents[0]
	ref := old.Record.ID + "@" + old.Edition
	preview := public(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref})
	apply := public(t, s, Request{Operation: "change", Action: "reauthor_apply", Ref: ref, RequestID: "public-tag-refusal", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]})
	if apply.Kind != "conflict" || !apply.IsError || len(readView(t, s).Documents) != 1 {
		t.Fatalf("public apply published unsupported conversion: %+v", apply)
	}
	oldRead := s.Call(context.Background(), Request{Format: delivery.Format, Operation: "recall", Ref: originalRef})
	if oldRead.Kind != "found" {
		t.Fatalf("historical source became unreadable: %+v", oldRead)
	}
}
