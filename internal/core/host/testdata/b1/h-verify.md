---
name: h-verify
description: Use when a claim needs an observation against its exact basis, or a saved basis needs comparison. Read selected evidence parts progressively; structure and implementation links are not proof.
---

Use haft.api/2. Default replies are summaries. Inspect delivery.complete, omissions and available; follow the supplied next_request or a part's request verbatim to read only needed detail. Parts and member directories are paged. Saved retained attachments expose verified decoded content parts. view=bytes restores exact bytes with digest/offset; it is for bulk clients, not routine model context. Read a complete claim (including extensions) before replacing it, and the full governing source body before assessing applicability. Stale means repeat the original query; expired means the disposable result was lost. A transient result is not saved evidence. Delivery completeness does not establish truth, attribution or current basis.

Resolve the exact claim and read its declared implementation/check bindings and
scope. Prepare a declared check with the real claim ref, check ref and explicit
scope. This example matches the order fixture when that fixture is present:

{"format":"haft.api/2","operation":"check","action":"prepare","ref":"spec:order-cancel#total-preserved","check_ref":"pbt:order_test.go::TestCancelPreservesTotal","scope":"Generated new and paid orders; cancellation preserves total","seed":23}

Read the expected, command, run_environment and basis_capture parts, code_complete, diagnostics and limits. Preparation does
not run anything. Judge whether the oracle actually covers the claim. If the
current task authorizes the test, run the returned exact command in the intended
environment using the returned run_environment. Independently capture and compare
claim, code, oracle and dependency bytes before and after the run; record the
actual environment and toolchain. Preserve raw stdout/stderr, exit code, selector
and full observed basis. Never copy expected basis fields into an observation
without establishing them from those captures. If the basis changes during the
run, retain that diagnostic and do not attribute the result to the earlier basis.
Use basis_capture's exact JSON preimage bytes, file digests and oracle byte span
to verify the expected hashes against actual files; do not reverse-engineer a
hash or mistake a symbol-span digest for the whole test-file digest. Preserve
the before/after capture and the actual build tags, toolchain and environment.
Use .context/verification for temporary runner reports and captures, which are
outside the code-index scope. For a durable handoff, embed the needed raw bytes
in the published evidence body; an ignored scratch file alone is not portable.
For large captured results, remember can retain exact server-side bytes using
retain: [{ref: <returned result ref>, part: "result"}]. This is explicit persistence;
a transient result alone is not a saved observation.
Submit operation check/action observe with observation containing that exact
expected contract and independently captured observed run, and inspect the
returned current_basis after the run. Byte fields are base64 JSON strings. The
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

To retain a real observation, use remember with a haft/1 evidence carrier. Its
frontmatter needs kind: evidence, a readable title, about, claim (the bounded
observation), observed_at, method, source (where the raw report lives), basis
with kind: code and ref equal to the observation ID, and uses. Each use contains
id, target (the exact claim ref including its edition), check, polarity
(supports, weakens or inconclusive), and scope. Put the complete observation,
raw outputs and capture report in the Markdown body or a preserved addressable
report. ID, created_at and origin may use remember defaults. An observational
record may have status: active; this does not accept its proposed target spec.
Do not set operator_confirmed for an observation. Preserve the original claim
snapshot and full scope even when a later run changes the support relation.
