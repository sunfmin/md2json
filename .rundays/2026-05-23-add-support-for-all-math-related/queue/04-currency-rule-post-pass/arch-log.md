# Arch log: 04-currency-rule-post-pass

Started: 2026-05-24
Scope: mini
File set (from tdd-log.md refactor pass): `internal/translate/translate.go` — the ~30 LoC added by S04 (`currencyPostPass`, `currencyPredicatesPass`, `inlineMathSpan`, `isASCIIWhitespace`, `isASCIIDigit`).

## Baseline

- Tests: 252 passing, 0 failing (`go test ./... -count=1`, all six packages green: `md2json`, `internal/cli`, `internal/emit`, `internal/parse`, `internal/read`, `internal/translate`).

## Candidates (deletion-test on each S04 addition)

1. `inlineMathSpan` — **2 callers** (`currencyPredicatesPass`, `translateInlineMath`). Real seam (two adapters). Earned. Naming is the friction: "span" is generic; the function's load-bearing semantic is that the returned range *includes the `$` delimiter runs*, not just the interior. CONTEXT.md `inlineMath node` makes "delimiters" a load-bearing word ("byte-for-byte, delimiters stripped" is the `value` rule; the position rule is the inverse — delimiters included). The existing translate-internal naming family is `childrenSpan`, `textChildrenSpan`, `spanWithDelimiter`. `inlineMathDelimitedSpan` slots into that family AND pins the delimiter-inclusion semantic in the name. → **Strong** (naming clarity + glossary alignment).

2. `currencyPredicatesPass` — 1 caller (`currencyPostPass`). One-adapter ⇒ hypothetical seam by the two-adapter rule. BUT deletion test: inlining the 12-line predicate body into `currencyPostPass` would entangle "should this match demote?" with "walk children, collect, replace" — two concepts in one function. The name groups the three remark-math predicate checks under a single load-bearing label that the CONTEXT.md entry explicitly enumerates ("(i)/(ii)/(iii)"). Locality argues to keep it. → **Worth exploring** (note only — leaving as-is).

3. `isASCIIWhitespace` / `isASCIIDigit` — 1 caller each (`currencyPredicatesPass`). Bodies are 1-line. Inline would save 4 lines. Loses the CommonMark-whitespace naming hook (CommonMark's whitespace set is the load-bearing concept, not just "any of these six bytes"). → **Speculative** (note only — leaving as-is).

4. Three predicate checks → table/closure? The three checks read different bytes (`opener+1`, `closer-1`, `closer+1`) with different rules (whitespace vs digit) and different EOF-handling (predicate (iii) PASSES at EOF; predicates (i) and (ii) FAIL at OOB). They are not three instances of one shape; they are three distinct rules that happen to live next to each other. A table-driven collapse would obscure the per-rule differences and the per-rule comments. → **Speculative** (no action — collapsing here would *reduce* clarity).

5. `currencyPostPass` wiring in `Translate` — single line call `currencyPostPass(doc, src)` directly inside `Translate`, with a paragraph-long comment citing ADR-0004 Decision 3 + CONTEXT.md `remark-math currency rule`. Symmetric with the existing footnote pre-pass call `collectFootnoteLabels(doc, pt)` on the line above. Not a shallow seam — the wiring point IS the natural anchor for "after parse, before translateChildren, the goldmark doc gets two pre/post-walks". → **No friction**.

## Pass 1: rename `inlineMathSpan` → `inlineMathDelimitedSpan`

- Files touched: `internal/translate/translate.go` (3 call sites + 1 declaration + 1 cross-reference comment).
- Rationale (lens-by-lens):
  - **Glossary alignment**: pins "delimiters included" semantic against CONTEXT.md `inlineMath node` ("delimiters stripped" is the *interior* rule; this function returns the *delimited* range). The word "delimited" is load-bearing.
  - **Naming clarity**: removes the bare `Span` ambiguity (could mean interior, could mean delimiter-included). The qualifier makes it self-explaining.
  - **Locality (consistency)**: slots into the existing in-file naming family — `childrenSpan`, `textChildrenSpan`, `spanWithDelimiter`. The qualifier-before-`Span` pattern is now repeated three times in the same file, which is the right amount of repetition for an established convention.
  - **Module depth**: unchanged (same body, same interface shape).
- Doc-comment also updated to spell out the two-caller rationale (`currencyPredicatesPass` + `translateInlineMath`), pinning why the seam earns its keep.
- Tests after: 252 passing (`go test ./... -count=1`, all six packages green).
- Reverted? no.

## Final

- Tests: 252 passing (unchanged from baseline).
- LOC delta: +14 (function doc-comment expanded; no code-line additions, only renames + comment growth).
- Most consequential change: `inlineMathSpan` → `inlineMathDelimitedSpan`. The new name pins the load-bearing semantic — the returned range *includes* the `$` delimiter runs, mirroring (inverse of) CONTEXT.md `inlineMath node`'s "delimiters stripped" rule for `value`. Slots into the in-file `*Span` naming family alongside `childrenSpan`, `textChildrenSpan`, `spanWithDelimiter`.
- No new ADR (rename is a naming refinement; no architectural decision pinned).
- No CONTEXT.md edits (no glossary terms added or shifted; the rename internal to `translate` does not surface on the wire).

VERDICT: accept
