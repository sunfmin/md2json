# Arch log: mini

Started: 2026-05-23
Scope: mini
File set (from S07 tdd-log "Files touched"):
- internal/translate/translate.go (extended: 5 new dispatch cases — Table,
  TableHeader, TableRow, TableCell, Strikethrough; AutoLink case; new
  Align field on Node; new translators translateTable / translateTableRow /
  translateTableCell / translateAutoLink / translateStrikethrough; new
  helpers alignmentsToMdast, extractTaskCheckboxChecked; translateListItem
  rewired to hoist task-checkbox.checked)
- internal/translate/translate_test.go (5 new unit tests anchoring S07
  load-bearing rules)
- internal/emit/emit.go (extended writeNode with `case "table"`; new
  writeJSONNullableStringSlice helper)
- testdata/fixtures/34..38 (five new fixtures)

## Baseline

- Tests: 39 passing, 0 failing across all packages (`go test ./... -count=1`).
- `go vet ./...`: clean.
- LOC in scope: translate.go 959, emit.go 342, translate_test.go 628.

## Candidate inventory + scores

Reading CONTEXT.md, ADR-0001 + ADR-0002, prior arch-logs S05+S06, the S07
tdd-log, and the three S07 source files; applying CONTEXT.md "mdast
node-set v1" as the naming lens and the four refactor lenses (glossary
alignment, module depth, naming clarity, locality). Re-confirming or
re-scoring the candidates earlier arch-logs already weighed, plus the
S07-specific candidates.

### C1 — `translateNode` dispatch (~22 case labels / 21 distinct branches). Score: NOT-STRONG.

S05's re-evaluation threshold was ~15 cases; S06 reached 16 cases; S07
adds 6 case labels (AutoLink, Table, TableHeader, TableRow, TableCell,
Strikethrough). The switch is now at ~22 case labels (TableHeader and
TableRow share `translateTableRow`, so 21 distinct branches).

Applying the deletion test rigorously on a registry-of-handlers approach
(`map[reflect.Type]func(ast.Node, []byte, *positionTracker) *Node`):
- Each handler would need a redundant `n.(*ast.Heading)` cast at the top
  to recover the static type the switch currently gives for free —
  trading compile-time exhaustiveness for runtime indirection.
- The two-row case (TableHeader and TableRow → same function) is the
  ONLY case that fits a map naturally; everywhere else the map is pure
  indirection.
- Map-init would require either `init()` registration (action-at-a-
  distance, hostile to navigability) OR a one-shot reflect.TypeOf table
  in the package init path (heavier than the switch).
- A goldmark version bump that adds a new Node kind currently surfaces
  at build time as a missing case in the exhaustive switch (when paired
  with a vet-style linter) or silently passes through to the default
  arm. The map dispatch loses even the linter signal.

Deletion test result: replace switch with map → file is the same length
or longer, indirection is added, compile-time signal is lost. Reject.
Re-evaluate around ~30 cases or when the dispatch genuinely becomes a
multi-edit grind beyond "add case + add func". S08's footnotes slice
adds 2 more (FootnoteRef, FootnoteDef); reasonable to revisit at S08
arch if the count crosses 25.

### C2 — Split translate.go into block/inline files within the package. Score: NOT-STRONG.

translate.go is now 959 LOC (S05: 447, S06: 712, S07: 959 — roughly
+250 per slice as mdast types come in; this growth is the type-count
itself, not a quality signal).

Candidate split:
- `translate.go` — Translate, Options, Node, translateNode dispatch,
  translateChildren, position helpers (childrenSpan, etc.).
- `translate_block.go` — translateHeading, translateParagraph,
  translateList, translateListItem, translateTextBlock, translateBlockquote,
  translateThematicBreak, translateFencedCodeBlock, translateCodeBlock,
  translateHTMLBlock, translateTable, translateTableRow, translateTableCell.
- `translate_inline.go` — translateText, translateEmphasis, translateRawHTML,
  translateCodeSpan, translateLink, translateImage, translateAutoLink,
  translateStrikethrough.

Locality argument AGAINST: the dispatch switch IS the package index ("which
goldmark type → which translator"). Readers using the switch as a map of
"go here to read the translator" benefit from all translators being a
PageDown away in the same file. Splitting them across two files weakens
that property — the dispatch table loses its role as a navigation aid
because each case-line now mentions a function that lives elsewhere.

Functions average ~20 LOC each (the LOC driver is the per-function doc
comments — substantial and load-bearing, not bloat to fix). The file is
linearly organized: dispatch → block translators → inline translators →
helpers. That linear order already does the same separation that a file
split would, without losing the package-index property of the switch.

Re-evaluate at ~1300 LOC or when scrolling the dispatch table requires
PageDown more than once. Not Strong this pass.

### C3 — Extract `spanWithDelimiter(children, delim, srcLen)` from `translateEmphasis` + `translateStrikethrough`. Score: STRONG.

The S07 tdd-log explicitly teed up this refactor in Test 3's "Refactor"
note: "the delimiter-extension idiom is identical to translateEmphasis's
… Will revisit during the broader S07 refactor pass at end of slice."

Both functions contain the same 6-line block:

```go
children := translateChildren(<n>, src, pt)
startOff, endOff := childrenSpan(children)
startOff -= <delim>
endOff   += <delim>
if startOff < 0      { startOff = 0 }
if endOff   > len(src) { endOff   = len(src) }
```

The two adapters differ only in the delim source — `e.Level` (1 or 2)
for emphasis/strong, the constant `2` for strikethrough. Same operation,
same shape, two real callers — past S05's "two adapters = real seam"
threshold (and same threshold S06 cited when extracting `textChildrenSpan`).

Deletion test: drop the helper, the 6-line block reappears twice. Future
GFM/extension inline nodes whose container has no segment of its own and
relies on the children-span+delimiter-extension trick would re-introduce
it a third time. (Goldmark's `BaseInline` with no segment is the
container shape that forces this idiom — Emphasis, Strikethrough, and
plausibly future inline extensions all share it.)

Locality + module depth: the "expand by N, clamp to source bounds"
arithmetic is concentrated behind a 1-line callsite, and the helper's
doc comment names the convention (and the goldmark-container-without-
segment property that forces this idiom). Sibling to the existing
`childrenSpan` (which gives the inner span) and `textChildrenSpan`
(S06's goldmark-walk flavor) — the three together name the position-
span vocabulary the inline family relies on:
- `childrenSpan(mdast children)` — span over already-translated mdast.
- `textChildrenSpan(goldmark parent)` — span over goldmark `*ast.Text` leaves.
- `spanWithDelimiter(mdast children, delim, srcLen)` — `childrenSpan`
  + clamped extension by the paired-delimiter width.

Acting on this one.

### C4 — `Align []*string` shape vs `[]string` with `""` sentinel. Score: ALREADY-ALIGNED.

S07 tdd-log weighed and chose `[]*string` matching S05/S06's "pointer-to-T
for nullable mdast fields" convention at the per-slot case. CONTEXT.md
`writeJSONNullableString` rule pins `""` as a distinct on-the-wire value
(not the null sentinel), so the slice-of-pointers shape is the right
mapping of the per-element rule. Not a candidate.

### C5 — `extractTaskCheckboxChecked` lives in translate.go. Score: NOT-STRONG.

Single use site (`translateListItem`). The doc comment carries the
load-bearing scoping rule ("only the first inline child of the first
container child" — matches what the goldmark tasklist parser itself
enforces; recursing further would catch stray `[x]`-shaped inline text
elsewhere in the item body, which is the bug-shape the helper exists to
prevent). Promotion to a shared util / standalone package would lose the
tight coupling to the calling translator (`*ast.ListItem`-specific) and
add ceremony for no locality gain. Defer.

### C6 — Naming consistency vs CONTEXT.md "mdast node-set v1" glossary. Score: ALREADY-ALIGNED.

Every new field on `Node` matches CONTEXT.md verbatim:
- `Align []*string` ↔ `table.align: ["left"|"right"|"center"|null, ...]`.
- `Checked *bool` hoist for task items ↔ `listItem.checked` true/false/null.
- New types `table`, `tableRow`, `tableCell`, `delete`, `link` (collapsed
  from `*ast.AutoLink`) — every string literal in `translateNode` and
  `writeNode` is one of CONTEXT.md's enumerated mdast types. No
  `_Avoid_` synonyms (no `strikethrough`, no `autolink`, no
  `goldmark*` term) appear on the wire or in field names.

### C7 — `writeNode` per-type switch (~11 cases at S07). Score: NOT-STRONG.

The switch is the canonical mdast-key-order spec, with `case "table"`
added at S07. Same argument as S06 C2: concentrating the per-type
key-order writers in one switch is exactly the locality we want;
splitting per type would create one-caller helpers. Reject.

### C8 — `writeJSONNullableStringSlice` lives in emit.go. Score: ALREADY-ALIGNED.

S07 added this helper next to the existing `writeJSONNullable{Int,Bool,String}`
trio. It delegates the per-slot rule to `writeJSONNullableString` so the
"null vs quoted string" rule remains in exactly one named place; the
slice helper only owns the bracketing-and-comma machinery. The doc
comment correctly names the "empty slice → `[]` not `null`" rule
(distinct from "field absent" or "all elements null") matching the
existing helpers' precedent.

## Pass 1: extract spanWithDelimiter from translateEmphasis + translateStrikethrough

- Files touched: internal/translate/translate.go
- Action:
  - Added helper `spanWithDelimiter(children []*Node, delim, srcLen int) (start, end int)`
    next to `childrenSpan`, with a doc comment naming the
    "paired-delimiter inline container without its own goldmark segment"
    convention.
  - Rewrote `translateEmphasis` to call `spanWithDelimiter(children, e.Level, len(src))`,
    dropping the inline 4-line expand-and-clamp block.
  - Rewrote `translateStrikethrough` to call `spanWithDelimiter(children, delim, len(src))`,
    dropping the inline 4-line expand-and-clamp block. The local
    `const delim = 2` survives at the callsite — it documents the `~~`-delimiter
    width inline, where the comment refers to the `~~` markers; pushing the
    constant into the helper would erase that documentation.
- Tests after: 39 PASS, 0 FAIL (`go test ./... -count=1`); `go vet ./...` clean.
- Reverted? no.

## Final

- Tests: 39 passing (unchanged from baseline), 0 failing. `go vet ./...` clean.
- LOC delta: translate.go 959 → 972 (+13). emit.go unchanged (342).
  The new helper plus its doc comment is ~22 lines; the two inline
  expand-and-clamp blocks shrank by ~8 lines each (16 total).
  Net is a small line increase — but the win is in NAMING and LOCALITY,
  not byte count: the "expand children-span by N, clamp to source bounds"
  arithmetic now has a named function whose doc comment also explains
  WHY the idiom exists (goldmark BaseInline containers without their
  own segment accessor must reconstruct their span from children +
  delimiter width). Same shape as S06's `textChildrenSpan` extraction.
- Most consequential change: completed the position-span vocabulary
  for the inline-translator family. The three named helpers
  (`childrenSpan` / `textChildrenSpan` / `spanWithDelimiter`) now
  cover the three flavors of "compute a source span from descendants"
  that translate needs: union-over-mdast-children (post-translate),
  union-over-goldmark-text-leaves (pre-translate, leaf positioning),
  and union-over-mdast-children-plus-delimiter-extension (paired
  delimiter inline containers). The next inline GFM extension that
  joins the paired-delimiter family — e.g. footnote references' `[^id]`
  shape if their span ever needs the same trick — inherits the named
  seam rather than re-introducing the 6-line arithmetic.

VERDICT: accept
