# Haft10 private alpha package

This archive is a separate `haft10` candidate for new projects or disposable copies. It does not replace the ordinary v9 `haft` installer, migrate a live project, grant decision authority, establish a supported host, or authorize public release. The local version `10.0.0-alpha.<source SHA prefix>` identifies the exact private source commit; it is not a Git tag.

## Build and verify the archive

The builder requires a clean private checkout at one exact commit and clean pinned `FPF`, `data/FPF` and `docs` submodules; macOS arm64; Git, Python 3, `xcrun`, clang and a macOS SDK; a selected **local** Go 1.25.8 toolchain; and all selected dependency versions in the **default account-local Go module cache**. The PATH `go` launcher may be older only when the exact toolchain is already cached. Custom inherited `GOPATH` or `GOMODCACHE` are not selected build inputs. The script uses `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `GOENV=off` and `GOWORK=off`; it does not download dependencies, refresh source pins or copy credentials. `go.sum`, cache ziphash metadata, extracted notice bytes and actual binary buildinfo are distinct evidence.

From that clean checkout, use a new absolute output path in an existing private output directory:

```text
python3 scripts/v10/package-dev.py \
  --version 10.0.0-alpha.<first-12-hex-of-clean-HEAD> \
  --source-repository https://github.com/ailev/FPF.git \
  --output /absolute/private/output/haft10-alpha-darwin-arm64.tar.gz
python3 scripts/v10/alpha-protocol-smoke.py \
  --archive /absolute/private/output/haft10-alpha-darwin-arm64.tar.gz \
  --go /absolute/path/to/selected/go1.25.8/bin/go \
  --other-repository /absolute/path/to/the/other/source/checkout \
  --raw-dir /absolute/private/output/alpha-protocol-raw \
  --output /absolute/private/output/alpha-protocol-receipt.json
```

The builder rejects a dirty root or submodule, wrong source pin or repository identity, stale module/version/h1 notice mapping, changed notice source bytes, and an existing or protected output path. It records the Go/compiler/SDK, source tree, binary buildinfo, modes and SHA-256 for every archive member. `notices/MAP.json` maps the linked modules, selected Go runtime and vendored Tree-sitter/Go grammar/Unicode inputs to exact notice paths; it is source attribution, not a legal compliance determination. The bundled `LICENSE` applies to Haft; `FPF/{LICENSE,LICENSING.md}` and Engineering DPF notices apply to their separately pinned source. The archive contains no v9 binary, full checkout, project `.haft`, host configuration or credentials.

## Install in a disposable project

Unpack the archive outside the source checkout. Its top directory is `haft10-alpha/`; keep that directory in place while the project uses its binary and pinned FPF/Engineering DPF source. Use the **binary from this archive** and an isolated HOME/config if reproducing the example:

```text
PACKAGE=/absolute/unpacked/haft10-alpha
PROJECT=/absolute/disposable-project
mkdir -p "$PROJECT"
cp "$PACKAGE/examples/go.mod" "$PACKAGE/examples/order.go" "$PACKAGE/examples/order_test.go" "$PROJECT/"
"$PACKAGE/bin/haft10" init --codex --root "$PROJECT" \
  --source-root "$PACKAGE/FPF" --source-repository https://github.com/ailev/FPF.git
"$PACKAGE/bin/haft10" version
"$PACKAGE/bin/haft10" fpf inspect --root "$PROJECT" \
  --source-root "$PACKAGE/FPF" --source-repository https://github.com/ailev/FPF.git \
  --ref fpf-usage-guide
```

Use `haft10 api --input FILE|- --root "$PROJECT" --source-root "$PACKAGE/FPF" --source-repository https://github.com/ailev/FPF.git` for one complete `haft.api/2` JSON request. A Markdown carrier is the **string value** of `carrier`, not the outer JSON object. For example, construct a request from the packaged complete spec carrier without manual JSON escaping:

```text
python3 - "$PACKAGE/examples/spec.md" > /tmp/alpha-spec-request.json <<'PY'
import json, pathlib, sys
carrier = pathlib.Path(sys.argv[1]).read_text()
request = {"format": "haft.api/2", "operation": "remember", "request_id": "alpha-spec-create", "carrier": carrier}
print(json.dumps(request))
PY
"$PACKAGE/bin/haft10" api --input /tmp/alpha-spec-request.json --root "$PROJECT" \
  --source-root "$PACKAGE/FPF" --source-repository https://github.com/ailev/FPF.git
```

`examples/spec.md` is a new `haft/2` **current-content** spec. Its claim carries `text`, `implemented_by`, a declared `checks` selector and retained `x-*` extensions. It has no `status`, `origin` or `operator_confirmed` assertion. A spec records intended content; it does not bind a decision, prove the Go code correct or run the check. `examples/failure-note.md` and `pass-note.md` are ordinary nonbinding notes. Their IDs, timestamps and provenance are assigned by the writer; do not add `format: haft/2` or guess an `origin` for a note.

## Follow the actual check and change path

1. Use `check`/`structural` to validate the carrier graph. It sets `checks_executed=false`. Use `check`/`prepare` with `ref: "spec:alpha-cancel#cancellation"`, `check_ref: "test:order_test.go::TestCanCancel"`, `scope: "Cancellation behavior for pending, paid and shipped orders"`, `failure_contract: "The declared Go test reports an unequal cancellation decision with t.Errorf"` and `failure_pattern: "cancellation mismatch:"`. Preparation returns the selected command and basis with `runner_started=false`; it does not run Go.
2. An explicitly authorized `check`/`capture` uses the same fields plus a stable `request_id` and `capture: {"timeout_ms": 120000, "max_output_bytes": 65536}`. With the packaged `order.go`, the process starts, exits nonzero and records `assertion_failure` for `paid`. A missing compiler/cache or failed build is `environment_failure`; a skipped/zero-test run is `not_run`; neither is an assertion failure or pass. Inspect the returned `delivery.parts_request` and follow the advertised `read_tool`/`next_request` unchanged. `haft10 api --input FILE|-` accepts those returned request objects directly. A response summary is not the whole result.
3. Save the actual failed result separately: `remember` with the contents of `examples/failure-note.md` as `carrier`, a fresh `request_id`, and `retain: [{"ref": "<capture delivery.parts_request.ref>", "part": "result"}]`. The server attaches the exact captured result; the note's prose does not become a check receipt. Inspect its returned exact note ref and `report_1_content` through read requests.
4. `examples/change.md` contains a complete `haft.change/1` carrier with ordered `MODIFIED` operation, full preserved claim, retained extensions and a reason. Replace only `__EXACT_BASE_REF__` with the exact pinned spec edition returned by recall; submit that text as `change`/`create` `carrier`. `change`/`preview` on the returned exact change ref is pure and returns `basis.preview_digest` and `basis.memory_generation`. Submit `change`/`apply` with that ref, a **new** stable request ID and exactly those returned CAS values. The clarification makes pending/shipped boundaries explicit; the separate code correction fixes the paid-order defect. Preview and apply do not accept a decision or reuse old failure as evidence for the new text.
5. Copy `examples/order.fixed.txt` over the disposable project's `order.go`, then prepare and capture the new claim/check/code basis with a new request ID. The actual process must start and exit zero with `passed`; retain its `result` in `examples/pass-note.md`. Keep the old failure and new pass as separate historical records. A same-ID apply retry is a nonexecuting `replayed` receipt; changing the request ID while reusing stale preview generation/digest must refuse, not publish another successor. A fresh server can read the old pinned spec bytes/extensions, the successor and both retained results independently.

The source-checkout driver `scripts/v10/alpha-protocol-smoke.py` performs this path on its own outside-repository fixture and records the exact request/response outcomes and raw stdio MCP result sizes. The driver is not in this archive. It is a qualification example, not an agent session or a test of usefulness. `delivery.parts_request` names the directory, `delivery.next_request` advances a page, and member `read.request` names exact child bytes. A returned request reads data; it never replays a write. The complete serialized MCPResult, including text, structured content, `isError` and JSON escaping, is limited to 8192 bytes.

Moving the binary or source after init changes recorded host addresses; rerun project-local `init --codex` with explicit current paths. Go remains an external prerequisite for code capture. This package has been qualified only on the stated local macOS arm64 basis. Linux, other hosts, native agent behavior, installer cutover, migration and public distribution require separate evidence and authority.
