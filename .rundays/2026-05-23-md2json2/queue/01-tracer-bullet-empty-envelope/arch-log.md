# Arch log: 01-tracer-bullet-empty-envelope

Started: 2026-05-23T06:17:40Z
Scope: mini
File set (from S01 tdd-log):
- `go.mod`
- `main.go`
- `internal/cli/cli.go`
- `internal/cli/cli_test.go`
- `integration_test.go`
- `testdata/fixtures/01-empty-stdin/{args,input.md,stdout,stderr,exit}`

## Baseline

- Tests: 18 passing, 0 failing
  - `internal/cli` package: 14 leaf (5 top-level + 9 subtests across `TestHelpAndVersionExitZero`, `TestUnknownFlagExitsNonZero`, `TestKnownFlagsRecognizedAsNoop`)
  - root integration package: 2 top-level (`TestHarnessDetectsSingleByteStdoutDiff`, `TestFixtures`) + 1 fixture subtest (`TestFixtures/01-empty-stdin`)
- `go vet ./...`: clean.
- Total LOC (in-scope files): 548 (`main.go` 17, `internal/cli/cli.go` 143, `internal/cli/cli_test.go` 165, `integration_test.go` 223). `go.mod` not counted.

## Exploration / candidates considered

Reviewed against the four refactor lenses (glossary alignment / module depth / naming clarity / locality), using CONTEXT.md and ADR-0001 as the reference frame.

| Candidate | Lens | Score | Notes |
|---|---|---|---|
| Move `parseArgs` + `options` into a sibling `internal/cli/args.go` file | locality | Speculative | Same package, same surface, single caller — file split would not deepen the module. |
| Rename `ok` → `valid` on `parseArgs`'s second return | naming | Speculative | Style preference; `(opts, ok)` is idiomatic Go. |
| Tighten the `case a == "--"` branch with `for _, p := range args[i+1:]` | clarity | Speculative | Cosmetic; current form is correct and self-explanatory. |
| Rename `options.filePath` → `inputPath` | glossary | Speculative | CONTEXT.md says "always speak in terms of stdin vs positional FILE"; `filePath` already reads as "the positional FILE's path." No glossary miss. |
| Extract `resolveInput(opts, stdin) (io.Reader, io.Closer, error)` from `Run` | depth | Speculative | Single caller, ~7 lines, no leverage today. S03+ (read/parse/translate/emit) is where the real plumbing lives; pre-deepening now would create churn S03 must undo. |
| Map `parseArgs` failures to distinct return codes (1 vs 2 vs canonical stderr line) | glossary (exit codes) | Out of scope | tdd-log explicitly defers exit code 2 + canonical stderr regex to S11; S01's contract is only "non-zero exit." Acting now would conflict with the slice plan. |
| Make `compareFixture` deeper / structured-diff result | depth | Speculative | Already a pure function with two real call sites (`assertFixture` + `TestHarnessDetectsSingleByteStdoutDiff`); deletion test says it's earning its keep at current depth. |

**Deletion test on `internal/cli`:** if removed and inlined into `main`, the `cli_test.go` in-process unit tests would lose their seam (cannot inject `argv`/`stdin`/`stdout`/`stderr` without going through `os.*` globals + a separate binary build). The seam is real, the module is deep enough for its current job. Keep.

**Deletion test on `compareFixture`:** if removed, `assertFixture` would inline three `bytes.Equal` checks AND the harness sanity-check `TestHarnessDetectsSingleByteStdoutDiff` would either duplicate them or vanish. Two real adapters → real seam. Keep.

**CONTEXT.md / ADR-0001 alignment scan:** every term currently in code matches a glossary entry:
- `emptyEnvelope` (const) ↔ "JSON envelope" + "v1 ship criterion" empty case.
- `options.{pretty, noPosition, frontmatterOnly, output}` ↔ "v1 flags" entries one-for-one.
- `options.{filePath, hasPositional}` ↔ "CLI contract" (positional FILE, stdin sentinel).
- `Run(argv, stdin, stdout, stderr) int` ↔ PRD's explicit-IO rule and CONTEXT.md's "CLI contract" / "Exit codes."
No `_Avoid_` term ("stdin"/"<stdin>" as a path token, "goldmark AST" leaking, "Markdown" unqualified, "input mode") appears anywhere in the in-scope files.

## Passes applied

**No strong candidates this pass.** S01 is an intentionally minimal tracer bullet whose interface (`cli.Run`) already matches PRD/CONTEXT vocabulary exactly, whose only real seam (in-process testability of `Run`) is already in place, and whose IO resolution is well-localized to a 7-line tail of `Run`. The candidates above are all `Speculative` (style-only) or `Worth exploring` only once S03's parse/translate/emit pipeline lands and gives `parseArgs`'s outputs and `Run`'s IO-resolution real consumers. Acting on any of them now would be churn — at best stylistic, at worst conflicting with the slice plan (e.g., exit-code mapping is reserved for S11).

Per the Proposer-Arch contract: "If there are no Strong candidates, write an arch-log noting 'no strong candidates this pass' and emit `VERDICT: accept` — that is a legitimate acceptable outcome."

## Final

- Tests: 18 passing (unchanged from baseline). `go vet ./...` still clean.
- LOC delta: 0 (no code changes).
- Most consequential observation (carried forward, not acted on): once S03 adds the read/parse/translate/emit pipeline, `Run`'s body will grow past the point where the IO-resolution tail and the pipeline driver coexist comfortably. At that point an `internal/cli/run.go` split (resolve input → drive pipeline → write envelope) or a small `internal/io` helper for "open positional or fall back to stdin, with a close hook" becomes a real depth win. For S01, the function fits in one screen and the interface (`Run`) is the right shape; no premature deepening.

VERDICT: accept
