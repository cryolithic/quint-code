package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/change"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01BReauthorSelectedV1PreservesContentHistoryAndCAS(t *testing.T) {
	s := service(t)
	old, oldRef := seedT01BActiveSpec(t, s)
	before := readView(t, s)
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: oldRef}, "ready")
	again := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: oldRef}, "ready")
	if preview.Basis["preview_digest"] == "" || preview.Basis["preview_digest"] != again.Basis["preview_digest"] {
		t.Fatal("preview is not deterministic")
	}
	untouched := readView(t, s)
	if untouched.Generation != before.Generation || len(untouched.Documents) != len(before.Documents) || len(untouched.Snapshots) != len(before.Snapshots) {
		t.Fatal("preview changed canonical project memory")
	}
	data := preview.Data.(map[string]any)
	prototype := data["new_content"].(map[string]any)["prototype_carrier"].(string)
	if strings.Contains(prototype, "operator_confirmed:") || strings.Contains(prototype, "status:") || !strings.Contains(prototype, "custom_claim:") || !strings.Contains(prototype, "extension:") || !strings.Contains(prototype, "Preserve this prose and its **formatting**.") {
		t.Fatalf("prototype lost content or retained lifecycle field: %s", prototype)
	}
	q := Request{Operation: "change", Action: "reauthor_apply", Ref: oldRef, RequestID: "reauthor-once", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	wrong := q
	wrong.PreviewDigest = carrier.Digest([]byte("wrong preview"))
	run(t, s, wrong, "conflict")
	if len(readView(t, s).Documents) != 1 {
		t.Fatal("changed preview published content")
	}
	run(t, s, q, "written")
	run(t, s, q, "replayed")
	wrong = q
	wrong.PreviewDigest = carrier.Digest([]byte("different payload"))
	run(t, s, wrong, "request_conflict")
	after := readView(t, s)
	if len(after.Documents) != 2 {
		t.Fatalf("want original and one successor, got %d", len(after.Documents))
	}
	if !bytes.Equal(before.Documents[0].Raw, after.Documents[0].Raw) || !bytes.Equal(before.CurrentSnapshots[old.Edition], after.Snapshots[old.Edition]) {
		t.Fatal("reauthor changed original carrier or its persisted snapshot")
	}
	var current carrier.Document
	for _, d := range after.Documents {
		if d.Record.Format == "haft/2" {
			current = d
		}
	}
	if current.Record.ID == "" || current.Record.Status != "" || current.Record.OperatorConfirmed || current.Record.Origin != "agent_edit" || len(current.Record.Supersedes) != 1 || current.Record.Supersedes[0] != oldRef {
		t.Fatalf("wrong current-content successor: %+v", current.Record)
	}
	if !sameJSON(current.Record.Claims, old.Record.Claims) || !sameJSON(current.Record.Extra, old.Record.Extra) || !sameJSON(current.Record.Sources, old.Record.Sources) || !bytes.Equal(current.Body, old.Body) || current.Record.About != old.Record.About || !sameJSON(current.Record.Terms, old.Record.Terms) {
		t.Fatal("reauthor lost claim, extension, source, body, about or terms")
	}
	if e := after.Projection.Resolve("spec:order-cancel"); e.Kind != "found" || e.Document.Record.ID != current.Record.ID {
		t.Fatalf("v2 is not current content: %+v", e)
	}
	run(t, s, Request{Operation: "recall", Ref: oldRef + "#L1"}, "found")
}

func TestT01BNewSpecHasNoStatusAndCannotBindDecision(t *testing.T) {
	s := service(t)
	base := "---\nformat: haft/2\nkind: spec\ntitle: Drafted rule\nabout: domain:Billing.OrderCancellation\nslug: drafted-rule\nreceiving_use: Review a current content claim\nclaims:\n  - id: rule\n    kind: definition\n    text: A drafted rule is inspectable.\n---\nBody.\n"
	run(t, s, Request{Operation: "remember", RequestID: "v2-new", Carrier: base}, "written")
	doc := readView(t, s).Documents[0]
	if doc.Record.Format != "haft/2" || doc.Record.Status != "" || doc.Record.OperatorConfirmed || doc.Record.Origin != "agent_edit" || readView(t, s).Projection.Resolve("spec:drafted-rule").Kind != "found" {
		t.Fatal("new v2 spec did not become current content")
	}
	if strings.Contains(string(doc.Raw), "status:") || strings.Contains(string(doc.Raw), "operator_confirmed:") {
		t.Fatal("new v2 spec serialized a forbidden status field")
	}
	for _, field := range []string{"status: proposed\n", "operator_confirmed: false\n", "legacy_status: active\n", "origin: operator_edit\n"} {
		candidate := strings.Replace(base, "kind: spec\n", "kind: spec\n"+field, 1)
		run(t, s, Request{Operation: "remember", RequestID: "reject-" + strings.TrimSpace(field), Carrier: candidate}, "invalid")
	}
	unsupported := strings.Replace(base, "format: haft/2", "format: haft/3", 1)
	run(t, s, Request{Operation: "remember", RequestID: "unsupported-v3", Carrier: unsupported}, "invalid")
	decision := "---\nformat: haft/1\nkind: decision\ntitle: Bind cancellation policy\nabout: domain:Billing.OrderCancellation\nstatus: active\nobject: Billing.OrderCancellation\nquestion: Which policy governs cancellation?\nwhy: The domain needs one choice.\ndisposition: choose_now\nchosen: allow\nno_alternative_reason: Only one supplied candidate.\n---\n"
	run(t, s, Request{Operation: "remember", RequestID: "unconfirmed-decision", Carrier: decision}, "invalid")
	if len(readView(t, s).Documents) != 1 {
		t.Fatal("invalid v2 fields or unconfirmed decision published")
	}
}

func TestT01BReauthorRefusesMovedGenerationOrSelectedHead(t *testing.T) {
	s := service(t)
	old, oldRef := seed(t, s)
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: oldRef}, "ready")
	q := Request{Operation: "change", Action: "reauthor_apply", Ref: oldRef, RequestID: "stale-generation", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	run(t, s, Request{Operation: "remember", RequestID: "intervening-note", Carrier: "---\nkind: note\ntitle: Independent note\nabout: domain:Billing.OrderCancellation\n---\nIndependent.\n"}, "written")
	run(t, s, q, "conflict")
	if len(readView(t, s).Documents) != 2 {
		t.Fatal("stale generation created successor")
	}
	path := filepath.Join(s.Root, ".haft", "specs", old.Record.ID+".md")
	if err := os.WriteFile(path, append(old.Raw, []byte("\nChanged live bytes.\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: oldRef}, "conflict")
	if len(readView(t, s).Documents) != 2 {
		t.Fatal("stale predecessor preview created successor")
	}
}

func TestT01BProposedBranchRequiresExactSelectedHead(t *testing.T) {
	s := service(t)
	old, activeRef := seedT01BActiveSpec(t, s)
	proposal := old.Record
	proposal.ID = "spec-20260923-00000002"
	proposal.Status = "proposed"
	proposal.Origin = "agent_proposal"
	proposal.OperatorConfirmed = false
	proposal.Supersedes = []string{activeRef}
	proposal.SupersedeReason = "Propose an authored refinement"
	run(t, s, Request{Operation: "remember", RequestID: "v1-proposal", Carrier: encode(t, proposal, old.Body), ExpectedHeads: []string{activeRef}}, "written")
	v := readView(t, s)
	var proposedRef string
	for _, d := range v.Documents {
		if d.Record.ID == proposal.ID {
			proposedRef = proposal.ID + "@" + d.Edition
		}
	}
	if proposedRef == "" {
		t.Fatal("missing proposed ref")
	}
	got := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: proposedRef}, "conflict")
	if !hasAppDiagnostic(got, "reauthor_head_conflict") {
		t.Fatalf("parallel active head not named: %+v", got.Diagnostics)
	}
	if len(readView(t, s).Documents) != 2 {
		t.Fatal("conflict created successor")
	}
}

func TestT01BRReauthorPreviewDisclosesPendingV1Proposal(t *testing.T) {
	s := service(t)
	old, activeRef := seedT01BActiveSpec(t, s)
	proposal := old.Record
	proposal.ID = "spec-20260923-00000002"
	proposal.Title = "Pending cancellation refinement"
	proposal.Status = "proposed"
	proposal.Origin = "agent_proposal"
	proposal.OperatorConfirmed = false
	proposal.Supersedes = []string{activeRef}
	proposal.SupersedeReason = "Suggest a separate refinement"
	run(t, s, Request{Operation: "remember", RequestID: "pending-proposal", Carrier: encode(t, proposal, old.Body), ExpectedHeads: []string{activeRef}}, "written")
	before := readView(t, s)
	var proposedRef string
	var proposedBytes []byte
	for _, document := range before.Documents {
		if document.Record.ID == proposal.ID {
			proposedRef = proposal.ID + "@" + document.Edition
			proposedBytes = bytes.Clone(document.Raw)
		}
	}
	if proposedRef == "" {
		t.Fatal("pending proposal was not published")
	}
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: activeRef}, "ready")
	data := preview.Data.(map[string]any)
	pending := data["pending_v1_proposals"].([]map[string]string)
	if len(pending) != 1 || pending[0]["ref"] != proposedRef || pending[0]["title"] != proposal.Title || pending[0]["state"] != carrier.Proposed {
		t.Fatalf("preview omitted exact pending proposal: %+v", pending)
	}
	if consequence := data["proposal_consequence"].(string); !strings.Contains(consequence, "unchanged and unaccepted") || !strings.Contains(consequence, "separate authored change") {
		t.Fatalf("preview hid conversion consequence: %q", consequence)
	}
	if got := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: activeRef}, "ready"); got.Basis["preview_digest"] != preview.Basis["preview_digest"] {
		t.Fatal("proposal-aware preview digest changed without a write")
	}
	public := s.Call(context.Background(), Request{Format: delivery.Format, Operation: "change", Action: "reauthor_preview", Ref: activeRef})
	if public.Kind != "ready" || public.Delivery.Catalog == nil || delivery.Size(public) > delivery.Budget {
		t.Fatalf("pending-proposal public preview unavailable or unbounded: %+v", public)
	}
	summary := object(public.Data)
	if !bytes.Contains(rawJSON(summary), []byte(`"pending_v1_proposal_count":1`)) || !strings.Contains(str(summary["proposal_consequence"]), "separate authored change") {
		t.Fatalf("public preview summary omitted proposal consequence: %+v", summary)
	}
	part := s.Call(context.Background(), Request{Format: delivery.Format, Operation: "read", Ref: public.Delivery.Catalog.Ref, View: "detail", Part: "pending_v1_proposals", ExpectedGeneration: public.Basis["memory_generation"]})
	if part.Kind != "ready" || !part.Delivery.Complete || !bytes.Contains(rawJSON(part.Data), []byte(proposedRef)) {
		t.Fatalf("pending proposal part cannot be read exactly: %+v", part)
	}
	apply := Request{Operation: "change", Action: "reauthor_apply", Ref: activeRef, RequestID: "reauthor-with-pending", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	run(t, s, apply, "written")
	after := readView(t, s)
	foundProposal := false
	for _, document := range after.Documents {
		if document.Record.ID == proposal.ID {
			foundProposal = true
			if !bytes.Equal(document.Raw, proposedBytes) || document.Record.Status != carrier.Proposed {
				t.Fatal("reauthor converted or accepted the pending v1 proposal")
			}
		}
	}
	if !foundProposal {
		t.Fatal("reauthor removed the pending v1 proposal")
	}
	refused := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: proposedRef}, "conflict")
	if !hasAppDiagnostic(refused, "reauthor_head_conflict") {
		t.Fatal("pending proposal was implicitly converted after selected reauthor")
	}
}

func TestT01BRReauthorPreviewDistinguishesNormalizationFromSemanticLoss(t *testing.T) {
	s := service(t)
	terms, err := os.ReadFile("../testdata/order/terms.md")
	if err != nil {
		t.Fatal(err)
	}
	run(t, s, Request{Operation: "remember", Action: "terms", Carrier: string(terms), RequestID: "normalization-terms"}, "written")
	raw, err := os.ReadFile("../carrier/testdata/valid/spec.md")
	if err != nil {
		t.Fatal(err)
	}
	commented := strings.Replace(string(raw), "title: Cancel preserves total", "# Historical frontmatter comment\ntitle: Cancel preserves total", 1)
	commented = strings.Replace(commented, "extension:\n", "future_limit: 0x1F\nextension:\n", 1)
	run(t, s, Request{Operation: "remember", RequestID: "commented-spec", Carrier: commented}, "written")
	// The writer itself normalizes YAML. Recreate a valid externally edited
	// historical carrier in the owned fixture so preview sees its exact bytes.
	old := readView(t, s).Documents[0]
	carrierPath := filepath.Join(s.Root, ".haft", "specs", old.Record.ID+".md")
	commented = strings.Replace(string(old.Raw), "title: Cancel preserves total", "# Historical frontmatter comment\ntitle: Cancel preserves total", 1)
	commented = strings.Replace(commented, "future_limit: 31", "future_limit: 0x1F", 1)
	if err := os.WriteFile(carrierPath, []byte(commented), 0600); err != nil {
		t.Fatal(err)
	}
	old = readView(t, s).Documents[0]
	ref := old.Record.ID + "@" + old.Edition
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "ready")
	data := preview.Data.(map[string]any)
	oldCarrier := data["old"].(map[string]any)["carrier"].(string)
	prototype := data["new_content"].(map[string]any)["prototype_carrier"].(string)
	if !strings.Contains(oldCarrier, "# Historical frontmatter comment") || !strings.Contains(oldCarrier, "future_limit: 0x1F") || strings.Contains(prototype, "# Historical frontmatter comment") || strings.Contains(prototype, "future_limit: 0x1F") || !strings.Contains(prototype, "future_limit: 31") {
		t.Fatalf("preview did not distinguish exact old bytes from normalized prototype: %s", prototype)
	}
	note := data["normalization_note"].(string)
	if !strings.Contains(note, "comments and YAML lexical form") || !strings.Contains(note, "Exact predecessor carrier, snapshot and body bytes") || len(data["material_losses"].([]string)) != 0 {
		t.Fatalf("normalization and semantic loss were conflated: %q %+v", note, data["material_losses"])
	}
	public := s.Call(context.Background(), Request{Format: delivery.Format, Operation: "change", Action: "reauthor_preview", Ref: ref})
	if public.Kind != "ready" || !strings.Contains(str(object(public.Data)["normalization_note"]), "comments and YAML lexical form") || delivery.Size(public) > delivery.Budget {
		t.Fatalf("normalization note was not publicly disclosed: %+v", public)
	}
}

func TestT01BRReauthorPreviewKeepsDecisionMatchAdvisory(t *testing.T) {
	s := service(t)
	old, ref := seedT01BActiveSpec(t, s)
	decision := contextDecision("dec-20260923-00000001", carrier.Active)
	decision.About = old.Record.About
	run(t, s, Request{Operation: "remember", RequestID: "reauthor-same-about-choice", Carrier: encode(t, decision, nil)}, "written")
	preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "ready")
	authority := preview.Data.(map[string]any)["decision_authority"].(map[string]any)
	rows := authority["binding_decisions"].([]map[string]any)
	if authority["assessment"] != "not_assessed" || authority["selection_rule"] != "same_about_or_shared_declared_selector" || authority["candidate_count"] != 1 || len(rows) != 1 {
		t.Fatalf("preview did not use the bounded unassessed decision candidate band: %+v", authority)
	}
	matches := rows[0]["match_kinds"].([]string)
	if rows[0]["ref"] == "" || len(matches) != 1 || matches[0] != "same_about" {
		t.Fatalf("preview omitted the same-about match basis: %+v", rows)
	}
}

func TestT01BSelectedStandaloneProposalAndHistoricalMigration(t *testing.T) {
	for _, scenario := range []string{"standalone_proposed", "migrated_historical"} {
		t.Run(scenario, func(t *testing.T) {
			s := service(t)
			var selected carrier.Document
			var ref string
			if scenario == "standalone_proposed" {
				selected, ref = seed(t, s)
			} else {
				terms, err := os.ReadFile("../testdata/order/terms.md")
				if err != nil {
					t.Fatal(err)
				}
				run(t, s, Request{Operation: "remember", Action: "terms", RequestID: "migration-terms", Carrier: string(terms)}, "written")
				raw, err := os.ReadFile("../carrier/testdata/valid/spec.md")
				if err != nil {
					t.Fatal(err)
				}
				d := carrier.Parse(raw)
				d.Record.Origin = "migrated_9x"
				d.Record.OperatorConfirmed = false
				d.Record.LegacyStatus = "migrated legacy status"
				run(t, s, Request{Operation: "remember", RequestID: "historical-v1", Carrier: encode(t, d.Record, d.Body)}, "written")
				selected = readView(t, s).Documents[0]
				ref = selected.Record.ID + "@" + selected.Edition
				if got := readView(t, s).Projection.Entries[0].State; got != carrier.Historical {
					t.Fatalf("migrated v1 became %s", got)
				}
			}
			oldSnapshot := readView(t, s).CurrentSnapshots[selected.Edition]
			preview := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: ref}, "ready")
			run(t, s, Request{Operation: "change", Action: "reauthor_apply", Ref: ref, RequestID: "reauthor-" + scenario, ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}, "written")
			v := readView(t, s)
			if len(v.Documents) != 2 || !bytes.Equal(v.Documents[0].Raw, selected.Raw) || !bytes.Equal(v.Snapshots[selected.Edition], oldSnapshot) {
				t.Fatal("selected v1 history was lost")
			}
			current := v.Projection.Resolve("spec:" + selected.Record.Slug)
			if current.Kind != "found" || current.Document.Record.Format != "haft/2" {
				t.Fatalf("selected %s failed to produce current v2: %+v", scenario, current)
			}
		})
	}
}

func TestT01BV2PatchProducesCurrentWithoutAuthorityMetadata(t *testing.T) {
	s := service(t)
	_, oldRef := seed(t, s)
	p := run(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: oldRef}, "ready")
	run(t, s, Request{Operation: "change", Action: "reauthor_apply", Ref: oldRef, RequestID: "v2-start", ExpectedGeneration: p.Basis["memory_generation"], PreviewDigest: p.Basis["preview_digest"]}, "written")
	v := readView(t, s)
	var current carrier.Document
	for _, d := range v.Documents {
		if d.Record.Format == "haft/2" {
			current = d
		}
	}
	ref := current.Record.ID + "@" + current.Edition
	claim := current.Record.Claims[0]
	claim.Text += " Explicit v2 edit."
	c := change.Change{Format: change.Format, ID: "chg-20260923-00000099", ChangeKey: "chg-20260923-00000099", Title: "Refine current content", Intent: "Clarify exact v2 law", State: "open", CreatedAt: s.now(), Patches: []change.SectionPatch{{Base: ref, Operations: []change.Operation{{Op: "MODIFIED", ClaimID: claim.ID, Claim: &claim, Reason: "Clarify stated content"}}}}}
	raw, err := change.Encode(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	run(t, s, Request{Operation: "change", Action: "create", RequestID: "v2-patch", Carrier: string(raw)}, "written")
	preview := run(t, s, Request{Operation: "change", Action: "preview", Ref: c.ID}, "ready")
	output := preview.Data.(change.PreviewResult).Outputs[0]
	if output.Successor.Format != "haft/2" || output.Successor.Status != "" || output.Successor.OperatorConfirmed || output.Successor.Origin != "agent_edit" {
		t.Fatalf("v2 patch changed authority model: %+v", output.Successor)
	}
	apply := Request{Operation: "change", Action: "apply", Ref: preview.Basis["change_ref"], RequestID: "v2-patch-apply", ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	bad := apply
	bad.Metadata = map[string]change.Metadata{ref: {Status: "active", OperatorConfirmed: true}}
	run(t, s, bad, "invalid")
	run(t, s, apply, "written")
	current = carrier.Document{}
	for _, d := range readView(t, s).Documents {
		if d.Record.Format == "haft/2" && d.Record.ID != output.Successor.ID && d.Record.ID != strings.Split(ref, "@")[0] {
			current = d
		}
	}
	if current.Record.ID == "" || current.Record.Status != "" || current.Record.OperatorConfirmed || current.Record.Origin != "agent_edit" || current.Record.Claims[0].Text != claim.Text {
		t.Fatal("v2 patch did not publish current content")
	}
}

func hasAppDiagnostic(r Result, code string) bool {
	for _, d := range r.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

func seedT01BActiveSpec(t *testing.T, s Service) (carrier.Document, string) {
	t.Helper()
	terms, err := os.ReadFile("../testdata/order/terms.md")
	if err != nil {
		t.Fatal(err)
	}
	run(t, s, Request{Operation: "remember", Action: "terms", Carrier: string(terms), RequestID: "t01b-terms"}, "written")
	raw, err := os.ReadFile("../carrier/testdata/valid/spec.md")
	if err != nil {
		t.Fatal(err)
	}
	run(t, s, Request{Operation: "remember", Carrier: string(raw), RequestID: "t01b-active-spec"}, "written")
	d := readView(t, s).Documents[0]
	return d, d.Record.ID + "@" + d.Edition
}
