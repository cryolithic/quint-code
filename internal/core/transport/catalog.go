package transport

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

const ProfileDefault = "default"
const ProfileLegacy = "legacy"

// An action is one closed public request shape. Fields excludes the three
// common selectors; Required adds branch-specific presence requirements.
type catalogAction struct {
	Name     string
	Fields   string
	Required string
	Effect   catalogEffect
	Result   string
	Example  string
}

type applicationEffect uint8

const (
	applicationRead applicationEffect = iota
	applicationPublication
)

type disposableEffect uint8

const (
	disposableNone disposableEffect = iota
	disposableCacheAndLock
)

type catalogEffect struct {
	Application        applicationEffect
	Disposable         disposableEffect
	RunsProjectCommand bool
	Description        string
}

func readEffect(description string) catalogEffect {
	return catalogEffect{Application: applicationRead, Disposable: disposableCacheAndLock, Description: description}
}

func writeEffect(description string) catalogEffect {
	return catalogEffect{Application: applicationPublication, Disposable: disposableCacheAndLock, Description: description}
}

func captureEffect(description string) catalogEffect {
	return catalogEffect{Application: applicationRead, Disposable: disposableCacheAndLock, RunsProjectCommand: true, Description: description}
}

// Publication changes canonical project state. Disposable writes create the
// shared lock or content-addressed delivery cache without publishing a record.
// They are independent dimensions: a domain read can still modify local files.
func (e catalogEffect) publishes() bool { return e.Application == applicationPublication }

func (e catalogEffect) writesEnvironment() bool {
	return e.publishes() || e.RunsProjectCommand || e.Disposable != disposableNone
}

func (e catalogEffect) potentiallyDestructive() bool { return e.publishes() || e.RunsProjectCommand }

func (e catalogEffect) detail() string {
	if e.Disposable == disposableCacheAndLock {
		return e.Description + "; may write disposable local cache/lock"
	}
	return e.Description
}

const toolEnvironmentNotice = "Calls may create .haft/.runtime/writer.lock and disposable .haft/.cache/disclosure entries. Explicit check/capture also writes .haft/.capture-receipts safety metadata and runs project test code, which can change project or environment files. These paths need write access; initial .haft creation also needs a writable project root. On a read-only root or .haft, operations may fail or return an unavailable continuation. A cache publication failure after execution can return a known bounded outcome with continuation_unavailable and no result ref; receipt IO failure adds persistence uncertainty. Non-effectful recall/context/prepare/observe may be repeated after disposable result expiry; for capture retry the same request_id for status and inspect the original before any deliberate new-ID run. Cache/lock/claim writes do not publish project memory."
const specAuthorityNotice = "Explicit haft/2 spec writing publishes current content without status or operator_confirmed; it does not accept a product choice, prove implementation or evidence, or resolve a binding-decision conflict. Exact supersession is order-stable; mixed active v1/current v2 heads are a conflict with no automatic merge. Same-about or shared declared scope decisions are advisory candidates, never inferred governing authority. The task-level writer uses agent_edit provenance and refuses caller-authored operator_edit until a trusted route exists. Active haft/1 decision binding still requires a direct operator request. haft/1 spec readers and exact snapshots retain their historical proposed/active/migrated meanings."
const semanticYAMLNotice = "Remember across carrier kinds and change create/revise/preview/apply/sync/reauthor refuse unsupported semantic YAML conversion before publication when retained or supplied content would lose a tag, type or exact numeric value. Schema-owned fields use their declared string and optional-absence meanings; extensions retain raw YAML meaning. Only fields edited by the actual effect are exempt. Omitted examples on MODIFIED retain base scenarios; explicit removal needs remove_examples and a reason. Matched proposal extensions omitted in update/rebase are retained; remove_fields addresses existing base claim fields, not a proposal-only extension. One-step proposal-only extension retraction through omission is unavailable. A changed task with no extension map is a whole-task replacement, with removed extension paths in revision diagnostics; an unchanged same-ID task retains ordinary omitted extensions. A refusal identifies exact affected paths and leaves existing carriers and pinned history readable; inspect original bytes through a returned exact read route or pinned ref. Equivalent lexical normalization is separate. A separately authored supported representation needs an established meaning."

type catalogOperation struct {
	Name    string
	Purpose string
	Actions []catalogAction
}

type catalogTool struct {
	Name       string
	Purpose    string
	Operations []catalogOperation
}

// The catalog owns routing, admitted fields, effects, result guidance and
// examples. CLI and both MCP profiles still submit the same app.Request;
// catalog validation never rewrites receipt-bearing logical inputs.
var taskCatalog = []catalogTool{
	{
		Name: "haft_read", Purpose: "Find exact project records, inspect code context and impact, then read returned named parts or continuations.",
		Operations: []catalogOperation{
			{Name: "recall", Purpose: "Search project records or resolve an exact record or claim reference.", Actions: []catalogAction{
				{Name: "", Fields: "ref query limit view part expected_digest", Effect: readEffect("read"), Result: "results or exact resolution; summary and named parts", Example: `{"format":"haft.api/2","operation":"recall","query":"order cancellation","limit":8}`},
				{Name: "legacy", Fields: "ref view part expected_digest", Required: "ref", Effect: readEffect("read"), Result: "legacy source-ID matches with their limits", Example: `{"format":"haft.api/2","operation":"recall","action":"legacy","ref":"old-id"}`},
			}},
			{Name: "context", Purpose: "Inspect a captured local code and memory context.", Actions: []catalogAction{
				{Name: "", Fields: "ref query limit code_config capture_code prior_code view part expected_digest", Effect: readEffect("read local code and memory"), Result: "bounded context, code basis and coverage; spec-ref decision candidates are advisory and explicitly unassessed, even when the band is empty. Mixed v1/v2 heads name exact conflicting refs and an unsupported merge boundary", Example: `{"format":"haft.api/2","operation":"context","ref":"file:order.go"}`},
			}},
			{Name: "impact", Purpose: "Inspect bounded affected relations on a captured basis.", Actions: []catalogAction{
				{Name: "", Fields: "ref query limit code_config capture_code prior_code view part expected_digest", Effect: readEffect("read local code and memory"), Result: "affected relations with coverage limits", Example: `{"format":"haft.api/2","operation":"impact","ref":"file:order.go"}`},
			}},
			{Name: "read", Purpose: "Read a returned exact or transient result reference progressively.", Actions: []catalogAction{
				{Name: "", Fields: "ref view part cursor expected_digest expected_generation limit", Required: "ref", Effect: readEffect("read"), Result: "summary, named part, directory or exact byte page; follow supplied next_request", Example: `{"format":"haft.api/2","operation":"read","ref":"<returned result ref>","view":"detail","part":"<returned part>"}`},
			}},
		},
	},
	{
		Name: "haft_write", Purpose: "Publish an authored carrier or terms. Explicit haft/2 spec is current content without authoring status; haft/1 decision acceptance remains separate. Remember guards semantic YAML meaning across record kinds and refuses unsupported conversion with exact affected paths. A supplied request_id enables an exact receipt and identical-payload replay; retained captured parts remain attributed data.",
		Operations: []catalogOperation{
			{Name: "remember", Purpose: "Publish an authored record or terms map.", Actions: []catalogAction{
				{Name: "", Fields: "request_id carrier expected_generation expected_heads expected_target_digests snapshots retain view", Required: "carrier", Effect: writeEffect("write project memory"), Result: "write result, exact refs and readable parts; explicit haft/2 spec becomes current content without status/operator_confirmed, while active haft/1 decision requires a direct operator choice. Unsupported semantic YAML conversion refuses publication with exact paths; existing carriers and pinned history remain readable. Receipt and identical-payload replay when request_id is supplied", Example: `{"format":"haft.api/2","operation":"remember","request_id":"alpha-spec-create","carrier":"---\nformat: haft/2\nkind: spec\ntitle: Alpha cancellation rule\nabout: domain:Alpha.OrderCancellation\nslug: alpha-cancel\nreceiving_use: Check the local cancellation implementation\nx-fixture:\n  preserved: true\n  source: packaged-alpha-example\nclaims:\n  - id: cancellation\n    kind: law\n    text: A paid order may be cancelled.\n    x-policy:\n      review: retain this extension through a claim clarification\n    implemented_by:\n      - ref: sym:order.go::CanCancel\n        covers: Local cancellation decision for the three fixture states\n    checks:\n      - ref: test:order_test.go::TestCanCancel\n        covers: Pending, paid and shipped examples in this fixture only\n---\nThis is current authored content. It does not bind a decision or certify a check.\n"}`},
				{Name: "terms", Fields: "request_id carrier expected_generation view", Required: "carrier", Effect: writeEffect("write project terms"), Result: "terms write result; receipt when request_id is supplied", Example: `{"format":"haft.api/2","operation":"remember","action":"terms","request_id":"<stable key>","carrier":"---\nformat: haft.terms/1\nterms: []\n---\n"}`},
			}},
		},
	},
	{
		Name: "haft_change", Purpose: "Preview and apply bounded claim changes or one selected v1 spec reauthoring with CAS; current spec content is separate from decision authority, implementation and evidence. Create, revisions, preview, apply, sync and reauthor guard retained YAML meaning; unsupported conversion refuses a lossy output with exact paths. MODIFIED omitted examples retain scenarios; remove_examples names intentional removal. Matched proposal extensions omitted on revision are retained; remove_fields addresses an existing base field, not a proposal-only extension. A changed task without its extension map is a whole-task replacement and reports removed fields in revision diagnostics. Explicitly recover interrupted publication.",
		Operations: []catalogOperation{
			{Name: "change", Purpose: "Work with an exact change and its captured basis, or explicitly reauthor one pinned v1 spec edition into v2 current content.", Actions: []catalogAction{
				{Name: "list", Fields: "limit view part expected_digest", Effect: readEffect("read"), Result: "change lineages and current heads", Example: `{"format":"haft.api/2","operation":"change","action":"list"}`},
				{Name: "show", Fields: "ref view part expected_digest", Required: "ref", Effect: readEffect("read"), Result: "exact change and applied status", Example: `{"format":"haft.api/2","operation":"change","action":"show","ref":"<exact change ref>"}`},
				{Name: "preview", Fields: "ref view part expected_digest", Required: "ref", Effect: readEffect("read"), Result: "preview digest, conflicts and prospective outputs; no publication. Unsupported semantic YAML conversion identifies affected paths before a lossy output. A v2 spec patch yields current content without status or confirmation; v1 keeps historical semantics", Example: `{"format":"haft.api/2","operation":"change","action":"preview","ref":"<exact change ref>"}`},
				{Name: "reauthor_preview", Fields: "ref view part expected_digest", Required: "ref", Effect: readEffect("read selected exact v1 spec edition and current heads"), Result: "deterministic v2 current-content prototype, exact pending v1 proposal refs and their separate-change consequence, frontmatter normalization apart from semantic losses, advisory same-about/shared-scope binding decision candidates and explicit unassessed coverage, preview_digest and memory_generation; no ID, time, receipt or publication. Unsupported semantic YAML conversion refuses a lossy prototype with exact affected paths; the old carrier remains readable. Mixed heads require separate explicit repair, not an automatic merge; preview does not prove implementation/check", Example: `{"format":"haft.api/2","operation":"change","action":"reauthor_preview","ref":"<exact pinned haft/1 spec ref>"}`},
				{Name: "create", Fields: "request_id carrier snapshots expected_generation view", Required: "carrier", Effect: writeEffect("write change carrier"), Result: "exact change ref or exact-path semantic YAML conversion refusal before loss; receipt when request_id is supplied", Example: `{"format":"haft.api/2","operation":"change","action":"create","request_id":"alpha-change-create","carrier":"---\nformat: haft.change/1\nid: chg-20260928-35c0ffee\nchange_key: chg-20260928-35c0ffee\ntitle: Clarify the cancellation state boundary\nintent: Make the shipped exclusion and supported pending state explicit while keeping paid cancellation\nstate: open\ncreated_at: 2026-09-28T12:00:00Z\nsupersedes: []\ntasks:\n  - id: correct-code\n    text: Correct CanCancel for paid orders and rerun the declared check\n    done: false\npatches:\n  - base: spec-20260928-a1b2c3d4@sha256:0000000000000000000000000000000000000000000000000000000000000000\n    operations:\n      - op: MODIFIED\n        claim_id: cancellation\n        claim:\n          id: cancellation\n          kind: law\n          text: Pending and paid orders may be cancelled; shipped orders may not.\n          x-policy:\n            review: retain this extension through a claim clarification\n          implemented_by:\n            - ref: sym:order.go::CanCancel\n              covers: Local cancellation decision for the three fixture states\n          checks:\n            - ref: test:order_test.go::TestCanCancel\n              covers: Pending, paid and shipped examples in this fixture only\n        reason: Clarify the state boundary after reviewing the declared fixture; the paid-order requirement is unchanged\n---\nThe code correction fixes the failed assertion; the content edit separately clarifies the domain boundary.\n"}`},
				{Name: "apply", Fields: "ref request_id expected_generation preview_digest metadata view", Required: "ref request_id expected_generation preview_digest", Effect: writeEffect("publish previewed outputs"), Result: "receipt or exact conflict against the preview basis; unsupported semantic YAML conversion refuses a lossy output before publication. A v2 spec successor is current content without acceptance; v1 patch rules remain unchanged", Example: `{"format":"haft.api/2","operation":"change","action":"apply","ref":"<exact change ref>","request_id":"<stable key>","expected_generation":"<returned generation>","preview_digest":"<returned digest>"}`},
				{Name: "reauthor_apply", Fields: "ref request_id expected_generation preview_digest view", Required: "ref request_id expected_generation preview_digest", Effect: writeEffect("CAS-publish one selected v2 spec successor and freeze exact v1 predecessor"), Result: "recomputes preview under the writer lock; stale edition/head set, preview or generation rejects before publication. Unsupported semantic YAML conversion also refuses with exact affected paths. New v2 spec has fresh ID, agent_edit provenance and exact supersedes; predecessor bytes, extensions, claim IDs and historic refs remain. Successful apply and same-ID replay summaries return published_successor_ref and executable exact_read_request; that historical output does not claim the edition remains current, bind a decision or clear an authority conflict. Interrupted/error replies may omit those fields: retry the identical request ID; changed or missing published output is replay_conflict", Example: `{"format":"haft.api/2","operation":"change","action":"reauthor_apply","ref":"<same exact pinned haft/1 spec ref>","request_id":"<stable reauthor key>","expected_generation":"<preview memory_generation>","preview_digest":"<preview digest>"}`},
				{Name: "sync", Fields: "ref request_id expected_generation preview_digest metadata view", Required: "ref request_id expected_generation preview_digest", Effect: writeEffect("publish previewed outputs"), Result: "receipt or exact conflict against the preview basis; unsupported semantic YAML conversion refuses a lossy output before publication", Example: `{"format":"haft.api/2","operation":"change","action":"sync","ref":"<exact change ref>","request_id":"<stable key>","expected_generation":"<returned generation>","preview_digest":"<returned digest>"}`},
				{Name: "archive", Fields: "ref request_id expected_generation revision view", Required: "ref revision", Effect: writeEffect("write an archive revision"), Result: "new exact change ref or conflict; retained YAML stays guarded because supplied intent/tasks/patches are ignored by this effect; receipt when request_id is supplied", Example: `{"format":"haft.api/2","operation":"change","action":"archive","ref":"<exact change ref>","revision":{"reason":"<reason>"}}`},
				{Name: "reopen", Fields: "ref request_id expected_generation revision view", Required: "ref revision", Effect: writeEffect("write a reopen revision"), Result: "new exact change ref or conflict; retained YAML stays guarded because supplied intent/tasks/patches are ignored by this effect; receipt when request_id is supplied", Example: `{"format":"haft.api/2","operation":"change","action":"reopen","ref":"<exact change ref>","revision":{"reason":"<reason>"}}`},
				{Name: "rebase", Fields: "ref request_id expected_generation revision view", Required: "ref revision", Effect: writeEffect("write a rebase revision"), Result: "new exact change ref or conflict; only applied revision fields are authored and other retained YAML stays guarded; receipt when request_id is supplied", Example: `{"format":"haft.api/2","operation":"change","action":"rebase","ref":"<exact change ref>","revision":{"reason":"<reason>"}}`},
				{Name: "update", Fields: "ref request_id expected_generation revision view", Required: "ref revision", Effect: writeEffect("write an update revision"), Result: "new exact change ref or conflict; only applied revision fields are authored and other retained YAML stays guarded; receipt when request_id is supplied", Example: `{"format":"haft.api/2","operation":"change","action":"update","ref":"<exact change ref>","revision":{"reason":"<reason>"}}`},
			}},
			{Name: "recover", Purpose: "Explicitly finish a safe interrupted publication.", Actions: []catalogAction{
				{Name: "", Fields: "view", Effect: writeEffect("write recovery state"), Result: "recovered receipt, no-op or exact conflict; never overwrite a conflicting publication", Example: `{"format":"haft.api/2","operation":"recover"}`},
			}},
		},
	},
	{
		Name: "haft_check", Purpose: "Check structure, prepare an exact Go test basis, explicitly capture one bounded project test, or classify a supplied runner observation.",
		Operations: []catalogOperation{
			{Name: "check", Purpose: "Structural checks, exact evidence-basis preparation, explicit bounded capture or external observation.", Actions: []catalogAction{
				{Name: "", Fields: "carrier ref strict view part expected_digest", Effect: readEffect("read and structural validation"), Result: "diagnostics; checks_executed=false", Example: `{"format":"haft.api/2","operation":"check","strict":true}`},
				{Name: "structural", Fields: "carrier ref strict view part expected_digest", Effect: readEffect("read and structural validation"), Result: "diagnostics; checks_executed=false. Mixed active v1/current v2 lineage heads report exact participants and unsupported automatic merge. Whole-project head_refs is a flat union across lineages, not a lineage grouping", Example: `{"format":"haft.api/2","operation":"check","action":"structural","strict":true}`},
				{Name: "prepare", Fields: "ref check_ref scope failure_contract failure_pattern seed code_config view part expected_digest", Required: "ref check_ref scope", Effect: readEffect("read declaration and capture local code basis"), Result: "expected contract, command and runner_started=false", Example: `{"format":"haft.api/2","operation":"check","action":"prepare","ref":"<exact claim ref>","check_ref":"test:order_test.go::TestOrder","scope":"<declared scope>"}`},
				{Name: "capture", Fields: "ref check_ref scope request_id failure_contract failure_pattern seed code_config capture view", Required: "ref check_ref scope request_id capture", Effect: captureEffect("run the derived exact project Go test with bounded time and aggregate output; record noncanonical retry safety metadata"), Result: "bounded process outcome and exact basis; result ref when cache publication succeeds, otherwise continuation_unavailable. A capture_capacity_exceeded refusal occurs before execution and consumes no request_id: use check/prepare, a separately authorized bounded external run, then check/observe with independently captured facts. Same-ID retry never reruns while its claim remains; completed retry marks replay and separately assesses current basis without execution. Pending may still be running: retry the identical request later for status. New ID is a deliberate new execution after inspection", Example: `{"format":"haft.api/2","operation":"check","action":"capture","ref":"<exact claim ref>","check_ref":"test:order_test.go::TestOrder","scope":"<declared scope>","request_id":"<stable capture key>","capture":{"timeout_ms":30000,"max_output_bytes":1048576}}`},
				{Name: "observe", Fields: "observation code_config view part expected_digest", Required: "observation", Effect: readEffect("classify supplied runner bytes against the same optional code_config used by prepare"), Result: "honest observed outcome and currentness; no test launch or project-memory publication", Example: `{"format":"haft.api/2","operation":"check","action":"observe","observation":{"expected":{},"observed":{}}}`},
			}},
		},
	},
	{
		Name: "haft_fpf", Purpose: "Read the pinned FPF and Engineering DPF corpus: status, search, then inspect a complete source unit.",
		Operations: []catalogOperation{
			{Name: "fpf", Purpose: "Source-native FPF and Engineering DPF reading.", Actions: sourceActions()},
			{Name: "source", Purpose: "Compatibility spelling for the same non-publishing source handler.", Actions: sourceActions()},
		},
	},
}

func sourceActions() []catalogAction {
	return []catalogAction{
		{Name: "", Fields: "view part expected_digest", Effect: readEffect("read pinned source"), Result: "source status and exact source tree basis", Example: `{"format":"haft.api/2","operation":"fpf"}`},
		{Name: "status", Fields: "view part expected_digest", Effect: readEffect("read pinned source"), Result: "source status and exact source tree basis", Example: `{"format":"haft.api/2","operation":"fpf","action":"status"}`},
		{Name: "search", Fields: "query limit view part expected_digest", Required: "query", Effect: readEffect("read pinned source"), Result: "source candidates, not applicability or authority", Example: `{"format":"haft.api/2","operation":"fpf","action":"search","query":"claim evidence observation scope","limit":5}`},
		{Name: "inspect", Fields: "ref snapshots view part expected_digest", Required: "ref", Effect: readEffect("read pinned or supplied exact source snapshot"), Result: "complete selected source unit and byte continuation", Example: `{"format":"haft.api/2","operation":"fpf","action":"inspect","ref":"<returned source ref>"}`},
	}
}

func operationNames() []string {
	names := []string{}
	for _, tool := range taskCatalog {
		for _, operation := range tool.Operations {
			names = append(names, operation.Name)
		}
	}
	return names
}

func ToolForOperation(operation string) string {
	for _, tool := range taskCatalog {
		for _, candidate := range tool.Operations {
			if candidate.Name == operation {
				return tool.Name
			}
		}
	}
	return ""
}

func ToolExample(operation, action string) string {
	for _, tool := range taskCatalog {
		for _, candidate := range tool.Operations {
			if candidate.Name != operation {
				continue
			}
			for _, branch := range candidate.Actions {
				if branch.Name == action {
					return strings.Replace(branch.Example, `"operation":"fpf"`, `"operation":"`+operation+`"`, 1)
				}
			}
		}
	}
	return ""
}

// OperationHelp is the same action catalog rendered for human CLI help.
func OperationHelp() string {
	var guidance strings.Builder
	for _, tool := range taskCatalog {
		guidance.WriteString("\n  ")
		guidance.WriteString(tool.Name)
		guidance.WriteString(": ")
		for index, operation := range tool.Operations {
			if index > 0 {
				guidance.WriteString("; ")
			}
			guidance.WriteString(operation.Name)
			guidance.WriteString(" [")
			for actionIndex, action := range operation.Actions {
				if actionIndex > 0 {
					guidance.WriteString(", ")
				}
				if action.Name == "" {
					guidance.WriteString("omitted")
				} else {
					guidance.WriteString(action.Name)
				}
			}
			guidance.WriteByte(']')
		}
	}
	return guidance.String()
}

func ToolGuidance() string {
	var guidance strings.Builder
	guidance.WriteString("Default MCP profile has five task tools; arguments retain format=haft.api/2 and operation. ")
	for _, tool := range taskCatalog {
		guidance.WriteString(tool.Name)
		guidance.WriteString(": ")
		guidance.WriteString(tool.Purpose)
		guidance.WriteByte(' ')
	}
	guidance.WriteString("Default replies are summaries. Read named parts and copy returned next_request unchanged to its advertised read_tool (haft_read by default, haft in the explicit legacy profile). A continuation reads data; it does not replay a write. ")
	guidance.WriteString(specAuthorityNotice)
	guidance.WriteByte(' ')
	guidance.WriteString(semanticYAMLNotice)
	guidance.WriteByte(' ')
	guidance.WriteString("Whole-project structural head_refs is a flat union across lineages, not a grouping of lineage participants. ")
	guidance.WriteString("Reauthor preview lists pending v1 proposal refs, normalization separately from material losses, and unassessed decision candidates. Successful apply and identical replay summaries contain published_successor_ref and exact_read_request; execute that route to read the historical output, then reassess live currentness separately. An interrupted/error reply may omit the fields; retry the identical request ID and respect replay_conflict if published output changed or disappeared. ")
	guidance.WriteString(toolEnvironmentNotice)
	guidance.WriteString(" Annotations describe possible effects, not authority.")
	return guidance.String()
}

func ToolDefinitions(profile string) []map[string]any {
	if profile == ProfileLegacy {
		return []map[string]any{{
			"name": "haft", "description": "Compatibility tool for the complete haft.api/2 request. " + delivery.Guide + " " + specAuthorityNotice + " " + semanticYAMLNotice + " " + toolEnvironmentNotice,
			"inputSchema": catalogSchema(taskCatalog),
			"annotations": toolAnnotations(taskCatalog),
		}}
	}
	if profile != "" && profile != ProfileDefault {
		return nil
	}
	definitions := make([]map[string]any, 0, len(taskCatalog))
	for _, tool := range taskCatalog {
		definitions = append(definitions, map[string]any{
			"name": tool.Name, "description": tool.Purpose + " Returns a summary; follow delivery.read_tool using the returned part or next_request. " + toolEnvironmentNotice,
			"inputSchema": catalogSchema([]catalogTool{tool}),
			"annotations": toolAnnotations([]catalogTool{tool}),
		})
	}
	return definitions
}

func toolAnnotations(tools []catalogTool) map[string]any {
	writesEnvironment := false
	destructive := false
	for _, tool := range tools {
		for _, operation := range tool.Operations {
			for _, action := range operation.Actions {
				writesEnvironment = writesEnvironment || action.Effect.writesEnvironment()
				destructive = destructive || action.Effect.potentiallyDestructive()
			}
		}
	}
	// A repeated read can capture changed input into another cache entry, and
	// publication does not require request_id. No current tool has a proven
	// same-arguments no-additional-effect guarantee.
	return map[string]any{
		"readOnlyHint": !writesEnvironment, "destructiveHint": destructive,
		"idempotentHint": false, "openWorldHint": false,
	}
}

func catalogSchema(tools []catalogTool) map[string]any {
	base := RequestSchema()["properties"].(map[string]any)
	variants := []any{}
	properties := map[string]any{}
	names := []string{}
	for _, tool := range tools {
		for _, operation := range tool.Operations {
			names = append(names, operation.Name)
			for _, action := range operation.Actions {
				branch := branchSchema(operation, action)
				variants = append(variants, branch)
				for name := range branch["properties"].(map[string]any) {
					properties[name] = base[name]
				}
			}
		}
	}
	properties["operation"] = map[string]any{"type": "string", "enum": names}
	properties["offset"] = map[string]any{"type": "integer", "enum": []int{0}}
	return map[string]any{
		"type": "object", "properties": properties, "additionalProperties": false,
		"required": []string{"format", "operation"}, "oneOf": variants,
	}
}

func branchSchema(operation catalogOperation, action catalogAction) map[string]any {
	properties := map[string]any{}
	for _, field := range append([]string{"format", "operation", "action", "offset"}, strings.Fields(action.Fields)...) {
		properties[field] = map[string]any{}
	}
	properties["operation"] = map[string]any{"enum": []string{operation.Name}}
	properties["action"] = map[string]any{"enum": []string{action.Name}}
	properties["offset"] = map[string]any{"enum": []int{0}}
	if action.Effect.potentiallyDestructive() {
		properties["view"] = map[string]any{"enum": []string{"", "summary"}}
	}
	required := []string{"format", "operation"}
	if action.Name != "" {
		required = append(required, "action")
	}
	required = append(required, strings.Fields(action.Required)...)
	fields := fieldTypes(reflect.TypeOf(app.Request{}))
	for _, field := range required {
		if fields[field].Kind() != reflect.String {
			if nilableWireType(fields[field]) {
				properties[field] = map[string]any{"not": map[string]any{"type": "null"}}
			}
			continue
		}
		copy := map[string]any{}
		for key, value := range properties[field].(map[string]any) {
			copy[key] = value
		}
		copy["minLength"] = 1
		properties[field] = copy
	}
	label := operation.Name
	if action.Name != "" {
		label += "/" + action.Name
	}
	example := strings.Replace(action.Example, `"operation":"fpf"`, `"operation":"`+operation.Name+`"`, 1)
	return map[string]any{
		"type": "object", "title": label,
		"description": operation.Purpose + " Effect: " + action.Effect.detail() + ". Result: " + action.Result + ". Example: " + example,
		"properties":  properties, "additionalProperties": false, "required": required,
	}
}

func ValidateToolCall(profile, name string, raw []byte) (app.Request, error) {
	request, err := DecodeRequest(raw)
	if err != nil {
		return request, err
	}
	if profile != "" && profile != ProfileDefault && profile != ProfileLegacy {
		return request, fmt.Errorf("unsupported_profile: %s", profile)
	}
	if profile == ProfileLegacy && name != "haft" {
		return request, fmt.Errorf("unknown_tool: use advertised tool haft; called %s", toolNameExcerpt(name))
	}
	if profile != ProfileLegacy && ToolForOperation(request.Operation) != name {
		advertised := ToolForOperation(request.Operation)
		if advertised != "" {
			return request, fmt.Errorf("unknown_tool_or_operation: use advertised tool %s for %s; called %s", advertised, request.Operation, toolNameExcerpt(name))
		}
		return request, fmt.Errorf("unknown_tool_or_operation: no advertised tool for %s; called %s", request.Operation, toolNameExcerpt(name))
	}
	if request.Format != delivery.Format {
		return request, fmt.Errorf("unsupported_format: use %s", delivery.Format)
	}
	var supplied map[string]json.RawMessage
	if err := json.Unmarshal(raw, &supplied); err != nil {
		return request, fmt.Errorf("invalid_json: %w", err)
	}
	for _, tool := range taskCatalog {
		for _, operation := range tool.Operations {
			if operation.Name != request.Operation {
				continue
			}
			for _, action := range operation.Actions {
				if action.Name == request.Action {
					return validateAction(request, supplied, action)
				}
			}
			return request, fmt.Errorf("unsupported_action: %s/%s", operation.Name, request.Action)
		}
	}
	return request, fmt.Errorf("unsupported_operation: %s", request.Operation)
}

func validateAction(request app.Request, supplied map[string]json.RawMessage, action catalogAction) (app.Request, error) {
	allowed := map[string]bool{"format": true, "operation": true, "action": true, "offset": true}
	for _, field := range strings.Fields(action.Fields) {
		allowed[field] = true
	}
	required := append([]string{"format", "operation"}, strings.Fields(action.Required)...)
	if action.Name != "" {
		required = append(required, "action")
	}
	requiredFields := map[string]bool{}
	for _, field := range required {
		requiredFields[field] = true
	}
	fields := fieldTypes(reflect.TypeOf(app.Request{}))
	// A request can violate more than one control. Inspect supplied fields in a
	// stable order so CLI and both MCP profiles return the same first diagnostic.
	suppliedFields := make([]string, 0, len(supplied))
	for field := range supplied {
		suppliedFields = append(suppliedFields, field)
	}
	sort.Strings(suppliedFields)
	for _, field := range suppliedFields {
		value := supplied[field]
		if !allowed[field] {
			return request, fmt.Errorf("unsupported_field: %s for %s/%s", field, request.Operation, request.Action)
		}
		if string(value) == "null" {
			if requiredFields[field] {
				return request, fmt.Errorf("required_field: %s for %s/%s", field, request.Operation, request.Action)
			}
			if nilableWireType(fields[field]) {
				continue
			}
			return request, fmt.Errorf("null_control: %s for %s/%s", field, request.Operation, request.Action)
		}
	}
	if request.Offset != 0 {
		return request, fmt.Errorf("unsupported_offset: use returned cursors")
	}
	for _, field := range required {
		value, present := supplied[field]
		if !present || string(value) == "null" || requiredStringEmpty(value, fields[field]) {
			return request, fmt.Errorf("required_field: %s for %s/%s", field, request.Operation, request.Action)
		}
	}
	if action.Effect.potentiallyDestructive() {
		if request.View != "" && request.View != "summary" {
			return request, fmt.Errorf("unsupported_view: effectful actions return a summary; use the returned read request for detail")
		}
	}
	return request, nil
}

// Arbitrary caller names are never needed in full to correct a routing error.
// Keep the advertised owner before the excerpt so the public diagnostic stays
// actionable even when a caller supplies a very long name.
func toolNameExcerpt(name string) string {
	const limit = 80
	if len(name) <= limit {
		return fmt.Sprintf("%q", name)
	}
	n := limit
	for n > 0 && !utf8.RuneStart(name[n]) {
		n--
	}
	return fmt.Sprintf("%q (%d bytes omitted)", name[:n], len(name)-n)
}

func nilableWireType(field reflect.Type) bool {
	switch field.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return true
	default:
		return false
	}
}

func requiredStringEmpty(raw json.RawMessage, field reflect.Type) bool {
	if field.Kind() != reflect.String {
		return false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return true
	}
	return value == ""
}
