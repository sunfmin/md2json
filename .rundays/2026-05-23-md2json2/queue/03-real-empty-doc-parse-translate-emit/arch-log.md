# Arch log: 03-real-empty-doc-parse-translate-emit

Started: 2026-05-23T08:30:00Z
Scope: mini
File set (from S03 tdd-log):
- `internal/parse/parse.go` (new)
- `internal/translate/translate.go` (new)
- `internal/translate/translate_test.go` (new)
- `internal/emit/emit.go` (new)
- `internal/cli/cli.go` (rewired)
- `internal/cli/cli_test.go` (modified — added S03 tests + re-pinned S01 tests)
- `anti_globals_test.go` (new module-root test)
- new fixtures `testdata/fixtures/04-bom-only-stdin-nopos/` through `11-empty-doc-frontmatter-only/`

## Baseline

- `go test ./...`: all green; 0 failing.
  - root integration package: 13 leaf (`TestNoProcessGlobalsOutsideMain`, `TestHarnessDetectsSingleByteStdoutDiff`, `TestFixtures` + 11 fixture subtests).
  - `internal/cli` package: 24 leaf:
    - 9 top-level singletons: `TestEmptyStdinNoPositionEmitsEnvelope`, `TestEmptyStdinDefaultEmitsEnvelopeWithPosition`, `TestPositionalFileEmitsEnvelope`, `TestMissingPositionalFileObservable`, `TestStdinSentinelDashBehavesLikeNoPositional`, `TestFrontmatterOnlyEmptyDocEmitsNull`, `TestLeadingInvalidUTF8StdinHardErrors`, `TestMidDocumentInvalidUTF8StdinHardErrors`, + (S01 test cluster wrappers).
    - 4 subtests under `TestHelpAndVersionExitZero`.
    - 2 subtests under `TestUnknownFlagExitsNonZero`.
    - 6 subtests under `TestKnownFlagsRecognizedAsNoop`.
  - `internal/read` package: 8 leaf (unchanged from S02).
  - `internal/translate` package: 2 leaf (`TestTranslateEmptyDocProducesGoValueTreeNotGoldmarkNode`, `TestTranslateSingleNewlineRootPosition`).
  - `internal/emit` package: no test files (per tdd-log; emit is exercised indirectly via cli + fixture tests).
  - `internal/parse` package: no test files (per tdd-log; parse is exercised indirectly via cli + fixture tests).
- Total leaf PASS lines reported by `-v`: 47 (parent groups counted once + their subtests, plus singletons; net no failures).
- `go vet ./...`: clean.
- LOC (in-scope source files only): `parse.go` 85, `translate.go` 127, `emit.go` 162, `cli.go` 191, `anti_globals_test.go` 103, `translate_test.go` 78, `cli_test.go` 252. Total source+test ~998 LOC.

## Exploration / candidates considered

Reviewed against the four refactor lenses (glossary alignment / module depth / naming clarity / locality), using CONTEXT.md, ADR-0001, ADR-0002, and the prior arch-logs (S01, S02) as the reference frame.

### Glossary alignment scan

- `Markdown`/`Frontmatter`/`AST (output) / mdast`/`Position info`/`JSON envelope` — every CONTEXT.md term is honored. `parse.New()` registers exactly the GFM + Footnote + YAML-only frontmatter extensions ADR-0002 pins. The mdast key ordering in `emit.writeNode` (type → type-specific → children → position) matches CONTEXT.md "JSON envelope" verbatim.
- `parse.Result.Frontmatter` is `any`, matching the CONTEXT.md "Frontmatter" entry's note that YAML scalars AND YAML maps flow through.
- `translate.Node{Type, Children, Position}` matches mdast. `Point{Line, Column, Offset}` matches mdast's `start`/`end` Point shape. The S01/S02 arch-logs already confirmed `_Avoid_` terms (`stdin`/`<stdin>`, "goldmark AST", "Markdown" unqualified, "input mode") are absent; S03's new code does not regress on this.
- Lossiness policy ("silent drop", no `unknown` node, no `html` fallback for non-HTML constructs) — at S03 only `root` is translated so the policy is vacuously honored. No `_Avoid_` shape (`unknown` type node, etc.) appears in `translate.go`.
- The package docstring on `translate.go` says: *"Output type: a Go value tree (pointers to `Node` structs), NOT goldmark's native AST. This is the structural guarantee acceptance criterion #4 pins: after translate, the rest of the pipeline never sees goldmark types."* This is **accurate**: the goldmark seam lives between parse and translate (parse produces `*ast.Document`, translate consumes it); after translate, only `*translate.Node` flows. The S03 implementation matches the docstring contract.

No glossary miss. No `trigger-grill` warranted.

### Module depth analysis

| Candidate | Lens | Score | Notes |
|---|---|---|---|
| Encapsulate goldmark types inside `parse` (don't expose `*ast.Document` on `Result.Doc`) | depth/leak | Worth exploring | Argument for: translate.go's docstring says "after translate, the rest of the pipeline never sees goldmark types"; today cli.Run DOES pass `pr.Doc` to `translate.Translate(pr.Doc, ...)`, so cli briefly handles a goldmark pointer. Argument against: this is intentional — goldmark is the seam between parse (owns parser config) and translate (owns goldmark→mdast mapping). cli is a pass-through, never destructures the type. Two adapters (parse-internal + translate-input), real seam — but the seam shape is "goldmark Document," not an md2json2-owned interface. Inverting the dependency (making parse depend on translate, or defining a `parse.AST` interface that wraps goldmark) is **speculative deepening**: no second parser implementation is anticipated. Defer. |
| Drop `translate.Options{}` (currently empty + docstring placeholder) | depth | Speculative | The struct's docstring telegraphs "knobs go here in later slices." Removing now and adding back later is churn-symmetric; the empty struct does no harm and signals intent. Leave. |
| Split `emit.writeNode` / `writePosition` / `writePoint` further | depth | Speculative | Already at the right granularity: each function emits exactly one JSON shape, callers compose them. Deletion test: inlining produces a 50-line single function harder to scan. Keep current split. |
| Move the goldmark→mdast call into `parse.Result.Translate(src, opts)` | depth | Speculative | Inverts the parse→translate dependency direction; worse coupling. Reject. |
| Hoist the zero-width root position (`{1,1,0}-{1,1,0}`) into a `translate.zeroWidthRootPosition` constant or helper | locality | Speculative | The literal exists in exactly ONE place (`translate.rootPosition` lines 91–96). No duplication. A named constant would add indirection without removing repetition. |

**Deletion test on `internal/parse`:** if removed and inlined into `cli` or `translate`, the goldmark extension registration (`extension.GFM`, `extension.Footnote`, `frontmatter.Extender` with `Formats: YAML`) plus the parser.Context wiring + frontmatter.Get decode would either duplicate across two consumers or balloon a downstream module with a parser-library concern that ADR-0002 explicitly puts inside `parse`. Real seam. Keep.

**Deletion test on `internal/translate`:** if removed, the goldmark→mdast mapping (currently just root + rootPosition) would either move into `parse` (mixing parser config with schema translation) or `emit` (mixing wire-shape with translation logic). Both are worse — the mapping IS the schema-conformance boundary. Real seam. Keep.

**Deletion test on `internal/emit`:** if removed, the JSON key-order rule (`type` → type-specific → `children` → `position`) would either go into `translate` (mixing wire concerns with mdast tree construction) or `cli` (mixing IO+format concerns with composition). The emit module localizes "how mdast becomes JSON bytes." Real seam. Keep.

### Naming clarity

- `parse.Result.Doc` field — generic name; the **type** `*ast.Document` qualifies it as a goldmark doc. A reader has to glance at the type. Alternative: `GoldmarkDoc`. Marginal; the `parse` package's docstring already opens with "wraps github.com/yuin/goldmark", so the type origin is obvious from package context. Speculative.
- `translate.Node` vs goldmark's `ast.Node` — both named `Node`, in different packages. The test file `translate_test.go` deliberately contrasts them. Go's package qualification (`translate.Node` vs `ast.Node`) makes the distinction safe at the language level. No action.
- `translate.Point` and `translate.Position` — common type names; package-qualified uses (`translate.Point`, `translate.Position`) in `emit.go` are unambiguous. ✓
- `pr` (parse result) in cli — single letter, locally scoped, OK.
- `eopts` (emit options) in cli — locally scoped, OK.

No name in scope is overloaded, misleading, or competing with a glossary entry.

### Locality

- **The exact empty-doc envelope JSON literal** lives in `internal/cli/cli_test.go` as `wantEnvelopeDefault` and `wantEnvelopeNoPosition`. That's the **test-side** spec for the v1 ship criterion. The **production-side** components are split across `emit.go` (envelope shape) and `translate.go` (root position). Good split: production code does not hardcode the envelope string; the test pins the wire result.
- **The empty-doc default `position` value** lives in `translate.rootPosition` as the inline `Point` literal `{Line: 1, Column: 1, Offset: 0}` × 2 — ONE place, no duplication.
- **The GFM + footnote + YAML-frontmatter extension registration** lives in `parse.New()` — ONE place. ADR-0002 references `parse.New` by name. ✓
- **The JSON envelope key-order rule** (`type` → type-specific → `children` → `position`) lives in `emit.writeNode` only. ✓
- **STRONG CANDIDATE — error format duplication in `cli.Run`:** the document-scoped error template `"md2json2: %s:0:0: %s\n"` is repeated at **four** call sites in `cli.Run` (the read-fallthrough fallback line 153, the parse error line 166, the frontmatter-only emit error line 176, and the default emit error line 186). All four take the same two values (`pathToken`, `err.Error()`) and render the same canonical document-scoped error sentinel. CONTEXT.md "Error format" pins this template; today it lives in 4 places in one file. Per S02 arch-log: *"once there's a second source of typed errors that follow CONTEXT.md's `<path>:<line>:<col>: <msg>` shape, a `read.FormatError` / `errfmt.Format` helper… becomes a Strong candidate."* S03 added three new write-points of the same template, all in `cli.Run`, all rendering the same `:0:0:` sentinel — that's the consummation of S02's deferred candidate.
  - Two adapters (originally): one was the typed-ReadError path, one was the untyped fallback.
  - Four adapters (now): each error site below the typed-ReadError dispatch.
  - **Real seam**, four real call sites → centralize into a private helper in `cli.go`.

### ADR cross-check

- ADR-0001 (input encoding) — entirely about `read.go`, not changed by S03. The cli passes the normalized bytes to parse exactly as ADR-0001 prescribes. ✓
- ADR-0002 (goldmark extensions) — explicitly references `parse.New()` as the single function that owns the enabled-extension set, and the YAML-only frontmatter pin. Both honored in `parse.go` lines 30–44. ✓ No ADR conflict.

## Passes applied

### Pass 1: centralize the document-scoped error format string in `cli.go`

- **Lens hit**: locality (one place owns the `:0:0:` document-scoped error sentinel, not four).
- **Files touched**: `internal/cli/cli.go`.
- **Change**: introduce a private helper `writeDocScopedError(stderr io.Writer, pathToken string, err error)` that wraps the `fmt.Fprintf(stderr, "md2json2: %s:0:0: %s\n", pathToken, err.Error())` template. Replace all four inline `fmt.Fprintf` call sites with the helper.
- **Why this is a depth win, not just style**:
  - The `:0:0:` sentinel is a **CONTEXT.md-pinned shape** (Error format entry: *"When the error is document-scoped with no position at all, use the sentinel `<path>:0:0:` — the same regex still matches and `0:0` conventionally means 'no position available.'"*). Four inline renderings let a future maintainer change one and miss the others. One helper localizes the rule.
  - When a future slice (e.g., S09 invalid-frontmatter) introduces a *typed* parse error with real line/col info, the dispatch will look like the read path — `errors.As` → use embedded position. Today the read path lives at lines 148–155 of cli.go and the other three paths flow into `:0:0:`. After this refactor, **all four** non-read sites call the same helper, so the future S09 addition has a single insertion point ("add an `errors.As` branch inside `writeDocScopedError`") rather than four parallel edits.
  - This is a deletion test win: if the helper is inlined back, the same four lines appear at four sites, and the structural similarity (same template, same args) is hidden.
- **Tests after**: `go test ./...` — all green, same count.
- **Reverted?** No.

### Pass 2 (considered, not applied): rename `parse.Result.Doc` → `parse.Result.GoldmarkDoc`

- **Lens**: naming/clarity — make explicit that crossing this seam exposes a goldmark type.
- **Why deferred**: Two adapters today (parse-internal + `cli.Run` → `translate.Translate`). The `parse` package docstring already opens with "wraps github.com/yuin/goldmark"; the type `*ast.Document` is visible to anyone who reads `Result`'s definition. Adding "Goldmark" to the field name leaks the parser library identifier into the field's wire-side name (the field is exported), reversing the encapsulation direction we want long-term. **Speculative**, not Strong. Leave.

### Pass 3 (considered, not applied): hoist zero-width root position to a constant

- **Lens**: locality.
- **Why deferred**: the literal exists in exactly ONE place (`translate.rootPosition` line 91–96). No duplication. A named constant would add indirection without removing repetition.

## Final

- Tests: `go test ./...` — all green (47 PASS lines including parent-test wrappers, unchanged from baseline). `go vet ./...` clean.
- LOC delta: `internal/cli/cli.go` net +5 lines (helper added at the bottom of the file, four 1-line call sites replace four 1-line `fmt.Fprintf` sites).
- Most consequential change: the `:0:0:` document-scoped error sentinel now has a single owner in `cli.go` (the `writeDocScopedError` helper). This concentrates the CONTEXT.md "Error format" rendering rule into one place and creates the structural slot where S09's typed-frontmatter-error dispatch will plug in (an `errors.As` branch inside the helper) without spreading the change across four call sites.

VERDICT: accept
