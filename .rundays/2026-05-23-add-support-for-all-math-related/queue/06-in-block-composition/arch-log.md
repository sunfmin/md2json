# Arch log: 06-in-block-composition (mini)

Started: 2026-05-24T00:00:00Z
Scope: mini
File set (from S06 tdd-log "Final"):
- internal/translate/translate.go (~5 LoC inside `displayMathClosed`)
- internal/translate/translate_test.go (+6 tests, +1 helper `assertNoMathNodeAnywhere`)
- testdata/fixtures/76..81 (6 wire fixtures)

## Baseline

- Tests: `go test ./...` from /Users/sunfmin/Developments/md2json — 6 packages PASS
  - github.com/sunfmin/md2json
  - github.com/sunfmin/md2json/internal/cli
  - github.com/sunfmin/md2json/internal/emit
  - github.com/sunfmin/md2json/internal/parse
  - github.com/sunfmin/md2json/internal/read
  - github.com/sunfmin/md2json/internal/translate
- 0 failing. (S06 GREEN per tdd-log.)

## Survey

Per the proposer-arch lenses (glossary alignment / module depth / naming clarity / locality), walked the S06 touched files looking for friction. Findings:

### Candidate A: extract `skipLeadingASCIISpacesTabs` helper out of `displayMathClosed`

- Code: `for j < len(line) && (line[j] == ' ' || line[j] == '\t') { j++ }`
- Score: **Speculative**. One call site. Two-line inline loop is already English-readable with the comment immediately above it. Extracting would be a name without depth — interface ~= impl. Fails the depth test ("one adapter = hypothetical seam"). No locality gain (no scattered duplicates).
- Action: none.

### Candidate B: extract `countDollarRun` helper

- Code: `for k < len(line) && line[k] == '$' { k++ }`
- Score: **Speculative**. Same shape as A. Single call site. No depth. No locality.
- Action: none.

### Candidate C: deletion test on `assertNoMathNodeAnywhere`

- Caller count: 1 non-recursive call site (line 1681) plus the helper's own recursive self-call (line 1693). Total 12 lines incl. brace.
- Deletion test: if removed, the test body would have to inline the recursive walk. Cannot collapse to "three lines" — recursion over `*Node.Children` needs either a closure or a helper. Inlining via closure clutters the test and obscures the assertion intent. Helper's interface (one call, one node) is meaningfully smaller than its impl (recursive walk + t.Helper + per-node type check).
- Per CONTEXT.md the wire schema is mdast and `math` vs `inlineMath` is load-bearing; an assertion named "no math node anywhere" is a primitive that pays its keep on its naming alone.
- Score: **not a candidate**. Earns its keep.
- Action: keep as-is.

### Candidate D: variable renames inside `displayMathClosed` (i / j / k → e.g. `cursor` / `afterIndent` / `afterDollars`)

- Score: **Speculative, style-only**. The proposer-arch guard "Refactor on style alone" rejects this. Each loop has an English comment immediately above; the `j` / `k` indices are tightly scoped (function-local, ~10 lines).
- Action: none.

### Candidate E (raised by user prompt): one-line ADR-0004 clarification for indented closer tolerance

- Decision 5 text: "walk forward, skip LF/blank lines, check whether the next non-blank line consists of two-or-more `$` chars followed by a (whitespace-only) tail."
- S06 fix added a leading-whitespace skip before counting the `$`-run.
- Reading: Decision 5's phrasing is silent on leading-whitespace on the closer line. Strict reading says "the line consists of `$` chars + whitespace tail" — under that reading `  $$\n` does NOT match (leading spaces are before, not after, the `$`-run). The S06 fix is therefore a refinement of Decision 5's predicate, not a contradiction.
- Per proposer-arch's "ADR conflicts" guidance: "Friction must be substantial" before editing an ADR; "Don't re-litigate casually." The S06 fix is fixture-pinned (`testdata/fixtures/79-display-math-in-list-item-nopos/`), source-commented (translate.go lines 627-635 explain the indented-closer case + cite the listItem fixture by name + bytes), and tdd-log-Test-4-documented (with the full case-analysis safety argument).
- Score: **Worth exploring, but not Strong.** A one-line ADR clarification would be cheap insurance for future maintainers, but the three existing references (source comment + tdd-log Test 4 + fixture 79) already cover the discovery path. The refinement does not contradict Decision 5; a future maintainer who reaches the predicate from Decision 5 will land on lines 627-635 which name the case explicitly.
- Action: none. The predicate behavior change is captured in the source + tdd-log + wire fixture; no ADR-0004 amendment.
- Note for future: if a second nested-context refinement of `displayMathClosed` lands (e.g. tab-tolerance edge cases, or a multi-paragraph unclosed-`$$` scope expansion that interacts with the indented-closer rule), bundle that work with a Decision 5 clarification then.

## Pass selection

No Strong candidates this pass.

Per proposer-arch §2: "No Strong → 'no strong candidates this pass' + VERDICT: accept. Legitimate."

## Final

- Tests: 6 packages PASS (unchanged from baseline). No code edited; no revert needed.
- LOC delta: 0.
- Most consequential observation: `assertNoMathNodeAnywhere` is a deep-enough test helper to earn its keep on a single call site, because the recursive mdast walk it encapsulates would clutter the test body if inlined; the helper's name is also a load-bearing wire-contract assertion ("GFM cells are inline-only; no `math` node anywhere"). And the S06 `displayMathClosed` indented-closer tolerance is a refinement of ADR-0004 Decision 5, not a contradiction, so no ADR amendment is warranted — the three existing references (source comment, tdd-log Test 4, fixture 79) cover the discovery path.

## Flagged terms

None. CONTEXT.md vocabulary (`math`, `inlineMath`, `display math`, `Unclosed-display-math fall-through rule`, `closing fence`) is exercised exactly in the S06 source comments + tdd-log; no `_Avoid_` synonyms appeared in the touched code.

VERDICT: accept
