# S04: enforce remark-math currency rule via translate-layer demote-only post-pass

Status: ready-for-agent

A writer mentioning money — "It costs $5 and they had $10" — sees the document round-trip as ordinary prose with zero `inlineMath` nodes on the wire. The three remark-math predicates (opener-followed-by-non-whitespace, closer-preceded-by-non-whitespace, closer-not-followed-by-digit) decide membership at the translate layer as a demote-only post-pass over each `inlineMath` node produced for the input: predicate-failing nodes collapse back to `text` covering the full original `$...$` range, and adjacent contiguous-by-offset `text` siblings coalesce into a single `text` node. Predicate-passing nodes survive unchanged. The post-pass is demote-only — once a span is demoted to `text` it is not re-scanned for an inner valid `$...$`, accepting one bounded divergence from pure remark-math on inputs with a leading-whitespace opener.

## Acceptance

- [ ] Input `It costs $5 and they had $10` produces one `paragraph` whose only child is `text{value:"It costs $5 and they had $10"}`; zero `inlineMath` nodes; exit `0`.
- [ ] Input `Use $x$ and $y$.` continues to produce two `inlineMath` nodes (predicates pass on both matches); regression against S02 is held.
- [ ] Input `$5 and $x$` produces one `paragraph` whose only child is `inlineMath{value:"5 and $x"}` (library greedy-match passes all three predicates against the original source bytes; no demote).
- [ ] Input `$ 5 and $x$` produces one `paragraph` whose `children` is exactly `[text{value:"$ 5 and $x$"}]` — the inner `$x$` is NOT re-promoted to `inlineMath`; the demote-only post-pass leaves no `inlineMath` node on the wire for this input.
- [ ] Display `$$...$$` matches are not touched by the post-pass (predicates apply to inline math only).
