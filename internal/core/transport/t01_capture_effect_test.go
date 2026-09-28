package transport

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01CaptureSchemaAndAdmissionAcrossSurfaces(t *testing.T) {
	valid := []byte(`{"format":"haft.api/2","operation":"check","action":"capture","ref":"spec:order-cancel#total-preserved","check_ref":"pbt:order_test.go::TestCancelPreservesTotal","scope":"1000 generated new/paid cases, seed 23","request_id":"one-run","capture":{"timeout_ms":30000,"max_output_bytes":1048576},"code_config":null}`)
	for _, profile := range []struct{ name, tool string }{{ProfileDefault, "haft_check"}, {ProfileLegacy, "haft"}} {
		request, err := ValidateToolCall(profile.name, profile.tool, valid)
		if err != nil || request.Capture == nil || request.Capture.TimeoutMillis != 30000 || request.CodeConfig != nil {
			t.Fatalf("%s did not admit bounded capture with nullable config: %+v / %v", profile.name, request, err)
		}
		definitions := ToolDefinitions(profile.name)
		var schema map[string]any
		for _, definition := range definitions {
			if definition["name"] == profile.tool {
				schema = definition["inputSchema"].(map[string]any)
			}
		}
		if schema == nil {
			t.Fatalf("%s omitted capture owner", profile.name)
		}
		rootCapture := schema["properties"].(map[string]any)["capture"].(map[string]any)
		captureFields := rootCapture["properties"].(map[string]any)
		timeout := captureFields["timeout_ms"].(map[string]any)
		output := captureFields["max_output_bytes"].(map[string]any)
		if !reflect.DeepEqual(rootCapture["required"], []string{"timeout_ms", "max_output_bytes"}) ||
			timeout["minimum"] != 1 || timeout["maximum"] != 120000 ||
			output["minimum"] != 1 || output["maximum"] != 8<<20 {
			t.Fatalf("%s advertises incomplete nested capture bounds: %+v", profile.name, rootCapture)
		}
		found := false
		for _, variant := range schema["oneOf"].([]any) {
			branch := variant.(map[string]any)
			if branch["title"] != "check/capture" {
				continue
			}
			found = true
			properties := branch["properties"].(map[string]any)
			if properties["capture"].(map[string]any)["not"] == nil || properties["view"].(map[string]any)["enum"] == nil {
				t.Fatalf("%s capture branch allows null or direct detail: %+v", profile.name, branch)
			}
		}
		if !found {
			t.Fatalf("%s omitted check/capture schema branch", profile.name)
		}
	}

	// These are actual CLI and both MCP rejection paths. The nested numeric
	// omission reaches application validation; null and wrong-branch controls
	// fail in transport admission. None may start a test or create .haft.
	root := t.TempDir()
	binary := t01arCLI(t)
	defaultClient := t01aStartClient(t, app.Service{Root: root}, ProfileDefault)
	legacyClient := t01aStartClient(t, app.Service{Root: root}, ProfileLegacy)
	defaultClient.list(t)
	legacyClient.list(t)
	cases := []struct {
		name, raw, kind string
	}{
		{"required null", `{"format":"haft.api/2","operation":"check","action":"capture","ref":"claim","check_ref":"test:file.go::TestX","scope":"scope","request_id":"one","capture":null}`, "required_field"},
		{"wrong branch null", `{"format":"haft.api/2","operation":"check","action":"prepare","ref":"claim","check_ref":"test:file.go::TestX","scope":"scope","capture":null}`, "unsupported_field"},
		{"missing nested timeout", `{"format":"haft.api/2","operation":"check","action":"capture","ref":"claim","check_ref":"test:file.go::TestX","scope":"scope","request_id":"one","capture":{"max_output_bytes":1024}}`, "invalid_capture"},
		{"zero output bound", `{"format":"haft.api/2","operation":"check","action":"capture","ref":"claim","check_ref":"test:file.go::TestX","scope":"scope","request_id":"one","capture":{"timeout_ms":30000,"max_output_bytes":0}}`, "invalid_capture"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli := t01CaptureInvalidCLI(t, binary, root, tc.raw)
			for _, profile := range []struct {
				client *t01aProtocolClient
				tool   string
			}{{defaultClient, "haft_check"}, {legacyClient, "haft"}} {
				mcp := profile.client.mustCall(t, profile.tool, json.RawMessage(tc.raw))
				if !mcp.IsError || mcp.Kind != tc.kind || !reflect.DeepEqual(mcp, cli) {
					t.Fatalf("%s CLI/MCP rejection drift: CLI=%+v MCP=%+v", profile.tool, cli, mcp)
				}
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, ".haft")); !os.IsNotExist(err) {
		t.Fatalf("invalid capture controls caused .haft effects: %v", err)
	}
}

func t01CaptureInvalidCLI(t *testing.T, binary, root, raw string) delivery.Response {
	t.Helper()
	command := exec.Command(binary, "api", "--root", root, "--input", "-")
	command.Stdin = strings.NewReader(raw)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("CLI did not reject capture control: %v: %s %s", err, stdout.String(), stderr.String())
	}
	var result delivery.Response
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("CLI returned invalid JSON %q: %v", stdout.String(), err)
	}
	return result
}

// The advertised check tool includes an action that can execute project code.
// This test observes that effect through the MCP surface and then verifies that
// prepare, retry, and returned read continuations do not repeat it.
func TestT01CaptureMetadataMatchesActualEffects(t *testing.T) {
	if testing.Short() {
		t.Skip("actual isolated Go test process")
	}
	service, defaultClient := t01arProject(t)
	legacyClient := t01aStartClient(t, service, ProfileLegacy)
	for _, profile := range []struct {
		client *t01aProtocolClient
		name   string
	}{
		{defaultClient, "haft_check"},
		{legacyClient, "haft"},
	} {
		found := false
		for _, tool := range profile.client.list(t) {
			if tool.Name != profile.name {
				continue
			}
			found = true
			if tool.Annotations["destructiveHint"] != true || tool.Annotations["readOnlyHint"] != false {
				t.Fatalf("%s omitted possible project test effects: %+v", tool.Name, tool.Annotations)
			}
			for _, claim := range []string{"check/capture", "project test code", "change project or environment files", ".capture-receipts", "do not publish project memory"} {
				if !strings.Contains(tool.Description, claim) {
					t.Fatalf("%s description omits %q", tool.Name, claim)
				}
			}
		}
		if !found {
			t.Fatalf("%s not advertised", profile.name)
		}
	}

	// A test-owned init hook gives an externally visible execution count without
	// changing the declared selector. The marker lives under disposable runtime.
	marker := filepath.Join(service.Root, ".haft", ".runtime", "transport-capture-runs.marker")
	hook := `package orders
import "os"
func init() {
	f, err := os.OpenFile(".haft/.runtime/transport-capture-runs.marker", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil { panic(err) }
	if _, err := f.Write([]byte("run\n")); err != nil { panic(err) }
	if err := f.Close(); err != nil { panic(err) }
}
`
	if err := os.WriteFile(filepath.Join(service.Root, "capture_effect_test.go"), []byte(hook), 0600); err != nil {
		t.Fatal(err)
	}
	transactions := func() []string {
		t.Helper()
		entries, err := os.ReadDir(filepath.Join(service.Root, ".haft", "transactions"))
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return names
	}
	runs := func() int {
		t.Helper()
		raw, err := os.ReadFile(marker)
		if os.IsNotExist(err) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(string(raw), "run\n")
	}
	initialTransactions := transactions()
	request := app.Request{
		Format: delivery.Format, Operation: "check", Action: "capture",
		Ref: "spec:order-cancel#total-preserved", CheckRef: "pbt:order_test.go::TestCancelPreservesTotal",
		Scope: "1000 generated new/paid cases, seed 23", RequestID: "t01-transport-capture-once",
		Capture: &app.CaptureOptions{TimeoutMillis: 30000, MaxOutputBytes: 1 << 20},
	}
	prepare := request
	prepare.Action = "prepare"
	prepare.RequestID = ""
	prepare.Capture = nil
	prepared := defaultClient.mustCall(t, "haft_check", prepare)
	if prepared.Kind != "prepared" || runs() != 0 {
		t.Fatalf("prepare ran a project test: %+v, runs=%d", prepared, runs())
	}
	var preparedEnvironment map[string]string
	t01arPart(t, defaultClient, prepared, "run_environment", &preparedEnvironment)
	if len(t01aParts(t, defaultClient, prepared)) == 0 || runs() != 0 {
		t.Fatal("reading a prepared result ran a project test")
	}

	first := defaultClient.mustCall(t, "haft_check", request)
	if first.Kind != "passed" || first.IsError || first.Delivery.Catalog == nil || runs() != 1 {
		t.Fatalf("explicit capture did not run once: %+v, runs=%d", first, runs())
	}
	ref := first.Delivery.Catalog.Ref
	retry := legacyClient.mustCall(t, "haft", request)
	if retry.Kind != first.Kind || retry.IsError || retry.Delivery.Catalog == nil || retry.Delivery.Catalog.Ref != ref || runs() != 1 {
		t.Fatalf("lost-reply retry reran project code: %+v, runs=%d", retry, runs())
	}
	for _, profile := range []struct {
		client *t01aProtocolClient
		tool   string
		value  delivery.Response
	}{{defaultClient, "haft_read", first}, {legacyClient, "haft", retry}} {
		if profile.value.Basis["memory_generation"] == "" || profile.value.Delivery.Omissions.Basis == 0 || profile.value.Delivery.BasisRequest == nil || profile.value.Delivery.ReadTool != profile.tool {
			t.Fatalf("%s omitted useful basis or exact route: %+v", profile.tool, profile.value)
		}
		exact := profile.client.mustCall(t, profile.tool, *profile.value.Delivery.BasisRequest)
		var full map[string]string
		encoded, err := json.Marshal(exact.Data)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &full); err != nil {
			t.Fatal(err)
		}
		if !exact.Delivery.Complete || full["memory_generation"] != profile.value.Basis["memory_generation"] || len(full) != len(profile.value.Basis)+profile.value.Delivery.Omissions.Basis {
			t.Fatalf("%s exact basis route incomplete: %+v", profile.tool, exact)
		}
	}
	var process struct {
		Started     bool              `json:"started"`
		ExitCode    *int              `json:"exit_code"`
		Environment map[string]string `json:"environment"`
	}
	t01arPart(t, defaultClient, first, "process", &process)
	if !process.Started || process.ExitCode == nil || *process.ExitCode != 0 || runs() != 1 {
		t.Fatalf("continuation reran or misreported execution: %+v, runs=%d", process, runs())
	}
	if preparedEnvironment["GOPROXY"] != "off" || preparedEnvironment["GOSUMDB"] != "off" || !reflect.DeepEqual(process.Environment, preparedEnvironment) {
		t.Fatalf("prepare environment %+v differs from actual capture %+v", preparedEnvironment, process.Environment)
	}
	cliRequest := request
	cliRequest.RequestID = "t01-cli-capture-once"
	cliInput, err := json.Marshal(cliRequest)
	if err != nil {
		t.Fatal(err)
	}
	cliBinary := t01arCLI(t)
	cliCapture := func() delivery.Response {
		t.Helper()
		command := exec.Command(cliBinary, "api", "--root", service.Root, "--input", "-")
		command.Stdin = bytes.NewReader(cliInput)
		output, err := command.Output()
		if err != nil {
			t.Fatalf("CLI capture failed: %v", err)
		}
		var result delivery.Response
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("CLI returned invalid capture result: %v", err)
		}
		return result
	}
	cliFirst := cliCapture()
	if cliFirst.Kind != "passed" || cliFirst.Delivery.Catalog == nil || runs() != 2 {
		t.Fatalf("CLI did not execute exact capture once: %+v, runs=%d", cliFirst, runs())
	}
	if cliFirst.Basis["memory_generation"] == "" || cliFirst.Delivery.Omissions.Basis == 0 || cliFirst.Delivery.BasisRequest == nil {
		t.Fatalf("CLI omitted useful basis or exact route: %+v", cliFirst)
	}
	cliReadInput, err := json.Marshal(cliFirst.Delivery.BasisRequest)
	if err != nil {
		t.Fatal(err)
	}
	cliReadCommand := exec.Command(cliBinary, "api", "--root", service.Root, "--input", "-")
	cliReadCommand.Stdin = bytes.NewReader(cliReadInput)
	cliReadOutput, err := cliReadCommand.Output()
	if err != nil {
		t.Fatalf("CLI basis continuation failed: %v", err)
	}
	var cliBasis delivery.Response
	if err := json.Unmarshal(cliReadOutput, &cliBasis); err != nil || !cliBasis.Delivery.Complete || cliBasis.Delivery.Part != "basis" {
		t.Fatalf("CLI exact basis continuation failed: %v %+v", err, cliBasis)
	}
	cliRetry := cliCapture()
	if cliRetry.Kind != cliFirst.Kind || cliRetry.Delivery.Catalog == nil || cliRetry.Delivery.Catalog.Ref != cliFirst.Delivery.Catalog.Ref || runs() != 2 {
		t.Fatalf("CLI lost-reply retry reran project code: %+v, runs=%d", cliRetry, runs())
	}
	if !reflect.DeepEqual(transactions(), initialTransactions) {
		t.Fatal("capture published a canonical project-memory transaction")
	}
}
