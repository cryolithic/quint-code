package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01BR3ReauthorDistinguishesControlRewriteFromRetainedExtension(t *testing.T) {
	t.Run("explicit status and provenance", func(t *testing.T) {
		s := service(t)
		old, _ := seed(t, s)
		raw := strings.Replace(string(old.Raw), "status: proposed", "status: !!str proposed", 1)
		raw = strings.Replace(raw, "origin: agent_proposal", "origin: !!str agent_proposal", 1)
		before, ref := t01br3ExternalSpec(t, s, old, raw)
		preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "ready")
		if losses := preview.Data.(map[string]any)["material_losses"].([]string); len(losses) != 0 {
			t.Fatalf("deliberate status/provenance rewrite reported loss: %+v", losses)
		}
		apply := Request{Operation: "change", Action: "reauthor_apply", Ref: ref, RequestID: "tagged-control-rewrite", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
		run(t, s, apply, "written")
		run(t, s, apply, "replayed")
		after := readView(t, s)
		if !bytes.Equal(after.Documents[0].Raw, before.Raw) || len(after.Documents) != 2 || after.Documents[1].Record.Format != "haft/2" || after.Documents[1].Record.Status != "" || after.Documents[1].Record.Origin != "agent_edit" {
			t.Fatal("intentional reauthor did not preserve old bytes and publish one v2 successor")
		}
	})

	t.Run("unknown field remains guarded", func(t *testing.T) {
		s := service(t)
		old, _ := seed(t, s)
		raw := strings.Replace(string(old.Raw), "status: proposed", "status: !!str proposed\nx-vendor: !vendor abc", 1)
		before, ref := t01br3ExternalSpec(t, s, old, raw)
		preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "conflict")
		t01br3AssertLossPath(t, preview.Diagnostics, "x-vendor")
		if len(readView(t, s).Documents) != 1 || !bytes.Equal(readView(t, s).Documents[0].Raw, before.Raw) {
			t.Fatal("refused reauthor mutated the selected source")
		}
	})
}

func TestT01BR3ReauthorRefusesImplicitScalarLossAtRetainedPaths(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		path string
	}{
		{"root-large-int", "slug: order-cancel", "slug: order-cancel\nx-value: 123456789012345678901234", "x-value"},
		{"root-float", "slug: order-cancel", "slug: order-cancel\nx-value: 1.0", "x-value"},
		{"claim-large-int", "      text: Successful cancellation", "      x-value: 123456789012345678901234\n      text: Successful cancellation", "claims[0].x-value"},
		{"claim-float", "      text: Successful cancellation", "      x-value: 1.0\n      text: Successful cancellation", "claims[0].x-value"},
		{"scenario-float", "          then: Status is cancelled and total remains 123", "          then: Status is cancelled and total remains 123\n          x-value: 1.0", "claims[0].examples[0].x-value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := service(t)
			old, _ := seed(t, s)
			raw := strings.Replace(string(old.Raw), tc.old, tc.new, 1)
			before, ref := t01br3ExternalSpec(t, s, old, raw)
			viewBefore := readView(t, s)
			preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "conflict")
			t01br3AssertLossPath(t, preview.Diagnostics, tc.path)
			data := preview.Data.(map[string]any)
			if data["selected_ref"] != ref || data["old"].(map[string]any)["carrier_digest"] != carrier.Digest(before.Raw) || !strings.Contains(strings.Join(data["material_losses"].([]string), "\n"), tc.path) {
				t.Fatalf("refusal omitted exact source and loss: %+v", data)
			}
			apply := Request{Operation: "change", Action: "reauthor_apply", Ref: ref, RequestID: "reject-" + tc.name, ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
			run(t, s, apply, "conflict")
			run(t, s, apply, "conflict")
			viewAfter := readView(t, s)
			if viewAfter.Generation != viewBefore.Generation || len(viewAfter.Documents) != len(viewBefore.Documents) || !bytes.Equal(viewAfter.Documents[0].Raw, before.Raw) {
				t.Fatal("refused reauthor changed durable memory")
			}
			if tc.name == "root-large-int" {
				response := public(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref})
				if response.Kind != "conflict" || delivery.Size(response) > delivery.Budget {
					t.Fatalf("public scalar refusal unavailable or unbounded: %+v", response)
				}
				oldBytes := collectPart(t, Service{Root: s.Root}, t01br3PartRequest(t, s, response, "old"))
				var oldData map[string]any
				if err := json.Unmarshal(oldBytes, &oldData); err != nil {
					t.Fatal(err)
				}
				if oldData["carrier"] != string(before.Raw) || oldData["carrier_digest"] != carrier.Digest(before.Raw) || oldData["snapshot_digest"] != before.Edition {
					t.Fatalf("public continuation lost exact source/digests: %+v", oldData)
				}
				lossBytes := collectPart(t, s, t01br3PartRequest(t, s, response, "material_losses"))
				if !bytes.Contains(lossBytes, []byte(tc.path)) {
					t.Fatalf("public continuation lost scalar diagnostic: %s", lossBytes)
				}
				diagnosticBytes := collectPart(t, s, t01br3PartRequest(t, s, response, "diagnostics"))
				var diagnostics []carrier.Diagnostic
				if err := json.Unmarshal(diagnosticBytes, &diagnostics); err != nil {
					t.Fatal(err)
				}
				t01br3AssertLossPath(t, diagnostics, tc.path)
			}
		})
	}
}

func TestT01BR3RememberRefusesImplicitScalarLoss(t *testing.T) {
	cases := []struct {
		name   string
		needle string
		add    string
		path   string
	}{
		{"root-large-int", "receiving_use: Review", "x-value: 123456789012345678901234\nreceiving_use: Review", "x-value"},
		{"claim-float", "    text: A drafted rule", "    x-value: 1.0\n    text: A drafted rule", "claims[0].x-value"},
		{"scenario-large-int", "        then: Rule applies", "        then: Rule applies\n        x-value: 123456789012345678901234", "claims[0].examples[0].x-value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := service(t)
			base := "---\nformat: haft/2\nkind: spec\ntitle: Drafted rule\nabout: domain:Billing.OrderCancellation\ncreated_at: 2026-09-23T10:00:00Z\nslug: drafted-rule\nreceiving_use: Review a current content claim\nclaims:\n  - id: rule\n    kind: definition\n    text: A drafted rule is inspectable.\n    examples:\n      - id: case-one\n        given: Fixture\n        then: Rule applies\n---\nBody.\n"
			raw := strings.Replace(base, tc.needle, tc.add, 1)
			if raw == base {
				t.Fatal("scalar fixture not injected")
			}
			before := readView(t, s)
			refused := run(t, s, Request{Operation: "remember", RequestID: "scalar-" + tc.name, Carrier: raw}, "invalid")
			t01br3AssertLossPath(t, refused.Diagnostics, tc.path)
			if tc.name == "root-large-int" {
				response := public(t, s, Request{Operation: "remember", RequestID: "scalar-" + tc.name, Carrier: raw})
				if response.Kind != "invalid" || delivery.Size(response) > delivery.Budget {
					t.Fatalf("public remember refusal unavailable or unbounded: %+v", response)
				}
				t01br3AssertLossPath(t, response.Diagnostics, tc.path)
			}
			after := readView(t, s)
			if after.Generation != before.Generation || len(after.Documents) != 0 {
				t.Fatal("rejected remember changed durable memory")
			}
		})
	}
}

func TestT01BR3RememberAllowsWriterProvenanceAndHarmlessHex(t *testing.T) {
	s := service(t)
	raw := "---\nkind: note\ntitle: Fixture observation\nabout: domain:Billing.OrderCancellation\ncreated_at: 2026-09-23T10:00:00Z\nx-value: 0x1F\n---\nBody.\n"
	request := Request{Operation: "remember", RequestID: "provenance-and-hex", Carrier: raw}
	run(t, s, request, "written")
	run(t, s, request, "replayed")
	documents := readView(t, s).Documents
	if len(documents) != 1 || documents[0].Record.CreatedAt != "2026-09-23T10:00:00Z" || documents[0].Record.Extra["x-value"] == nil {
		t.Fatalf("writer-owned timestamp or retained hex value was lost: %+v", documents)
	}
	tagged := strings.Replace(raw, "created_at: 2026-09-23T10:00:00Z", "created_at: !vendor 2026-09-23T10:00:00Z", 1)
	run(t, s, Request{Operation: "remember", RequestID: "custom-created-at", Carrier: tagged}, "invalid")
	if len(readView(t, s).Documents) != 1 {
		t.Fatal("custom-tagged provenance was published through timestamp normalization")
	}
}

func t01br3ExternalSpec(t *testing.T, s Service, original carrier.Document, raw string) (carrier.Document, string) {
	t.Helper()
	if raw == string(original.Raw) {
		t.Fatal("external fixture did not change source")
	}
	path := filepath.Join(s.Root, ".haft", "specs", original.Record.ID+".md")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	d := readView(t, s).Documents[0]
	if !d.Valid() {
		t.Fatalf("externally authored source invalid: %+v", d.Diagnostics)
	}
	return d, d.Record.ID + "@" + d.Edition
}

func t01br3AssertLossPath(t *testing.T, diagnostics []carrier.Diagnostic, path string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Path == path && diagnostic.Severity == "error" {
			return
		}
	}
	t.Fatalf("semantic loss at %q not diagnosed: %+v", path, diagnostics)
}

func t01br3PartRequest(t *testing.T, s Service, response delivery.Response, name string) delivery.Request {
	t.Helper()
	for _, part := range response.Delivery.Available {
		if part.Name == name {
			return part.Request
		}
	}
	query := response.Delivery.Catalog
	for pageNumber := 0; pageNumber < 64 && query != nil; pageNumber++ {
		page := public(t, s, nextApp(*query))
		if page.Delivery.Encoding != "part_directory" {
			t.Fatalf("missing public part directory for %q: %+v", name, page)
		}
		var directory struct {
			Parts []delivery.Descriptor `json:"parts"`
		}
		if err := json.Unmarshal(rawJSON(page.Data), &directory); err != nil {
			t.Fatal(err)
		}
		for _, part := range directory.Parts {
			if part.Name == name {
				return part.Request
			}
		}
		query = page.Delivery.Next
	}
	t.Fatalf("public conflict did not expose part %q", name)
	return delivery.Request{}
}
