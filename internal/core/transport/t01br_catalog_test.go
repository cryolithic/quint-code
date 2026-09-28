package transport

import (
	"strings"
	"testing"
)

func TestT01BRPublicCatalogDescribesRepairedSpecBoundaries(t *testing.T) {
	for _, profile := range []string{ProfileDefault, ProfileLegacy} {
		t.Run(profile, func(t *testing.T) {
			definitions := ToolDefinitions(profile)
			for _, target := range []struct {
				tool, action string
				terms        []string
			}{
				{"haft_read", "context", []string{"spec-ref decision candidates", "explicitly unassessed", "Mixed v1/v2 heads", "exact conflicting refs"}},
				{"haft_check", "check/structural", []string{"Mixed active v1/current v2", "exact participants", "unsupported automatic merge"}},
				{"haft_change", "change/reauthor_preview", []string{"pending v1 proposal refs", "normalization apart from semantic losses", "binding decision candidates", "unassessed coverage", "preview_digest"}},
				{"haft_change", "change/reauthor_apply", []string{"same-ID replay", "published_successor_ref", "exact_read_request", "does not claim the edition remains current", "Interrupted/error replies may omit", "replay_conflict"}},
			} {
				definition := t01bDefinition(t, definitions, profile, target.tool)
				schema := definition["inputSchema"].(map[string]any)
				branch := t01bBranch(t, schema, target.action)
				description := branch["description"].(string)
				for _, term := range target.terms {
					if !strings.Contains(description, term) {
						t.Fatalf("%s/%s omits %q: %s", profile, target.action, term, description)
					}
				}
			}
			change := t01bDefinition(t, definitions, profile, "haft_change")
			fields := change["inputSchema"].(map[string]any)["properties"].(map[string]any)
			if !strings.Contains(fields["ref"].(map[string]any)["description"].(string), "published_successor_ref and exact_read_request") || !strings.Contains(fields["preview_digest"].(map[string]any)["description"].(string), "pending proposals") {
				t.Fatal("shared request schema omits exact replay route or proposal-bound digest")
			}
			if strings.Contains(fields["ref"].(map[string]any)["description"].(string), "new_ref") {
				t.Fatal("shared request schema advertises a field absent from the bounded summary")
			}
		})
	}
	for _, term := range []string{"order-stable", "mixed active v1/current v2 heads", "pending v1 proposal refs", "normalization separately from material losses", "published_successor_ref", "exact_read_request", "interrupted/error reply", "replay_conflict"} {
		if !strings.Contains(ToolGuidance(), term) {
			t.Fatalf("CLI/shared guidance omits %q", term)
		}
	}
}

func TestT01BR3PublicCatalogDisclosesSemanticYAMLRefusal(t *testing.T) {
	for _, profile := range []string{ProfileDefault, ProfileLegacy} {
		t.Run(profile, func(t *testing.T) {
			definitions := ToolDefinitions(profile)
			for _, target := range []struct {
				tool, action, term string
			}{
				{"haft_write", "remember", "semantic YAML conversion"},
				{"haft_change", "change/create", "semantic YAML conversion"},
				{"haft_change", "change/preview", "semantic YAML conversion"},
				{"haft_change", "change/apply", "semantic YAML conversion"},
				{"haft_change", "change/sync", "semantic YAML conversion"},
				{"haft_change", "change/reauthor_preview", "semantic YAML conversion"},
				{"haft_change", "change/reauthor_apply", "semantic YAML conversion"},
				{"haft_check", "check/structural", "flat union across lineages"},
			} {
				definition := t01bDefinition(t, definitions, profile, target.tool)
				branch := t01bBranch(t, definition["inputSchema"].(map[string]any), target.action)
				if !strings.Contains(branch["description"].(string), target.term) {
					t.Fatalf("%s/%s omits %q", profile, target.action, target.term)
				}
			}
			carrier := t01bDefinition(t, definitions, profile, "haft_write")["inputSchema"].(map[string]any)["properties"].(map[string]any)["carrier"].(map[string]any)["description"].(string)
			for _, term := range []string{"exact affected paths", "existing carrier bytes remain readable", "established meaning"} {
				if !strings.Contains(carrier, term) {
					t.Fatalf("%s carrier schema omits %q", profile, term)
				}
			}
		})
	}
	for _, term := range []string{"semantic YAML conversion", "retained or supplied content", "flat union across lineages", "existing carriers and pinned history readable"} {
		if !strings.Contains(ToolGuidance(), term) {
			t.Fatalf("shared guidance omits %q", term)
		}
	}
}
