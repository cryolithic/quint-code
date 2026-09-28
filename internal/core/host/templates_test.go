package host

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/transport"
)

func TestHostInstructionsNameTaskToolsAndContinuation(t *testing.T) {
	for _, operation := range []string{"read", "remember", "change", "check", "fpf"} {
		tool := transport.ToolForOperation(operation)
		if tool == "" || !strings.Contains(agents, tool) {
			t.Errorf("project instructions omit %s tool %q", operation, tool)
		}
	}
	read := transport.ToolForOperation("read")
	if !strings.Contains(agents, "pass a returned next_request") || !strings.Contains(agents, "as arguments to "+read) {
		t.Fatal("project instructions do not route disclosed continuation")
	}
	if strings.Contains(agents, "the haft tool") {
		t.Fatal("project instructions still advertise the singular B1 MCP tool")
	}
}

func TestSkillExamplesUseCatalogToolAndRequest(t *testing.T) {
	cases := []struct {
		skill, operation, action string
	}{
		{"h-reason", "recall", ""},
		{"h-reason", "fpf", "search"},
		{"h-decide", "recall", ""},
		{"h-spec", "context", ""},
		{"h-spec", "check", "structural"},
		{"h-spec", "change", "reauthor_preview"},
		{"h-spec", "change", "reauthor_apply"},
		{"h-verify", "check", "prepare"},
		{"h-verify", "check", "capture"},
	}
	skills := Skills()
	for _, tc := range cases {
		body := skills[tc.skill]
		tool := transport.ToolForOperation(tc.operation)
		example := transport.ToolExample(tc.operation, tc.action)
		if tool == "" || example == "" {
			t.Errorf("catalog omits %s/%s", tc.operation, tc.action)
			continue
		}
		if !strings.Contains(body, tool) || !strings.Contains(body, example) {
			t.Errorf("%s lacks catalog example for %s/%s", tc.skill, tc.operation, tc.action)
		}
		var request struct {
			Format    string `json:"format"`
			Operation string `json:"operation"`
			Action    string `json:"action"`
		}
		if err := json.Unmarshal([]byte(example), &request); err != nil {
			t.Errorf("invalid catalog example %s/%s: %v", tc.operation, tc.action, err)
			continue
		}
		if request.Format != "haft.api/2" || request.Operation != tc.operation || request.Action != tc.action {
			t.Errorf("wrong example request for %s/%s: %+v", tc.operation, tc.action, request)
		}
	}
}

func TestT01BGeneratedGuidanceSeparatesSpecContentAndDecisionBinding(t *testing.T) {
	skills := Skills()
	for _, term := range []string{
		"format: haft/2", "kind: spec", "refuses caller-authored operator_edit",
		"Do not supply status, operator_confirmed or legacy_status",
		"A v2 spec is current", "Existing haft/1 active, proposed and migrated",
		"exact pinned v1 spec", "Preview publishes nothing", "memory_generation",
		"predecessor bytes and snapshot remain", "does not clear a contested binding decision",
	} {
		if !strings.Contains(skills["h-spec"], term) {
			t.Fatalf("h-spec omits %q", term)
		}
	}
	for _, term := range []string{"direct operator request", "haft/1 decision's active status", "current haft/2 spec edit"} {
		if !strings.Contains(skills["h-decide"], term) {
			t.Fatalf("h-decide omits %q", term)
		}
	}
	for _, term := range []string{"haft/2 spec", "current content", "decision", "implementation", "evidence"} {
		if !strings.Contains(agents, term) {
			t.Fatalf("project instructions omit %q", term)
		}
	}
}

func TestT01RVerifySkillDistinguishesReplayAndCurrentBasis(t *testing.T) {
	guide := Skills()["h-verify"]
	for _, term := range []string{
		"1–512 UTF-8", "GOPROXY=off", "GOSUMDB=off",
		"continuation_unavailable", "A pending original may still be running",
		"A completed retry identifies", "An exact result-ref read is a historical snapshot",
		"Standalone incomplete stdout/stderr retention is refused",
		"capture_capacity_exceeded", "check/prepare", "bounded", "check/observe",
	} {
		if !strings.Contains(guide, term) {
			t.Fatalf("h-verify guidance omits %q", term)
		}
	}
}
