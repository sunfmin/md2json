# S12: CLI completeness — `-o`, `-h`, `-V`, exit codes, canonical stderr regex

Status: ready-for-agent

The `cli` module finishes its v1 contract: `-o, --output <FILE>` writes the JSON envelope to the named file instead of stdout; `-h, --help` prints usage on stdout and exits `0`; `-V, --version` prints a version string on stdout and exits `0`. (The flags themselves were already recognized in S01's tracer; this slice pins their behavior to the v1 contract.) Every stderr diagnostic matches exactly one regex: `^md2json: ([^:]+):(\d+):(\d+): (.+)$`. Exit codes follow the contract: `0` success, `1` parse/document-scoped error (invalid UTF-8, invalid frontmatter, unrecoverable goldmark error), `2` usage error (unknown flag, missing/unreadable `FILE` before any bytes are read). The `<path>` token in error lines is the file path when one is in play, the literal `-` when reading from stdin, and the literal `md2json` when no input source has been determined (pre-input usage errors). The `:0:0:` sentinel is used for no-position errors; when goldmark reports a line but no column, the column rounds up to `1` (never `0`).

## Acceptance

- [ ] `md2json -o out.json post.md` writes the JSON envelope to `out.json` (creating or truncating it), writes nothing to stdout, exit `0`.
- [ ] `md2json -h` (and `--help`) writes a usage message describing each v1 flag and the positional `FILE`/`-` convention to stdout, exit `0`.
- [ ] `md2json -V` (and `--version`) writes a version string to stdout, exit `0`.
- [ ] `md2json --no-such-flag` writes exactly one stderr line matching `^md2json: md2json:0:0: .+$` and exits `2`. Nothing on stdout.
- [ ] `md2json /does/not/exist.md` (unreadable FILE, error raised before any bytes are read) writes exactly one stderr line matching `^md2json: md2json:0:0: .+$` and exits `2`. Nothing on stdout.
- [ ] When a document-scoped error has no position (a goldmark error with no line/column), the stderr line uses the `<path>:0:0:` sentinel and exit code is `1`.
- [ ] When a goldmark error reports a line but no column, the stderr line uses `<path>:<line>:1:` (column rounded up to `1`, never `0`).
- [ ] Across the entire fixture suite, every emitted stderr line matches the single canonical regex `^md2json: ([^:]+):(\d+):(\d+): (.+)$` — verified by a test that grep-matches every fixture's expected `stderr.txt`.
- [ ] A stdin-source error fixture asserts `<path>` is the literal `-`; a pre-input usage-error fixture asserts `<path>` is the literal `md2json`.

## Blocked by

S11 — needs the fully-featured emitter (compact and pretty) and the translate-stage node set so `-o` can write the real envelope and the canonical-regex test can scan the full fixture suite.
