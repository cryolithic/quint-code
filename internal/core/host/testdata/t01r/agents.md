<!-- haft10:start -->
# Haft project memory

Use the project-local Haft MCP task tools or the configured haft10 binary. MCP
tool arguments and CLI api --input both carry a haft.api/2 request. Results
include result_kind, data, diagnostics, basis, coverage and limits. Read these
fields before relying on a result.

Default MCP profile has five task tools; arguments retain format=haft.api/2 and operation. haft_read: Find exact project records, inspect code context and impact, then read returned named parts or continuations. haft_write: Publish an authored carrier or terms. A supplied request_id enables an exact receipt and identical-payload replay; retained captured parts remain attributed data. haft_change: List and preview bounded changes, publish authorized revisions or applications, and explicitly recover interrupted publication. haft_check: Check structure, prepare an exact Go test basis, explicitly capture one bounded project test, or classify a supplied runner observation. haft_fpf: Read the pinned FPF and Engineering DPF corpus: status, search, then inspect a complete source unit. Default replies are summaries. Read named parts and copy returned next_request unchanged to its advertised read_tool (haft_read by default, haft in the explicit legacy profile). A continuation reads data; it does not replay a write. Calls may create .haft/.runtime/writer.lock and disposable .haft/.cache/disclosure entries. Explicit check/capture also writes .haft/.capture-receipts safety metadata and runs project test code, which can change project or environment files. These paths need write access; initial .haft creation also needs a writable project root. On a read-only root or .haft, operations may fail or return an unavailable continuation. A cache publication failure after execution can return a known bounded outcome with continuation_unavailable and no result ref; receipt IO failure adds persistence uncertainty. Non-effectful recall/context/prepare/observe may be repeated after disposable result expiry; for capture retry the same request_id for status and inspect the original before any deliberate new-ID run. Cache/lock/claim writes do not publish project memory. Annotations describe possible effects, not authority.

Use haft.api/2. Default replies are summaries. Inspect delivery.complete, omissions and available; pass each supplied next_request or part request verbatim as arguments to delivery.read_tool. Parts and member directories are paged. Saved retained attachments expose verified decoded content parts. view=bytes restores exact bytes with digest/offset; it is for bulk clients, not routine model context. Read a complete claim (including extensions) before replacing it, and the full governing source body before assessing applicability. Stale means repeat the original query; expired means the disposable result was lost. A transient result is not saved evidence. Delivery completeness does not establish truth, attribution or current basis.

With this default profile, pass a returned next_request or named part request
as arguments to haft_read.
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
