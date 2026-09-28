package host

import (
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/transport"
)

var agents = projectInstructions()

func projectInstructions() string {
	guidance := transport.ToolGuidance()
	readTool := transport.ToolForOperation("read")
	return `<!-- haft10:start -->
# Haft project memory

Use the project-local Haft MCP task tools or the configured haft10 binary. MCP
tool arguments and CLI api --input both carry a haft.api/2 request. Results
include result_kind, data, diagnostics, basis, coverage and limits. Read these
fields before relying on a result.

` + guidance + `

` + delivery.Guide + `

With this default profile, pass a returned next_request or named part request
as arguments to ` + readTool + `.
The returned operation remains in the request; do not change its pinned basis.

Recover the current object and question. h-reason, h-decide, h-spec and h-verify
are independent capabilities, selected by the task. No universal sequence is
required. Source search retrieves candidates; inspect governing bodies before
judging applicability. A source result is not a recommendation or proof.

Persist when the operator requests it or a concrete receiving use requires
replay, handoff or delayed feedback. Keep proposed text, accepted choices,
implementation links and observed evidence distinct. Never manufacture operator
confirmation from a skill invocation, model prose, a record or tool output.
Explicit API inputs are local trusted data; the tool cannot authenticate their
real-world origin. Apply only the exact operator-authorized effect and scope.

Go navigation is syntactic and conservative. Missing or ambiguous bindings need
explicit repair. Among Haft API actions, only explicitly authorized
check/capture starts a project test; structural checks, prepare, observe and
reads do not. Project test code can write to its environment. A historical
observation keeps its exact claim, code, oracle, dependency and environment
basis; formatting can change raw evidence bytes even when navigation is
token-equivalent.
<!-- haft10:end -->
`
}

// Skills returns independent task-conditioned instruction carriers. MCP tool
// names and request examples come from the same catalog as tools/list and help.
func Skills() map[string]string {
	recallTool := transport.ToolForOperation("recall")
	fpfTool := transport.ToolForOperation("fpf")
	rememberTool := transport.ToolForOperation("remember")
	checkTool := transport.ToolForOperation("check")
	contextTool := transport.ToolForOperation("context")
	changeTool := transport.ToolForOperation("change")
	recallExample := transport.ToolExample("recall", "")
	fpfSearchExample := transport.ToolExample("fpf", "search")
	contextExample := transport.ToolExample("context", "")
	structuralExample := transport.ToolExample("check", "structural")
	prepareExample := transport.ToolExample("check", "prepare")
	captureExample := transport.ToolExample("check", "capture")
	reauthorPreviewExample := transport.ToolExample("change", "reauthor_preview")
	reauthorApplyExample := transport.ToolExample("change", "reauthor_apply")
	skills := map[string]string{
		"h-reason": `---
name: h-reason
description: Use for a project question needing source-grounded reasoning. Read needed source parts through bounded MCP continuations; mechanical edits and exact lookups need no source search.
---

Recover the object, question, known constraints and intended result from the
conversation. When stored context matters, call ` + recallTool + ` with arguments:

` + recallExample + `

For a substantive source question, preserve the actual concern. Call ` + fpfTool + ` with arguments:

` + fpfSearchExample + `

Search results are candidates. Inspect a returned exact ref using ` + fpfTool + `,
action inspect and ref equal to that returned value. Read the complete governing
body, conditions and limits before judging applicability. Do not treat ranking,
fresh capture or source availability as recommendation or upstream currentness.
If source is unavailable or ambiguous, retain that diagnostic and abstain from
source-dependent claims. Explain the smallest useful result conversationally.
Persist only when an explicit request or concrete receiving use requires it.
`,
		"h-decide": `---
name: h-decide
description: Use when a direct operator request selects a bounded option for a named subject and scope. Read related memory progressively; recommendations and tool output do not supply a choice.
---

Recover the exact choice, alternatives, rationale, scope and weakest link. Read
current related records if needed through ` + recallTool + `:

` + recallExample + `

If the operator's effect, option or scope is unresolved, explain that specific
choice. Otherwise continue the already-authorized bounded write without a second
confirmation ceremony. Compose a decision carrier from actual current content;
preserve rationale and honest origin. Do not copy an example as a real decision.
Use ` + rememberTool + `, request_id equal to a unique stable write key, carrier
equal to that complete Markdown, and expected_generation from the current result
where requested by the application. Check with ` + checkTool + `, action structural
and carrier equal to the same Markdown before publication.

Only a real direct operator request supports a haft/1 decision's active status
with operator_confirmed true. A current haft/2 spec edit, Git review, check or
tool output does not supply that request. The API treats binding fields as
trusted local inputs; it cannot verify speech or invent a receipt. A decision
proposal remains proposed. Reuse a request_id only for the identical payload.
Inspect conflicts and current heads; never silently select a predecessor or
overwrite a concurrent choice.
`,
		"h-spec": `---
name: h-spec
description: Use for spec claims, bindings or bounded changes. Read complete authoring parts through MCP continuations; structural validity, acceptance and observed correctness remain separate.
---

Recover the current subject, claim and exact basis. Inspect its record via recall
and its code via context. For an existing Go project, a non-publishing ` + contextTool + ` example is:

` + contextExample + `

To author a new spec, use ` + rememberTool + ` with a stable request_id and Markdown
carrier. The frontmatter is YAML (or JSON) between --- lines; the body preserves
rationale and scope. Declare format: haft/2, kind: spec, title, about (the
domain subject), slug, receiving_use and claims. A claim needs a stable id,
kind (law, definition, guard or prescription) and text. Laws and guards need
either checks or unchecked with an explicit reason for strict validation.
implemented_by and checks are lists of {ref, covers};
examples are {id, given, when, then}. Use sym:path.go::Type.Method for a method,
test:path_test.go::TestName for a scenario, or pbt:path_test.go::TestName for a
finite property. Preserve supplied limitations in covers and the body. Initial
v2 records may omit id, created_at and origin; the writer defaults origin to
agent_edit. The task-level writer refuses caller-authored operator_edit until
a trusted route exists; the reader can still show it in historical/external
bytes. Either origin describes edit provenance only, never acceptance.
Do not supply status, operator_confirmed or legacy_status, even false or empty.
Read data.exact_ref from recall; alias spec:<slug> resolves a current candidate,
but use the exact pinned ref for an edition-sensitive write. A v2 spec is current
content, not an accepted decision, implementation, passed check or release
authority. Existing haft/1 active, proposed and migrated specs keep their
historical meaning and bytes; reading a proposed v1 spec never activates it.
Binding decisions remain haft/1 with a direct operator-request boundary.
Exact supersession selects one final content head independently of file order.
Competing branches remain conflicts. A lineage with an active v1 head and a
current v2 head reports both exact refs in context and structural checks;
there is no automatic merge or authority choice. Do not use reauthor to merge
that lineage: its effect is limited to one selected predecessor.

Whole-project structural head_refs is a flat union across lineages. Inspect
exact pinned refs to determine which heads participate in one lineage;
the whole-project list alone does not group them.

Explicit term meanings use ` + rememberTool + `/action terms with frontmatter
{"format":"haft.terms/1","terms":[{"id":"Domain.Term","definition":"<meaning>","aliases":["<word>"],"exclusions":["<excluded meaning>"]}]}.
Record and claim terms list those qualified IDs. The terms write creates the
initial map without replacing an existing map. Inspect existing terms before
reusing them; preserve changed meanings with their historical snapshots.

Remember across carrier kinds and change create/revise/preview/apply/sync,
including selected reauthor, refuse unsupported semantic YAML conversion when
retained source or supplied change content would lose a tag, type or exact
numeric value. Known schema fields keep their declared string or optional
absence meaning; extensions at any depth keep raw YAML meaning. Only fields
changed by the requested effect are exempt, including within same-ref bindings.
Read the exact affected path and original bytes through the returned read route
or pinned ref. Existing carrier bytes, snapshots and historical refs stay
readable. Comment and equivalent lexical normalization are separate from a
value or type change. A separately authored supported representation needs an
established meaning; a structural pass alone cannot establish that meaning.

Validate current carriers without executing a check:

Call ` + checkTool + ` with arguments:

` + structuralExample + `

Inspect diagnostics and coverage; green structure is not implementation proof.
For one selected v1 spec that needs v2 content, use the exact pinned v1 spec
ref returned by recall. Do not use a live alias or supply a replacement carrier.
Preview the content conversion through ` + changeTool + ` with arguments:

` + reauthorPreviewExample + `

Inspect the old status, origin, selected and competing heads, preserved claim
IDs, extensions, body, sources, provenance and historical ref, plus any loss or
conflict. Read pending_v1_proposals by exact ref: converting the selected head
leaves each proposal unchanged and unaccepted; incorporating one requires a
separately authored v2 change against current content. Read normalization_note
for re-encoded frontmatter comments and lexical YAML form, separately from
material_losses; the old carrier and snapshot remain byte-exact. The
decision_authority band contains advisory same-about or shared declared scope
choose-now candidates. Its assessment is not_assessed even when empty; do not
infer that no governing decision exists. Preview publishes nothing and returns
preview_digest and memory_generation. If the selected proposed v1 branch
differs from an active branch, resolve that conflict explicitly; do not
retire the active branch by guessing. Apply only that selected conversion
with the same exact ref through
` + changeTool + ` with arguments:

` + reauthorApplyExample + `

Replace placeholders with that preview's digest and memory_generation and a
stable request_id. Apply rechecks the predecessor, heads and generation under
the writer lock. A changed basis needs a new preview; never substitute a newer
head. The predecessor bytes and snapshot remain available at their old refs.
The new v2 carrier has a new ID, agent_edit provenance and exact supersedes.
On a successful written or replayed result, read published_successor_ref and
execute exact_read_request to inspect the exact published successor. An
identical request_id replay returns that same historical output, including
after a later edit; re-query context to assess currentness separately. An
interrupted or error reply may omit these fields. Retry the identical request
ID; if the published output is missing or changed, replay_conflict prevents a
success receipt rather than guessing an edition ref.
Reauthoring is an explicit content edit, not acceptance of the old proposal,
binding of a decision, proof of implementation or a migration of every spec.

For subsequent edits, author a bounded change with exact section bases, explicit operations
and reasons. Create it through ` + changeTool + `/action create with the complete
carrier and stable request_id. Preview with action preview and the exact returned
change ref. Apply only the intended authorized effect using action apply, ref,
preview_digest and expected_generation returned by that preview, plus a new
request_id. A stale basis requires another review; never substitute newer heads.
V2 patches create current content without status or confirmation metadata; v1
patches retain their historical rules. Read tool schema for complete fields.
A successful spec edit does not clear a contested binding decision. Keep the
decision, implementation and evidence rows separate from current content.

The carrier is Markdown with YAML frontmatter between two --- lines. JSON is
also valid YAML. For a clarification, copy the complete claim object from
the returned claim detail part (or reconstruct its complete JSON bytes), change only intended fields,
and use this frontmatter shape (replace angle-bracket placeholders):

Claim extensions and extensions on checks, implemented_by, examples and
evidence_inputs appear inline in JSON, exactly as in YAML: for example
{"id":"rule","kind":"definition","text":"Bounded meaning","x-bound":5}.
Keep those unknown fields in their original objects; edit x-bound directly.
Do not introduce an extra wrapper. A literal field named extra is ordinary
authored data. Re-recall claims saved from an older API's extra envelope before
authoring a change; do not guess whether an existing extra field is a wrapper.
This inline rule applies to claims and these nested objects, not other record
or source-snapshot JSON envelopes. Existing carrier/snapshot bytes are unchanged.

{"format":"haft.change/1","title":"<readable title>","intent":"<intended effect>","patches":[{"base":"<data.exact_ref of the spec, without #claim>","operations":[{"op":"MODIFIED","claim_id":"<existing claim ID>","claim":<complete revised claim object>,"reason":"<why this change>"}]}]}

Omit id, change_key, state and created_at on initial creation to use API defaults.
Supplied IDs require chg-YYYYMMDD-xxxxxxxx with eight hexadecimal suffix digits.
Operation names are uppercase ADDED, MODIFIED, REMOVED and RENAMED. MODIFIED
replaces the complete claim; omitted examples are preserved, explicit removals
need remove_examples and a reason. Omitted claim extensions are retained; use
remove_fields with a reason for an existing base field. A matched proposal's
omitted extension is retained through update/rebase. A changed task with no
extension map is a whole-task replacement; inspect returned diagnostics for removed
extension fields. An unchanged same-ID task with omitted ordinary extensions
retains them. A matched proposal-only extension has no one-step removal through
omission in update/rebase; do not edit historical carrier bytes. RENAMED uses
claim_id and new_id. Omit patch
body to preserve spec prose; replacing it requires expected_body_digest and
body_change_reason. Inspect preview losses before applying. New spec records
use format haft/2; change carriers use the separate haft.change/1 format.

Preserve authored checks and implementation selectors. Rename or ambiguity needs
explicit repair; do not silently redirect a claim. Archive, sync and apply have
separate meanings. A completed task checkbox is not evidence that a claim holds.
`,
		"h-verify": `---
name: h-verify
description: Use when a claim needs an observation against its exact basis, or a saved basis needs comparison. Read selected evidence parts progressively; structure and implementation links are not proof.
---

Resolve the exact claim and read its declared implementation/check bindings and
scope. Prepare a declared check with the real claim ref, check ref and explicit
scope. Call ` + checkTool + ` with arguments:

` + prepareExample + `

Read the expected, command, run_environment and basis_capture parts, code_complete, diagnostics and limits. Preparation does
not run tests or publish project memory; it may write a disposable local cache
or lock. Judge whether the oracle actually covers the claim.

When the current task explicitly authorizes this declared project test and
scope, call ` + checkTool + `/action capture with the same exact ref, check_ref,
scope and optional code_config as prepare, a stable request_id (1–512 UTF-8
bytes, no control characters or surrounding whitespace), and explicit
timeout_ms and max_output_bytes limits. The catalog example shows the request
shape; replace its placeholders with the selected claim and check:

` + captureExample + `

Capture runs the derived exact command once in the project root with the same
pinned Go environment as prepare, including GOPROXY=off and GOSUMDB=off.
Project test code can write to its environment, although capture
does not publish project memory. Read the returned summary and named observation,
process and basis_capture parts. Follow the returned read_tool and byte-page
requests for stdout/stderr; continuations only read stored bytes. Check status,
reason_code, current_basis, output_exceeded, observed drained byte counts/digests and
limits before relying on the run. A stored prefix after output overflow cannot
establish a pass. A pipe capture that stopped draining is also incomplete,
even below the byte cap. If the reply is lost, retry the identical request_id
and payload; it does not start another test. A completed retry identifies
replay: its observation and immediate post-run basis remain historical, while
a separate current-basis assessment checks same/changed/unknown without running
the test. An exact result-ref read is a historical snapshot without that new
assessment. A pending original may still be running; retry its identical
request later to recover completion. Inspect an unknown attempt before deciding
on a new request_id and deliberate new execution. An expired or corrupt result
cannot be restored by a read; inspect any retained terminal facts and current
basis before deciding on a fresh run. A transient result ref is not durable
evidence. If cache publication fails after execution, the bounded known outcome
can be returned with continuation_unavailable and no result ref; failed receipt
IO is reported separately as persistence uncertainty.
If the exact capture cannot fit its bounded receipt or reply,
capture_capacity_exceeded refuses before claiming the request_id or starting
the test. Repeating the unchanged request will be refused. Use check/prepare
for its exact expected contract, obtain separate authority for a bounded
external run, and submit independently captured facts through check/observe.

For a durable handoff with an available result ref, author a complete evidence
carrier and call ` + rememberTool + `
with retain: [{ref: <returned capture result ref>, part: "result"}]. The server
attaches exact captured bytes; do not copy stdout/stderr or base64 through the
model. Standalone incomplete stdout/stderr retention is refused; retain result
for the observed drained count, digest and incompleteness context. Inspect the publication
receipt and retained attachment. Capture alone
does not publish evidence or accept a specification.

For an externally executed test, use prepare's returned exact command and
run_environment. Independently capture and compare claim, code, oracle and
dependency bytes before and after the run; record the actual environment and
toolchain. Preserve raw stdout/stderr, exit code, selector and full observed
basis. Never copy expected basis fields into an observation
without establishing them from those captures. If the basis changes during the
run, retain that diagnostic and do not attribute the result to the earlier basis.
Use basis_capture's exact JSON preimage bytes, file digests and oracle byte span
to verify the expected hashes against actual files; do not reverse-engineer a
hash or mistake a symbol-span digest for the whole test-file digest. Preserve
the before/after capture and the actual build tags, toolchain and environment.
Use .context/verification for temporary runner reports and captures, which are
outside the code-index scope. For a durable handoff, embed the needed raw bytes
in the published evidence body; an ignored scratch file alone is not portable.
Submit ` + checkTool + `/action observe with observation containing that exact
expected contract and independently captured observed run. If prepare used an
optional code_config, pass that same code_config to observe so currentness is
compared on the same code basis. Observe does not run tests or publish project
memory, but may write a disposable local cache or lock. Inspect the returned
current_basis after the run. Byte fields are base64 JSON strings. The
declared oracle contract must explicitly justify any assertion-failure marker;
never guess an assertion failure from arbitrary output text.

Report passed, assertion_failure, skipped, not_run, environment_failure or
unattributable together with reason_code and current_basis. An old pass does not
transfer to a different claim edition, oracle, dependency or environment. Preserve
failed and skipped results. A pass establishes only the declared observed scope.

For a replay or handoff receiving use, context/impact may request capture_code true
and later supply that returned code_capture as prior_code. Token-equivalent
formatting can update navigation while the raw evidence basis changes. Persist
only the exact observation and portable snapshots needed by the receiving use;
the observe operation itself does not publish an evidence record.

To retain a real observation, use ` + rememberTool + ` with a haft/1 evidence carrier. Its
frontmatter needs kind: evidence, a readable title, about, claim (the bounded
observation), observed_at, method, source (where the raw report lives), basis
with kind: code and ref equal to the observation ID, and uses. Each use contains
id, target (the exact claim ref including its edition), check, polarity
(supports, weakens or inconclusive), and scope. Preserve the complete observation,
raw outputs and capture report in the Markdown body, a preserved addressable
report or exact server-side retained attachments. ID, created_at and origin may
use remember defaults. An observational record may have status: active; this
does not accept its proposed target spec.
Do not set operator_confirmed for an observation. Preserve the original claim
snapshot and full scope even when a later run changes the support relation.
`,
	}
	toolGuidance := transport.ToolGuidance()
	guide := delivery.Guide + "\n" + toolGuidance
	for name, body := range skills {
		// The first frontmatter closing delimiter is the shared body insertion point.
		for i := 4; i+5 <= len(body); i++ {
			if body[i:i+5] == "\n---\n" {
				body = body[:i+5] + "\n" + guide + "\n" + body[i+5:]
				break
			}
		}
		skills[name] = body
	}
	return skills
}
