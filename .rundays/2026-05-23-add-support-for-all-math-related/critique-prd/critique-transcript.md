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

## Round 2

### Critic

1. **ADR-0004 Decision 5 pins library behavior that has never been observed.** ADR-0004 §Decision 5 asserts as fact: "The library's block parser on an opening `$$` with no closing `$$` before EOF emits a partial `ast.Math` whose interior is the scanned body (it does not decline-to-match; it does not hard-error)." Grill Round 2 A7 explicitly said "behavior of `litao91/goldmark-mathjax` on unclosed `$$` must be **verified at TDD time** to in fact be (a) — if the library emits a partial `ast.Math` or hard-errors, that is a TDD-blocking finding and the `translate` layer must compensate." Grill kept three live branches (decline-to-match / partial / hard-error). ADR-0004 collapses to one branch without evidence. PRD §Implementation Decisions `translate` bullet then bakes the compensation in unconditionally, and Testing Decisions §fixture #14 calls it "the upstream assumption ADR-0004 Decision 5 rests on" — i.e., the artifact stack admits the assumption is unverified. Round 1 defect #4 explicitly asked: "run litao91 on the unclosed-`$$` input now, record actual library behavior in PRD." Proposer split fixtures #5 and #14 (cosmetic separation) but never ran the library. Both fixtures still rest on the same unverified premise; if the library declines-to-match (the simpler outcome explicitly named in grill A7), the compensation is dead code, fixture #14 fails on day one of TDD, and ADR-0004 reopens immediately.

2. **Fixture #5 wire shape `text{value: "$$\n\\frac{a}{b}\n"}` is incoherent with prose handling.** PRD pins a single `text` node carrying an embedded LF spanning two source lines (`$$` line + `\frac{a}{b}` line). CONTEXT.md `Text/Code value preservation` governs `text.value` as "the literal textual run from the normalized source" but does not pin whether a single-paragraph multi-line prose run lands as one `text` node or splits at soft-line-breaks. The rest of md2json's wire contract (CONTEXT.md `mdast node-set v1`, `break` entry) names `break` for hard line breaks but is silent on soft-break handling. Fixture #5 asserts one shape for the demoted-prose case without any fixture pinning the analogous shape for ordinary two-line prose paragraphs elsewhere in the corpus. Either pin soft-break handling globally in CONTEXT.md / a prose-shape fixture this PRD references, or split fixture #5's expected children list to mirror whatever shape ordinary two-line `paragraph` text takes (e.g., two `text` nodes, or one with embedded LF). As written, fixture #5 manufactures a wire shape orthogonal to the rest of the spec.

3. **`translate` compensation pre-condition is unobservable from the goldmark AST description given.** ADR-0004 Decision 5 + PRD §`translate` bullet describe the compensation trigger as "the missing-closing-fence shape" / "block math reaching EOF without a closing-fence sentinel" / "single AST-shape check, not heuristic." Neither artifact says what the AST-shape check actually inspects on `ast.Math`. Does the library set a `Closed bool` on the node? Does it leave the end-position pointing at EOF rather than a `$$` line? Is there an unterminated-segment field? Without a named field/predicate, "single AST-shape check" is hand-waved. Fixture #14 is supposed to pin the library's behavior, but its current body only asserts "an `ast.Math` node whose interior covers the body bytes and which has no closing-fence sentinel" — the term "closing-fence sentinel" is invented here and undefined. A future maintainer reading PRD + ADR-0004 cannot write the compensation predicate from the artifacts alone.

4. **PRD §Implementation Decisions §"Currency disambiguation = library-implemented" assumes byte-identical fidelity with no observation.** PRD asserts: "The chosen extension implements the **remark-math currency rule** byte-identically." ADR-0004 Decision 3 repeats: "implements the remark-math rule byte-identically." Grill Round 2 A4 PO: "implements the Q5(a) remark-math currency rule byte-identically." Same chain of assertion-from-assertion; no observation pinned. CONTEXT.md `remark-math currency rule` ends with: "The chosen goldmark math extension must implement this rule; if it does not, that is an extension-pick blocker, not a rule reopen." Same level of risk as defect #1: the load-bearing pick criterion is unverified. At minimum, fixture #3 (`It costs $5 and they had $10`), fixture #4 (`Use $x$ and $y$.`), and a `$ x = 5 $` (whitespace-after-opening) fixture all need to be TDD-blocking on the library's actual behavior, with a documented falsification path ("if any of these fixtures fail against litao91, ADR-0004 reopens") — Proposer added fixture #3 + #4 but did not name them as library-pick blockers.

5. **Fixture #8 derivation contradicts the remark-math currency rule's closing-`$` precondition.** PRD fixture #8 derivation: "opening `$` is followed by `\` (non-whitespace, non-`$`) — match opens; closing `$` is preceded by `b` (non-whitespace) and followed by nothing (no digit) — match closes." Granting fidelity to remark-math, the JS implementation has multi-line and balanced-brace edge cases not captured by the four conditions named in CONTEXT.md. Fixture #8's "exit `0` with `inlineMath{value: "\\frac{a}{b"}`" is plausible but unverified; another plausible outcome under the same `remark-math` rule text is "no match, falls through to `text`." Disjunctive acceptance was the Round 1 defect; replacing it with one of the two branches asserted-without-evidence is not a fix — it is hidden disjunction.

### VERDICT: continue

## Round 3

### Critic

(Post-triggered-grill, post-branch-(c)-collapse, post-library-probe.)

1. **Predicate (i) drift — "non-`$`" clause is phantom; breaks fixture #11.** PRD §Implementation Decisions §`translate` sub-point 2 states predicate (i) as "**opener-followed-by-non-whitespace-non-`$`** — the byte at `inlineMath.opener-pos + 1` is neither whitespace nor `$`." CONTEXT.md `remark-math currency rule` says only "opening `$` must be immediately followed by a non-whitespace character" — no "non-`$`" clause. ADR-0004 Decision 3 says "opener-followed-by-non-whitespace, closer-preceded-by-non-whitespace, closer-not-followed-by-digit" — no "non-`$`" clause. Triggered-grill PO A1 ratified branch (c) re-applying "the three remark-math predicate checks" — three, not three-plus-an-extra. PRD invented a fourth constraint folded into (i). The drift is load-bearing: fixture #11 (`$$x$$` in a tableCell) produces a library `*ast.InlineMath` with opener-pos at the cell-content's first `$`; under PRD's drifted predicate (i), src[opener-pos+1] = `$` → FAIL → demote to text → wire output `[text{value:"$$x$$"}]`, NOT the `[inlineMath{value:"x"}]` fixture #11 asserts. PRD self-contradicts. Either restate predicate (i) verbatim from CONTEXT.md ("non-whitespace" only, drop "non-`$`"), or fixture #11's pinned tree is wrong, or post-pass must opt-out for opener>=2 matches (no spec for this exists). Pick one and reconcile.

2. **Divergence-example `$ 5 and $x$` named in PRD prose, not pinned as fixture.** PRD names the actual branch-(c) divergence shape: "inputs whose library greedy-match consumes through a later remark-math-valid inline-math span (e.g., a leading-whitespace opener like `$ 5 and $x$` — library matches and post-pass demotes the whole 11-byte span to one `text`, losing the inner `$x$`; recursive rescan would have found `$x$` as a separate match)." ADR-0004 §Consequences relies on this scenario. But the only fixture for branch-(c) cost behavior is #4a (`$5 and $x$`, no leading space) which PRD admits CONVERGES with remark-math. The divergence the PO ratification accepted exists ONLY in prose; no exact-tree fixture pins it. Add a fixture pinning input `$ 5 and $x$` → expected wire tree.

3. **Unclosed-`$$` predicate undefined on blank-line-internal body.** PRD §Unclosed-fence behavior says the predicate walks `src[Lines().Last().Stop:]` and decides closed-vs-unclosed. But `probe/goldmark-mathjax/block.go:45-65` Continue appends segments unconditionally on non-closer lines — including blank lines. So input `$$\n\frac{a}{b}\n\nother text\n` produces a `MathBlock` whose `Lines()` covers `\frac{a}{b}\n` + `\n` + `other text\n`; `Lines().Last().Stop` points past `other text\n`; predicate sees EOF → unclosed → "demote to a paragraph" (singular). But goldmark prose-parsing of the same source range would split at the blank line into TWO paragraphs. PRD's demoted shape ("a paragraph whose `text` children mirror goldmark's standard prose-paragraph text-segmentation") cannot mirror a two-paragraph shape with a single paragraph. Pin the blank-line-internal case as a fixture and resolve the shape, or declare it out-of-scope.

4. **Fixture #14 still pins "no closed-state field" via reflection/struct introspection — fragile test method.** "No closed-state field exists" is a structural negation testable only via reflection over an external package's struct — breaks on any unrelated upstream field addition. The contract the compensation actually relies on is *behavioral*: "when the source has no closing `$$`, `MathBlock.Lines()` covers the body bytes and the parsed AST shape is indistinguishable from a closed case parsed only up to the same body extent." Rewrite #14 as a behavioral assertion: parse input A (unclosed `$$\nx\n`) and input B (closed `$$\nx\n$$\n`); assert `MathBlock.Lines().Last().Stop` is identical on A and B (modulo the closing fence's segment). Then "library upgrade adds closed-state field" is irrelevant if behavior is unchanged.

### VERDICT: continue

## Round 4

### Critic

(Post-Round-3 fixes. Trace re-verified against `probe/goldmark-mathjax/inline.go` + `block.go` + `block_node.go` + `CONTEXT.md`.)

**Verified fixes:**
- Defect 1 (predicate (i) drift): fixed. Predicate (i) reads "non-whitespace" verbatim. Fixture #11 Survival check trace holds: `src[opener-pos+1]='$'`, `$` is not whitespace → PASS → no demote → `[inlineMath{value:"x"}]` pinned-tree survives.
- Defect 2 (divergence fixture): added as #4b. Re-traced through `inline.go:24-52` + post-pass + coalesce. Library emits one `InlineMath` (value=` 5 and $x`); predicate (i) `src[1]=' '` → FAIL → demote → wire = `[text{value:"$ 5 and $x$"}]`. Pinned tree holds.
- Defect 4 (fixture #14 behavioral A vs B): trace holds. A and B both have `Lines().Last().Stop = 5` because B's closing fence enters the `parser.Close` branch at `block.go:49-57` BEFORE the `Lines().Append(seg)` at `:62`. Behavioral equivalence is a robust assertion.

**Remaining defects:**

1. **Fixture #14 input B byte count wrong.** PRD says "Input **B** (closed): `$$\nx\n$$\n` (7 bytes; `$$`, LF, `x`, LF, `$$`, LF)". Actual byte count = 8 (`$`,`$`,`\n`,`x`,`\n`,`$`,`$`,`\n`). The 7-byte claim is a quantitative error in a TDD-blocking fixture spec. Does not invalidate the `Last().Stop=5` assertion (which is correct), but the byte-count is wrong by one.

2. **ADR-0004 cross-refs to PRD fixtures are stale post-Round-3.** PRD now uses #4a as convergence trace and #4b as divergence trace. ADR-0004 still says: "narrow divergence pinned in PRD fixture #4a" — wrong, divergence is #4b. "explicit fixtures (PRD fixtures #3, #4a, #5)" — missing #4b. "PRD fixture #4a pins the trace on input `$5 and $x$`" then "divergence manifests on inputs where..." — old framing where #4a WAS the divergence fixture; current PRD says #4a converges. ADR-0004 was not refreshed when PRD added #4b.

3. **Out-of-scope §"Unclosed `$$` block with internal blank line" cross-refs an unrestricted ADR-0004 Decision 5.** PRD: "Cross-ref ADR-0004 Decision 5 (unclosed-`$$` compensation is for the no-internal-blank shape only)." ADR-0004 Decision 5 does NOT scope its compensation to the no-internal-blank shape; it specifies a general unclosed-`$$`-at-EOF compensation. Either revise ADR-0004 Decision 5 to add the no-internal-blank scope restriction explicitly, or drop the parenthetical from PRD §Out of Scope.

4. **Fixture #11 derivation cites `inline.go:26-28` for the opener-count loop, but the actual opener-count for-statement is at `inline.go:27`.** Cosmetic; flagging because every other fixture cites line:line ranges that match the actual code precisely.

### VERDICT: continue

## Round 5

### Critic

(Post-Round-4 mechanical fixes. Verified against PRD, ADR-0004, and `probe/goldmark-mathjax/inline.go`.)

**Verified fixes:**
- Defect 1 (byte count): fixed. PRD fixture #14 reads `(8 bytes; $$\nx\n$$\n)`. ✓
- Defect 3 (ADR-0004 Decision 5 scope restriction): fixed. Decision 5 now carries explicit no-internal-blank scope restriction with cross-ref to PRD §Out of Scope. ✓
- Defect 4 (fixture #11 line cite): fixed. Cites `inline.go:27`. ✓

**Remaining defect:**

1. **ADR-0004 Decision 3 carries stale divergence cross-ref to fixture #4a — Round-4 defect 2 only partially fixed.** Decision 3 body still ends: "divergence is fixture-pinned in PRD Testing Decisions §fixture #4a (input `$5 and $x$`)". PRD #4a now pins the CONVERGENCE trace; divergence is pinned in #4b (`$ 5 and $x$`). ADR-0004 Consequences section (lines 35, 45) was updated correctly in Round 4; Decision 3 body was not. Decision 3 contradicts its own Consequences — a future maintainer reading the canonical decision record follows a broken pointer. Load-bearing.

### VERDICT: continue
