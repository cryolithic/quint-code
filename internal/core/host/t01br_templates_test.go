package host

import (
	"strings"
	"testing"
)

func TestT01BRGeneratedSpecGuidanceExplainsRepairAndReadRoutes(t *testing.T) {
	guide := Skills()["h-spec"]
	for _, term := range []string{
		"independently of file order", "both exact refs", "no automatic merge",
		"pending_v1_proposals", "separately authored v2 change",
		"normalization_note", "material_losses", "byte-exact",
		"advisory same-about", "not_assessed even when empty",
		"published_successor_ref", "exact_read_request", "identical request_id replay",
		"interrupted or error reply", "replay_conflict", "re-query context",
	} {
		if !strings.Contains(guide, term) {
			t.Fatalf("generated h-spec omits %q", term)
		}
	}
	if !strings.Contains(agents, "Exact supersession is order-stable") || !strings.Contains(agents, "published_successor_ref") || !strings.Contains(agents, "exact_read_request") {
		t.Fatal("generated project guidance omits the shared spec repair contract")
	}
}

func TestT01BR3GeneratedSpecGuidanceExplainsSemanticRefusal(t *testing.T) {
	guide := Skills()["h-spec"]
	for _, term := range []string{
		"flat union across lineages", "unsupported semantic YAML conversion",
		"retained source or supplied change content", "exact affected path",
		"original bytes", "carrier bytes, snapshots and", "numeric value",
		"separately authored supported", "established meaning",
	} {
		if !strings.Contains(guide, term) {
			t.Fatalf("generated h-spec omits %q", term)
		}
	}
}

func TestT01BR6GeneratedSpecGuidanceNamesRevisionReplacementBoundary(t *testing.T) {
	guide := Skills()["h-spec"]
	for _, term := range []string{
		"omitted examples are preserved", "remove_examples", "remove_fields",
		"omitted extension is retained", "whole-task replacement",
		"returned diagnostics", "unchanged same-ID task", "no one-step removal",
	} {
		if !strings.Contains(guide, term) {
			t.Fatalf("generated h-spec omits %q", term)
		}
	}
}
