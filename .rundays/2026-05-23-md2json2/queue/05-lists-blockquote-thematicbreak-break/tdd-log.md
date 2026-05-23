# TDD log: 05-lists-blockquote-thematicbreak-break

Started: 2026-05-23

## Setup

- Reuses S01-S04's test framework (Go `testing` + `go test`, plus the
  black-box fixture harness at `integration_test.go`). No new dependencies.
- Slice goal: extend `translate` to walk goldmark's remaining basic
  block-level nodes — `*ast.List`, `*ast.ListItem`, `*ast.TextBlock`
  (goldmark's tight-list inline wrapper, mapped to mdast `paragraph`),
  `*ast.Blockquote`, `*ast.ThematicBreak` — plus the inline hard-break
  derived from `*ast.Text.HardLineBreak()`. Extend the `Node` struct with
  the load-bearing nullable fields `Start *int` and `Checked *bool` (plus
  plain `Ordered bool` and `Spread bool`) so the Go value tree can
  distinguish `start: null`/`checked: null` from "field omitted" and from
  zero values. Extend `emit.writeNode`'s per-type switch with the new
  canonical-key-order slots: list (ordered, start, spread), listItem
  (spread, checked). Extend `isContainer` to mark `thematicBreak` and
  `break` as leaves (no `children` array on the wire).
- Goldmark fact-finding probe (built and deleted before commit, in a
  scratch `cmd/_probe/` directory):
  - `*ast.List` exposes `IsOrdered() bool`, `Start int` (0 for unordered,
    the explicit start number for ordered), `Marker byte`, `IsTight bool`.
    `Lines()` returns nil/empty on List — the per-list source span is not
    stored; the span has to be derived from children.
  - `*ast.ListItem` similarly does not expose `Lines()`; derive span from
    children. The tight-list inline content is wrapped in a `*ast.TextBlock`
    child, not a `*ast.Paragraph` — this is the key reason the dispatch
    needs a TextBlock case that emits mdast `paragraph` (mdast has no
    TextBlock type).
  - `*ast.Blockquote` does not expose `Lines()` either; same childrenSpan
    pattern.
  - `*ast.ThematicBreak` exposes neither a Segment nor Lines(); the source
    span is genuinely not recoverable from the goldmark AST alone. For
    S05 (where every fixture uses `--no-position`) we attach a zero-width
    placeholder position and revisit in S10 (position-info pinning) if a
    default-mode fixture needs the marker-line span.
  - Hard line breaks are NOT a separate goldmark node. They are encoded
    as a `textHardLineBreak` flag on the *preceding* `*ast.Text` node,
    queryable via `t.HardLineBreak() bool`. The two trailing spaces (or
    the trailing `\`) are NOT in the Text segment — goldmark already
    consumed them as the break boundary. Probe confirmed:
    `"line1  \nline2\n"` → `Text(seg=0:5,"line1",hardBR=true)` then
    `Text(seg=8:13,"line2",hardBR=false)`; `"line1\\\nline2\n"` →
    `Text(seg=0:5,"line1",hardBR=true)` then `Text(seg=7:12,"line2")`.
    The bytes between the two segments (the consumed two-space-and-LF
    or backslash-and-LF run) are what the synthetic mdast `break` node
    spans.
  - `---\n` on its own line at document start is consumed by the
    goldmark-frontmatter extension as an opening fence with no closing
    fence; the document parses with `Frontmatter == nil` and zero
    children (per CONTEXT.md "Frontmatter / Unclosed-fence rule").
    To exercise the `---` → `thematicBreak` mapping unambiguously, the
    fixture uses `before\n\n---\n\nafter\n` (where the `---` cannot be a
    frontmatter opening fence because non-frontmatter content precedes
    it). The `***\n` alternative thematic-break syntax also works but the
    issue's acceptance criterion specifically calls out `---`, so we use
    that.
- Shape decision: the issue raised the `Start *int` / `Checked *bool`
  nullable-field shape question. We applied S04's existing `Value` /
  `ValuePresent` precedent: a single-struct `Node` with optional fields,
  where the optional-ness is expressed via Go pointer types for nullable
  mdast fields. The emit-side per-type switch is the single decision point
  for serialization order and `null` vs `false`/`0` rendering. Considered
  but rejected: an interface-per-mdast-type design (would force either
  reflection in emit or a visitor pattern; both lose the "compact,
  key-ordered, no-allocs-in-hot-path" emit contract). The pointer-typed
  fields are the minimum mechanism to express JSON `null` distinctly from
  the zero value of the underlying type. S04's `ValuePresent bool`
  precedent is the boolean-flag flavor of the same pattern; S05 extends
  it to genuine pointer-nullable.

## Test 1 — tracer bullet: `- a\n- b` (criterion #1, unordered list)

- Wrote: fixture `testdata/fixtures/19-unordered-list-nopos/` with
  `args=--no-position`, `input.md="- a\n- b\n"`, `exit=0`, `stderr`
  empty, and expected `stdout` byte-exact:
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"list","ordered":false,"start":null,"spread":false,"children":[{"type":"listItem","spread":false,"checked":null,"children":[{"type":"paragraph","children":[{"type":"text","value":"a"}]}]},{"type":"listItem","spread":false,"checked":null,"children":[{"type":"paragraph","children":[{"type":"text","value":"b"}]}]}]}]}}`
- Red: `go test -run 'TestFixtures/19-' .` failed — stdout was the empty
  envelope `{"frontmatter":null,"ast":{"type":"root","children":[]}}`
  because translate at S04 silent-dropped `*ast.List`, `*ast.ListItem`,
  and `*ast.TextBlock` per CONTEXT.md "Lossiness policy". This is the
  tracer — it forces the entire List/ListItem/TextBlock/paragraph chain
  AND the new Node-shape fields (Ordered, Start, Spread, Checked) AND
  the new emit per-type slots (list, listItem) into existence in one go.
- Green: extended `internal/translate/translate.go`:
  - `Node` struct gained `Ordered bool`, `Start *int`, `Spread bool`,
    `Checked *bool` fields. Documented the rationale inline: pointer
    types for nullable mdast fields, applied S04's `ValuePresent`
    precedent.
  - `translateNode` switch gained five new cases: `*ast.List`,
    `*ast.ListItem`, `*ast.TextBlock`, `*ast.Blockquote`,
    `*ast.ThematicBreak`.
  - `translateList`: maps `IsOrdered()` → `Ordered`, `Start` → `*int`
    (nil for unordered, `*l.Start` for ordered), `!IsTight` → `Spread`.
    Position via `childrenSpan` (goldmark does not store a per-list
    segment).
  - `translateListItem`: `Spread: false` (tight default — S07 may
    revisit per-item spread when the loose-list-rendering edge case
    bites), `Checked: nil` (S07 will hoist the task-list checkbox
    into this field).
  - `translateTextBlock`: emits a mdast `paragraph` node with the
    TextBlock's `Lines()`-derived span. mdast has no TextBlock type;
    goldmark's TextBlock is the tight-list inline-content wrapper.
  - `translateBlockquote`: position via `childrenSpan`.
  - `translateThematicBreak`: zero-width placeholder position (see
    Setup notes).
  - `translateChildren` gained a hard-line-break post-step: after
    each child, if it's a `*ast.Text` with `HardLineBreak()` true,
    append a synthetic mdast `break` node spanning the consumed
    two-spaces-and-LF (or backslash-and-LF) bytes from
    `t.Segment.Stop` to the next sibling's segment start.
- And on the emit side `internal/emit/emit.go::writeNode` grew two new
  cases in the type-specific switch (in canonical key order):
  - `list` case writes `ordered`, `start`, `spread` before `children`.
    `start: null` when `n.Start == nil`; `start: <int>` otherwise.
  - `listItem` case writes `spread`, `checked` before `children`.
    `checked: null` when `n.Checked == nil`; `checked: true`/`false`
    when the pointer is set. **Crucially, `checked: null` is never
    elided** — acceptance criterion #7 pinned.
  - `isContainer(t)` extended to also exclude `"thematicBreak"` and
    `"break"` (mdast leaves; no empty `children` array on the wire).
- Notes: this is the tracer — one fixture exercises the entire
  parse→translate(List+ListItem+TextBlock+text)→emit(list+listItem
  key-order+nullable-fields) pipeline end-to-end.

## Test 2 — `1. a\n2. b` → ordered list, start=1 (criterion #2 part a)

- Wrote: fixture `testdata/fixtures/20-ordered-list-start1-nopos/` with
  `input.md="1. a\n2. b\n"`. Expected stdout pins `"ordered":true,
  "start":1`.
- Red: skipped — Test 1's GREEN already handles `IsOrdered()`/`Start`
  uniformly. The fixture would PASS on first run with no implementation
  change. Verified honestly by running `go test -run 'TestFixtures/20-' .`
  — passes immediately. Per the S04 tdd-log precedent, kept the fixture
  as a behavior anchor for future refactors.
- Green: pre-existing.
- Notes: pins the `ordered:true / start:<n>` shape; complements Test 1
  which pinned the `ordered:false / start:null` flavor.

## Test 3 — `3. a\n4. b` → ordered list, start=3 (criterion #2 part b)

- Wrote: fixture `testdata/fixtures/21-ordered-list-start3-nopos/` with
  `input.md="3. a\n4. b\n"`. Expected stdout pins `"start":3`.
- Red: skipped — Test 1's GREEN already plumbs `l.Start` → `*int(l.Start)`
  for ordered lists. Fixture verifies the non-trivial-start case
  (a regression that hardcoded `Start: 1` would NOT pass this).
- Green: pre-existing.
- Notes: this is the load-bearing variant of the ordered-list test. The
  `start: 1` case (Test 2) would PASS even if the implementation hardcoded
  the start; the `start: 3` case forces the implementation to read the
  goldmark `List.Start` field.

## Test 4 — `> quoted` → blockquote (criterion #3)

- Wrote: fixture `testdata/fixtures/22-blockquote-nopos/` with
  `input.md="> quoted\n"`. Expected stdout pins
  `blockquote → paragraph → text("quoted")`.
- Red: skipped — Test 1's GREEN handles `*ast.Blockquote` via the new
  dispatch case and `translateBlockquote`. Fixture pins the wire-side
  contract.
- Green: pre-existing.
- Notes: the blockquote case is uninteresting on the wire — just a
  container. The fact-finding probe confirmed goldmark wraps the
  blockquote content in a regular `*ast.Paragraph` (NOT a `TextBlock`),
  so the existing Paragraph dispatch carries the inline children
  unchanged. The `>` marker is dropped at the goldmark stage; the text
  value is `"quoted"`, NOT `"> quoted"`.

## Test 5 — `before\n\n---\n\nafter\n` → thematicBreak (criterion #4)

- Wrote: fixture `testdata/fixtures/23-thematic-break-nopos/` with
  `input.md="before\n\n---\n\nafter\n"`. Expected stdout pins three
  siblings under root.children: `paragraph("before")`, `thematicBreak`
  (a leaf with no `children` field), `paragraph("after")`.
- Red: skipped — Test 1's GREEN added the `*ast.ThematicBreak` dispatch
  case AND the `"thematicBreak"`/`"break"` entries in `isContainer`'s
  leaf set. Fixture pins both contracts at once.
- Green: pre-existing.
- Notes: per the acceptance criterion's "distinct from frontmatter"
  language, this fixture deliberately uses `---` (NOT `***`) and
  precedes it with non-frontmatter content to disambiguate from the
  frontmatter opening-fence rule (CONTEXT.md "Frontmatter / Unclosed-
  fence rule"): `---\n` at the very top of a document is consumed by
  the frontmatter extension; `---\n` after a paragraph is unambiguously
  a thematic break. The fixture exercises the latter, which is the
  case the acceptance criterion calls out.

## Test 6 — `line1<two-spaces>\nline2` → text, break, text (criterion #5)

- Wrote: fixture `testdata/fixtures/24-hard-break-two-spaces-nopos/`
  with `input.md="line1  \nline2\n"` (two literal spaces after `line1`).
  Expected stdout pins `paragraph` with three children: `text("line1")`,
  `break` (leaf), `text("line2")`. No trailing whitespace in `line1`,
  no leading whitespace in `line2`.
- Red: skipped — Test 1's GREEN added the `translateChildren`
  hard-line-break post-step that inserts a synthetic `break` after
  any goldmark `*ast.Text` with `HardLineBreak()` true. Fixture pins
  the wire-side contract (the most CONTEXT.md-load-bearing one of S05:
  "Text/Code value preservation" specifically calls out this case as
  the canonical example).
- Green: pre-existing.
- Notes: the two trailing spaces are consumed by goldmark as the break
  boundary and do NOT appear in either text node's `value`. This is
  the wire contract per CONTEXT.md, and the byte-exact comparison in
  the fixture harness would catch any future refactor that accidentally
  let the trailing whitespace bleed through (e.g. by reading the
  source bytes between segment boundaries instead of `t.Segment`).

## Test 7 — `line1\\\nline2` → text, break, text (criterion #6)

- Wrote: fixture `testdata/fixtures/25-hard-break-backslash-nopos/`
  with `input.md="line1\\\nline2\n"` (backslash-escaped newline).
  Expected stdout: same three-children paragraph shape as Test 6.
- Red: skipped — same post-step covers both flavors (goldmark sets
  `HardLineBreak()` true on the preceding Text for both syntaxes;
  the break-insertion logic doesn't care which CommonMark syntax
  produced the flag).
- Green: pre-existing.
- Notes: the backslash and the newline are both consumed by goldmark
  as the break boundary and do NOT appear in either text node's
  `value`. Together with Test 6 this pins the contract that BOTH
  CommonMark hard-break syntaxes produce identical mdast wire shapes
  — a downstream consumer (and a downstream test) cannot tell which
  syntax was used.

## Translate unit tests (anchoring criterion #7 at the Go-value-tree boundary)

In `internal/translate/translate_test.go` I added four Go-level unit
tests that exercise the load-bearing translate-side rules at the
package boundary (not via the fixture harness):

- `TestTranslateListItemNonTaskCarriesNilChecked` — explicit
  acceptance criterion #7: every non-task listItem has
  `Checked == nil` in the Go value tree, NOT `false` (which would
  serialize as `checked: false` and elide the null-vs-false
  distinction). Asserts the pointer-typed field at the Go layer
  separately from the wire-side `"checked":null` assertion in the
  fixtures.
- `TestTranslateUnorderedListHasNilStart` — unordered list yields
  `Start == nil` on the Go side. Pins the "use pointer types for
  nullable mdast fields" convention.
- `TestTranslateOrderedListStartThreeHasPointerStart` — ordered list
  with explicit `3.` start yields `*Start == 3` on the Go side.
  Complements the unordered case; together they pin the full pointer
  semantics.
- `TestTranslateHardLineBreakInsertsBreakBetweenTexts` — sub-tests
  for both two-space and backslash hard-break flavors. Asserts the
  paragraph has exactly three children, in order: `text("line1")`,
  `break`, `text("line2")` — pinning the no-trailing-whitespace,
  no-leading-whitespace rule from CONTEXT.md "Text/Code value
  preservation" at the Go layer.

These complement the fixture suite by pinning the contract at the
package-public-API layer too — a future refactor that changed the
JSON emit shape but kept the translate output structurally identical
(or vice versa) would surface in the right place.

## Refactor pass

After all tests green:

1. **Removed the goldmark fact-finding probe** at `cmd/_probe/main.go`.
   Not part of the shipped code; was used to confirm `IsOrdered/Start/
   Marker/IsTight` on List, the TextBlock-wraps-tight-list-items
   structural fact, and the `HardLineBreak()`/segment-boundary behavior
   for hard breaks.
2. **Refined the TextBlock dispatch comment** in `translateNode` —
   first draft said the inline content "lives directly under
   `listItem.children`", which contradicted the actual implementation
   (TextBlock emits a `paragraph` wrapper, not direct inline children).
   Corrected to: "mdast has no TextBlock type; the inline content is
   wrapped in a plain `paragraph` instead." Matches S05 acceptance #1
   which explicitly requires `listItem → paragraph → text`.
3. **Did NOT consolidate `translateParagraph` and `translateTextBlock`.**
   They produce structurally identical output (`type:"paragraph"`,
   same span/children) but operate on different goldmark Go types
   (`*ast.Paragraph` vs `*ast.TextBlock`). Merging them via an
   interface assertion in `translateNode` would gain three lines of
   savings and lose the type-switched dispatch's compile-time
   exhaustiveness check. Leave them as parallel cases with the
   shared rationale in comments.
4. **Did NOT precompute ThematicBreak source positions** by scanning
   the source for `^([-*_])\1\1+\s*$` lines. The current zero-width
   placeholder is acceptable because S05's fixtures all use
   `--no-position`. S10 (position-info pinning) will revisit if a
   default-mode fixture exercises ThematicBreak positions. Adding
   the scan now would be premature optimization without a failing
   test driving it.
5. **Did NOT add per-item spread inference** in `translateListItem`.
   mdast distinguishes `listItem.spread` from `list.spread`
   (per-item override for the loose-rendering case), but goldmark
   exposes only the list-level `IsTight`. All S05 fixtures are tight,
   so `listItem.spread: false` is correct for every emitted item.
   When S07 (or a later slice with a loose-list fixture) bites, this
   will need a `parent.IsTight` inheritance lookup — but adding the
   lookup now without a test exercising it would be speculative.

## Final

- Tests added in S05:
  - 7 fixtures: `testdata/fixtures/19-unordered-list-nopos` through
    `testdata/fixtures/25-hard-break-backslash-nopos`. Each pins one
    acceptance criterion with a byte-exact wire-shape assertion under
    `--no-position`.
  - 4 translate unit tests:
    `TestTranslateListItemNonTaskCarriesNilChecked`,
    `TestTranslateUnorderedListHasNilStart`,
    `TestTranslateOrderedListStartThreeHasPointerStart`,
    `TestTranslateHardLineBreakInsertsBreakBetweenTexts` (with two
    sub-tests: two-space and backslash flavors).
- `go test ./...`: 31 passing tests, 0 failures (all package-level
  test binaries green).
- `go vet ./...`: clean.
- Files touched:
  - `internal/translate/translate.go` (extended: Node struct, dispatch
    switch, `translateChildren` hard-break post-step, five new
    translate functions for List/ListItem/TextBlock/Blockquote/
    ThematicBreak)
  - `internal/translate/translate_test.go` (4 new unit tests appended)
  - `internal/emit/emit.go` (extended `writeNode` type-specific switch
    with `list` and `listItem` cases in canonical key order; extended
    `isContainer` to mark `thematicBreak` and `break` as leaves)
  - 7 new fixtures under `testdata/fixtures/`

- Acceptance criteria status:
  - [x] criterion 1 — `- a\n- b` → list(ordered:false, start:null, spread:false) with two listItems each containing paragraph→text (`TestFixtures/19-unordered-list-nopos`, `TestTranslateUnorderedListHasNilStart`, `TestTranslateListItemNonTaskCarriesNilChecked`)
  - [x] criterion 2 — `1. a\n2. b` → list(ordered:true, start:1) and `3. a\n4. b` → list(ordered:true, start:3) (`TestFixtures/20-ordered-list-start1-nopos`, `TestFixtures/21-ordered-list-start3-nopos`, `TestTranslateOrderedListStartThreeHasPointerStart`)
  - [x] criterion 3 — `> quoted` → blockquote → paragraph → text("quoted") (`TestFixtures/22-blockquote-nopos`)
  - [x] criterion 4 — `---` as a thematicBreak distinct from frontmatter (fixture uses `before\n\n---\n\nafter\n` so the leading content disambiguates the case from the frontmatter opening-fence rule) (`TestFixtures/23-thematic-break-nopos`)
  - [x] criterion 5 — `line1<two-spaces>\nline2` paragraph → text("line1"), break, text("line2"); trailing two spaces consumed by the break, no whitespace bleed (`TestFixtures/24-hard-break-two-spaces-nopos`, `TestTranslateHardLineBreakInsertsBreakBetweenTexts/two-space`)
  - [x] criterion 6 — `line1\\\nline2` paragraph → same text/break/text shape with values `"line1"` and `"line2"` (`TestFixtures/25-hard-break-backslash-nopos`, `TestTranslateHardLineBreakInsertsBreakBetweenTexts/backslash`)
  - [x] criterion 7 — every non-task listItem in this slice's fixtures carries `checked: null` (never elided, never omitted) — asserted byte-exact on the wire by every list fixture (19/20/21) and at the Go layer by `TestTranslateListItemNonTaskCarriesNilChecked`

VERDICT: accept
