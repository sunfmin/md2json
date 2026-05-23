# Arch log: 01-wire-math-extension

Started: 2026-05-23
Scope: mini
File set (from tdd-log):
- `internal/parse/parse.go`
- `internal/parse/parse_test.go`
- `go.mod` / `go.sum`
- `testdata/fixtures/61-smoke-non-math-gfm-blog-post-nopos/`

## Baseline

- `go test ./...` — all six packages green (md2json root, internal/cli, internal/emit, internal/parse, internal/read, internal/translate). 0 failing.
- LOC: not tallied; mini scope, single-issue diff is small (one import + one `append` arg + slice-cap bump in `parse.go`, plus one test + one helper in `parse_test.go`, plus the smoke fixture).

## Friction scan (deletion test per candidate)

1. **`newGoldmarkWith` (the v1 base-extension-set seam).** Concentrates `GFM + Footnote + math` plus the caller-supplied extras into a single function called by `New` and `newWithoutFrontmatter`. Deletion test: removing it forces both callers to spell the base set by hand and re-coordinate registration order — exactly the maintainer hazard ADR-0002 §"Negative (no central registry)" calls out and ADR-0004 Decision 2 explicitly leans on ("appended to that function's extension list. No new wiring mechanism is introduced."). Two real adapters use the seam (with-frontmatter via `New`, without-frontmatter via `newWithoutFrontmatter`) → real seam, not hypothetical. Earned its keep. Not shallow.

2. **`mathjax.NewMathJax()` registration line.** A single `append` arg in `newGoldmarkWith`, with a ~10-line comment citing ADR-0004 Decisions 1+2 and a forward-pointer to S02/S03. Glossary-aligned: per ADR-0004 Decision 4, the goldmark-side type names (`*ast.InlineMath`, `*ast.Math`) and the library identity live in the ADR, not in CONTEXT.md (which speaks only wire-contract terms `inlineMath` / `math` / `Dollar-sign math (transport-only)`). The comment correctly stays at the implementation layer. No drift.

3. **`containsNodeKind` test helper.** Six-line recursive preorder, one caller (`TestParseRegistersMathExtension`). Could be inlined into the test. Doesn't move glossary alignment, module depth, naming clarity, or locality — style only. Skip per rules ("Refactor on style alone" forbidden).

4. **`ast` import in `parse_test.go`.** Brought in solely for the `ast.Node` parameter type of `containsNodeKind`. Style; same disposition as above.

5. **Slice-cap literal `3+len(extras)` in `newGoldmarkWith`.** Tracks the three base extenders (GFM, Footnote, math). Could be derived (named const, runtime `len`), but the cap is a hint, not a contract — caller observes nothing different. Style.

6. **Smoke fixture name `61-smoke-non-math-gfm-blog-post-nopos`.** "nopos" tag matches CONTEXT.md `v1 flags` (`--no-position`); "smoke" / "non-math" / "gfm-blog-post" are scope tags consistent with sibling `*-nopos` fixtures. Glossary-aligned.

7. **Comment block on `newGoldmarkWith`.** ~12 lines of comment for a 3-line body. But it concentrates the load-bearing ADR-0004 references and the S02/S03 forward-pointer at the point of registration — that is the locality lens (knowledge concentrated where the change-prone bytes live), not bloat. Not a refactor target.

## Scoring

- `newGoldmarkWith` seam: **Worth exploring** (consider whether the "base set + extras varargs" shape should become a typed `[]goldmark.Extender` value plus a single `goldmark.New(WithExtensions(...))` call to flatten the indirection — but the indirection IS the seam ADR-0002 + ADR-0004 pin; tearing it down would re-litigate ADR-0002 §"Negative" without new information). Note for future, do not act this pass.
- All other candidates: **Speculative** or **style-only**.

No Strong candidates this pass.

## Pass count

Zero passes. No refactor applied. No tests re-run (no change to validate).

## Final

- Tests: 6/6 packages passing (unchanged from baseline).
- LOC delta: 0.
- Most consequential change: none. The S01 wiring slice touched a small, ADR-pinned seam (`newGoldmarkWith`) in the exact shape ADR-0004 Decision 2 prescribes; the resulting code is already at the right depth and naming. Refactoring on style alone is out of scope per the proposer-arch rules. Forward-pointer for the next arch pass (post-S03, once `translate` carries the currency-rule demote-only post-pass and the unclosed-`$$` compensation per ADR-0004 Decisions 3 and 5): re-examine whether the two compensations in `translate` constitute a real seam (two adapters) that should be lifted into a named submodule, or whether they remain inline branches.

VERDICT: accept
