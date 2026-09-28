# Frozen B1 publication fixture

This fixture was produced by the B1 haft10 binary built from exact commit
3d3ad688944ed7587ad38f406d7efa8999fa758f (binary SHA-256
a8430ba6ec9a4d1124a003ca080f127e86bd209cdb600314685bceccd2a359fe).

The input is request.json. The producer ran
haft10 api --root <test-owned-temp-root> --input - with that JSON on stdin
and exited 0 with result_kind=written. The generated .haft/ record, edition
snapshot, transaction manifest, commit marker and staged output bytes are kept
unaltered. Disposable disclosure cache and empty writer lock are excluded.
basis.json pins each retained file's digest and the observed generation,
transaction ID and request payload digest.

This is synthetic test data. It does not establish native host behaviour,
authority, acceptance or release status. Tests copy the frozen files into a
temporary project root; they never write into this fixture.
