# Haft10 alpha

`haft10` is a separate experimental alpha for **new projects or disposable
copies on macOS arm64 with Codex**. It does not replace the ordinary `haft`
installation or migrate a live v9 project. Linux and other hosts are not yet
qualified.

Download the
[`v10.0.0-alpha.f5893beb2164`](https://github.com/m0n0x41d/haft/releases/tag/v10.0.0-alpha.f5893beb2164)
prerelease, titled **Haft v10 alpha — 10.0.0-alpha.f5893beb2164**. Select
`haft10_10.0.0-alpha.f5893beb2164_darwin_arm64.tar.gz` and `SHA256SUMS`
from its assets. Verify the archive before unpacking:

```sh
shasum -a 256 -c SHA256SUMS
tar -xzf haft10_10.0.0-alpha.f5893beb2164_darwin_arm64.tar.gz
```

The expected archive SHA-256 is
`01d8d9689d5abbec819054f35e9ba84e348714bff829f0b06f0015cd437bce65`.
Keep the unpacked `haft10-alpha/` directory in place and follow its
`README.md` for the isolated install, `init --codex`, pinned FPF source,
worked Go check, change preview/apply, and fresh-session recovery. The same
guide is retained in this repository at
[`scripts/v10/ALPHA-QUICKSTART.md`](scripts/v10/ALPHA-QUICKSTART.md).
Its prepublication wording describes the accepted archive's build-time status.

The archive was built from private source commit
`f5893beb216464cc307f69cfb43a9fa28de6f6ff`. The public `main`
integration carries the same alpha executable Go, module files, packaged
guide/examples/notices, and pinned `FPF` source. A build from a different Git
commit has different VCS build metadata; it is not claimed to reproduce the
accepted archive byte-for-byte. The public source retains v9 consumers until
their later cutover.

Developers can compile the separate command from this source with
`go build -trimpath -o haft10 ./cmd/haft10`. That produces a local build, not
the reviewed prerelease archive.

The alpha supports project records with exact editions/history, explicit
change creation and preview/CAS application, real Go check capture with
retained outcomes, source reading, and CLI/MCP disclosure. A guided Codex
session and fresh read-only recovery completed for the accepted archive.
This is not broad host qualification or a comparative usefulness claim.

Known limits:

- Upgrade of caches from experimental pre-alpha packages is unqualified.
- For an oversized summary, a detail child may report `missing_part`.
  Request `part=summary, view=bytes` and follow the returned continuations,
  or read named claim/record parts.
- The exact upstream ICU revision for vendored Unicode headers remains
  unestablished; included notices identify known provenance but do not make
  a legal compliance claim.
- Current spec content is not binding approval. Selected checks were run;
  new race, full suite and extra vet runs were waived by the operator.

Technical carrier contracts for the alpha implementation are in
[`plans/v10`](plans/v10/README.md). They document source and behavior; they
do not themselves establish release qualification.
