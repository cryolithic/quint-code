---
name: h-spec
description: Use for spec claims, bindings or bounded changes. Read complete authoring parts through MCP continuations; structural validity, acceptance and observed correctness remain separate.
---

Use haft.api/2. Default replies are summaries. Inspect delivery.complete, omissions and available; follow the supplied next_request or a part's request verbatim to read only needed detail. Parts and member directories are paged. Saved retained attachments expose verified decoded content parts. view=bytes restores exact bytes with digest/offset; it is for bulk clients, not routine model context. Read a complete claim (including extensions) before replacing it, and the full governing source body before assessing applicability. Stale means repeat the original query; expired means the disposable result was lost. A transient result is not saved evidence. Delivery completeness does not establish truth, attribution or current basis.

Recover the current subject, claim and exact basis. Inspect its record via recall
and its code via context. For an existing Go project, a read-only example is:

{"format":"haft.api/2","operation":"context","ref":"file:order.go"}

To author new memory, use remember with a stable request_id and Markdown carrier.
The frontmatter is YAML (or JSON) between --- lines; the body preserves rationale
and scope. A proposed spec uses format: haft/1, kind: spec, title, about (the
domain subject), slug, receiving_use, and claims. A claim needs a stable id,
kind (law, definition, guard or prescription) and text. Laws and guards need
either checks or unchecked with an explicit reason for strict validation.
implemented_by and checks are lists of {ref, covers};
examples are {id, given, when, then}. Use sym:path.go::Type.Method for a method,
test:path_test.go::TestName for a scenario, or pbt:path_test.go::TestName for a
finite property. Preserve supplied limitations in covers and the body. Initial
records may omit id, created_at, status and origin for proposed agent defaults.
Read data.exact_ref from recall; alias spec:<slug> resolves the current candidate.

Explicit term meanings use remember/action terms with frontmatter
{"format":"haft.terms/1","terms":[{"id":"Domain.Term","definition":"<meaning>","aliases":["<word>"],"exclusions":["<excluded meaning>"]}]}.
Record and claim terms list those qualified IDs. The terms write creates the
initial map without replacing an existing map. Inspect existing terms before
reusing them; preserve changed meanings with their historical snapshots.

Validate current carriers without executing a check:

{"format":"haft.api/2","operation":"check","action":"structural","strict":true}

Inspect diagnostics and coverage; green structure is not implementation proof.
For edits, author a bounded change with exact section bases, explicit operations
and reasons. Create it through operation change/action create with the complete
carrier and stable request_id. Preview with action preview and the exact returned
change ref. Apply only the intended authorized effect using action apply, ref,
preview_digest and expected_generation returned by that preview, plus a new
request_id. A stale basis requires another review; never substitute newer heads.
Read tool schema for complete fields. Application does not infer acceptance.

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
body_change_reason. Inspect preview losses before applying. Remembered records
use format haft/1; change carriers use the separate haft.change/1 format.

Preserve authored checks and implementation selectors. Rename or ambiguity needs
explicit repair; do not silently redirect a claim. Archive, sync and apply have
separate meanings. A completed task checkbox is not evidence that a claim holds.
