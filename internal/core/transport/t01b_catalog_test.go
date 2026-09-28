package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestT01BReauthorSchemaAndDescriptionsInBothMCPProfiles(t *testing.T) {
	for _, profile := range []string{ProfileDefault, ProfileLegacy} {
		t.Run(profile, func(t *testing.T) {
			definitions := ToolDefinitions(profile)
			for _, operation := range []struct {
				tool   string
				action string
				terms  []string
			}{
				{"haft_write", "remember", []string{"haft/2 spec", "current content", "active haft/1 decision", "direct operator choice"}},
				{"haft_change", "change/reauthor_preview", []string{"exact v1 spec edition", "no ID, time, receipt or publication", "preview_digest", "memory_generation", "binding decision", "implementation/check"}},
				{"haft_change", "change/reauthor_apply", []string{"CAS", "stale", "predecessor bytes", "claim IDs", "historic refs", "authority conflict"}},
			} {
				definition := t01bDefinition(t, definitions, profile, operation.tool)
				schema := definition["inputSchema"].(map[string]any)
				branch := t01bBranch(t, schema, operation.action)
				description := branch["description"].(string)
				for _, term := range operation.terms {
					if !strings.Contains(description, term) {
						t.Fatalf("%s/%s metadata omits %q: %s", profile, operation.action, term, description)
					}
				}
				if operation.action != "change/reauthor_preview" && operation.action != "change/reauthor_apply" {
					continue
				}
				properties := branch["properties"].(map[string]any)
				if _, acceptsCarrier := properties["carrier"]; acceptsCarrier {
					t.Fatalf("%s %s admits replacement carrier", profile, operation.action)
				}
				for _, field := range []string{"ref", "preview_digest", "expected_generation", "request_id"} {
					_, present := properties[field]
					if field == "ref" && !present || field != "ref" && present != (operation.action == "change/reauthor_apply") {
						t.Fatalf("%s %s field %s presence = %t", profile, operation.action, field, present)
					}
				}
			}
			change := t01bDefinition(t, definitions, profile, "haft_change")
			root := change["inputSchema"].(map[string]any)["properties"].(map[string]any)
			for field, term := range map[string]string{
				"ref":                 "exact pinned haft/1 spec edition",
				"preview_digest":      "writer lock",
				"expected_generation": "memory_generation",
			} {
				description := root[field].(map[string]any)["description"].(string)
				if !strings.Contains(description, term) {
					t.Fatalf("%s field %s omits %q", profile, field, term)
				}
			}
		})
	}
	if !strings.Contains(OperationHelp(), "reauthor_preview") || !strings.Contains(OperationHelp(), "reauthor_apply") || !strings.Contains(ToolGuidance(), "current content") {
		t.Fatal("CLI help/catalog guidance omits selected reauthor route")
	}
}

func TestT01BReauthorExamplesAreClosedAndRoutable(t *testing.T) {
	for _, action := range []string{"reauthor_preview", "reauthor_apply"} {
		example := ToolExample("change", action)
		for _, profile := range []string{ProfileDefault, ProfileLegacy} {
			tool := "haft_change"
			if profile == ProfileLegacy {
				tool = "haft"
			}
			request, err := ValidateToolCall(profile, tool, []byte(example))
			if err != nil || request.Operation != "change" || request.Action != action {
				t.Fatalf("%s %s rejected advertised request: %+v %v", profile, action, request, err)
			}
			var supplied map[string]json.RawMessage
			if err := json.Unmarshal([]byte(example), &supplied); err != nil {
				t.Fatal(err)
			}
			supplied["carrier"] = json.RawMessage(`"unselected replacement"`)
			raw, _ := json.Marshal(supplied)
			if _, err := ValidateToolCall(profile, tool, raw); err == nil || !strings.Contains(err.Error(), "unsupported_field") {
				t.Fatalf("%s %s admitted caller replacement: %v", profile, action, err)
			}
		}
	}
}

func t01bDefinition(t *testing.T, definitions []map[string]any, profile, tool string) map[string]any {
	t.Helper()
	if profile == ProfileLegacy {
		tool = "haft"
	}
	for _, definition := range definitions {
		if definition["name"] == tool {
			return definition
		}
	}
	t.Fatalf("%s omits %s", profile, tool)
	return nil
}

func t01bBranch(t *testing.T, schema map[string]any, title string) map[string]any {
	t.Helper()
	for _, raw := range schema["oneOf"].([]any) {
		branch := raw.(map[string]any)
		if branch["title"] == title {
			return branch
		}
	}
	t.Fatalf("schema omits %s", title)
	return nil
}
