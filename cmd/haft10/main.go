// Command haft10 is the isolated candidate entrypoint for the shared API.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/host"
	"github.com/m0n0x41d/haft/internal/core/transport"
)

// Version is set by the isolated installer to the candidate commit SHA.
var Version = "development"

var help = `haft10 <operation> [action] [options]

  api --input FILE|-          Execute one complete haft.api/2 request
  serve [--profile legacy]    Serve five task MCP tools, or explicit single-tool legacy profile
  init [--codex]              Install project-local instructions and Codex assets
  migrate --from-9x --dry-run ...  Stage an explicit source in a separate output root
  migrate queue|assist ...    Review or record a proposed staging repair
  version                    Print candidate version as JSON

Shared options:
  --root ROOT                Project root (default current directory)
  --source-root ROOT         Offline source checkout
  --source-repository TOKEN  Source repository identity
  --profile default|legacy   MCP catalog selection for serve only
  --input FILE|-             One JSON request; '-' reads stdin
  --ref REF --query TEXT --limit N --strict

Use api --input for exact CLI/MCP request parity. Other operations fill omitted
format/operation/action fields; conflicting flags or input fields are rejected.
To follow a returned next_request or named-part request in CLI, pass its JSON
unchanged to haft10 api --input FILE|-. delivery.read_tool names an MCP tool.
After upgrading a B1 Codex installation, rerun haft10 init --codex to update
exact generated B1 instructions; edited generated assets cause a conflict.
After copying a project or moving the binary/source, rerun haft10 init --codex
with the current addresses. An omitted source root is inherited only on the
same physical project root or when it was external to the prior project root;
an internal source on a copied project requires explicit --source-root. An
omitted source repository keeps the prior identity; an explicit value replaces
it. Init reports completed and unresolved paths on error.
JSON results go to stdout; transport diagnostics go to stderr. Structural check
and check/prepare do not run tests. check/capture explicitly starts the selected
bounded project Go test; check/observe classifies supplied external run data.
` + "\n" + transport.OperationHelp() + "\n" + transport.ToolGuidance() + "\n" + delivery.Guide + "\n"

func main() { os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
func run(ctx context.Context, args []string, in io.Reader, out, log io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, help)
		return 0
	}
	op := args[0]
	args = args[1:]
	if op == "migrate" {
		return runMigration(ctx, args, in, out, log)
	}
	if op == "version" || op == "--version" {
		if len(args) > 0 {
			return cliError(out, log, "invalid_arguments", fmt.Errorf("version takes no arguments"))
		}
		return emit(out, map[string]any{"format": "haft.version/1", "version": Version}, log)
	}
	action := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}
	f := flag.NewFlagSet("haft10 "+op, flag.ContinueOnError)
	f.SetOutput(log)
	root := f.String("root", ".", "project root")
	sourceRoot := f.String("source-root", "", "offline source checkout")
	sourceRepository := f.String("source-repository", "", "source repository identity")
	profile := f.String("profile", transport.ProfileDefault, "serve MCP profile: default or legacy")
	input := f.String("input", "", "request JSON path or -")
	ref := f.String("ref", "", "exact reference")
	query := f.String("query", "", "search query")
	limit := f.Int("limit", 0, "maximum result count")
	view := f.String("view", "", "summary, detail or bytes")
	part := f.String("part", "", "returned named part; parts lists the directory")
	cursor := f.String("cursor", "", "opaque returned continuation")
	strict := f.Bool("strict", false, "strict structural validation")
	codex := f.Bool("codex", false, "install project-local Codex assets")
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Fprint(out, help)
			return 0
		}
		return cliError(out, log, "invalid_arguments", err)
	}
	if f.NArg() != 0 {
		return cliError(out, log, "invalid_arguments", fmt.Errorf("unexpected positional arguments"))
	}
	seen := map[string]bool{}
	f.Visit(func(v *flag.Flag) { seen[v.Name] = true })
	if seen["codex"] && op != "init" {
		return cliError(out, log, "invalid_arguments", fmt.Errorf("--codex requires init"))
	}
	if seen["profile"] && op != "serve" {
		return cliError(out, log, "invalid_arguments", fmt.Errorf("--profile requires serve"))
	}
	if op == "serve" && *profile != transport.ProfileDefault && *profile != transport.ProfileLegacy {
		return cliError(out, log, "invalid_arguments", fmt.Errorf("unknown MCP profile %q", *profile))
	}
	absolute, err := filepath.Abs(*root)
	if err != nil {
		return cliError(out, log, "invalid_root", err)
	}
	s := app.Service{Root: absolute, SourceRoot: *sourceRoot, SourceRepository: *sourceRepository}
	if op == "serve" || op == "init" {
		if action != "" || *input != "" || seen["ref"] || seen["query"] || seen["limit"] || seen["strict"] || seen["view"] || seen["part"] || seen["cursor"] {
			return cliError(out, log, "invalid_arguments", fmt.Errorf("%s does not accept request fields", op))
		}
		if op == "serve" {
			server := transport.Server{Service: s, Version: Version, Profile: *profile}
			if err := server.Serve(ctx, in, out); err != nil {
				fmt.Fprintln(log, err)
				return 1
			}
			return 0
		}
		binary, err := os.Executable()
		if err != nil {
			return cliError(out, log, "host_init", err)
		}
		binary, err = filepath.EvalSymlinks(binary)
		if err != nil {
			return cliError(out, log, "host_init", err)
		}
		result, err := host.Init(host.Config{Root: absolute, Binary: binary, SourceRoot: *sourceRoot, SourceRepository: *sourceRepository, Codex: *codex})
		if err != nil {
			return hostInitError(out, log, result, err)
		}
		return emit(out, app.Result{Format: app.Format, Operation: "init", Kind: "initialized", Data: result, Diagnostics: []carrier.Diagnostic{}, Basis: map[string]string{"candidate_version": Version}, Coverage: "complete", Limits: []string{"Project-local delivery only; host execution must be qualified separately"}}, log)
	}
	var request app.Request
	fields := map[string]json.RawMessage{}
	var rawInput []byte
	if *input != "" {
		reader := in
		var file *os.File
		if *input != "-" {
			file, err = os.Open(*input)
			if err != nil {
				return cliError(out, log, "input_read", err)
			}
			defer file.Close()
			reader = file
		}
		raw, e := transport.ReadInput(reader)
		if e != nil {
			return cliError(out, log, "input_read", e)
		}
		rawInput = raw
		request, err = transport.DecodeRequest(raw)
		if err == nil {
			err = json.Unmarshal(raw, &fields)
		}
		if err != nil {
			return cliError(out, log, "input_decode", err)
		}
	}
	if op == "api" {
		if *input == "" || action != "" || seen["ref"] || seen["query"] || seen["limit"] || seen["strict"] || seen["view"] || seen["part"] || seen["cursor"] {
			return cliError(out, log, "invalid_arguments", fmt.Errorf("api requires --input and accepts no request overrides"))
		}
	} else {
		if request.Operation != "" && request.Operation != op {
			return cliError(out, log, "request_conflict", fmt.Errorf("operation differs from JSON input"))
		}
		request.Operation = op
		if request.Format == "" {
			request.Format = delivery.Format
		}
		if action != "" {
			if request.Action != "" && request.Action != action {
				return cliError(out, log, "request_conflict", fmt.Errorf("action differs from JSON input"))
			}
			request.Action = action
		}
		if seen["ref"] {
			if fields["ref"] != nil && request.Ref != *ref {
				return cliError(out, log, "request_conflict", fmt.Errorf("ref differs from JSON input"))
			}
			request.Ref = *ref
		}
		if seen["query"] {
			if fields["query"] != nil && request.Query != *query {
				return cliError(out, log, "request_conflict", fmt.Errorf("query differs from JSON input"))
			}
			request.Query = *query
		}
		if seen["limit"] {
			if fields["limit"] != nil && request.Limit != *limit {
				return cliError(out, log, "request_conflict", fmt.Errorf("limit differs from JSON input"))
			}
			request.Limit = *limit
		}
		for name, pair := range map[string]struct {
			target *string
			value  string
		}{"view": {&request.View, *view}, "part": {&request.Part, *part}, "cursor": {&request.Cursor, *cursor}} {
			if seen[name] {
				if fields[name] != nil && *pair.target != pair.value {
					return cliError(out, log, "request_conflict", fmt.Errorf("%s differs from JSON input", name))
				}
				*pair.target = pair.value
			}
		}
		if seen["strict"] {
			if fields["strict"] != nil && request.Strict != *strict {
				return cliError(out, log, "request_conflict", fmt.Errorf("strict differs from JSON input"))
			}
			request.Strict = *strict
		}
	}
	validationInput := rawInput
	if op != "api" {
		encoded, encodeErr := json.Marshal(request)
		if encodeErr != nil {
			return cliError(out, log, "invalid_request", encodeErr)
		}
		merged := map[string]json.RawMessage{}
		if decodeErr := json.Unmarshal(encoded, &merged); decodeErr != nil {
			return cliError(out, log, "invalid_request", decodeErr)
		}
		for name, value := range fields {
			if _, present := merged[name]; !present {
				merged[name] = value
			}
		}
		validationInput, err = json.Marshal(merged)
		if err != nil {
			return cliError(out, log, "invalid_request", err)
		}
	}
	tool := transport.ToolForOperation(request.Operation)
	if _, err := transport.ValidateToolCall(transport.ProfileDefault, tool, validationInput); err != nil {
		result := transport.ToolCallError(err)
		if emit(out, result, log) != 0 {
			return 1
		}
		return 1
	}
	result := s.Call(ctx, request)
	if exit := emit(out, result, log); exit != 0 {
		return exit
	}
	if result.Failed() {
		return 1
	}
	return 0
}
func emit(out io.Writer, value any, log io.Writer) int {
	if err := json.NewEncoder(out).Encode(value); err != nil {
		fmt.Fprintln(log, err)
		return 1
	}
	return 0
}
func cliError(out, log io.Writer, code string, err error) int {
	fmt.Fprintln(log, "haft10:", transport.BoundedDiagnostic(err.Error()))
	result := delivery.Error("invalid", transport.BoundedDiagnostic(err.Error()))
	result.Diagnostics[0].Code = code
	emit(out, result, log)
	return 2
}

func hostInitError(out, log io.Writer, state host.Result, err error) int {
	message := transport.BoundedDiagnostic(err.Error())
	fmt.Fprintln(log, "haft10:", message)
	code := "host_init"
	for _, candidate := range []string{"host_conflict", "host_init_busy", "host_source_selection_required", "host_source_unavailable"} {
		if strings.HasPrefix(err.Error(), candidate+":") || err.Error() == candidate {
			code = candidate
		}
	}
	coverage := "unavailable"
	if len(state.Created)+len(state.Updated)+len(state.Unchanged) > 0 {
		coverage = "partial"
	}
	result := app.Result{
		Format:      app.Format,
		Operation:   "init",
		Kind:        "init_failed",
		Data:        state,
		Diagnostics: []carrier.Diagnostic{{Code: code, Message: message, Severity: "error"}},
		Basis:       map[string]string{"candidate_version": Version},
		Coverage:    coverage,
		Limits:      []string{"Unresolved includes paths not yet published or rechecked; resolve the named condition, then retry init"},
	}
	emit(out, result, log)
	return 2
}
