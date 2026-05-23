# critique-issues transcript

Target: queue/ (7 issues numbered 01-07)

## Round 1

### Critic

1. **S04 acceptance bullet 5 unobservable.** "The post-pass is demote-only: no recursive rescan of demoted bytes, no re-promotion, no second pass over the post-demote tree." Negative-property impl claim, not wire-observable from a JSON envelope. TDD cannot fail on "second pass happened internally" — only on a wire shape. Either drop the bullet or restate as a wire-observable consequence (e.g., reference PRD fixture #4b's specific divergence shape, which IS the externally observable manifestation of demote-only).

2. **S05 acceptance bullet 6 unobservable adjective.** "The unclosed-`$$` predicate is a forward source-byte scan after `Lines().Last().Stop`, skipping LF/blank lines until a non-blank line is found; deterministic, no heuristic." Implementation prescription, not behavior. "Deterministic, no heuristic" is an adjective not a fixture. The wire-observable content of this rule lives in bullets 1-2 + 4. Drop bullet 6 or fold its content into a property-test bullet that asserts "for any input matching pattern X, output is Y" — observable.

3. **S01 bullet 5 release-engineering, not TDD.** "The resulting binary is still a single static Go executable; no new non-Go runtime is introduced." Not testable inside a TDD cycle that operates on Go packages; verifiable only by inspecting `go.mod` deps + a build step. Belongs in a separate release-gate check, or restate as "`go.mod` introduces no `cgo`-requiring dependency" if that is the actual observable.

4. **S07 bullet 3 conjunctive 14-fixture roll-up — split or restate.** Fourteen fixtures concatenated by "and" violates the >2-ands split heuristic. If S02–S06 have already pinned fixtures #1–#13 individually, S07's role collapses to (a) NEW fixture #8 (mismatched braces inline), and (b) integration assertion that all-fixtures-pass-together. Restate bullet 3 as "PRD fixtures #1 through #14 all pass when the full slice is merged; this issue introduces no new fixtures beyond #8 and #14-coverage that S07 itself owns." Otherwise S07 reads as "rerun everything" and adds no testable contract a CI green wouldn't already give.

5. **S04 / S05 leak library symbols into acceptance bullets.** `*ast.InlineMath`, `MathBlock`, `Lines().Last().Stop` appear in acceptance text. CONTEXT.md `AST (output) / mdast` _Avoid_ clause says "goldmark AST (implementation detail, not contract)." For S05 bullet 4 (the in-process library behavior unit test, PRD fixture #14), library symbols are load-bearing — that bullet pins library behavior by design. But S04's "each inline-math match the library emits" in the story paragraph, and S05's bullet 6 referencing `Lines().Last().Stop` outside the library-behavior bullet, drift goldmark internal names into the contract layer. Restrict library-symbol mentions to S05 bullet 4 only; everywhere else speak wire shapes.

### VERDICT: continue

## Round 2

### Critic

Round-1 fixes validated against queue/{01,04,05,07}/issue.md:

1. S04 bullet 5 — old negative-property "demote-only" gone. New bullet 5 wire-observable; bullet 4 pins PRD #4b's `$ 5 and $x$` divergence shape. Holds.
2. S05 bullet 6 dropped. Library symbols (`MathBlock`, `Lines().Last().Stop`) confined to bullet 4, the PRD #14 library-contract pin. Holds.
3. S01 bullet 5 now reads `go mod tidy` + `grep -l cgo $(go list -deps -f '{{.Dir}}' ./...)` — observable command-level acceptance. Holds.
4. S07 bullet 3 restated as merge-time #1-#14 roll-up with explicit S07-owned carve-out (#8 mismatched-braces + #14 lossiness property test). Holds.
5. S04 acceptance has zero library-symbol leakage. S05 confines library names to load-bearing bullet 4. S07 bullet 4's `*ast.InlineMath`/`*ast.Math` mention is grounded in CONTEXT.md `Lossiness policy (goldmark → mdast)` — load-bearing tree-walk surface assertion, not contract drift. Holds.

No new defects.

### VERDICT: accept
