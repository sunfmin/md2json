# S01: Tracer bullet — `md2json2` emits a hard-coded empty JSON envelope end-to-end, with the v1 flag surface recognized

Status: ready-for-agent

A user can run `md2json2` reading from stdin or a positional `FILE`, and the tool writes a single-line empty JSON envelope to stdout and exits `0`. This slice stands up the CLI skeleton (build target, entry point, stdin/stdout/stderr wiring), the black-box integration-test harness that every subsequent slice will reuse, and the full v1 flag surface as a parsed/recognized set so later slices never have to retro-fit flag parsing into the entry point. The JSON envelope is hard-coded for now and no parsing of any kind happens yet, but the flag set (`-o/--output`, `--pretty`, `--no-position`, `--frontmatter-only`, `-h/--help`, `-V/--version`) is recognized — passing any of them does not fail; their behavior is a no-op or minimal stub at this stage (e.g. `-h`/`-V` may print a placeholder line; `--no-position`/`--frontmatter-only`/`--pretty` simply set internal flags that nothing reads yet). Unknown flags exit non-zero. This eliminates the topological inversion where later slices (S03 invoking `--no-position`/`--frontmatter-only`, S11 owning `-o`/`-h`/`-V`) would otherwise reach back into S01's entry-point shape.

## What to build

Stand up the `cli` module entry point with argument parsing covering the v1 flag set. The parser distinguishes known flags (recognized, value-bound where applicable, no-op behavior at this stage) from unknown flags (rejected). The positional `FILE` argument and the `-` stdin sentinel are accepted as input source selectors — when present, the file is opened for reading and its bytes are consumed-and-discarded so the stdin/file path is exercised, but the output envelope is still hard-coded. The fixture-driven integration-test harness lives under a `testdata/fixtures/` directory or equivalent; each fixture is a directory of (`args`, optional `input.md`, expected `stdout`, expected `stderr`, expected `exit`). The harness invokes the built binary as a black box and compares byte-for-byte.

## Acceptance

- [ ] `md2json2 < /dev/null` writes exactly `{"frontmatter":null,"ast":{"type":"root","children":[]}}` (no trailing newline) to stdout, writes nothing to stderr, and exits `0`.
- [ ] `md2json2 /path/to/any-file.md` (with any readable file as positional argument) writes the same hard-coded envelope to stdout and exits `0` — the file's bytes are read off disk (so a missing/unreadable file would be observable, though that error path is not yet polished) but the contents are not yet inspected for content.
- [ ] `md2json2 -` (stdin sentinel) behaves identically to `md2json2` with no positional argument.
- [ ] `md2json2 --no-position < /dev/null`, `md2json2 --pretty < /dev/null`, `md2json2 --frontmatter-only < /dev/null`, and `md2json2 -o /tmp/x < /dev/null` all exit `0` without erroring on the flag (the flag is recognized; its behavior may be a no-op at this stage, except `-o` which is permitted to either write to the file or be a stub — later slices pin its behavior).
- [ ] `md2json2 -h` and `md2json2 -V` exit `0` (output text is a placeholder; later slices pin the message bytes).
- [ ] `md2json2 --no-such-flag` exits non-zero (any non-zero code; S11 pins it to `2` and the stderr regex shape).
- [ ] A fixture-driven integration test harness exists: each fixture directory contains an `args` file, optional `input.md`, expected `stdout`, expected `stderr`, and expected `exit` code; the harness invokes the built binary as a black box and compares byte-for-byte. At least one fixture under this harness exercises the empty-stdin case above and passes.
- [ ] Running the harness against a fixture whose `exit` is `0` but whose `stdout` differs by one byte fails the test (sanity check that comparison is byte-exact).

## Blocked by

None - can start immediately
