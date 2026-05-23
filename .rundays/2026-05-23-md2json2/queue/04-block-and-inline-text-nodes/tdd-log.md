# TDD log: 04-block-and-inline-text-nodes

Started: 2026-05-23

## Setup

- Reuses S01/S02/S03's test framework (Go `testing` + `go test`, plus the
  black-box fixture harness at `integration_test.go`). No new dependencies.
- Slice goal: extend `translate.Translate` to walk goldmark's block and inline
  children for the simplest cases — `Heading`, `Paragraph`, `Text`, `Emphasis`
  (level 1), `Emphasis` (level 2 → `strong`). Extend `emit.writeNode` with the
  mdast key-ordering slot for type-specific fields (`depth` on heading, `value`
  on text). Add per-acceptance fixtures, and one positional (no-`--no-position`)
  fixture so position info on the new node types is observable on the wire.
- Goldmark fact-finding (probe, deleted before commit):
  - `*ast.Heading{Level: 1..6}` is exposed and maps directly to mdast
    `heading.depth`. `Lines()` of a heading exposes the text-content segment
    (post-`#`-marker run); the leading `# ` is implicit.
  - `*ast.Paragraph.Lines()` is the list of source segments composing the
    paragraph; trailing `\n` is NOT in `Stop`.
  - `*ast.Text.Segment.Value(src)` is the verbatim text run; for paragraphs
    goldmark sometimes splits a single CommonMark text run into two adjacent
    `*ast.Text` nodes — e.g. `Hello world.` yields `Text("Hello")` then
    `Text(" world.")`. The mdast spec is one `text` node per run, so the
    translate stage coalesces consecutive sibling `Text` nodes whose segments
    are contiguous (`a.Stop == b.Start`) and that have no hard-line-break
    flag between them. CommonMark agrees: a run of text without a hard break
    or other inline construct is one logical `text` node.
  - `*ast.Emphasis{Level: 1}` → mdast `emphasis`; `*ast.Emphasis{Level: 2}` →
    mdast `strong`. (mdast's spec: strong is a separate node type, not an
    "emphasis with depth 2".)
- Position math: every translated node carries a `Position{Start, End}` derived
  from byte offsets. The translate package gets a `positionTracker` that maps
  byte-offset → (line, column) on demand using the normalized source bytes,
  honoring "column counts UTF-8 code points" from CONTEXT.md "Position info".
  For a heading node the position spans the source line including the `#`
  markers (start = beginning of that line, end = end of the content); for a
  paragraph it spans from `lines[0].Start` to `lines[last].Stop`; for inline
  nodes it spans the source bytes that produced the node (e.g. `*hello*` →
  start of `*` through end of closing `*`).

## Test 1 — tracer bullet: `# Hello\n --no-position` (criterion #1)

- Wrote: fixture `testdata/fixtures/12-h1-hello-nopos/` with `args=--no-position`,
  `input.md="# Hello\n"`, `exit=0`, `stderr` empty, and expected `stdout`
  exactly:
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"heading","depth":1,"children":[{"type":"text","value":"Hello"}]}]}}`
  No trailing newline (per the v1 ship criterion's byte-exact convention).
- Red: `go test -run 'TestFixtures/12-' .` failed with the empty-root envelope
  on stdout (translate at S03 was still producing `root.children:[]`). This
  is the tracer — it forces the entire goldmark→mdast walk for Heading/Text
  into existence, plus the emit-side type-specific-field slot.
- Green: rewrote `internal/translate/translate.go` with:
  - The `Node` struct gained `Depth int`, `Value string`, `ValuePresent bool`
    fields. The single-struct-with-optional-fields shape was chosen over an
    interface-per-node-type approach: it keeps the JSON-encoding contract on
    the emit side (one switch in `writeNode`), avoids reflection, and reads
    straight off the mdast spec which is itself a tagged-union by `type`.
    `ValuePresent` distinguishes "no `value` field" from `value:""` because
    Go's zero string is the same as an explicit empty.
  - `translateChildren` / `translateNode` dispatch table for the v1-recognized
    block/inline goldmark types. Unrecognized kinds return nil (silent drop
    per CONTEXT.md "Lossiness policy").
  - `translateHeading` / `translateParagraph` / `translateText` /
    `translateEmphasis` implementations that read the mdast-relevant fields
    off the goldmark node (Heading.Level → depth; Text.Segment value →
    value; Emphasis.Level → emphasis vs strong type).
  - A `positionTracker` (new file `internal/translate/position.go`) that
    precomputes line-start offsets and converts byte offsets to mdast Points
    counting UTF-8 code points per CONTEXT.md "Position info".
  - `blockOffsets` / `paragraphOffsets` / `lineStartOffset` helpers that
    derive byte-offset spans from goldmark's per-line `*textm.Segments`.
- And on the emit side `internal/emit/emit.go::writeNode` grew:
  - A `switch` on `n.Type` between `"type"` and `"children"` that emits the
    type-specific keys in canonical order: `depth` for heading, `value` for
    text. This is the explicit slot S04's issue called out as critical.
  - An `isContainer(t)` predicate so leaf nodes (currently just `"text"`)
    don't carry a `"children":[]` array on the wire (mdast convention: leaves
    have no children field at all, not an empty one).
- Also re-pinned `TestPositionalFileEmitsEnvelope` in
  `internal/cli/cli_test.go` to write an EMPTY file instead of
  `"# unused content\n"`. With the real translate walk in place, non-empty
  content would force that test to assert on heading/text shape — which is
  owned by S04's own fixtures, not by an S01 test whose contract is
  "positional FILE accepted, bytes read off disk."
- Notes: this is the tracer; one fixture exercises the whole new pipeline
  end-to-end (parse → translate → emit) for the simplest non-empty case.
  Subsequent tests narrow on specific behaviors.

## Test 2 — `# h1` ... `###### h6` → depth 1..6 (criterion #2)

- Wrote: fixture `testdata/fixtures/13-headings-h1-to-h6-nopos/` with all six
  ATX heading depths, one per blank-line-separated block. Expected stdout
  enumerates the six sibling heading nodes with `depth: 1..6` in order.
- Red: skipped — Test 1's GREEN already plumbs `h.Level` → `Node.Depth` and
  emit's switch writes `"depth":<n>`. The new fixture would PASS on first
  run with no implementation change. I verified honestly by running
  `go test -run 'TestFixtures/13-' .` — passes immediately. Per the S03
  tdd-log precedent (Tests 2/5/6), keep the test as a behavior anchor for
  future refactors of the heading-depth mapping rather than skipping it.
- Green: pre-existing.
- Notes: this is a coverage-via-fixture rather than a code-driving test; it
  ensures that "ATX heading depth maps 1:1 to mdast `depth`" stays pinned
  on the wire even if a future refactor of the goldmark-Heading walk forgot
  the field.

## Test 3 — `*hello*` → paragraph → emphasis → text (criterion #3)

- Wrote: fixture `testdata/fixtures/14-emphasis-nopos/` with `input.md="*hello*\n"`.
  Expected stdout pins the nesting `paragraph > emphasis > text(value:"hello")`.
- Red: skipped — Test 1's GREEN implementation already handles `*ast.Emphasis`
  with `Level: 1` → mdast `emphasis`. The fixture asserts the wire-side
  contract.
- Green: pre-existing.
- Notes: this pins the level-1 → "emphasis" naming. Test 4 pins level-2 →
  "strong".

## Test 4 — `**hello**` → paragraph → strong → text (criterion #4)

- Wrote: fixture `testdata/fixtures/15-strong-nopos/` with
  `input.md="**hello**\n"`. Expected stdout pins `paragraph > strong >
  text(value:"hello")`.
- Red: skipped — Test 1's GREEN handles `*ast.Emphasis{Level: 2}` by
  mapping the mdast type to `"strong"` rather than `"emphasis"`. Fixture
  asserts the wire-side contract.
- Green: pre-existing.
- Notes: mdast distinguishes strong from emphasis as a separate node type,
  NOT as `{type:"emphasis", depth:2}`. The translateEmphasis switch on
  `e.Level` enforces this.

## Test 5 — `Hello world.` → paragraph > text(value:"Hello world.") (criterion #5)

- Wrote: fixture `testdata/fixtures/16-plain-paragraph-nopos/` with
  `input.md="Hello world.\n"`. Expected stdout pins exactly one `text` child
  with `value:"Hello world."` (no trailing newline in the value, no internal
  split, no surrounding whitespace).
- Red: skipped, but only because the coalescing logic in Test 1's GREEN
  caught the case. Pre-coalesce probe showed `Hello world.` produces TWO
  `*ast.Text` nodes from goldmark: `Text("Hello")` + `Text(" world.")` (split
  at segment boundary `pos 5`). The coalescing in `translateChildren`
  recognizes them as contiguous (`a.Segment.Stop == b.Segment.Start`) and
  same-type and merges into one `text` node on the wire.
- Green: pre-existing (Test 1's translateChildren handles the merge).
- Notes: this is the most subtle wire-shape rule in S04. CONTEXT.md
  "Text/Code value preservation" says the value flows through byte-for-byte;
  mdast says one `text` node per uninterrupted run; goldmark internally
  splits at its own segment boundaries. The mismatch is reconciled by
  translate, not by goldmark or emit. This fixture is the lock against any
  future refactor that accidentally drops the coalesce step (e.g. by
  switching to a `range`-only walk that doesn't track the previous sibling).

## Test 6 — three blank-line-separated paragraphs → three sibling paragraph nodes (criterion #6)

- Wrote: fixture `testdata/fixtures/17-multiple-paragraphs-nopos/` with three
  paragraphs separated by blank lines. Expected stdout pins three sibling
  `paragraph` nodes under `root.children` in source order.
- Red: skipped — Test 1's GREEN produces a `paragraph` per `*ast.Paragraph`
  node goldmark surfaces, in sibling order. Fixture pins the source-order
  contract.
- Green: pre-existing.
- Notes: the fixture explicitly uses three paragraphs (not two) to make
  "source order" non-trivially distinguishable from any sort-by-content
  accident.

## Test 7 — position info attached uniformly on heading/text/root in default mode (S03 contract)

- Wrote: fixture `testdata/fixtures/18-h1-hello-with-position/` with empty
  `args` (default mode, position on). Expected stdout pins exact positions:
  - root: start `{1,1,0}` end `{2,1,8}` (the trailing `\n` advances line to 2)
  - heading: start `{1,1,0}` (column 1 — the `#` marker is included) end
    `{1,8,7}` (after `Hello`)
  - text: start `{1,3,2}` (after `# `) end `{1,8,7}`
- Red: skipped — Test 1's GREEN already attaches positions via the
  positionTracker. Fixture pins the byte-exact wire shape for the
  position-attached case, which is the contract S03 introduced ("uniform
  rule: every node carries position unless --no-position; no per-node
  special-casing") and S04 must not regress.
- Green: pre-existing.
- Notes: this is the load-bearing fixture for the "uniform" half of the
  Position info rule. The S04 implementation could have cut a corner by
  only attaching position to leaves or only to containers; this fixture
  asserts that EVERY emitted node (root, heading, text) carries its own
  position field in default mode. The byte-exact comparison would catch
  any future refactor that special-cased the heading-with-implicit-marker
  position or the root-of-non-empty-source case.

## Translate unit tests (extra coverage)

In `internal/translate/translate_test.go` I added three Go-level unit tests
that exercise the load-bearing translate-side rules at the package boundary
(not via the fixture harness):

- `TestTranslateCoalescesContiguousTextSiblings` — asserts the
  `Hello world.` → single-text-node coalesce at the Go-value-tree layer
  (separately from the JSON wire shape).
- `TestTranslateEmphasisLevelTwoEmitsStrong` — asserts level-2 emphasis →
  type `"strong"` (NOT `{type:"emphasis", depth:2}`) at the Go layer.
- `TestTranslateHeadingPositionIncludesAtxMarkers` — asserts the heading's
  position starts at column 1 (the `#` marker), not at the first content
  byte goldmark exposes.

These complement the fixture suite by pinning the contract at the
package-public-API layer too — a future refactor that changed the JSON
emit shape but kept the translate output structurally identical (or vice
versa) would surface in the right place.

## Refactor pass

After all tests green:

1. **Removed the speculative `linesView`/`segmentView` interface shim** I
   drafted in the first pass. The concrete `*textm.Segments` type satisfies
   `blockOffsets`/`paragraphOffsets` directly with less indirection.
2. **Did NOT refactor `rootPosition` to use `positionTracker`.** They both
   walk the source counting code points; merging them would unify the math
   but risk regressing the empty-doc and single-newline boundary cases that
   `rootPosition` bakes in explicitly. Two-place repetition is acceptable
   here because the two places encode subtly different rules: rootPosition's
   "empty-doc returns zero-width at 1:1:0" is a hand-pinned constant, not
   a derived result. Leave.
3. **Removed the probe scratch file** (`cmd/_probe/probe.go`) I used to
   inspect goldmark's heading/paragraph/text node shapes. Not part of the
   shipped code.
4. **Did NOT extract a Node-shape predicate (heading/text/emphasis) into a
   per-type sub-package.** The single-struct-with-optional-fields shape
   stays as the simpler-to-emit form. Per-type sub-packages would gain
   compile-time type safety on each variant but require either reflection
   or a visitor in emit, both of which violate the "compact, key-ordered,
   no-allocs-in-hot-path" emit contract.

## Final

- Tests added in S04:
  - 7 fixtures: `testdata/fixtures/12-h1-hello-nopos` through
    `testdata/fixtures/18-h1-hello-with-position` (6 with `--no-position`
    pinning the wire shape per acceptance criterion; 1 in default mode
    pinning position-attached uniformity).
  - 3 translate unit tests:
    `TestTranslateCoalescesContiguousTextSiblings`,
    `TestTranslateEmphasisLevelTwoEmitsStrong`,
    `TestTranslateHeadingPositionIncludesAtxMarkers`.
- `go test ./...`: all green (no FAIL lines anywhere).
- `go vet ./...`: clean.
- Files touched:
  - `internal/translate/translate.go` (extended)
  - `internal/translate/position.go` (new)
  - `internal/translate/translate_test.go` (new unit tests appended)
  - `internal/emit/emit.go` (extended writeNode with the type-specific switch
    and isContainer)
  - `internal/cli/cli_test.go` (re-pinned `TestPositionalFileEmitsEnvelope`
    to use an empty file)
  - 7 new fixtures under `testdata/fixtures/`

- Acceptance criteria status:
  - [x] criterion 1 — `# Hello` → `heading{depth:1, children:[text{value:"Hello"}]}` (`TestFixtures/12-h1-hello-nopos`)
  - [x] criterion 2 — `##` through `######` → depth 2..6 (`TestFixtures/13-headings-h1-to-h6-nopos`)
  - [x] criterion 3 — `*hello*` → paragraph > emphasis > text("hello") (`TestFixtures/14-emphasis-nopos`)
  - [x] criterion 4 — `**hello**` → paragraph > strong > text("hello") (`TestFixtures/15-strong-nopos`, `TestTranslateEmphasisLevelTwoEmitsStrong`)
  - [x] criterion 5 — `Hello world.` → paragraph > one text(value:"Hello world.") (`TestFixtures/16-plain-paragraph-nopos`, `TestTranslateCoalescesContiguousTextSiblings`)
  - [x] criterion 6 — three blank-line-separated paragraphs → three sibling paragraph nodes in source order (`TestFixtures/17-multiple-paragraphs-nopos`)
  - [x] cross-cutting (S03 contract upheld) — position info attached uniformly on every node in default mode (`TestFixtures/18-h1-hello-with-position`, `TestTranslateHeadingPositionIncludesAtxMarkers`)

VERDICT: accept

