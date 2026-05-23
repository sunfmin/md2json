# critique-prd transcript

Target: `/Users/sunfmin/Developments/md2json/.rundays/2026-05-23-add-support-for-all-math-related/prd/prd.md`

## Round 1

### Critic

1. **ADR-0004 phantom.** PRD references ADR-0004 7+ times (library pick home, ADR-0002 math-bullet superseder, home for goldmark-side `ast.InlineMath`/`ast.Math` names). Grill Round 3 explicitly: "ADR-0004 (library pick + supersede of ADR-0002 math bullet) is `to-prd` Stage's authoring responsibility, not this Stage's." Disk check: `/Users/sunfmin/Developments/md2json/docs/adr/` contains 0001, 0002, 0003 only — no 0004. PRD asserts the supersede has happened ("ADR-0002's math bullet... is superseded by ADR-0004") and that rule-vs-implementation split is settled, but the artifact carrying both is missing. Either ADR-0004 is a parallel output of this Stage that the proposer forgot to emit, or PRD asserts state that does not exist on disk. Load-bearing reference to unwritten artifact.

2. **Fixture #8 (mismatched braces) — disjunctive acceptance.** "exits `0` and either emits an `inlineMath` whose `value` carries the broken LaTeX or falls through to `text`." Either/or is unobservable. Tests cannot assert "one of these two trees." Either split into two fixtures (one per branch with pinned tree), or run litao91 on `$\frac{a}{b$` once and pin the actual output now. As written, this fixture passes for any implementation, including a broken one returning empty.

3. **Fixture #11 + Impl Decision §"In-block composition" (table cells) — non-deterministic input.** Fixture #11: "produces `text` (and possibly `inlineMath` from the inline matcher depending on the exact span)." Impl Decision: "either land in `text.value` or get consumed in part by the inline `$...$` matcher depending on cell contents." Input `$$x$$` in a cell is fully specified — output should be one specific tree. "Possibly" / "depending on cell contents" is unobservable. Contradicts grill Round 3 audit claim that in-block composition is "derivable from existing entries" — not derivable if PRD itself shrugs.

4. **Fixture #14 (library contract test) — circular acceptance.** "if the library's reported behavior diverges from 'decline to match,' the test fails and `translate`'s demote-to-prose compensation is invoked." Pass criterion depends on whether `translate` compensates; compensation is conditional on test result. No pinned tree. Should be: run litao91 on the unclosed-`$$` input now, record actual library behavior in PRD, then write one deterministic fixture asserting the final mdast tree (post-compensation if needed). As written, "test fails" and "compensation invoked" are tangled.

5. **Vocab drift — "single-source-of-truth registry function".** PRD Impl Decisions §Extension library pick: "Wiring goes through the existing single-source-of-truth registry function in `parse` (the one ADR-0002 §'no central registry' maintainer-hazard note is addressed by)." ADR-0002 explicitly: "Negative (no central registry). The choice of 'which extensions are enabled' is a single function (`parse.New`) by convention." PRD invents "single-source-of-truth registry function" — contradicts ADR-0002's own naming ("no central registry, by convention"). Use ADR-0002 vocabulary verbatim or this is fresh terminology absent from CONTEXT.md.

6. **Stray `VERDICT: accept` in PRD body** (line 119). PRDs do not carry VERDICTs — VERDICTs belong to grill / critique transcripts. Proposer leaked a transcript-style closer into the artifact. Cosmetic but indicates Stage-output conflation.

### VERDICT: continue
