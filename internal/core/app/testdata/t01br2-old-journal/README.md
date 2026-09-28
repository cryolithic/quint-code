# Old reauthor journal fixture

This synthetic project memory was written by the T01B `haft10` binary built
from Git `b6874589aa9247f588a3bfd29a6ba22ccee40d63` (binary SHA-256
`5341548f53edd67b6512f17cb59d87dfa5f9129ed3842f945439729837684c16`).
The construction called `haft10 api --root PROJECT --input FILE` with
`seed-request.json`, recalled `spec:replay-test` for its pinned predecessor,
called `change/reauthor_preview` for that ref, then applied `request.json`.
`expected.json` records the exact predecessor and published successor observed
through public recall. The two original committed journals,
their staged outputs, carriers, and snapshots are copied unchanged. Disposable
`.cache` and `.runtime` files are excluded so the test replays after a cache-free
restart. The fixture has no live project data or machine paths.
