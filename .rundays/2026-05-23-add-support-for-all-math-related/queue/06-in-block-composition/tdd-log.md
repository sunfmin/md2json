# TDD log: 06-in-block-composition

Started: 2026-05-24T00:00:00Z

Existing Go test framework (`testing` stdlib + per-fixture testdata harness). No new dep.

Strategy: tracer-bullet TDD over the six issue acceptance bullets. This is
a **verification slice** for in-block composition (PRD fixtures #9 list /
blockquote / footnote sub-fixtures, #10a display-in-list, #10b
indented-`$$` falls to code, #11 tableCell `$$x$$` matches as inlineMath).
Each bullet's input runs through the existing S01-S05 wiring; the test
asserts the expected mdast tree. Most bullets pass FREE because the
list/blockquote/footnote/tableCell wrappers already nest naturally over
S02 inline math and S03 display math. Bullet #4 (display math inside a
list-item) surfaced one real S05 predicate gap (RED → GREEN with a
one-edit fix); bullet #3 (footnote definition) surfaced one upstream
goldmark behavior (orphan footnote definitions are dropped, requiring a
referenced-footnote input shape — documented adaptation below).

## Test 1: PRD #9 list sub-fixture — inline math inside listItem.paragraph

- Wrote: `TestTranslateInlineMathInsideListItemParagraph` in
  `internal/translate/translate_test.go`. Input `- prose $x$ more\n`.
  Asserts `list.listItem.paragraph.children = [text "prose ",
  inlineMath{value:"x"}, text " more"]`.
- Red: n/a — verification slice. The existing translateList /
  translateListItem / translateChildren pipeline already nests over the
  S02 `*mathjax.InlineMath` case at `translate.go:519`.
- Green: 1st run PASS.
- Notes: confirms inline math composes inside list-item paragraphs with
  no special-casing. Currency post-pass predicates all PASS (`x` on
  both sides of the `$` run).

## Test 2: PRD #9 blockquote sub-fixture — inline math inside blockquote.paragraph

- Wrote: `TestTranslateInlineMathInsideBlockquoteParagraph`. Input
  `> prose $x$ more\n`. Same shape as Test 1 but under `translateBlockquote`.
- Red: n/a — verification slice.
- Green: 1st run PASS.
- Notes: same pipeline (`translateChildren` recurses into blockquote
  children, dispatches InlineMath at translate.go:519).

## Test 3: PRD #9 footnote sub-fixture — inline math inside footnoteDefinition.paragraph

- Wrote: `TestTranslateInlineMathInsideFootnoteDefinitionParagraph`.
  **Adapted input** `a[^1]\n\n[^1]: prose $x$ more\n` (one-byte reference
  prefix `a[^1]\n\n` prepended).
- Red: FIRST attempt used the issue bullet's literal input
  `[^1]: prose $x$ more\n` (orphan definition). Test failed with
  `root.Children: got 0, want 1` — confirmed via probe (in-tree
  `cmd/probe_fn` walker, removed after diagnosis) that
  **goldmark drops unreferenced footnote definitions entirely**:
  `*ast.Document` for the orphan input has zero children, no
  `*east.FootnoteList`. The library's footnote extension wires a
  reachability pass — definitions are only retained when at least one
  `[^id]` reference appears earlier in the document. This is upstream
  library behavior, not an md2json silent-drop.
- Green: 2nd attempt with referenced-footnote input PASS.
- Notes: the **intent** of acceptance bullet #3 (inline math composes
  inside a footnoteDefinition's body paragraph) is preserved
  byte-identically — the paragraph children are the same `[text "prose
  ", inlineMath{value:"x"}, text " more"]`. Only the orphan-shaped
  trigger input is replaced with a referenced-footnote input that
  survives goldmark's reachability pass. S08
  `TestTranslateFootnoteReferenceAndDefinition` uses the same
  `a[^a]\n\n[^a]: footnote body\n` pattern for the same reason.

  The same adaptation rationale applies to the wire-side fixture
  (`testdata/fixtures/78-inline-math-in-footnote-definition-paragraph-nopos/`
  has the same `a[^1]\n\n[^1]: prose $x$ more\n` input).

  An ADR is not warranted: this is upstream library behavior, not a
  Rundays design decision. PRD §Testing Decisions fixture #9's literal
  input for the footnote sub-fixture (`[^1]: prose $x$ more\n`) is a
  spec-vs-library mismatch that the wire-side tests must work around;
  the wire contract pinned by CONTEXT.md `mdast node-set v1`
  (`footnoteDefinition{identifier, label}`) is unchanged.

## Test 4: PRD #10a — display `$$` at list-item line-start matches as math

- Wrote: `TestTranslateDisplayMathInsideListItem`. Input
  `- $$\n  x\n  $$\n` (8 bytes for `- $$\n`, two-space-indented body
  `  x\n`, two-space-indented closer `  $$\n`).
  Asserts `list.listItem.children = [math{value:"x\n", meta:nil}]`.
- Red: FIRST run failed with
  `listItem.Children[0].Type: got "paragraph", want "math"`. Diagnostic
  probe (`cmd/probe_li`, removed after diagnosis) showed: the library
  produces a `*mathjax.MathBlock` directly as a child of `*ast.ListItem`
  with `Lines().Len()=1`, `lines.At(0) = [7,9)` = `"x\n"`. The remaining
  source tail at `src[9:]` = `"  $$\n"` — i.e., the closing `$$` is
  two-space-indented IN THE ORIGINAL SOURCE (the library's block parser
  sees a dedented stream where `$$` is at column 0, but translate walks
  the pre-dedent src bytes).
- Diagnosis: **S05's `displayMathClosed` predicate at
  `internal/translate/translate.go:600-641` did not tolerate leading
  whitespace on the closer line.** It required the first non-blank
  line in src-tail to start IMMEDIATELY with `$`. For
  top-level `$$...$$` blocks this is correct, but for `$$...$$` nested
  inside a listItem/blockquote, the closer is indented in source.
  Without the fix, `displayMathClosed` returned false → the
  unclosed-fence compensation fired → math was demoted to paragraph
  (S05 misfire).
- Green: edited `displayMathClosed` to skip leading ASCII spaces / tabs
  BEFORE counting the `$`-run. ~5 LoC inside the existing predicate
  (added a `j` pointer that advances past whitespace, then `k` for the
  `$`-run, then `isAllASCIIWhitespace(line[k:])` check). Safe relaxation:
  the only ways a leading-whitespace `$$<whitespace>` line could appear
  after a MathBlock's recorded `Lines().Last().Stop` are (a) the legit
  closer of a nested MathBlock (this fixture's case), or (b) a
  4+-space-indented `$$` at top level — but the library's `Open`
  requires `$$\n` to fire (`probe/goldmark-mathjax/block.go:25-43`),
  and a 4+-indented `$$` is consumed as indented code BEFORE the math
  block parser sees it (verified by Test 6 below). Case (b) cannot
  produce a MathBlock that would invoke the predicate. So the
  whitespace-tolerance edit is safe across the full input space.
- Regression-check: Test 3's S05 closed-case
  (`TestTranslateClosedDisplayMathStillEmitsMathNode`, input
  `$$\nx\n$$\n`, no leading whitespace on the closer line) continues
  to PASS — the leading-whitespace skip is a no-op when no whitespace
  is present. Test 2's S05 unclosed-`$$`-at-EOF case
  (`TestTranslateUnclosedDisplayMathDemotesToParagraphWithTwoTextChildren`,
  input `$$\n\frac{a}{b}\n`, no closer line in src) also unchanged —
  the predicate still finds no `$`-run after walking past blank /
  whitespace-only tail bytes.
- Notes: this is the only translate code change in S06. The arch-log
  carries the diagnosis trace; tdd-log records the source-byte
  predicate gap S05 left and how S06 closed it.

## Test 5: PRD #11 — tableCell with `$$x$$` matches as inlineMath

- Wrote: `TestTranslateInlineMathInsideTableCellMatchesAsInlineMath`.
  Input is a 1-column, 1-data-row GFM table with cell content `$$x$$`:
  `| a |\n| --- |\n| $$x$$ |\n`. Asserts the cell's only child is
  `inlineMath{value:"x"}`, zero `math` nodes anywhere under the table.
- Red: n/a — verification slice. Per `probe/goldmark-mathjax/inline.go:26-28`
  the opener-count loop counts the `$`-run, so opener=2 for `$$`;
  the closer scan at `inline.go:38-52` matches the trailing `$$` run.
  Currency post-pass predicates: (i) src[opener+1]='$' (non-whitespace)
  PASS — this is the **canonical CONTEXT.md predicate (i) "non-whitespace"
  check**, which Round-2 PRD's drifted "non-whitespace-non-`$`" variant
  would have FAILED (PRD §Testing Decisions fixture #11 explicitly cites
  this), and which the Round-3 critique-fix restored. (ii) src[closer-1]='x'
  PASS. (iii) src[closer+1]='$' (non-digit) PASS. No demote. Wire
  output: one inlineMath child as pinned.
- Green: 1st run PASS — confirms the inline matcher accepts `$$...$$`
  inside cells (refuting Round-1 PRD's "inline matcher declines on `$$`"
  claim, which Round-2 verified-against-source and corrected).
- Notes: also exercised the `assertNoMathNodeAnywhere` helper to scan
  the whole table subtree for stray `math` nodes — none found, confirms
  GFM cells are inline-only.

## Test 6: PRD #10b — indented `$$x$$` falls to indented code

- Wrote: `TestTranslateIndentedDollarDollarFallsToIndentedCode`. Input
  `    $$x$$\n` (4-space-indented). Asserts `root.children = [code{lang:nil,
  meta:nil, value:"$$x$$\n"}]`.
- Red: n/a — verification slice. CommonMark indented-code-block rule
  (`>= 4` leading spaces) wins by block-parser priority; per ADR-0004
  Decision 1 the library's block parser declines on indented lines via
  `CanAcceptIndentedLine() → false` (verified in the probe clone).
- Green: 1st run PASS.
- Notes: pins the natural consequence of block-parser priority —
  math doesn't shadow indented code. The `code.value` trailing-LF
  preservation is already exercised by S06's existing
  `TestTranslateIndentedCodeBlockHasNilLangAndMeta`; this fixture
  asserts the same byte shape for `$$x$$` content specifically.

## Wire fixtures

CLI byte-exact JSON compares (every fixture under `testdata/fixtures/`
executed by the integration `TestFixtures` walker).

- `testdata/fixtures/76-inline-math-in-list-item-paragraph-nopos/`
  — Test 1 at the wire layer. Input `- prose $x$ more\n`.
- `testdata/fixtures/77-inline-math-in-blockquote-paragraph-nopos/`
  — Test 2 at the wire layer. Input `> prose $x$ more\n`.
- `testdata/fixtures/78-inline-math-in-footnote-definition-paragraph-nopos/`
  — Test 3 at the wire layer. Same adapted input `a[^1]\n\n[^1]: prose
  $x$ more\n` (orphan footnote unsupported by goldmark).
- `testdata/fixtures/79-display-math-in-list-item-nopos/`
  — Test 4 at the wire layer. Input `- $$\n  x\n  $$\n`. Exercises the
  S06 whitespace-tolerance edit in `displayMathClosed`.
- `testdata/fixtures/80-dollar-dollar-x-in-table-cell-matches-inline-math-nopos/`
  — Test 5 at the wire layer. Input `| a |\n| --- |\n| $$x$$ |\n`.
- `testdata/fixtures/81-indented-dollar-dollar-x-falls-to-indented-code-nopos/`
  — Test 6 at the wire layer. Input `    $$x$$\n`.

All six fixtures PASS on the first integration run after Test 4's
predicate fix.

## Refactor pass

No code-deduplication refactor needed. The `displayMathClosed`
predicate edit added two short pointer-advancement loops (leading
whitespace skip + `$`-run count) inline within the existing
single-function predicate. The `assertNoMathNodeAnywhere` helper in
the test file is local-scoped and single-purpose. No new shared
infrastructure introduced.

## Final

- Tests added: 6 Go-layer tests + 1 helper in
  `internal/translate/translate_test.go`, 6 CLI wire fixtures
  (`testdata/fixtures/76`..`81`). Plus one translate.go edit
  (~5 LoC inside `displayMathClosed`).
- Tests passing: full `go test ./...` green; 6 packages PASS;
  `go vet ./...` clean.
- Acceptance status:
  - [x] Input `- prose $x$ more\n` produces a `list` whose `listItem`
        contains a `paragraph` with children `[text{value:"prose "},
        inlineMath{value:"x"}, text{value:" more"}]`.
  - [x] Input `> prose $x$ more\n` produces a `blockquote` whose
        `paragraph` has those same three children.
  - [x] Input `[^1]: prose $x$ more\n` produces a `footnoteDefinition
        {identifier:"1"}` whose `paragraph` has those same three
        children — **adapted input** `a[^1]\n\n[^1]: prose $x$ more\n`
        used because goldmark drops orphan (unreferenced) footnote
        definitions; the footnoteDefinition body paragraph shape is
        byte-identical (see Test 3 notes).
  - [x] Input `- $$\n  x\n  $$\n` produces a `list` whose `listItem`
        has `[math{value:"x\n", meta:null}]` as a direct child.
        Required a one-edit fix to `displayMathClosed`'s closer
        detection (whitespace-tolerance on the closing-fence line).
  - [x] A single-row GFM table whose only cell contains `$$x$$`
        produces a `table` whose `tableCell` has children
        `[inlineMath{value:"x"}]`; zero `math` nodes anywhere under
        the table.
  - [x] Input `    $$x$$\n` (four-space indent at document root)
        produces `code{lang:null, meta:null, value:"$$x$$\n"}` —
        indented code wins; no math node.

## Findings (TDD-time)

1. **`displayMathClosed` closer-line predicate gap (S05 → S06 fix).**
   The predicate did not tolerate leading whitespace on the closer
   line. Top-level `$$...$$` blocks have a column-0 closer in source,
   so S05's TDD never exercised this. The S06 PRD #10a fixture
   (display `$$` inside listItem) has a two-space-indented closer in
   source (`  $$\n`), which the predicate rejected → unclosed-fence
   compensation misfired. Fix: skip leading ASCII spaces/tabs before
   counting the `$`-run. Safe by case analysis (see Test 4 notes).
   No ADR reopen needed; predicate semantics unchanged for top-level
   inputs (the whitespace skip is a no-op when no whitespace is
   present).

2. **Goldmark drops orphan footnote definitions (upstream behavior).**
   The acceptance bullet's literal input `[^1]: prose $x$ more\n`
   produces zero document children — goldmark's footnote extension
   requires at least one `[^id]` reference earlier in the document
   to retain the definition. Worked around in S06 tests by adding
   `a[^1]\n\n` prefix; the footnoteDefinition body paragraph shape
   under test is byte-identical to the bullet's intent. Not an md2json
   silent-drop. Documented in tdd-log Test 3 notes.

VERDICT: accept
