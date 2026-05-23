# TDD log: 07-mismatched-braces-and-value-preservation

Started: 2026-05-24

Existing Go test framework (`testing` stdlib + per-fixture `testdata/fixtures/`
harness). No new dep.

This is the **consolidation slice** for the math Run. Issue 07's three load-
bearing pieces:

1. PRD fixture #8 (mismatched braces inline, `$\frac{a}{b$`) — net-new
   fixture; PRD's verified library trace predicted it would land free given
   S02's translate wiring.
2. Lossiness property test extension — add `inlineMath` and `math` to
   `mdastNodeSetV1` map AND corpus entries that exercise both, closing the
   drift S03's tdd-log flagged ("`mdastNodeSetV1` map drift: does NOT yet
   include `inlineMath` or `math`").
3. Verification roll-up — confirm PRD fixtures #1-#14 are all present and
   green at merge time (free; S01-S06 individually pinned them).

No new translate / parse / emit code added (all S07 inputs land via the
S02-S06 wiring). One new fixture, one new Go-layer translate test, two
lossiness-property-test edits.

## Test 1 (tracer bullet): PRD fixture #8 mismatched braces — wire-side CLI fixture

- Wrote: `testdata/fixtures/82-inline-math-mismatched-braces-rides-through-as-value-nopos/{args,input.md,stdout,stderr,exit}`.
- Input: `$\frac{a}{b$` (12 bytes, no trailing newline) under `--no-position`.
- Expected stdout (byte-exact, no trailing newline per existing fixture
  convention):
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"paragraph","children":[{"type":"inlineMath","value":"\\frac{a}{b"}]}]}}`
- Expected stderr: empty. Expected exit: 0.
- Red: probe `printf '$\\frac{a}{b$' | go run . --no-position` BEFORE writing
  the fixture produced the exact expected stdout — issue.md bullet 1's
  prediction held ("Run; should pass already given S02's translate path. If
  it fails, investigate. (Likely passes.)"). So the wire-side test is GREEN
  on first run; no S07 translate code change required. Honest TDD reading:
  this is verification, not RED→GREEN — the issue's bullet 4 explicitly
  authorizes adding tests for behavior the code already implements.
- Green: `go test -run TestFixtures . -v` → fixture 82 PASS.
- Notes:
  - Library inline-parser trace (per `probe/goldmark-mathjax/inline.go:24-52`):
    opener=1 at pos 0; advance; scan slice `\frac{a}{b$` (11 chars); at
    i=10 (`$`), oldi=10, inner-loop slice[10]='$', i=11 hits
    `i<len(line)` boundary → close. Child segment covers orig pos 1..11
    `\frac{a}{b`. Trim-halfspace (`inline.go:62-82`): src[1]='\\' (not
    space) → no trim.
  - Translate currency post-pass predicates: (i) src[1]='\\' PASS, (ii)
    src[10]='b' PASS, (iii) src[12]=EOF PASS. No demote.
  - Transport-only posture: unbalanced `{` rides through inside
    `value` byte-for-byte. CONTEXT.md "Dollar-sign math (transport-only)"
    + "Text/Code value preservation".

## Test 2: PRD fixture #8 mismatched braces — Go-layer translate unit test

- Wrote: `TestTranslateInlineMathMismatchedBracesRideThroughAsValue` in
  `internal/translate/translate_test.go`. Mirrors S02's
  `TestTranslateInlineMathHappyPath` / S03's
  `TestTranslateDisplayMathPreservesMhchemValue` precedent (one Go-layer
  anchor per acceptance bullet that exercises a load-bearing transport
  rule).
- Input: `$\frac{a}{b$` (12 bytes).
- Asserts: `root.Children=[paragraph]`, `paragraph.Children=[inlineMath]`,
  `inlineMath.Type="inlineMath"`, `inlineMath.Value="\\frac{a}{b"`.
- Red: implicit (the assertions name the post-S02 behavior; without S02
  this test would have failed on `Type != "inlineMath"`).
- Green: first run PASS.
- Notes: anchors the transport-only contract at the translate layer so
  a future regression (e.g., someone adds a brace-balance check or trims
  trailing `\`) is caught at the Go layer with a precise diagnostic in
  addition to the byte-exact wire fixture diff.

## Test 3: lossiness property test extension — add `inlineMath` / `math` to mdastNodeSetV1 map

- Edited: `internal/translate/lossiness_property_test.go`.
- Two additions to the `mdastNodeSetV1` map:
  - `"math": true` (block half, next to `code` / `html` / `thematicBreak`).
  - `"inlineMath": true` (inline half, next to `inlineCode` / `break` /
    `delete`).
- Red phase (negative-direction probe): added the two map entries WITHOUT
  the corpus entries first, then ran
  `TestLossinessCorpusCoversEveryV1NodeType`:
  ```
  --- FAIL: TestLossinessCorpusCoversEveryV1NodeType (0.00s)
  lossiness_property_test.go:257: lossinessCorpus does not exercise the
    following mdast node-set v1 types: [inlineMath math]; add an input
    that produces each one so the silent-drop property test cannot
    trivially pass on a degenerate corpus
  ```
  This is exactly the regression the coverage-half test exists to catch
  (the property test would silently pass on a degenerate corpus). RED
  confirmed; the map-without-corpus state is detectable, not vacuous.

## Test 4: lossiness corpus — add inline-math + display-math entries

- Edited same file: added two corpus rows under `lossinessCorpus`:
  - `{"inline-math-happy-path", "prose $x = 5$ more\n"}` — exercises
    `inlineMath` plus surrounding `text` siblings, plus the currency
    post-pass PASS path (predicates (i)/(ii)/(iii) all PASS on `x`/`5`
    boundaries).
  - `{"display-math-happy-path", "$$\n\\frac{a}{b}\n$$\n"}` — exercises
    `math` with the canonical `\frac{a}{b}\n` body bytes.
- Also updated the doc-comment coverage map block (mirrors the
  one-line-per-type pattern):
  ```
  inlineMath            inline-math-happy-path (v1.x math Run)
  math                  display-math-happy-path (v1.x math Run)
  ```
- Green: re-ran the two property tests:
  - `TestEveryEmittedTypeIsInMdastNodeSetV1` — all 30 corpus subtests
    PASS, including the two new `inline-math-happy-path` and
    `display-math-happy-path` cases. Negative direction (`...DetectsOutOfSetTypes`)
    still detects the synthetic `goldmarkAutoLink` leak.
  - `TestLossinessCorpusCoversEveryV1NodeType` — PASS (the missing-set
    is now empty; every v1 node type is exercised by at least one corpus
    input).
- Notes:
  - This closes the S03 tdd-log drift note verbatim:
    `mdastNodeSetV1` map and `lossinessCorpus` now both carry the two math
    node types, so the wire-contract enumeration is enforced for math too.
  - The property test catches a regression in ANY layer (parse →
    translate → emit) that leaks a goldmark-native type name onto the
    wire. Math wiring is now under that enforcement net.
  - Bullet 4 of the issue's "Note" — "the lossiness property test
    addition is 'adding tests for behavior the code already implements'
    — within TDD scope per issue.md bullet 4. Don't add new translate
    code unless an honest RED surfaces." — honored: no translate code
    added; the test extension is pure coverage extension.

## Test 5 (verification roll-up): PRD fixtures #1-#14 all green at merge time

Per acceptance bullet #3 — "PRD fixtures #1 through #14 all pass when the
full slice is merged". Cross-referenced PRD §Testing Decisions vs. the
wire-side `testdata/fixtures/` inventory plus the Go-layer / parse-layer
tests for the non-CLI fixture (#14):

| PRD # | Description | Pinned by |
|-------|-------------|-----------|
| #1  | Inline happy `$x = 5$` | fixtures 62 (nopos) + 63 (default) |
| #2  | Display happy `$$\n\frac{a}{b}\n$$\n` | fixtures 66 (nopos) + 67 (default) |
| #3  | Currency demote — money prose `It costs $5 and they had $10` | fixture 70 + `TestTranslateCurrencyPostPassDemotesPredicateFailingInlineMath` |
| #4  | Currency adjacent — `Use $x$ and $y$.` | fixture 64 + Go-layer in translate_test.go |
| #4a | Currency convergence — `$5 and $x$` | fixture 71 + Go-layer |
| #4b | Currency divergence — `$ 5 and $x$` | fixture 72 + Go-layer |
| #5  | Unclosed display `$$\n\frac{a}{b}\n` (EOF) | fixtures 73 (nopos) + 75 (default) + `TestTranslateUnclosedDisplayMathDemotesToParagraphWithTwoTextChildren` |
| #6  | Unclosed inline `prose $x = 5 still prose` | fixture 74 + `TestTranslateUnclosedInlineMathLibraryHandlesNoCompensation` |
| #7  | Value preservation — `$$\n\ce{H2O}\n$$\n` | fixture 68 + `TestTranslateDisplayMathPreservesMhchemValue` |
| #8  | Mismatched braces inline — `$\frac{a}{b$` | **fixture 82 (NEW S07)** + `TestTranslateInlineMathMismatchedBracesRideThroughAsValue` (NEW S07) |
| #9  | In-block composition — list, blockquote, footnote `prose $x$ more` | fixtures 76, 77, 78 + three Go-layer tests in translate_test.go |
| #10a | Display in list — `- $$\n  x\n  $$\n` | fixture 79 + `TestTranslateDisplayMathInsideListItem` |
| #10b | Indented `$$x$$` → indented code | fixture 81 + `TestTranslateIndentedDollarDollarFallsToIndentedCode` |
| #11 | tableCell `$$x$$` → inlineMath | fixture 80 + `TestTranslateInlineMathInsideTableCellMatchesAsInlineMath` |
| #12 | Frontmatter + display math co-existence | fixture 69 + `TestTranslateMathAfterClosedFrontmatter` (S03 anchor) |
| #13 | `--no-position` strips math nodes uniformly | fixture pairs 62/63, 66/67 (default vs nopos byte-exact compare) + extended `TestEmitNoPositionStripsPositionKeyFromEveryNode` lossiness property test in S07 (math now in mdastNodeSetV1, math corpus inputs added — coverage half catches a per-node-type regression in either direction) |
| #14 | Library-contract A-vs-B equivalence (unclosed-`$$`-at-EOF) | `TestMathBlockLinesLastStopIsBehaviorallyIdenticalForOpenAndClosed` in `internal/parse/parse_test.go` (S05 anchor, not a CLI fixture per PRD §Testing Decisions fixture #14) |

All 14 PRD fixtures green at the full-suite count=1 run:

```
ok  	github.com/sunfmin/md2json	3.083s
ok  	github.com/sunfmin/md2json/internal/cli	1.180s
ok  	github.com/sunfmin/md2json/internal/emit	0.762s
ok  	github.com/sunfmin/md2json/internal/parse	1.936s
ok  	github.com/sunfmin/md2json/internal/read	1.551s
ok  	github.com/sunfmin/md2json/internal/translate	2.212s
```

`go vet ./...` clean. 102 named subtests PASS, 0 FAIL.

## Refactor pass

No duplication surfaced across S01-S07. S07 added one wire fixture, one
Go-layer translate test, two `mdastNodeSetV1` map entries, two corpus
rows. All additions land on existing structural patterns (per-fixture
testdata harness; one-Go-layer-test-per-acceptance-bullet pattern;
coverage-map-mirrors-doc-comment pattern). No new shared helpers, no
extracted abstractions, no `translate.go` / `emit.go` / `parse.go`
edits. Skip refactor.

## Final fixture inventory (math Run, S01-S07)

CLI wire fixtures (`testdata/fixtures/`):

```
62-inline-math-happy-path-nopos
63-inline-math-happy-path-default
64-inline-math-adjacent-text-siblings-nopos
65-inline-math-in-frontmatter-envelope-nopos
66-display-math-happy-path-nopos
67-display-math-happy-path-default
68-display-math-value-preservation-nopos
69-display-math-in-frontmatter-envelope-nopos
70-currency-rule-demotes-money-prose-nopos
71-currency-rule-greedy-match-convergence-nopos
72-currency-rule-greedy-match-divergence-nopos
73-unclosed-display-math-demotes-to-paragraph-nopos
74-unclosed-inline-math-rides-through-as-text-nopos
75-unclosed-display-math-demotes-to-paragraph-default
76-inline-math-in-list-item-paragraph-nopos
77-inline-math-in-blockquote-paragraph-nopos
78-inline-math-in-footnote-definition-paragraph-nopos
79-display-math-in-list-item-nopos
80-dollar-dollar-x-in-table-cell-matches-inline-math-nopos
81-indented-dollar-dollar-x-falls-to-indented-code-nopos
82-inline-math-mismatched-braces-rides-through-as-value-nopos  ← S07 new
```

Go-layer / parse-layer tests covering math (load-bearing only; full list
in each Sub-Stage's tdd-log):

- `internal/translate/translate_test.go`:
  `TestTranslateDisplayMathHappyPath`,
  `TestTranslateDisplayMathPreservesMhchemValue`,
  `TestTranslateCurrencyPostPassDemotesPredicateFailingInlineMath`,
  `TestTranslateCurrencyPostPassDoesNotDemoteValidInlineMath`,
  the two S04 convergence/divergence fixture-Go-mirrors,
  `TestTranslateUnclosedInlineMathLibraryHandlesNoCompensation`,
  the S05 closed/unclosed display tests,
  the four S06 composition tests,
  `TestTranslateInlineMathMismatchedBracesRideThroughAsValue`  ← S07 new.

- `internal/translate/lossiness_property_test.go`:
  `TestEveryEmittedTypeIsInMdastNodeSetV1` (math now in mdastNodeSetV1 + corpus  ← S07 edit),
  `TestLossinessCorpusCoversEveryV1NodeType` (now requires math coverage  ← S07 edit),
  `TestEveryEmittedTypeIsInMdastNodeSetV1DetectsOutOfSetTypes`.

- `internal/parse/parse_test.go`:
  `TestMathBlockLinesLastStopIsBehaviorallyIdenticalForOpenAndClosed` (PRD #14, S05 anchor).

## Acceptance check

- [x] **#1** Input `$\frac{a}{b$` produces one `paragraph` whose only
      child is `inlineMath{value:"\\frac{a}{b"}`; exit `0`; the
      unbalanced `{` rides through inside `value`. — fixture 82 byte-exact
      green; `TestTranslateInlineMathMismatchedBracesRideThroughAsValue`
      green.
- [x] **#2** Input `$$\n\ce{H2O}\n$$\n` (regression from S03) produces
      `math{value:"\\ce{H2O}\n", meta:null}`; mhchem source is not
      validated or expanded. — fixture 68 byte-exact green (S03 origin);
      `TestTranslateDisplayMathPreservesMhchemValue` still green.
- [x] **#3** PRD fixtures #1 through #14 all pass when the full slice
      is merged. — full-suite roll-up table above, all 14 pinned and
      green at `go test ./... -count=1`.
- [x] **#4** The existing lossiness property test passes with
      `*ast.InlineMath` / `*ast.Math` in the goldmark-AST tree-walk
      surface — both have first-class mdast targets, the silent-drop
      set for math is empty. — `mdastNodeSetV1` map now lists
      `inlineMath` and `math`; `lossinessCorpus` exercises both via
      `inline-math-happy-path` and `display-math-happy-path`;
      `TestLossinessCorpusCoversEveryV1NodeType` enforces it.
- [x] **#5** Exit code is `0` for every input above; stdout carries the
      JSON envelope, stderr is empty. — `exit` files for fixtures 68 and
      82 both contain `0`; `stderr` files are empty (zero bytes).

## Findings (TDD-time)

None. PRD fixture #8 landed free under the S02 inline-math wiring as the
PRD's verified library trace predicted (no library quirk surfaced on
mismatched-braces input — the library's inline matcher does not inspect
brace balance, consistent with transport-only posture). The
`mdastNodeSetV1` map drift S03 flagged closed cleanly with the two map
entries plus two corpus entries; the negative-direction RED probe
confirmed the coverage-half guard is non-vacuous.

VERDICT: accept
