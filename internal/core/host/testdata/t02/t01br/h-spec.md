---
name: h-spec
description: Use for spec claims, bindings or bounded changes. Read complete authoring parts through MCP continuations; structural validity, acceptance and observed correctness remain separate.
---

Use haft.api/2. Default replies are summaries. Inspect delivery.complete, omissions and available; pass each supplied next_request or part request verbatim as arguments to delivery.read_tool. Basis may be projected: delivery.omissions.basis and basis_truncated count omitted and shortened members; use delivery.basis_request for the full exact basis when available. Parts and member directories are paged. Saved retained attachments expose verified decoded content parts. view=bytes restores exact bytes with digest/offset; it is for bulk clients, not routine model context. Read a complete claim (including extensions) before replacing it, and the full governing source body before assessing applicability. Stale means repeat the original query; expired means the disposable result was lost. A transient result is not saved evidence. Delivery completeness does not establish truth, attribution or current basis.
Default MCP profile has five task tools; arguments retain format=haft.api/2 and operation. haft_read: Find exact project records, inspect code context and impact, then read returned named parts or continuations. haft_write: Publish an authored carrier or terms. Explicit haft/2 spec is current content without authoring status; haft/1 decision acceptance remains separate. A supplied request_id enables an exact receipt and identical-payload replay; retained captured parts remain attributed data. haft_change: Preview and apply bounded claim changes or one selected v1 spec reauthoring with CAS; current spec content is separate from decision authority, implementation and evidence. Explicitly recover interrupted publication. haft_check: Check structure, prepare an exact Go test basis, explicitly capture one bounded project test, or classify a supplied runner observation. haft_fpf: Read the pinned FPF and Engineering DPF corpus: status, search, then inspect a complete source unit. Default replies are summaries. Read named parts and copy returned next_request unchanged to its advertised read_tool (haft_read by default, haft in the explicit legacy profile). A continuation reads data; it does not replay a write. Explicit haft/2 spec writing publishes current content without status or operator_confirmed; it does not accept a product choice, prove implementation or evidence, or resolve a binding-decision conflict. Exact supersession is order-stable; mixed active v1/current v2 heads are a conflict with no automatic merge. Same-about or shared declared scope decisions are advisory candidates, never inferred governing authority. The task-level writer uses agent_edit provenance and refuses caller-authored operator_edit until a trusted route exists. Active haft/1 decision binding still requires a direct operator request. haft/1 spec readers and exact snapshots retain their historical proposed/active/migrated meanings. Reauthor preview lists pending v1 proposal refs, normalization separately from material losses, and unassessed decision candidates. Apply and identical replay return the exact published successor ref and an executable recall route; later currentness needs a separate read. Calls may create .haft/.runtime/writer.lock and disposable .haft/.cache/disclosure entries. Explicit check/capture also writes .haft/.capture-receipts safety metadata and runs project test code, which can change project or environment files. These paths need write access; initial .haft creation also needs a writable project root. On a read-only root or .haft, operations may fail or return an unavailable continuation. A cache publication failure after execution can return a known bounded outcome with continuation_unavailable and no result ref; receipt IO failure adds persistence uncertainty. Non-effectful recall/context/prepare/observe may be repeated after disposable result expiry; for capture retry the same request_id for status and inspect the original before any deliberate new-ID run. Cache/lock/claim writes do not publish project memory. Annotations describe possible effects, not authority.

Recover the current subject, claim and exact basis. Inspect its record via recall
and its code via context. For an existing Go project, a non-publishing haft_read example is:

{"format":"haft.api/2","operation":"context","ref":"file:order.go"}

To author a new spec, use haft_write with a stable request_id and Markdown
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

Explicit term meanings use haft_write/action terms with frontmatter
{"format":"haft.terms/1","terms":[{"id":"Domain.Term","definition":"<meaning>","aliases":["<word>"],"exclusions":["<excluded meaning>"]}]}.
Record and claim terms list those qualified IDs. The terms write creates the
initial map without replacing an existing map. Inspect existing terms before
reusing them; preserve changed meanings with their historical snapshots.

Validate current carriers without executing a check:

Call haft_check with arguments:

{"format":"haft.api/2","operation":"check","action":"structural","strict":true}

Inspect diagnostics and coverage; green structure is not implementation proof.
For one selected v1 spec that needs v2 content, use the exact pinned v1 spec
ref returned by recall. Do not use a live alias or supply a replacement carrier.
Preview the content conversion through haft_change with arguments:

{"format":"haft.api/2","operation":"change","action":"reauthor_preview","ref":"<exact pinned haft/1 spec ref>"}

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
haft_change with arguments:

{"format":"haft.api/2","operation":"change","action":"reauthor_apply","ref":"<same exact pinned haft/1 spec ref>","request_id":"<stable reauthor key>","expected_generation":"<preview memory_generation>","preview_digest":"<preview digest>"}

Replace placeholders with that preview's digest and memory_generation and a
stable request_id. Apply rechecks the predecessor, heads and generation under
the writer lock. A changed basis needs a new preview; never substitute a newer
head. The predecessor bytes and snapshot remain available at their old refs.
The new v2 carrier has a new ID, agent_edit provenance and exact supersedes.
Read apply's new_ref and supplied recall request to inspect the exact published
successor. An identical request_id replay returns that same published ref and
request, including after a later edit. It identifies historical output, not
current head status; re-query context to assess currentness separately.
Reauthoring is an explicit content edit, not acceptance of the old proposal,
binding of a decision, proof of implementation or a migration of every spec.

For subsequent edits, author a bounded change with exact section bases, explicit operations
and reasons. Create it through haft_change/action create with the complete
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
need remove_examples and a reason. RENAMED uses claim_id and new_id. Omit patch
body to preserve spec prose; replacing it requires expected_body_digest and
body_change_reason. Inspect preview losses before applying. New spec records
use format haft/2; change carriers use the separate haft.change/1 format.

Preserve authored checks and implementation selectors. Rename or ambiguity needs
explicit repair; do not silently redirect a claim. Archive, sync and apply have
separate meanings. A completed task checkbox is not evidence that a claim holds.
