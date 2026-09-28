---
name: h-verify
description: Use when a claim needs an observation against its exact basis, or a saved basis needs comparison. Read selected evidence parts progressively; structure and implementation links are not proof.
---

Use haft.api/2. Default replies are summaries. Inspect delivery.complete, omissions and available; pass each supplied next_request or part request verbatim as arguments to delivery.read_tool. Basis may be projected: delivery.omissions.basis and basis_truncated count omitted and shortened members; use delivery.basis_request for the full exact basis when available. Parts and member directories are paged. Saved retained attachments expose verified decoded content parts. view=bytes restores exact bytes with digest/offset; it is for bulk clients, not routine model context. Read a complete claim (including extensions) before replacing it, and the full governing source body before assessing applicability. Stale means repeat the original query; expired means the disposable result was lost. A transient result is not saved evidence. Delivery completeness does not establish truth, attribution or current basis.
Default MCP profile has five task tools; arguments retain format=haft.api/2 and operation. haft_read: Find exact project records, inspect code context and impact, then read returned named parts or continuations. haft_write: Publish an authored carrier or terms. Explicit haft/2 spec is current content without authoring status; haft/1 decision acceptance remains separate. A supplied request_id enables an exact receipt and identical-payload replay; retained captured parts remain attributed data. haft_change: Preview and apply bounded claim changes or one selected v1 spec reauthoring with CAS; current spec content is separate from decision authority, implementation and evidence. Explicitly recover interrupted publication. haft_check: Check structure, prepare an exact Go test basis, explicitly capture one bounded project test, or classify a supplied runner observation. haft_fpf: Read the pinned FPF and Engineering DPF corpus: status, search, then inspect a complete source unit. Default replies are summaries. Read named parts and copy returned next_request unchanged to its advertised read_tool (haft_read by default, haft in the explicit legacy profile). A continuation reads data; it does not replay a write. Explicit haft/2 spec writing publishes current content without status or operator_confirmed; it does not accept a product choice, prove implementation or evidence, or resolve a binding-decision conflict. Exact supersession is order-stable; mixed active v1/current v2 heads are a conflict with no automatic merge. Same-about or shared declared scope decisions are advisory candidates, never inferred governing authority. The task-level writer uses agent_edit provenance and refuses caller-authored operator_edit until a trusted route exists. Active haft/1 decision binding still requires a direct operator request. haft/1 spec readers and exact snapshots retain their historical proposed/active/migrated meanings. Reauthor preview lists pending v1 proposal refs, normalization separately from material losses, and unassessed decision candidates. Apply and identical replay return the exact published successor ref and an executable recall route; later currentness needs a separate read. Calls may create .haft/.runtime/writer.lock and disposable .haft/.cache/disclosure entries. Explicit check/capture also writes .haft/.capture-receipts safety metadata and runs project test code, which can change project or environment files. These paths need write access; initial .haft creation also needs a writable project root. On a read-only root or .haft, operations may fail or return an unavailable continuation. A cache publication failure after execution can return a known bounded outcome with continuation_unavailable and no result ref; receipt IO failure adds persistence uncertainty. Non-effectful recall/context/prepare/observe may be repeated after disposable result expiry; for capture retry the same request_id for status and inspect the original before any deliberate new-ID run. Cache/lock/claim writes do not publish project memory. Annotations describe possible effects, not authority.

Resolve the exact claim and read its declared implementation/check bindings and
scope. Prepare a declared check with the real claim ref, check ref and explicit
scope. Call haft_check with arguments:

{"format":"haft.api/2","operation":"check","action":"prepare","ref":"<exact claim ref>","check_ref":"test:order_test.go::TestOrder","scope":"<declared scope>"}

Read the expected, command, run_environment and basis_capture parts, code_complete, diagnostics and limits. Preparation does
not run tests or publish project memory; it may write a disposable local cache
or lock. Judge whether the oracle actually covers the claim.

When the current task explicitly authorizes this declared project test and
scope, call haft_check/action capture with the same exact ref, check_ref,
scope and optional code_config as prepare, a stable request_id (1–512 UTF-8
bytes, no control characters or surrounding whitespace), and explicit
timeout_ms and max_output_bytes limits. The catalog example shows the request
shape; replace its placeholders with the selected claim and check:

{"format":"haft.api/2","operation":"check","action":"capture","ref":"<exact claim ref>","check_ref":"test:order_test.go::TestOrder","scope":"<declared scope>","request_id":"<stable capture key>","capture":{"timeout_ms":30000,"max_output_bytes":1048576}}

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
carrier and call haft_write
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
Submit haft_check/action observe with observation containing that exact
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

To retain a real observation, use haft_write with a haft/1 evidence carrier. Its
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
