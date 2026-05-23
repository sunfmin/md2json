# TDD log: 04-currency-rule-post-pass

Started: 2026-05-24

Framework: Go's stdlib `testing` (existing harness — `internal/translate/translate_test.go` for Go-layer assertions, `testdata/fixtures/<NN>-...` + `integration_test.go` for byte-exact CLI wire compares).

Scope: implement the translate-layer demote-only currency post-pass over `*mathjax.InlineMath`, per ADR-0004 Decision 3 and PRD §Implementation Decisions sub-point 2. No new ADR (decision is already pinned in ADR-0004).

## Tracer bullet: pre-implementation probe

Before any test, ran a throw-away probe under `internal/probe/` (deleted before commit) parsing the four PRD fixture inputs and dumping the goldmark AST. Confirmed the exact `*ast.Text` child-segment offsets quoted in the PRD:
- `It costs $5 and they had $10` → `[Text@[0,9), InlineMath{child@[10,25)}, Text@[26,28)]`. opener=9, closer=25.
- `$5 and $x$` → `[InlineMath{child@[1,9)}]`. opener=0, closer=9.
- `$ 5 and $x$` → `[InlineMath{child@[1,10)}]`. opener=0, closer=10. (Trim-halfspace did NOT fire because last char of interior is `$`, not space — confirms PRD #4b derivation.)
- `Use $x$ and $y$.` → `[Text@[0,4), InlineMath{child@[5,6)}, Text@[7,12), InlineMath{child@[13,14)}, Text@[15,16)]`.

Probe also confirmed wire shape pre-implementation: input #3 currently emits `[text, inlineMath, text]` — three children where the PRD wants one `text`. RED confirmed before any test.

## Test 1 — predicate-failing demote (PRD fixture #3, acceptance bullet #1)

- Wrote: `TestTranslateCurrencyPostPassDemotesPredicateFailingInlineMath` in `internal/translate/translate_test.go`. Asserts input `It costs $5 and they had $10` produces one paragraph with one text child `"It costs $5 and they had $10"`.
- RED: `paragraph.Children: got 3, want 1` (pre-implementation — library emits text/inlineMath/text).
- GREEN: added `currencyPostPass`, `currencyPredicatesPass`, `inlineMathSpan` to `translate.go`; wired the post-pass into `Translate` before `translateChildren`. Demote replaces `*mathjax.InlineMath` with `ast.NewTextSegment(text.NewSegment(opener_pos, closer_pos+1))`; existing offset-contiguity coalesce at `translate.go:226-232` folds the three text siblings into one.
- Notes: opener/closer position derivation walks leftward from first-child segment start across the `$` run, rightward from last-child segment stop across the `$` run — identical to the position-recovery logic `translateInlineMath` already used. Predicates: (i) `src[opener+1]` non-whitespace, (ii) `src[closer-1]` non-whitespace, (iii) `src[closer+1]` is EOF or non-digit. Demote-only — no re-promote, no re-scan.

## Test 2 — predicate-passing survive (PRD fixture #4, acceptance bullet #2)

- Wrote: `TestTranslateCurrencyPostPassDoesNotDemoteValidInlineMath`. Asserts `Use $x$ and $y$.` produces a 5-child paragraph: `[text "Use ", inlineMath "x", text " and ", inlineMath "y", text "."]`.
- RED: n/a — implementation already covered this; ran to confirm no regression.
- GREEN: passes on first run (post-Test-1 implementation).
- Notes: both `$x$` and `$y$` pass all three predicates; post-pass leaves both untouched. Regression guard against S02.

## Test 3 — greedy-match convergence (PRD fixture #4a, acceptance bullet #3)

- Wrote: `TestTranslateCurrencyPostPassConvergenceFixture`. Asserts `$5 and $x$` produces one paragraph with one `inlineMath{value:"5 and $x"}` child.
- RED: n/a — implementation already covered this; ran to confirm.
- GREEN: passes. The library's `inline.go:45` `line[i+1] != '$'` check makes the first inner `$` yield to the longer-run closer, so library emits a single InlineMath spanning the whole input; all three predicates pass against original source bytes (opener+1='5', closer-1='x', closer+1=EOF), so the post-pass does NOT demote.
- Notes: this is the pinned convergence trace — library+post-pass and pure remark-math both produce the same wire on this input.

## Test 4 — greedy-match divergence (PRD fixture #4b, acceptance bullet #4)

- Wrote: `TestTranslateCurrencyPostPassDivergenceFixture`. Asserts `$ 5 and $x$` produces one paragraph with one `text{value:"$ 5 and $x$"}` child; zero `inlineMath` on the wire.
- RED: n/a — implementation already covered this; ran to confirm.
- GREEN: passes. Library greedy-matches the whole input (asymmetric trim-halfspace does NOT fire because last interior byte is `$`, not space); predicate (i) fails on `src[opener+1]=' '`; post-pass demotes the whole span to text covering [0,11). Demote-only — the inner `$x$` is NOT re-promoted.
- Notes: this is the pinned divergence trace — pure remark-math would emit `[text "$ 5 and ", inlineMath "x"]`; library+post-pass emits a single text. Any future change that produces the pure-remark-math shape will fail this test explicitly and trigger an ADR-0004 reopen.

## Test 5 — display math untouched (acceptance bullet #5)

- Wrote: `TestTranslateCurrencyPostPassDoesNotTouchDisplayMath`. Asserts input `$$\n x\n$$\n` (leading-whitespace body — would fail predicate (i) if inline rules were applied) produces a `math{value:" x\n"}` node, NOT demoted to text.
- RED: n/a — implementation only touches `*mathjax.InlineMath`, not `*mathjax.MathBlock`; test confirms the type-narrowing is correct.
- GREEN: passes on first run.
- Notes: pins CONTEXT.md `remark-math currency rule` clause "Display `$$...$$` has no such guard". The post-pass type-checks `*mathjax.InlineMath` and is a no-op for `*mathjax.MathBlock`.

## CLI wire fixtures (byte-exact compare)

Three new fixtures under `testdata/fixtures/`, each a one-line input with `--no-position` args and the exact JSON envelope on stdout:
- `70-currency-rule-demotes-money-prose-nopos/` — input `It costs $5 and they had $10` → `[paragraph[text "It costs $5 and they had $10"]]`.
- `71-currency-rule-greedy-match-convergence-nopos/` — input `$5 and $x$` → `[paragraph[inlineMath "5 and $x"]]`.
- `72-currency-rule-greedy-match-divergence-nopos/` — input `$ 5 and $x$` → `[paragraph[text "$ 5 and $x$"]]`.

All three pass under `go test .` (TestFixtures sub-tests).

## Refactor pass (post-green only)

`translateInlineMath` had duplicated the opener/closer-walk position-recovery logic. Extracted as `inlineMathSpan(im, src)` and replaced both call sites (the post-pass demote target and the position-span on the surviving mdast node). One named seam for "given an InlineMath, what byte range does its full `$...$` source span?". All tests still green after refactor.

## Final

- Tests added: 5 Go-layer (`TestTranslateCurrencyPostPass*`) + 3 CLI fixtures (70/71/72).
- Tests passing: 5/5 Go-layer; full suite (`go test ./...`) green across all packages.
- Acceptance status:
  - [x] criterion 1 — `It costs $5 and they had $10` → one paragraph, one text child, zero inlineMath. (`TestTranslateCurrencyPostPassDemotesPredicateFailingInlineMath` + fixture 70.)
  - [x] criterion 2 — `Use $x$ and $y$.` → two inlineMath nodes; regression against S02 held. (`TestTranslateCurrencyPostPassDoesNotDemoteValidInlineMath` + existing fixture 64.)
  - [x] criterion 3 — `$5 and $x$` → one inlineMath `{value:"5 and $x"}`. (`TestTranslateCurrencyPostPassConvergenceFixture` + fixture 71.)
  - [x] criterion 4 — `$ 5 and $x$` → one paragraph with `[text "$ 5 and $x$"]`; zero inlineMath. (`TestTranslateCurrencyPostPassDivergenceFixture` + fixture 72.)
  - [x] criterion 5 — display `$$...$$` not touched. (`TestTranslateCurrencyPostPassDoesNotTouchDisplayMath` + existing display-math fixtures 66/67/68/69 still green.)

VERDICT: accept.
