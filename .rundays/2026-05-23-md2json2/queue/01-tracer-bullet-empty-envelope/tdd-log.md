# TDD log: 01-tracer-bullet-empty-envelope

Started: 2026-05-23

## Setup

- Test framework: Go's standard `testing` package + `go test` (the project is Go per CONTEXT.md / PRD).
- Module path: `github.com/sunfmin/md2json2` (matches the repository owner; per CONTEXT.md the canonical install path is `go install github.com/<owner>/md2json2@latest`).
- Layout chosen:
  - `main.go` at the module root: thin shell that injects process globals (`os.Args`, `os.Stdin/Stdout/Stderr`) into `cli.Run` and is the **only** place that calls `os.Exit`.
  - `internal/cli/cli.go`: argument parsing + IO wiring (the deep `cli` module per PRD). Exposes `Run(argv, stdin, stdout, stderr) int` — callable as a pure function from tests.
  - `internal/cli/cli_test.go`: in-process unit tests against `cli.Run(...)` taking injected IO and argv (no `os.Exit`, no globals).
  - `integration_test.go` at the module root: black-box harness that builds the binary once via `TestMain`, then runs every directory under `testdata/fixtures/` as a fixture.
  - `testdata/fixtures/01-empty-stdin/`: first fixture (args/input.md/stdout/stderr/exit).
- Rationale for the in-process `cli` unit tests in addition to the binary-driven fixture harness: per the PRD, "the entire pipeline is callable as a pure function inside a test" — that property lets the per-cycle RED→GREEN feedback run in milliseconds without paying the binary rebuild cost while still exercising the public `cli.Run` boundary. The fixture harness remains the contract test for byte-exact output (acceptance criterion #7); the in-process tests are just faster feedback against the same surface.

## Test 1 — empty stdin emits hard-coded envelope (criterion #1)

- Wrote: `TestEmptyStdinEmitsHardcodedEnvelope` in `internal/cli/cli_test.go`
- Red: build failed (`undefined: cli.Run`) — confirmed before adding `Run`.
- Green: added minimal `cli.Run` that writes the hard-coded envelope and returns 0. No commit (no repo-level commit hook expected at greenfield stage).
- Notes: assertion is byte-exact; envelope is the v1 ship criterion's exact 56-byte string with no trailing newline.

## Test 2 — fixture harness with byte-exact empty-stdin fixture (criterion #7)

- Wrote: `integration_test.go` with `TestMain` (builds the binary into a temp dir), `loadFixture`, `runFixture`, `assertFixture`, `TestFixtures` (walks `testdata/fixtures/`), plus fixture `testdata/fixtures/01-empty-stdin/`.
- Red: `go test .` reported `no non-test Go files in /Users/sunfmin/Developments/md2json2` — the `TestMain` build step failed because no `main.go` existed yet.
- Green: added `main.go` that delegates to `cli.Run` and is the **only** place that calls `os.Exit`. Binary builds; the empty-stdin fixture's stdout/stderr/exit all match byte-for-byte.
- Notes: fixture file shapes — `args` (one line, space-separated argv excluding argv[0]), optional `input.md`, expected `stdout`, expected `stderr`, expected `exit`. New slices add new fixture directories with no harness changes.

## Test 3 — positional FILE accepted, bytes read off disk (criterion #2)

- Wrote: `TestPositionalFileEmitsEnvelope` (positive — a readable file produces the envelope, exit 0) and `TestMissingPositionalFileObservable` (negative — a missing file produces a non-zero exit, proving the file is actually opened rather than ignored).
- Red: `TestMissingPositionalFileObservable` failed with `exit=0` because the initial `Run` ignored argv entirely.
- Green: added the `parseArgs` walker plus the file-open path in `Run`; missing file → return 1.
- Notes: the criterion calls out that the error path is "not yet polished" — S11 pins exit code `2` and the canonical stderr regex. S01's contract is only that missing-file is observable.

## Test 4 — `-` stdin sentinel behaves like no positional (criterion #3)

- Wrote: `TestStdinSentinelDashBehavesLikeNoPositional`.
- Red: skipped — the `parseArgs` walker already treated `-` as the stdin sentinel (carried over from Test 3's implementation). I verified this honestly by re-reading the code and confirming the test would have failed had `-` been interpreted as a file path (it would have tried to `os.Open("-")` and returned 1).
- Green: pre-existing.
- Notes: TDD purity note — when a green test passes "for free" because earlier minimal-impl steps happened to cover the case, the discipline is to keep the test (to lock the contract) and acknowledge the lack of an explicit RED step in the log. Done here.

## Test 5 — known v1 flags recognized as no-ops, exit 0 (criterion #4)

- Wrote: `TestKnownFlagsRecognizedAsNoop` with subtests for `--no-position`, `--pretty`, `--frontmatter-only`, `--output <path>`, `--output=<path>`, `-o <path>`.
- Red: all six subtests failed with `exit=1` because the previous `Run` treated `--no-position`, `--pretty`, etc. as file paths and `os.Open` failed.
- Green: built out the `parseArgs` flag table (long/short forms, value-bound `-o`/`--output` in both `key value` and `key=value` forms, the `--` end-of-flags sentinel for future-proofing). Each flag sets a field on `options`; nothing reads the fields yet — that's S03's and S11's job.
- Notes: `-o`/`--output` value parsing supports three syntaxes (`-o path`, `--output path`, `--output=path`, `-o=path`) so later slices that pin the behavior do not have to add new syntactic forms.

## Test 6 — `-h` and `-V` exit 0 (criterion #5)

- Wrote: `TestHelpAndVersionExitZero` with subtests for `-h`, `--help`, `-V`, `--version`.
- Red: skipped — the `parseArgs` walker added in Test 5's GREEN step already handled `-h`/`-V` and `Run` already short-circuited with placeholder text on stdout. I verified honestly by re-reading the code; the test pins the contract for later refactors.
- Green: pre-existing.
- Notes: per criterion text, the output bytes are placeholder; S11 pins exact help/version messages.

## Test 7 — unknown flag exits non-zero (criterion #6)

- Wrote: `TestUnknownFlagExitsNonZero` with subtests for `--no-such-flag` and `-z`.
- Red: skipped — the `parseArgs` walker's default case for unrecognized `-`-prefixed args already returned `ok=false` (added in Test 5's impl), which `Run` translated to `return 1`. Test locks the contract.
- Green: pre-existing.
- Notes: S11 will pin the exit code to `2` and add the canonical stderr regex; S01's contract is non-zero exit only.

## Test 8 — harness comparison is byte-exact (criterion #8)

- Wrote: `TestHarnessDetectsSingleByteStdoutDiff` in `integration_test.go`. The fixture harness's comparison logic was extracted into a pure `compareFixture(fx, got...) []string` so it could be tested without reaching into `testing.T`'s internals. The test constructs a `wantStdout` and a `got` differing in exactly one byte (a trailing `}` flipped to `]`), runs the comparison, and asserts a mismatch is reported.
- Red: first attempt failed because my `got` string had a stray space and was one byte longer than `want` (the setup-error guard caught it). Fixed by switching to a true one-for-one byte substitution.
- Green: with the corrected fixture, `compareFixture` reports the diff and the test passes. The extracted `compareFixture` is used by the live `assertFixture` too, so this test pins the byte-exactness of the real harness, not a parallel copy.
- Notes: the fixture itself is synthetic (not on disk) — the point is the *comparison* primitive, which is the load-bearing part of the "would a one-byte diff fail the test?" sanity check.

## Refactor pass

After all tests green: replaced the awkward `len(a) > len("--output=") && a[:len("--output=")] == "--output="` slice-prefix pattern in `parseArgs` with `strings.HasPrefix` + `strings.TrimPrefix`. Tests stayed green; `go vet ./...` clean.

## Final

- Tests added: 8 top-level (with subtests bringing the leaf count to 18).
- Tests passing: 18/18 (`go test ./...` clean, including the integration harness that builds and runs the real binary).
- `go vet ./...`: clean.
- Manual end-to-end verification: `go build -o /tmp/md2json2 . && /tmp/md2json2 < /dev/null | xxd` returns exactly the 56-byte envelope with no trailing newline, exit 0 — the v1 ship criterion's `--no-position` flavor (S01 hard-codes the envelope, so this holds unconditionally at this stage).

- Acceptance criteria status:
  - [x] criterion 1 — `md2json2 < /dev/null` emits exact envelope, exit 0 (`TestEmptyStdinEmitsHardcodedEnvelope`, fixture `01-empty-stdin`)
  - [x] criterion 2 — positional FILE accepted, bytes read (`TestPositionalFileEmitsEnvelope` + `TestMissingPositionalFileObservable`)
  - [x] criterion 3 — `-` stdin sentinel identical to no positional (`TestStdinSentinelDashBehavesLikeNoPositional`)
  - [x] criterion 4 — `--no-position`, `--pretty`, `--frontmatter-only`, `-o` all exit 0 (`TestKnownFlagsRecognizedAsNoop`)
  - [x] criterion 5 — `-h` and `-V` exit 0 (`TestHelpAndVersionExitZero`)
  - [x] criterion 6 — `--no-such-flag` exits non-zero (`TestUnknownFlagExitsNonZero`)
  - [x] criterion 7 — fixture-driven harness exists with `args`/`input.md`/`stdout`/`stderr`/`exit`, byte-for-byte, ≥1 fixture passing (`TestFixtures/01-empty-stdin`)
  - [x] criterion 8 — harness comparison is byte-exact; a one-byte stdout diff fails (`TestHarnessDetectsSingleByteStdoutDiff`)

VERDICT: accept
