package transport

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
)

func TestT01BR4GuidanceDescribesSchemaAndEffectScopedPreservation(t *testing.T) {
	for _, profile := range []string{ProfileDefault, ProfileLegacy} {
		t.Run(profile, func(t *testing.T) {
			var initialized bytes.Buffer
			server := Server{Service: app.Service{Root: t.TempDir()}, Profile: profile}
			if err := server.Serve(context.Background(), strings.NewReader(initialize()), &initialized); err != nil {
				t.Fatal(err)
			}
			intro := rpcRead(t, bufio.NewReader(strings.NewReader(initialized.String())))
			instructions := intro["result"].(map[string]any)["instructions"].(string)
			for _, term := range []string{"Remember across carrier kinds", "Schema-owned fields", "extensions retain raw YAML meaning", "exact read route"} {
				if !strings.Contains(instructions, term) {
					t.Fatalf("%s initialize instructions omit %q", profile, term)
				}
			}
			definitions := ToolDefinitions(profile)
			write := t01bDefinition(t, definitions, profile, "haft_write")
			change := t01bDefinition(t, definitions, profile, "haft_change")
			for _, target := range []struct {
				tool   map[string]any
				action string
				terms  []string
			}{
				{write, "remember", []string{"semantic YAML conversion", "exact paths"}},
				{change, "change/create", []string{"semantic YAML conversion", "exact-path"}},
				{change, "change/preview", []string{"semantic YAML conversion", "affected paths"}},
				{change, "change/apply", []string{"semantic YAML conversion", "before publication"}},
				{change, "change/sync", []string{"semantic YAML conversion", "before publication"}},
				{change, "change/reauthor_preview", []string{"semantic YAML conversion", "old carrier remains readable"}},
				{change, "change/reauthor_apply", []string{"semantic YAML conversion", "exact affected paths"}},
				{change, "change/archive", []string{"retained YAML stays guarded", "intent/tasks/patches are ignored"}},
				{change, "change/reopen", []string{"retained YAML stays guarded", "intent/tasks/patches are ignored"}},
				{change, "change/rebase", []string{"only applied revision fields", "retained YAML stays guarded"}},
				{change, "change/update", []string{"only applied revision fields", "retained YAML stays guarded"}},
			} {
				branch := t01bBranch(t, target.tool["inputSchema"].(map[string]any), target.action)
				description := branch["description"].(string)
				for _, term := range target.terms {
					if !strings.Contains(description, term) {
						t.Fatalf("%s/%s omits %q", profile, target.action, term)
					}
				}
			}
			properties := write["inputSchema"].(map[string]any)["properties"].(map[string]any)
			carrier := properties["carrier"].(map[string]any)["description"].(string)
			for _, term := range []string{"Remember across carrier kinds", "change inputs", "declared schema fields", "extensions keep raw tags", "exact affected paths", "exact read route or pinned ref"} {
				if !strings.Contains(carrier, term) {
					t.Fatalf("%s carrier schema omits %q", profile, term)
				}
			}
			for _, tool := range []map[string]any{write, change} {
				description := tool["description"].(string)
				if strings.Contains(description, "Spec writers refuse") || strings.Contains(description, "lossy spec output") {
					t.Fatalf("%s tool description retains narrow preservation claim: %s", profile, description)
				}
			}
		})
	}
	for _, term := range []string{"Remember across carrier kinds", "change create/revise/preview/apply/sync/reauthor", "Schema-owned fields", "extensions retain raw YAML meaning", "actual effect", "exact affected paths", "exact read route"} {
		if !strings.Contains(ToolGuidance(), term) {
			t.Fatalf("shared guidance omits %q", term)
		}
	}
}
