# TDD log: 11-cli-flags-error-format-exit-codes

Started: 2026-05-23

Scope: S12 — pin v1 CLI contract for `-o`, `-h`, `-V`, exit codes 0/1/2, and the canonical stderr regex.

Acceptance criteria (issue.md):
1. `-o out.json post.md` writes envelope to `out.json`, nothing on stdout, exit 0.
2. `-h`/`--help` prints usage on stdout, exit 0; usage names each v1 flag + the `FILE`/`-` convention.
3. `-V`/`--version` prints a version string on stdout, exit 0.
4. Unknown flag → `md2json2: md2json2:0:0: ...` on stderr, exit 2, stdout empty.
5. Unreadable `FILE` (before any bytes read) → `md2json2: md2json2:0:0: ...` on stderr, exit 2, stdout empty.
6. Doc-scoped goldmark error with no line/col → `<path>:0:0:` + exit 1.
7. Goldmark error with line but no col → `<path>:<line>:1:` + exit 1.
8. Canonical regex `^md2json2: ([^:]+):(\d+):(\d+): (.+)$` matches every fixture's expected stderr.
9. Stdin-source error → `<path>` is `-`; pre-input usage error → `<path>` is `md2json2`.

Baseline: `go test ./...` green, `go vet ./...` clean before any change.

## Test 1 — unknown flag is a pre-input usage error (criteria #4 + #9 pre-input half)
- Wrote: `TestUnknownFlagPreInputUsageError` in `internal/cli/cli_test.go`
- Red: exit code was 1, stderr was empty (current parseArgs returned just `ok bool`; cli.Run returned 1, no stderr line written).
- Green: changed `parseArgs` signature to return `(opts, usageErr string)`; added `preInputPathToken = "md2json2"` constant; Run renders canonical stderr via `writePositionedError(stderr, preInputPathToken, 0, 0, usageErr)` and returns 2.
- Notes: usage error message format is `unknown flag <a>` / `flag <a> requires a value`; canonical regex enforced by the test.

## Test 2 — unreadable FILE is a pre-input usage error (criteria #5 + #9 pre-input half)
- Wrote: `TestMissingFilePreInputUsageError` in `internal/cli/cli_test.go`
- Red: exit was 1, stderr empty (cli.Run silently returned 1 on `os.Open` failure).
- Green: Run's `os.Open(opts.filePath)` failure now goes through `writePositionedError(stderr, preInputPathToken, 0, 0, err.Error())` and returns 2.
- Notes: criterion #9 pre-input half is locked in by the same canonical-regex assertion on the `<path>` token equalling `md2json2`.

## Test 3 — `-o <FILE>` writes envelope to file, stdout empty, exit 0 (criterion #1)
- Wrote: `TestOutputFlagWritesEnvelopeToFile` + `TestOutputFlagTruncatesExistingFile` in `internal/cli/cli_test.go`
- Red: envelope went to stdout regardless of `-o`; output file never written; existing-file contents survived.
- Green: Run resolves the output sink before calling read/parse/translate/emit: if `opts.output != ""`, `os.Create(opts.output)` opens a truncating file handle and the two Emit calls write to it instead of stdout. A failure to open the output file is itself a pre-input usage error (exit 2, `md2json2:0:0:`).
- Notes: short `-o`, long `--output`, and `--output=PATH` all covered.

## Test 4 — `-h`/`--help` prints usage on stdout (criterion #2)
- Wrote: `TestHelpFlagPrintsUsageNamingEveryFlag` in `internal/cli/cli_test.go`
- Red: usage was `md2json2: help (placeholder)\n`; did not name any flag or FILE/`-` convention.
- Green: replaced with a `usageText` const that names every v1 flag (`-o/--output`, `--pretty`, `--no-position`, `--frontmatter-only`, `-h/--help`, `-V/--version`) plus the positional `FILE`/`-` stdin sentinel and the exit-code map.
- Notes: `-h` and `--help` produce identical bytes (one source of truth).

## Test 5 — `-V`/`--version` prints version on stdout (criterion #3)
- Wrote: `TestVersionFlagPrintsVersion` in `internal/cli/cli_test.go`
- Red: previous placeholder did not satisfy "contains a digit"; replaced when Test 4's refactor introduced `versionText = "md2json2 v1.0.0\n"`.
- Green: passes immediately after Test 4's refactor — versionText contains both `md2json2` and a digit; `-V` and `--version` byte-identical.
- Notes: real semver stamping is deferred to S13's release pipeline; v1.0.0 is a static placeholder. The test asserts shape (contains-name + contains-digit + short==long), not exact bytes, so S13 can rewrite without churning this test.

## Test 6 — document-scoped error renders `<path>:0:0:` + exit 1 (criterion #6)
- Wrote: `TestDocumentScopedErrorUsesZeroPositionSentinel` in `internal/cli/cli_test.go`
- Red: n/a — the path already existed in S02 (`writeDocScopedError`). Test drives the path through a `failingWriter` so emit.Emit returns an error and cli's document-scoped branch fires.
- Green: passes immediately. The test pins both the `:0:0:` sentinel and the stdin path-token `-`.
- Notes: this test is the regression net for criterion #6; before this slice nothing was unit-asserting that emit-failure routed through `writeDocScopedError`. Stdin half of criterion #9 also gets a unit-level lock here.

## Test 7 — column rounds up to 1 when line is known but column is not (criterion #7)
- Wrote: `TestPositionedErrorRoundsUnknownColumnUpToOne` in `internal/cli/cli_test.go`
- Red: input `(line=5, col=0)` produced `:5:0:` (raw); contract requires `:5:1:`.
- Green: `writePositionedError` now rounds `col` to 1 when `line > 0 && col < 1`. The `(line=0, col=0)` sentinel branch is preserved (both stay 0).
- Notes: this is a property of the rendering helper; any future caller passing a line-only error from goldmark gets the right shape automatically.

## Test 8 — canonical regex matches every fixture's stderr (criterion #8)
- Wrote: `TestCanonicalStderrRegexMatchesEveryFixture` in `integration_test.go`
- Red: n/a — every existing fixture's stderr already conforms.
- Green: passes; scans 4 existing stderr lines + 1 new fixture (`60-pre-input-unknown-flag-usage-error`) and asserts every one matches `^md2json2: ([^:]+):(\d+):(\d+): (.+)$`.
- Notes: this is the property check that catches future fixture drift before it can hide in TestFixtures. Fails loudly if zero stderr lines scanned (vacuous-pass guard).

## Test 9 — stdin-source fixture has `<path>=-`, pre-input fixture has `<path>=md2json2` (criterion #9)
- Wrote: `TestStdinSourceFixtureUsesDashPathToken` + `TestPreInputUsageErrorFixtureUsesMd2json2PathToken` in `integration_test.go`. Added new fixture `testdata/fixtures/60-pre-input-unknown-flag-usage-error/` (args `--no-such-flag`; expected stderr `md2json2: md2json2:0:0: unknown flag --no-such-flag\n`; exit 2; stdout empty).
- Red: first attempt over-triggered — initial heuristic treated any non-positional erroring fixture as stdin-source, but the new exit-2 fixture matched. Refined to "exit == 1" for stdin-source (read/parse error) vs "exit == 2" for pre-input (which is what `md2json2` path token guards).
- Green: both tests pass; both halves of criterion #9 covered at the integration level.
- Notes: stdin-source half rides on existing fixtures `02-invalid-utf8-leading`, `03-invalid-utf8-mid-document`, `46-malformed-yaml-hard-error`, `51-frontmatter-only-malformed-yaml`. Pre-input half rides on the new `60-...` fixture. Did NOT add a missing-FILE byte-exact fixture: the OS-specific `os.Open` error message ("no such file or directory" on darwin/linux; different on Windows) would make it platform-flaky. Cross-platform stable assertion lives in the unit test `TestMissingFilePreInputUsageError` instead.

## Final
- Tests added: 9 new test functions (10 if you count Test 3 and Test 3b separately), plus 1 new fixture.
- Tests passing: full `go test ./...` green, `go vet ./...` clean.
- Acceptance status:
  - [x] criterion 1 — `-o` writes envelope to file, stdout empty, exit 0
  - [x] criterion 2 — `-h`/`--help` prints usage naming every v1 flag and `FILE`/`-`, exit 0
  - [x] criterion 3 — `-V`/`--version` prints version, exit 0
  - [x] criterion 4 — unknown flag → `md2json2: md2json2:0:0: ...`, exit 2, stdout empty
  - [x] criterion 5 — unreadable FILE → `md2json2: md2json2:0:0: ...`, exit 2, stdout empty
  - [x] criterion 6 — document-scoped error → `<path>:0:0:`, exit 1
  - [x] criterion 7 — goldmark line-only error → `<path>:<line>:1:` (column rounds up to 1)
  - [x] criterion 8 — canonical regex matches every fixture's stderr
  - [x] criterion 9 — stdin-source fixture has `<path>=-`; pre-input usage-error fixture has `<path>=md2json2`

No ADR added: the architectural decisions exercised here (pre-input path-token sentinel, column-rounding rule, exit-code mapping) are already pinned by CONTEXT.md "Error format", PRD user stories 19/20, and the PRD's "Error-format / exit-code mapping (contract)" table. This slice implements those contracts; it does not introduce new ones.

VERDICT: accept
