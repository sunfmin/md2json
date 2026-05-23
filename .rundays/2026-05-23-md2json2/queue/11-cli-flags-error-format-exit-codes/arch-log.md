# Arch log: 11-cli-flags-error-format-exit-codes

Started: 2026-05-23
Scope: mini
File set (from tdd-log.md):
- internal/cli/cli.go
- internal/cli/cli_test.go
- integration_test.go
- testdata/fixtures/60-pre-input-unknown-flag-usage-error/ (data files only — not refactorable)

## Baseline
- `go test ./...`: all packages PASS (6/6 packages green)
- `go vet ./...`: clean
- LOC in file set: 297 (cli.go) + 530 (cli_test.go) + 394 (integration_test.go) = 1221

## Candidate survey (lens: glossary alignment, depth, naming, locality)

### Strong
1. **Drop `options.hasPositional` field.** Dead weight: set in 3 places, read in 1, logically redundant with `filePath != ""`. Shallower `options` interface; one fewer field invariant for a future reader to internalize. Lens: naming clarity + module depth.
2. **Introduce `writePreInputUsageError` helper.** Pre-input usage error is a load-bearing CONTEXT.md / PRD concept (path token = `md2json2`, position `0:0`, exit 2). Today the three call sites (unknown-flag, unreadable FILE, unwritable output) each open-code `writePositionedError(stderr, preInputPathToken, 0, 0, msg); return 2`. Symmetrical with the existing `writeDocScopedError` wrapper. Lens: glossary alignment + locality.

### Worth exploring (notes only, not acted on)
- `parseArgs` returns `(opts, usageErr string)` with empty-string success sentinel. Could become `error`. No leverage today — the string is concatenated directly into the stderr line; wrapping in `error` only to unwrap in `Run` adds ceremony without depth.
- `failingWriter` / `errIO` / `stubError` in cli_test.go: three types for one Write-fails sink used by one test. Slight redundancy but clearly named and scoped to tests; deletion test does not pass strongly.

### Speculative (notes only, not acted on)
- `output==""` overloads "stdout", `filePath==""`/`"-"` both overload "stdin". A typed `inputSource` / `outputSink` ADT would be cleaner but is a deep refactor touching multiple modules; out of mini-scope.
- `parseArgs` switch ladder: the complexity *is* the v1 flag table; no compressible structure. Leave.

## Pass 1: drop `options.hasPositional`

Removed the redundant `hasPositional bool` field from `options`. The condition `opts.hasPositional && opts.filePath != "" && opts.filePath != "-"` reduces to `opts.filePath != "" && opts.filePath != "-"` because `parseArgs` is the only writer of `filePath` and every such write was paired with `hasPositional = true`. Future readers see exactly one positional-state field instead of an implicit two-field invariant.

- Files touched: `internal/cli/cli.go`
- Tests after: all packages PASS (go test ./... green)
- Reverted? no

## Pass 2: extract `writePreInputUsageError` helper

Added a third sibling to the canonical-stderr family in `cli.go`:
- `writePositionedError(...)` — the rendering primitive.
- `writeDocScopedError(...)` — wraps for the document-scoped `:0:0:` sentinel with a real `<path>`.
- `writePreInputUsageError(...)` — wraps for the pre-input usage error case where the `<path>` token is `md2json2` (no source ever in play) and the exit code is 2.

The new helper returns `int` (the exit code, always 2) so call sites read `return writePreInputUsageError(stderr, msg)`. The three pre-input branches (unknown flag, unreadable input FILE, uncreatable output FILE) now flow through one rendering+exit-code helper. CONTEXT.md "Error format" + PRD US20 "pre-input usage error" is now concrete at every call site.

- Files touched: `internal/cli/cli.go`
- Tests after: all packages PASS (go test ./... green)
- Reverted? no

## Final
- Tests: all packages PASS, `go vet` clean (unchanged from baseline)
- LOC delta: cli.go +6 / -10 net = -4 lines
- Most consequential change: Pass 2 — pre-input usage error is now a named, single-source-of-truth helper. The three call sites that mint exit-code 2 with `<path>=md2json2` no longer each carry the contract; future changes to the pre-input-error contract (e.g. a future ADR adding a position-rounding rule for usage errors) touch one helper.

VERDICT: accept
