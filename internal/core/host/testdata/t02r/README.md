# Historical managed-config fixtures

These four `.codex/config.toml` fixtures preserve the generated field shape
from the controller's test-owned projects. Their machine-specific path prefix
was replaced with `/Users/fixture/haft-test` for the public source copy; no
product code or other fixture field was changed. B1 was produced by an
independently built B1 binary at
`3d3ad688944ed7587ad38f406d7efa8999fa758f`; T01AR, T01R2 and T01BR were
produced by their corresponding independently built predecessor binaries.
The controller separately ran old/new CLI upgrades in the private evaluation.
These fixtures retain the generated config shape and outside foreign bytes;
no current Init output was substituted.

| Fixture | SHA-256 of public synthetic file |
|---|---|
| `b1-config.toml` | `ab8c91386b9626e86aba0074c0cc0458cf41a93554865f1e671e6dd0331a4d44` |
| `t01ar-config.toml` | `febae23d3a327ae841de969271dcd99f78274c7fee73a9c546956cc419a7f51b` |
| `t01r2-config.toml` | `731178616995c10c9def4248f083fa41344dcf1826b8194d188731e38a1beb6b` |
| `t01br-config.toml` | `24bd4ee2a494d83eea98d0aec3164f6475e0f37f755548bee1805b1c0cca67a2` |

The portable test verifies those hashes and exact generated block round-trip,
then substitutes only the three old local addresses with test-owned absolute
paths before running Init. The fixture paths are historical test addresses and
are never opened by the portable test.
