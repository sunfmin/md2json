# TDD log: 05-unclosed-display-math-compensation

Started: 2026-05-23T16:28:54Z

Existing Go test framework (`testing` stdlib + per-fixture testdata harness). No new dep.

Strategy: tracer-bullet TDD over the issue acceptance bullets, sequenced to
implement PRD §Unclosed-fence behavior + ADR-0004 Decision 5
("unclosed-`$$`-at-EOF translate compensation, src-byte predicate after
`MathBlock.Lines().Last().Stop`"). PRD fixture #14 (library A-vs-B
equivalence) is RED-FIRST: it pins the load-bearing library invariant
the compensation rests on; if it fails, ADR-0004 Decision 5's premise
is wrong and we re-open. Only after #14 passes do we touch any translate
code.

## Test 1: library-contract A-vs-B equivalence (PRD fixture #14)

- Wrote: `TestParseUnclosedAndClosedDisplayMathHaveIdenticalLinesLastStop` in
  `internal/parse/parse_test.go`. Behavioral A-vs-B (input A `$$\nx\n`,
  unclosed, 5 bytes; input B `$$\nx\n$$\n`, closed, 8 bytes). Asserts:
  both produce one `*mathjax.MathBlock` and both have
  `Lines().Last().Stop == 5` (offset past body's terminating LF).
- Red: n/a — this is a library-behavior probe; if the library already
  satisfies the invariant the test starts green by observation, and we
  know we can proceed with the translate compensation. If it had failed
  here, we'd re-open ADR-0004.
- Green: 1st run (PASS).
- Notes: behavioral, not structural — robust to library struct churn.
  Failure modes documented in the doc comment ("if A switches to
  decline-to-match, or closing fence gets appended to Lines()..."). Pins
  the "AST-alone cannot distinguish closed-vs-unclosed at the same body
  extent" invariant.

## Test 2: PRD fixture #5 — unclosed `$$` demotes to paragraph with two text children

- Wrote: `TestTranslateUnclosedDisplayMathDemotesToParagraphWithTwoTextChildren`
  in `internal/translate/translate_test.go`. Input `$$\n\frac{a}{b}\n`
  (no closer). Asserts paragraph with `[text{value:"$$"}, text{value:"\\frac{a}{b}"}]`,
  no `math` siblings, no embedded LF.
- Red: 1st run failed with `Type: got "math", want "paragraph"` — confirmed
  S03's `translateMath` was emitting a `math` node unconditionally for
  every `MathBlock`, regardless of closer presence (as expected for the
  pre-S05 state).
- Green: implemented `displayMathClosed` (src-tail predicate walking
  forward over LF/blank lines, checking for `$$+ whitespace-tail` closer)
  and `demoteUnclosedDisplayMath` (emits paragraph with one text per
  source line, segments stop BEFORE the LF, mirroring goldmark prose-
  paragraph segmentation per PRD §Notes "Soft-line-break handling note").
  Wired into `translateMath` as a closed-vs-unclosed branch before the
  normal `math`-node emit path. Single function-level Edit; ~110 lines
  total including doc comments.
- Notes: scope restriction (ADR-0004 Decision 5 + PRD §Out of Scope)
  applies — the no-internal-blank-line case is what's pinned; an
  unclosed `$$` with an internal blank line inside the body is OUT OF
  SCOPE for v1 (the singular-paragraph compensation cannot faithfully
  represent goldmark's multi-paragraph prose parsing for that case).
  We don't add a fixture for it. If real-world inputs hit it →
  tracking issue + future Run.

## Test 3: closed-case regression (issue 05 acceptance bullet #2)

- Wrote: `TestTranslateClosedDisplayMathStillEmitsMathNode` (input
  `$$\nx\n$$\n`, S03 happy path). Asserts the closed case STILL emits
  `math{value:"x\n", meta:nil}` after Test 2's compensation is wired in.
  Regression guard: the unclosed predicate must not misfire on a
  legitimately closed block.
- Red: n/a (already green on first run after Test 2's GREEN, because
  the predicate correctly identifies the closer line `$$\n` past the
  body).
- Green: 1st run after compensation (PASS).
- Notes: this is the load-bearing closed-vs-unclosed boundary test.
  If the predicate ever drifts (e.g. a future library upgrade changes
  what's in Lines()), this test fails explicitly.

## Test 4: PRD fixture #6 — unclosed inline `$` rides through as text

- Wrote: `TestTranslateUnclosedInlineMathLibraryHandlesNoCompensation`
  (input `prose $x = 5 still prose`). Asserts paragraph with one
  text child `"prose $x = 5 still prose"`, zero `inlineMath` nodes.
- Red: n/a — the library's inline parser at
  `probe/goldmark-mathjax/inline.go:33-37` already handles unclosed
  inline correctly (returns a Text segment for the opener `$` bytes
  when `line == nil` before a closer is seen). No translate-side
  compensation needed.
- Green: 1st run (PASS).
- Notes: this test pins the "library's own non-match path is the
  correct wire behavior for unclosed inline" claim from PRD
  §Unclosed-fence behavior. Sibling-coalescing
  (`translate.go:225-231`) folds the opener-`$` text and the
  rest-of-line text into a single mdast `text` node.

## Wire fixtures

- `testdata/fixtures/73-unclosed-display-math-demotes-to-paragraph-nopos/`
  — PRD fixture #5 at the CLI wire layer (`--no-position` JSON byte-exact
  compare). Input `$$\n\frac{a}{b}\n`, stdout
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"paragraph","children":[{"type":"text","value":"$$"},{"type":"text","value":"\\frac{a}{b}"}]}]}}`.
- `testdata/fixtures/74-unclosed-inline-math-rides-through-as-text-nopos/`
  — PRD fixture #6 at the CLI wire layer. Input `prose $x = 5 still prose\n`,
  one text child carrying the whole line.
- `testdata/fixtures/75-unclosed-display-math-demotes-to-paragraph-default/`
  — default-mode (positions present) variant of fixture 73; pins the
  source-range positions on the demoted paragraph + its two text
  children. Per CONTEXT.md "Position info" uniform rule (every node
  carries position unless `--no-position`).

All three fixtures executed via integration_test.go's `TestFixtures`
walker. PASS on first integration run after the translate compensation
was wired in.

## Refactor pass

No code-deduplication refactor needed. `displayMathClosed` and
`demoteUnclosedDisplayMath` each have a single caller (`translateMath`)
and clear single responsibility. `isAllASCIIWhitespace` was extracted
as a small named seam because it's called twice inside
`displayMathClosed` (once for blank-line skip, once for closing-fence
tail check) and reads as English at both call sites. `lineStartOffset`
was already in the file and reused for the opening-`$$` line start
recovery. No new shared infrastructure introduced.

## Final

- Tests added: 4 Go-layer tests (1 in `internal/parse/parse_test.go`,
  3 in `internal/translate/translate_test.go`) + 3 CLI wire fixtures
  (`testdata/fixtures/73`, `74`, `75`).
- Tests passing: full `go test ./...` green; 6 packages PASS.
- Acceptance status:
  - [x] Input `$$\n\frac{a}{b}\n` (no closing fence, EOF after body LF)
        produces one `paragraph` with children
        `[text{value:"$$"}, text{value:"\\frac{a}{b}"}]`; zero `math`
        nodes; exit 0.
  - [x] Input `$$\nx\n$$\n` continues to produce one
        `math{value:"x\n", meta:null}` (closed case, S03 regression held).
  - [x] Input `prose $x = 5 still prose` (unclosed inline) produces one
        `paragraph` whose only child is `text{value:"prose $x = 5 still prose"}`
        after sibling-coalescing; zero `inlineMath` nodes; no
        translate-side compensation triggered.
  - [x] A focused in-process unit test parses two inputs
        (`$$\nx\n` unclosed, `$$\nx\n$$\n` closed) through the library
        alone and asserts both produce one `MathBlock` whose
        `Lines().Last().Stop` is identical (5 in both) — PRD fixture #14,
        library-contract pin.

## Out-of-scope (per ADR-0004 Decision 5 + PRD §Out of Scope)

- Unclosed `$$` block whose body contains an internal blank line.
  Behavior is implementation-defined in v1; the singular-paragraph
  compensation emitted here cannot faithfully represent goldmark's
  multi-paragraph prose parsing for that source range. No fixture
  pinned. If TDD-time real-world inputs hit this → tracking issue +
  future Run.

VERDICT: accept
