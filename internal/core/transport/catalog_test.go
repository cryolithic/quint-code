package transport

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
)

func TestAlphaCatalogCarriersReachWriter(t *testing.T) {
	service := app.Service{Root: t.TempDir()}
	spec, err := DecodeRequest([]byte(ToolExample("remember", "")))
	if err != nil {
		t.Fatal(err)
	}
	written := service.Call(context.Background(), spec)
	if written.Kind != "written" || written.Failed() {
		t.Fatalf("catalog spec did not reach writer: %+v", written)
	}
	selected := service.Call(context.Background(), app.Request{Format: "haft.api/2", Operation: "recall", Ref: "spec:alpha-cancel"})
	if selected.Kind != "found" || selected.Failed() {
		t.Fatalf("catalog spec cannot be recalled: %+v", selected)
	}
	base := selected.Data.(map[string]any)["exact_ref"].(string)
	change, err := DecodeRequest([]byte(ToolExample("change", "create")))
	if err != nil {
		t.Fatal(err)
	}
	placeholder := "spec-20260928-a1b2c3d4@sha256:" + strings.Repeat("0", 64)
	if !strings.Contains(change.Carrier, placeholder) {
		t.Fatal("catalog change example omitted replaceable exact base")
	}
	change.Carrier = strings.Replace(change.Carrier, placeholder, base, 1)
	created := service.Call(context.Background(), change)
	if created.Kind != "written" || created.Failed() {
		t.Fatalf("catalog change did not reach writer: %+v", created)
	}
}

func TestCatalogExamplesAndClosedBranches(t *testing.T) {
	definitions := ToolDefinitions(ProfileDefault)
	if len(definitions) != 5 || len(ToolDefinitions(ProfileLegacy)) != 1 {
		t.Fatalf("unexpected profile tool counts: default=%d legacy=%d", len(definitions), len(ToolDefinitions(ProfileLegacy)))
	}
	legacy := ToolDefinitions(ProfileLegacy)[0]
	if legacy["name"] != "haft" {
		t.Fatalf("legacy catalog: %v", legacy["name"])
	}
	legacySchema := legacy["inputSchema"].(map[string]any)
	legacyBranches := legacySchema["oneOf"].([]any)
	branchCount := 0
	destructive := map[string]bool{"haft_write": true, "haft_change": true, "haft_check": true}
	for index, tool := range taskCatalog {
		definition := definitions[index]
		if definition["name"] != tool.Name {
			t.Fatalf("tool order/name: got %v want %s", definition["name"], tool.Name)
		}
		annotations := definition["annotations"].(map[string]any)
		if annotations["readOnlyHint"] != false || annotations["destructiveHint"] != destructive[tool.Name] || annotations["idempotentHint"] != false || annotations["openWorldHint"] != false {
			t.Fatalf("effect annotations for %s: %v", tool.Name, annotations)
		}
		schema := definition["inputSchema"].(map[string]any)
		if schema["additionalProperties"] != false {
			t.Fatalf("open tool schema: %s", tool.Name)
		}
		branches := schema["oneOf"].([]any)
		position := 0
		for _, operation := range tool.Operations {
			if ToolForOperation(operation.Name) != tool.Name {
				t.Fatalf("operation routing drift: %s", operation.Name)
			}
			for _, action := range operation.Actions {
				branch := branches[position].(map[string]any)
				assertCatalogBranch(t, schema, branch, operation, action)
				assertCatalogBranch(t, legacySchema, legacyBranches[branchCount].(map[string]any), operation, action)
				example := ToolExample(operation.Name, action.Name)
				var supplied map[string]json.RawMessage
				if err := json.Unmarshal([]byte(example), &supplied); err != nil {
					t.Fatalf("%s/%s example JSON: %v", operation.Name, action.Name, err)
				}
				request, err := ValidateToolCall(ProfileDefault, tool.Name, []byte(example))
				if err != nil || request.Operation != operation.Name || request.Action != action.Name {
					t.Fatalf("%s/%s example does not call advertised tool: %+v %v", operation.Name, action.Name, request, err)
				}
				if _, err := ValidateToolCall(ProfileLegacy, "haft", []byte(example)); err != nil {
					t.Fatalf("%s/%s legacy example: %v", operation.Name, action.Name, err)
				}
				assertInvalidCatalogControls(t, tool.Name, operation, action, supplied)
				position++
				branchCount++
			}
		}
		if position != len(branches) {
			t.Fatalf("%s has %d catalog actions but %d schema branches", tool.Name, position, len(branches))
		}
	}
	if branchCount != len(legacyBranches) {
		t.Fatalf("legacy schema has %d branches; catalog has %d", len(legacyBranches), branchCount)
	}
	toolList, err := json.Marshal(map[string]any{"tools": definitions})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("serialized default tools/list metadata: %d bytes", len(toolList))
}

func TestCatalogAcceptsOptionalNullControl(t *testing.T) {
	raw := []byte(`{"format":"haft.api/2","operation":"context","code_config":null}`)
	for _, profile := range []string{ProfileDefault, ProfileLegacy} {
		name := "haft_read"
		if profile == ProfileLegacy {
			name = "haft"
		}
		request, err := ValidateToolCall(profile, name, raw)
		if err != nil || request.CodeConfig != nil {
			t.Fatalf("%s did not treat optional null as omission: %+v / %v", profile, request.CodeConfig, err)
		}
	}
}

func assertCatalogBranch(t *testing.T, root, branch map[string]any, operation catalogOperation, action catalogAction) {
	t.Helper()
	if branch["additionalProperties"] != false {
		t.Fatalf("open branch %s/%s", operation.Name, action.Name)
	}
	properties := branch["properties"].(map[string]any)
	allowed := map[string]bool{"format": true, "operation": true, "action": true, "offset": true}
	for _, field := range strings.Fields(action.Fields) {
		allowed[field] = true
	}
	if len(properties) != len(allowed) {
		t.Fatalf("%s/%s branch field count %d want %d", operation.Name, action.Name, len(properties), len(allowed))
	}
	for field := range allowed {
		if _, present := properties[field]; !present {
			t.Fatalf("%s/%s missing schema field %s", operation.Name, action.Name, field)
		}
		if _, present := root["properties"].(map[string]any)[field]; !present {
			t.Fatalf("%s/%s missing root type for %s", operation.Name, action.Name, field)
		}
	}
	required := map[string]bool{"format": true, "operation": true}
	if action.Name != "" {
		required["action"] = true
	}
	for _, field := range strings.Fields(action.Required) {
		required[field] = true
	}
	actual := branch["required"].([]string)
	if len(actual) != len(required) {
		t.Fatalf("%s/%s required count %v want %v", operation.Name, action.Name, actual, required)
	}
	fields := fieldTypes(reflect.TypeOf(app.Request{}))
	for _, field := range actual {
		if !required[field] {
			t.Fatalf("%s/%s unexpected required field %s", operation.Name, action.Name, field)
		}
		if fields[field].Kind() == reflect.String && properties[field].(map[string]any)["minLength"] != 1 {
			t.Fatalf("%s/%s required string %s admits empty value", operation.Name, action.Name, field)
		}
	}
	if properties["operation"].(map[string]any)["enum"].([]string)[0] != operation.Name {
		t.Fatalf("%s/%s operation schema mismatch", operation.Name, action.Name)
	}
	if properties["action"].(map[string]any)["enum"].([]string)[0] != action.Name {
		t.Fatalf("%s/%s action schema mismatch", operation.Name, action.Name)
	}
}

func assertInvalidCatalogControls(t *testing.T, tool string, operation catalogOperation, action catalogAction, supplied map[string]json.RawMessage) {
	t.Helper()
	without := func(field string) []byte {
		copy := map[string]json.RawMessage{}
		for key, value := range supplied {
			if key != field {
				copy[key] = value
			}
		}
		raw, _ := json.Marshal(copy)
		return raw
	}
	for _, field := range strings.Fields(action.Required) {
		if _, err := ValidateToolCall(ProfileDefault, tool, without(field)); err == nil || !strings.Contains(err.Error(), "required_field") {
			t.Fatalf("%s/%s missing required %s: %v", operation.Name, action.Name, field, err)
		}
	}
	allowed := map[string]bool{"format": true, "operation": true, "action": true, "offset": true}
	for _, field := range strings.Fields(action.Fields) {
		allowed[field] = true
	}
	for _, candidate := range []string{"query", "carrier", "ref", "preview_digest", "check_ref", "scope", "part", "cursor", "failure_contract", "request_id"} {
		if allowed[candidate] {
			continue
		}
		supplied[candidate] = json.RawMessage(`"unrelated"`)
		raw, _ := json.Marshal(supplied)
		if _, err := ValidateToolCall(ProfileDefault, tool, raw); err == nil || !strings.Contains(err.Error(), "unsupported_field") {
			t.Fatalf("%s/%s accepted unrelated %s: %v", operation.Name, action.Name, candidate, err)
		}
		if _, err := ValidateToolCall(ProfileLegacy, "haft", raw); err == nil || !strings.Contains(err.Error(), "unsupported_field") {
			t.Fatalf("legacy %s/%s accepted unrelated %s: %v", operation.Name, action.Name, candidate, err)
		}
		delete(supplied, candidate)
		break
	}
	supplied["action"] = json.RawMessage(`"unsupported_action"`)
	raw, _ := json.Marshal(supplied)
	if _, err := ValidateToolCall(ProfileDefault, tool, raw); err == nil || !strings.Contains(err.Error(), "unsupported_action") {
		t.Fatalf("%s accepted unsupported action: %v", operation.Name, err)
	}
	if _, err := ValidateToolCall(ProfileLegacy, "haft", raw); err == nil || !strings.Contains(err.Error(), "unsupported_action") {
		t.Fatalf("legacy %s accepted unsupported action: %v", operation.Name, err)
	}
}
