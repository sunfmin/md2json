# Arch log: mini

Started: 2026-05-23
Scope: mini
File set (from tdd-log "Files touched"):
- internal/translate/translate.go (extended)
- internal/translate/position.go (new)
- internal/translate/translate_test.go (extended)
- internal/emit/emit.go (extended)
- internal/cli/cli_test.go (one test re-pinned)
- 7 new fixtures under testdata/fixtures/ (12..18)

## Baseline

- `go test ./...`: 27 top-level tests, all PASS, 0 FAIL
  (1 in repo-root anti_globals_test.go, 1 in integration_test.go's harness self-test,
   1 TestFixtures with 18 subtests, ~12 in internal/cli, ~8 in internal/read,
   4 in internal/translate)
- `go vet ./...`: clean (per tdd-log).
- Total LOC in scope: 174 (translate_test.go) + 298 (translate.go) + 80 (position.go) + 193 (emit.go) = 745 lines; cli_test.go untouched at the arch lens.

## Candidate inventory

Reviewed against the four lenses (CONTEXT.md glossary alignment, module depth,
naming clarity, locality):

### C1: Extract `internal/mdast` package for the Node value-tree types
- Score: **Speculative**
- Rationale: deletion test fails. `Node`/`Position`/`Point` have one consumer
  besides translate (emit), so this is one adapter — a hypothetical seam, not a
  real one. The struct definitions are small (~60 lines including comments) and
  consolidate the glossary-faithful field naming (`Depth`, `Value`, `Children`)
  in the place that owns the goldmark→mdast mapping. Moving them out adds an
  import-path level without consolidating logic. Re-evaluate if/when a third
  consumer materialises (e.g. a future `mdjson_test` external package that
  wants to construct Node trees by hand).

### C2: Replace the `Node` discriminated-union struct with per-type subtypes
- Score: **Speculative**
- Rationale: the tdd-log refactor pass already weighed and rejected this. The
  single-struct + tag-by-Type shape is the simpler emit form, and per-type
  subtypes would require either reflection or a visitor in the JSON writer,
  both of which violate the "compact, key-ordered, no-allocs-in-hot-path"
  emit contract. mdast itself is a discriminated union by `type`; the struct
  mirrors that wire shape directly. Not refactor-on-style-alone material.

### C3: Per-node-type logic spread across translate dispatch AND emit switch
- Score: **Worth exploring (not acted on)**
- Rationale: adding a new mdast node type (e.g. `list`) means three edits:
  `translateNode` dispatch case, new `translateList()` function, and an
  `emit.writeNode` switch case + possible `isContainer` update. This is
  expected for compile-checked exhaustiveness across two stages of the
  pipeline; the alternative (a registry-of-handlers map per node type) would
  trade compile-time safety for runtime indirection. Leave as-is until a
  concrete win materialises (e.g. when ~15 node types in, the switch becomes
  unwieldy).

### C4: Field naming vs CONTEXT.md "mdast node-set v1" glossary
- Score: not a candidate (already aligned)
- Rationale: `Node.Depth` matches `heading{depth}`; `Node.Value` matches the
  `value`-bearing nodes (`text`, `inlineCode`, `code`, `html`); `Children`
  matches mdast's `children`; `Position{Start,End}` and `Point{Line,Column,Offset}`
  match the `position: {start: {line, column, offset}, end: ...}` shape verbatim.
  No `_Avoid_` synonyms in use. The internal helper names (`positionTracker`,
  `blockOffsets`, `paragraphOffsets`, `lineStartOffset`, `childrenSpan`,
  `endPosition`, `rootPosition`) are descriptive, not in the glossary, and
  not naming-clashing.

### C5: `endPosition`/`rootPosition` duplicate position-tracking logic
- Score: **Strong**
- Rationale: `endPosition` in translate.go walks `src` byte-by-byte counting
  lines and UTF-8-code-point columns to compute the end-of-document Point.
  `positionTracker.point(offset)` in position.go does the same job — for any
  offset, including `len(src)` — by precomputing line starts and walking the
  containing line for column count. The UTF-8 continuation-byte skip rule
  (`b&0xC0 == 0x80`) is the load-bearing CONTEXT.md "Position info" rule, and
  it lives in two places. The tdd-log refactor pass explicitly considered this
  and rejected it citing "risk regressing the empty-doc and single-newline
  boundary cases that rootPosition bakes in explicitly," but the math is
  trivially equivalent:
  - Empty doc (`len(src)==0`): rootPosition early-returns `{1,1,0}-{1,1,0}`.
    Equivalently, `positionTracker` built from `[]byte{}` has
    `lineStarts=[0]`; `pt.point(0)` returns `{Line:1, Column:1, Offset:0}`.
  - Single-newline doc (`src=="\n"`): rootPosition computes via endPosition
    `{line:2, column:1, offset:1}`. Equivalently, `lineStarts=[0,1]`;
    `pt.point(1)` → `lastIndexLE([0,1],1)=1` → lineStart=1 → no column-walk
    iterations → `{Line:2, Column:1, Offset:1}`.
  Deleting `endPosition` (and the special-case branch in `rootPosition`)
  removes ~20 lines and centralises the UTF-8 column-counting rule in one
  place. Apply.

## Pass 1: Centralise end-of-document Point computation in positionTracker

- Files touched: internal/translate/translate.go
- Diff summary:
  - Removed `endPosition(src []byte) (line, col, offset int)` helper (16 lines
    incl. doc comment).
  - Replaced `rootPosition(doc *ast.Document, src []byte)` with
    `rootPosition(pt *positionTracker)` that simply returns
    `pt.position(0, len(pt.src))`. Translate's call site now passes the
    tracker it already built. Both the empty-doc baseline (`{1,1,0}-{1,1,0}`)
    and the single-newline boundary (`{1,1,0}-{2,1,1}`) fall out of
    `pt.point` arithmetic without any special-case branch.
- Tests after: 27 PASS, 0 FAIL (`go test ./...`), `go vet ./...` clean.
- Reverted? no.

## Final

- Tests: 27 passing (unchanged from baseline), 0 failing.
- LOC delta: -26 in internal/translate/translate.go (298 → 272). Removed the
  `endPosition` helper, its doc comment, and the special-case branch + the
  unused `doc *ast.Document` parameter from `rootPosition`. No other files
  touched.
- Most consequential change: the UTF-8 column-counting rule from CONTEXT.md
  "Position info" (lines/columns 1-indexed, column counts UTF-8 code points,
  offset is byte offset into normalized source) now lives in exactly one
  function, `positionTracker.point()`. Previously the same rule was encoded
  in two functions (`endPosition` walking the whole source linearly, and
  `point` walking from a binary-searched line start). The Start/End points of
  the root node are now derived through the same Point-derivation function
  that every other node uses, eliminating a class of "what if the two
  implementations of the column-count rule drift apart" bugs and making
  CONTEXT.md "Position info" map to a single code location.

VERDICT: accept
