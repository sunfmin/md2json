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

### Proposer-PRD

Revision applied. Six defects addressed.

1. **ADR-0004 phantom — resolved by writing the ADR.** Authored `/Users/sunfmin/Developments/md2json/docs/adr/0004-math-extension-library.md` as the sibling output grill-0 Round 3 said `to-prd` Stage owes. The ADR carries: library pick (`github.com/litao91/goldmark-mathjax`), wiring style (appended to `parse.New`'s extension list per ADR-0002 §"no central registry, by convention"), goldmark-side `ast.InlineMath`/`ast.Math` names (kept out of CONTEXT.md per PO direction), translate-layer unclosed-`$$` compensation, no-runtime-toggle posture, and explicit supersede of ADR-0002's `Math ($...$, $$...$$) extensions. PRD non-goal.` bullet. PRD's seven references to ADR-0004 now point at a real file.

2. **Fixture #8 (mismatched braces) — sharpened to an exact tree.** Was "either emits `inlineMath` whose `value` carries the broken LaTeX or falls through to `text`." Now: `root.children = [paragraph.children = [inlineMath{value: "\\frac{a}{b"}]]`. Derivation pinned in fixture body: opening `$` is followed by `\` (non-whitespace, non-`$`) so the remark-math rule opens; closing `$` is preceded by `b` (non-whitespace) and followed by nothing (no digit) so the rule closes; the unbalanced `{` rides through inside `value` per **Text/Code value preservation**. No disjunctive acceptance remains.

3. **Fixture #11 + Impl Decision §"In-block composition" (table cell) — sharpened to an exact tree.** Was "produces `text` (and possibly `inlineMath` from the inline matcher depending on the exact span)." Now: fixture asserts the `tableCell` contains exactly `[text{value: "$$x$$"}]`, with zero `math` and zero `inlineMath` nodes anywhere under the table. Impl Decision body now pins the derivation: GFM tables are inline-only so the block parser does not fire, and the library's inline matcher distinguishes `$$` from inline `$...$` (yields when the character after the opening `$` is another `$`), so both decline and the bytes fall through to `text`. "Possibly" / "depending on cell contents" removed.

4. **Fixture #14 (library contract test) — split into a pinned-tree wire fixture (#5) and a library-behavior unit test (#14).** Fixture #5 is now the wire-contract assertion: input `$$\n\frac{a}{b}\n` produces `root.children = [paragraph.children = [text{value: "$$\n\\frac{a}{b}\n"}]]` (post-translate-compensation). Fixture #14 is now a focused library-behavior unit test asserting that `litao91/goldmark-mathjax` emits a partial `ast.Math` on the same input — this is the upstream assumption ADR-0004 Decision 5 and `translate`'s compensation rest on. The two are no longer tangled: fixture #5 holds the wire contract, fixture #14 holds the library assumption, and a future library-behavior change makes the unit test fail (explicit ADR-0004 reopen) rather than silently breaking the wire fixture.

5. **Vocab drift — "single-source-of-truth registry function" removed.** PRD now uses ADR-0002's verbatim phrasing: "**`parse.New` is the single function by convention, with no central registry**, per ADR-0002 §'Negative (no central registry)'." Both Impl Decisions §"Extension library pick" and the §"Module touch-set" `parse` bullet now speak in terms of "appended to `parse.New`'s extension list" rather than invented "registry function" vocabulary.

6. **Stray `VERDICT: accept` in PRD body — removed.** PRD no longer carries a VERDICT line in its artifact body.

### VERDICT: accept
