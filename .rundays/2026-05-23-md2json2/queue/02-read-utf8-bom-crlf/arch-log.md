# Arch log: 02-read-utf8-bom-crlf

Started: 2026-05-23T07:05:00Z
Scope: mini
File set (from S02 tdd-log):
- `internal/read/read.go` (new)
- `internal/read/read_test.go` (new)
- `internal/cli/cli.go` (modified — `read.Read` wired into `Run`, typed-error route to stderr)
- `internal/cli/cli_test.go` (added `TestLeadingInvalidUTF8StdinHardErrors`, `TestMidDocumentInvalidUTF8StdinHardErrors`)
- `testdata/fixtures/02-invalid-utf8-leading/{args,input.md,stdout,stderr,exit}` (new)
- `testdata/fixtures/03-invalid-utf8-mid-document/{args,input.md,stdout,stderr,exit}` (new)

## Baseline

- Tests: 30 leaf passing, 0 failing.
  - root integration package: 4 leaf (`TestHarnessDetectsSingleByteStdoutDiff`, `TestFixtures/01-empty-stdin`, `TestFixtures/02-invalid-utf8-leading`, `TestFixtures/03-invalid-utf8-mid-document`).
  - `internal/cli` package: 18 leaf (the 14 from S01 baseline + 4 new: `TestLeadingInvalidUTF8StdinHardErrors`, `TestMidDocumentInvalidUTF8StdinHardErrors`, plus S01's two top-level cases already covered).
    - Actually counting: 5 top-level singletons (`TestEmptyStdinEmitsHardcodedEnvelope`, `TestPositionalFileEmitsEnvelope`, `TestMissingPositionalFileObservable`, `TestStdinSentinelDashBehavesLikeNoPositional`, `TestLeadingInvalidUTF8StdinHardErrors`, `TestMidDocumentInvalidUTF8StdinHardErrors`) + 4 subtests under `TestHelpAndVersionExitZero` + 2 subtests under `TestUnknownFlagExitsNonZero` + 6 subtests under `TestKnownFlagsRecognizedAsNoop` = 18 leaf.
  - `internal/read` package: 8 leaf (all top-level: `TestLeadingBOMIsStripped`, `TestCRLFNormalizedToLF`, `TestByteLengthReflectsBothTransforms`, `TestInvalidUTF8ReturnsTypedReadError`, `TestBOMShapedMidDocumentIsLeftAlone`, `TestNoBOMLFOnlyRoundTripsByteForByte`, `TestLeadingInvalidByteAtOriginPosition`, `TestBareCRNormalizedToLF`).
  - Total = 4 + 18 + 8 = 30 leaf, all green.
- `go vet ./...`: clean (no warnings under `go test ./...`).
- LOC (in-scope files only): 657 across the six source/test files (read.go 129, read_test.go 162, cli.go 163, cli_test.go 203; fixtures negligible).

## Exploration / candidates considered

Reviewed against the four refactor lenses (glossary alignment / module depth / naming clarity / locality), using CONTEXT.md and ADR-0001 as the reference frame.

### Glossary alignment

CONTEXT.md "Input handling" specifies the exact stderr message for invalid UTF-8: `md2json2: <path>:<line>:<col>: invalid utf-8 byte at offset <N>`. Verified:
- `ReadError.Msg` is constructed as `"invalid utf-8 byte at offset %d"` — matches verbatim.
- `(*ReadError).Error()` returns `"<path>:<line>:<col>: <msg>"` — matches the per-line shape, leaving the `md2json2: ` tool-name prefix to the cli layer (where it belongs — the read module is deliberately tool-agnostic per its package docstring).
- `ReadError.{Path, Line, Col, Offset, Msg}` field names tokenize the regex `^md2json2: ([^:]+):(\d+):(\d+): (.+)$` directly.
- The "stdin path token is literal `-` (not `stdin`, not `<stdin>`)" rule from CONTEXT.md "Error format" is honored by cli.go assigning `pathToken := "-"` as the default and only switching to a file path when one is supplied. Tested by both `TestLeadingInvalidUTF8StdinHardErrors` and the `02-invalid-utf8-leading` fixture.
- ADR-0001 §6 "`position.offset` is a byte offset into the normalized (LF-only, post-BOM-strip) document" matches `Read`'s implementation: the validator walks `norm` (after BOM-strip and CRLF-collapse), and `ReadError.Offset = i` where `i` indexes into `norm`. Verified against `TestMidDocumentInvalidUTF8StdinHardErrors` (`"hi\nworld\xC3\x28"` — no BOM, no CR, so norm == raw and the expected offset 8 matches).
- The `_Avoid_` terms — `<stdin>`/`stdin` as a path token, "auto-detect encoding," "lenient encoding," U+FFFD substitution — do not appear in the in-scope code. The single occurrence of "U+FFFD" is in the package doc comment of read.go where it is explicitly negated ("never substitute U+FFFD"), which matches CONTEXT.md's `_Avoid_` framing.

No glossary miss. No `trigger-grill` warranted.

### Module depth and locality

Candidate table:

| Candidate | Lens | Score | Notes |
|---|---|---|---|
| Move the typed-vs-fallback error formatting in cli.Run lines 151–159 into a `read.FormatError(err, pathToken) string` helper | depth/locality | Worth exploring | Only one caller (cli.Run) today; the `md2json2: ` prefix is a cli-layer concern (CONTEXT.md "Error format" calls out the tool name). One adapter = hypothetical seam, not a real one. S03+ may introduce its own error-source layers (parser, frontmatter, translate) whose formatting story is unknown — premature unification now would lock in a shape that S03's needs could invalidate. Defer until at least two read-shaped error sources need the same dispatch. |
| Extract `stripBOM(raw) []byte` / `normalizeLineEndings(raw) []byte` / `validateUTF8(buf, path) (*ReadError, ok)` helpers from `Read` | depth | Speculative | All three would be private package functions called exactly once from `Read`. Deletion test: inlining them back into `Read` does not surface complexity anywhere else — they would be pass-throughs. The three phases are already documented as a unit in the package docstring; tests pin each phase. Extracting splits the contract without adding leverage. |
| Rename `Read` to `ReadAndNormalize` (or similar) to telegraph the three-phase transform | naming | Speculative | The function name `read.Read` is idiomatic Go (`pkg.Verb`), and the package docstring already documents the transform. The CONTEXT.md "Input handling" entry describes the read stage as a single verb ("reads the whole document into memory" etc.). Renaming is stylistic only. |
| Fold the two `fmt.Fprintf` branches in cli.Run (typed vs untyped read error) into a single in-cli `formatReadError(err, pathToken) string` helper | locality | Speculative | Saves ~3 lines, same package, no new seam. Not a depth win — the two branches are visibly distinct (typed → use the embedded position; untyped → use the `:0:0:` sentinel from CONTEXT.md "Error format"). Inline form actually reads as a one-to-one rendering of the glossary rule. |
| Surface `Offset` semantics ("post-normalize") via a method like `(*ReadError).OffsetInNormalized() int` | naming/glossary | Speculative | Pre-deepening. The package docstring already explains the semantics; no consumer needs the disambiguation yet. CONTEXT.md "Position info" already says `offset` is into the normalized doc, so any downstream consumer that reads CONTEXT.md gets the same answer. |
| Hoist `utf8BOM` into a public `read.UTF8BOM` constant | depth | Speculative | No external consumer. Private suffices. |

**Deletion test on `internal/read`:** if removed and inlined into `internal/cli`, the unit tests (`TestLeadingBOMIsStripped`, `TestCRLFNormalizedToLF`, `TestBareCRNormalizedToLF`, `TestByteLengthReflectsBothTransforms`, `TestBOMShapedMidDocumentIsLeftAlone`, `TestNoBOMLFOnlyRoundTripsByteForByte`, `TestInvalidUTF8ReturnsTypedReadError`, `TestLeadingInvalidByteAtOriginPosition`) would lose their seam — every transform property currently testable in milliseconds against `bytes.NewReader` would have to go through the cli's full `Run(argv, stdin, stdout, stderr) int` shape, adding boilerplate and slowing the inner loop. Eight real adapters point at this module's interface (the eight unit tests) plus one indirect adapter (cli.Run). Real seam. Keep.

**Deletion test on `(*ReadError).Error()`:** if removed and inlined as a plain struct, cli.Run would lose the `re.Error()` call and have to format `"%s:%d:%d: %s"` itself — duplicating ADR-0001's stderr-line shape across modules. The `Error()` method localizes the format to the type that owns the data. Real seam. Keep.

**Module depth assessment of `Read` itself:** the function's interface (`Read(r io.Reader, path string) ([]byte, error)` with a typed `*ReadError`) is genuinely smaller than its implementation (three-phase pipeline + position tracking). One verb, three transforms. Callers do not need to compose `stripBOM`/`normalizeLineEndings`/`validateUTF8` separately — per ADR-0001 they are always-together. This is the canonical "deep module" shape. Don't fragment it.

### Naming clarity

- `raw` (pre-transform buffer) → `norm` (post-transform buffer) — clear directionality.
- `pathToken` in cli.go matches read.go's docstring vocabulary ("the `<path>` token used in stderr error lines").
- `emptyEnvelope` matches CONTEXT.md "JSON envelope" + "v1 ship criterion."
- `src` (line 134) names the input source generically; no glossary term competes.
- `opts.filePath` / `opts.hasPositional` already covered in S01 arch-log — match CONTEXT.md "CLI contract" ("positional FILE", stdin sentinel `-`).

No name in scope is overloaded, misleading, or competing with a glossary entry.

### Locality

The S02 surface concentrates change-related code well:
- All UTF-8 / BOM / CRLF logic lives in one file (read.go); changing the BOM rule or the CR-normalization rule is a one-file edit.
- The error format string template (`%s:%d:%d: %s`) lives on `(*ReadError).Error()` — one place — and the tool-name prefix lives in cli.Run, where the tool's identity is already established.
- Position-tracking arithmetic (line/col walks) lives next to the validator, not duplicated anywhere.

No scatter found.

## Passes applied

**No strong candidates this pass.** Every refactor lens lights up either green (already glossary-aligned, deep enough, clearly named, well-localized) or yellow (worth-exploring once a second consumer arrives in S03+). Speculative changes — extracting phase helpers, renaming `Read` to a more descriptive verb, or pre-emptively unifying error formatting — would either churn the slice plan or fragment a contract that is currently a single coherent verb.

Per the Proposer-Arch contract: "If there are no Strong candidates, write an arch-log noting 'no strong candidates this pass' and emit `VERDICT: accept` — that is a legitimate acceptable outcome."

## Final

- Tests: 30 leaf passing (unchanged from baseline). `go vet ./...` still clean.
- LOC delta: 0 (no code changes).
- Most consequential observation (carried forward, not acted on): the cli.Run error-routing branches (typed *ReadError vs fallthrough) are the natural seam where an upcoming Stage's parse-error / frontmatter-error types will plug in. When S03+ introduces a second source of typed errors that follow CONTEXT.md's `<path>:<line>:<col>: <msg>` shape, a `read.FormatError` / `errfmt.Format` helper (whichever package the second consumer lives in) becomes a Strong candidate — at that point there will be two adapters, a real seam, and a depth win in collapsing the dispatch. For S02, with one adapter, the inline form is more honest.

VERDICT: accept
