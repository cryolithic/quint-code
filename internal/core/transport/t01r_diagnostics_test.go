package transport

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func t01rInvalidCLI(t *testing.T, binary, root, raw string) delivery.Response {
	t.Helper()
	command := exec.Command(binary, "api", "--root", root, "--input", "-")
	command.Stdin = strings.NewReader(raw)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("CLI decoder exit = %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	var response delivery.Response
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("CLI response decode: %v: %s", err, stdout.String())
	}
	return response
}

func TestT01RDeterministicDecoderAcrossCLIAndMCP(t *testing.T) {
	root := t.TempDir()
	binary := t01arCLI(t)
	defaultClient := t01aStartClient(t, app.Service{Root: root}, ProfileDefault)
	legacyClient := t01aStartClient(t, app.Service{Root: root}, ProfileLegacy)
	defaultClient.list(t)
	legacyClient.list(t)
	cases := []struct {
		name, raw, want, tool string
		repetitions           int
	}{
		{"controller-two-unknown", `{"format":"haft.api/2","operation":"recall","bogus_a":1,"bogus_b":2}`, "unknown_field: $.bogus_a", "haft_read", 60},
		{"unknown-before-null", `{"format":"haft.api/2","operation":"recall","aaa_unknown":1,"limit":null}`, "unknown_field: $.aaa_unknown", "haft_read", 24},
		{"null-before-unknown", `{"format":"haft.api/2","operation":"recall","limit":null,"zzz_unknown":1}`, "invalid_json: null scalar at $.limit", "haft_read", 24},
		{"nested-two-unknown", `{"format":"haft.api/2","operation":"check","action":"capture","capture":{"bogus_a":1,"bogus_b":2}}`, "unknown_field: $.capture.bogus_a", "haft_check", 24},
		{"nested-null-before-unknown", `{"format":"haft.api/2","operation":"check","action":"prepare","code_config":{"goos":null,"zzz_unknown":2}}`, "invalid_json: null scalar at $.code_config.goos", "haft_check", 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for iteration := 0; iteration < tc.repetitions; iteration++ {
				cli := t01rInvalidCLI(t, binary, root, tc.raw)
				if len(cli.Diagnostics) == 0 || cli.Diagnostics[0].Message != tc.want {
					t.Fatalf("CLI iteration %d: %+v; want %s", iteration, cli, tc.want)
				}
				for _, profile := range []struct {
					client *t01aProtocolClient
					tool   string
				}{{defaultClient, tc.tool}, {legacyClient, "haft"}} {
					response := profile.client.mustCall(t, profile.tool, json.RawMessage(tc.raw))
					if len(response.Diagnostics) == 0 || response.Diagnostics[0].Message != tc.want {
						t.Fatalf("%s iteration %d: %+v; want %s", profile.tool, iteration, response, tc.want)
					}
				}
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, ".haft")); !os.IsNotExist(err) {
		t.Fatalf("invalid controls caused project writes: %v", err)
	}
}

func TestT01RCaptureGuidanceInBothMCPProfiles(t *testing.T) {
	for _, profile := range []string{ProfileDefault, ProfileLegacy} {
		definitions := ToolDefinitions(profile)
		for _, definition := range definitions {
			if definition["name"] != "haft_check" && definition["name"] != "haft" {
				continue
			}
			schema := definition["inputSchema"].(map[string]any)
			properties := schema["properties"].(map[string]any)
			requestID := properties["request_id"].(map[string]any)["description"].(string)
			for _, term := range []string{"1–512 UTF-8 bytes", "no control characters", "no surrounding whitespace", "same-ID retry"} {
				if !strings.Contains(requestID, term) {
					t.Fatalf("%s request_id omits %q: %s", profile, term, requestID)
				}
			}
			found := false
			for _, raw := range schema["oneOf"].([]any) {
				branch := raw.(map[string]any)
				if branch["title"] != "check/capture" {
					continue
				}
				found = true
				description := branch["description"].(string)
				for _, term := range []string{"continuation_unavailable", "replay", "Pending may still be running", "New ID is a deliberate new execution", "capture_capacity_exceeded", "check/prepare", "bounded external run", "check/observe"} {
					if !strings.Contains(description, term) {
						t.Fatalf("%s capture guidance omits %q: %s", profile, term, description)
					}
				}
			}
			if !found {
				t.Fatalf("%s omitted check/capture", profile)
			}
		}
	}
}

func TestT01RInvalidCaptureIDIsCallerValidationAcrossSurfaces(t *testing.T) {
	root := t.TempDir()
	binary := t01arCLI(t)
	defaultClient := t01aStartClient(t, app.Service{Root: root}, ProfileDefault)
	legacyClient := t01aStartClient(t, app.Service{Root: root}, ProfileLegacy)
	defaultClient.list(t)
	legacyClient.list(t)
	ids := []struct{ name, value string }{
		{"leading-space", " capture"},
		{"trailing-space", "capture "},
		{"control", "capture\x01id"},
		{"513-bytes", strings.Repeat("a", 513)},
	}
	for _, id := range ids {
		t.Run(id.name, func(t *testing.T) {
			request := app.Request{
				Format: delivery.Format, Operation: "check", Action: "capture",
				Ref: "claim", CheckRef: "test:file.go::TestX", Scope: "scope",
				RequestID: id.value,
				Capture:   &app.CaptureOptions{TimeoutMillis: 1000, MaxOutputBytes: 1024},
			}
			raw, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			cli := t01CaptureInvalidCLI(t, binary, root, string(raw))
			for _, response := range []delivery.Response{
				cli,
				defaultClient.mustCall(t, "haft_check", json.RawMessage(raw)),
				legacyClient.mustCall(t, "haft", json.RawMessage(raw)),
			} {
				if response.Kind != "invalid_capture" || len(response.Diagnostics) == 0 {
					t.Fatalf("%s classified invalid identity as %+v", id.name, response)
				}
				message := response.Diagnostics[0].Message
				if !strings.Contains(message, "invalid_capture_request_id") || !strings.Contains(message, "1–512") {
					t.Fatalf("%s omitted actionable identity rule: %s", id.name, message)
				}
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, ".haft")); !os.IsNotExist(err) {
		t.Fatalf("invalid request IDs caused project writes: %v", err)
	}
}
