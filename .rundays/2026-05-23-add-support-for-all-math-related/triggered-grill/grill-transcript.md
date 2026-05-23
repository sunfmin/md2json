# triggered-grill transcript — currency rule fidelity (return_stage=to-prd)

Trigger: critic-prd Round 2 + Orchestrator-fetched library probe. PRD/ADR-0004 both load-bear the claim that `github.com/litao91/goldmark-mathjax` implements the **remark-math currency rule** byte-identically (grill-0 Round 2 A4 PO ratification). Library source on disk at `<product_dir>/.rundays/<run_id>/probe/goldmark-mathjax/` falsifies that claim: `inline.go:24-52`'s inline parser matches purely by `$`-run-length equality between opener and closer, with **no** opener-whitespace check, **no** closer-preceded-whitespace check, **no** closer-followed-digit check. Concrete failure: `It costs $5 and they had $10` produces `inlineMath{value:"5 and they had "}` against the library — exactly the failure mode A5 picked the rule to prevent. Grill-0 Round 1 A4 PO's escalation precondition ("If Interviewer's Round-2 survey turns up that no maintained library implements the Q5(a) rule faithfully, escalate back to me before defaulting to hand-rolled") is **met, after the fact, by source observation rather than survey**.

Grill scope (narrow, per Interviewer triggered-grill contract):
1. Vocab drift on `remark-math currency rule` in CONTEXT.md — entry still says "the chosen extension must implement this rule; if it does not, that is an extension-pick blocker, not a rule reopen." Probe says litao91 does not. PO must resolve which half of that sentence wins.
2. Remediation pick — PRD §Open Questions and ADR-0004 §Open subquestion parameterize three branches:
   - (a) Reopen ADR-0004 — fork litao91's `inline.go` (~50 LoC vendored at `parse/internal/mathjax/`), or pick an alternate, or hand-roll. Byte-identical remark-math fidelity. Contradicts grill A4's "library, not hand-rolled" letter; matches its escalation-precondition spirit.
   - (b) Accept litao91's matching-`$`-run rule. Rewrite CONTEXT.md `remark-math currency rule` to describe the actual library behavior (rename or redefine). Loud regression vs A5 ratification — currency-prose corpora garble.
   - (c) Translate-layer currency post-pass — demote-only. `translate` re-applies the three remark-math predicate checks against `*ast.InlineMath`; on rejection, demote the matched range to `text`. ~30 LoC. Diverges from remark-math on mixed prose+math like `$5 and $x$` (library's greedy first match consumes through the later valid `$x$`; demotion converts the whole span to text; `$x$` is lost).

Grill-0 Q1–Q5 territory NOT re-litigated except where this finding directly invalidates.

## Round 1

### Interviewer

PO ratified `remark-math currency rule` in grill-0 A5 as load-bearing — explicitly because "picking a different *parse* rule than remark-math while emitting remark-math *nodes* would be the worst of both worlds." Picked litao91 in A4 on the chain-of-assertion that it implements that rule. The probe at `inline.go:38-52` (reproduced below) breaks that chain:

```go
for i := 0; i < len(line); i++ {
    c := line[i]
    if c == '$' {
        oldi := i
        for ; i < len(line) && line[i] == '$'; i++ {
        }
        closure := i - oldi
        if closure == opener && (i+1 >= len(line) || line[i+1] != '$') {
            // match closes
        }
    }
}
```

Three questions. Land one branch with rationale; defer-with-reason is OK only if you need to think out loud, "we'll see" is not.

**Q1. Branch pick — (a) fork inline parser / (b) accept matching-`$`-run rule + rewrite CONTEXT.md / (c) translate-layer demote-only post-pass?**

Pick one letter. The PRD's fixture #3 target shape is parametric on this answer; `to-issues` cannot proceed past `to-prd` until the pick lands. Concrete tradeoff matrix to anchor your answer:

| Branch | Code cost | CONTEXT.md change | Remark-math fidelity on `$5 ... $10` | Fidelity on `$5 and $x$` | Maintenance burden |
|--------|-----------|-------------------|--------------------------------------|--------------------------|--------------------|
| (a) fork | ~50 LoC vendored Go | none | byte-identical | byte-identical | ongoing drift vs upstream litao91 |
| (b) accept | 0 | rename rule, drop "extension-pick blocker" guard, vocab regression | garbles (`inlineMath{value:"5 and they had "}`) | both library greedy-matches `$5 and $x$` → one `inlineMath` | none |
| (c) demote-only | ~30 LoC translate | soften rule to "approximate; rare divergences" + add divergence fixture | byte-identical | lossy (whole span becomes `text`; the valid `$x$` is lost) | translate carries currency predicate |

**Q2. If (a): which fork target — vendor litao91 entirely as `parse/internal/mathjax/`, or vendor only `inline.go` and import the rest, or hand-roll from scratch against the `goldmark/parser.InlineParser` interface?**

Skip if not (a). The library surface is small (block.go + inline.go + block_node.go + inline_node.go + renderer files we don't use), so "vendor the whole thing minus renderers" is a real option distinct from "vendor inline.go only." The third option (hand-roll fresh) buys block-parser control too (currently `block.go:25-43` unconditionally opens on `$$`; you might want symmetric tightening there even though grill A7 said unclosed-`$$` is a translate-compensation concern).

**Q3. If (b): what is the new term?**

Skip if not (b). The existing `remark-math currency rule` entry in CONTEXT.md has an `_Avoid_` line warning against "Pandoc rule" (close but edge-case-divergent) and "strict `\$` opt-in." If we accept the library's actual rule, the term replacing `remark-math currency rule` must (i) be unambiguous about what it includes (`$`-run-length equality, closer-not-followed-by-`$`) and excludes (no whitespace check, no digit check), (ii) not collide with the existing `_Avoid_` synonyms, and (iii) carry a forward-pointer noting the divergence from remark-math so a downstream consumer hitting AST disagreement knows where to look. Propose a term, or say "Interviewer drafts" and I'll draft in Round 3.

---

Round 1 cap: three questions, the third gated on (b). Answer Q1 minimally; Q2/Q3 only if your Q1 pick lights them up. If you pick (c), Round 2 closes — the PRD's existing fixture #3 derivation and the `$5 and $x$` divergence fixture both already exist in §Open Questions, so vocab + ADR-0004 reopen are mechanical.

### PO

**A1 (Q1). Pick: (c) translate-layer currency post-pass — demote-only.** Library stands (no ADR-0004 reopen on the library identity), CONTEXT.md `remark-math currency rule` entry gets a softening edit (not a rewrite), `translate` carries a ~30-LoC post-pass that re-applies the three remark-math predicate checks against each `*ast.InlineMath` and demotes the matched range to `text` on rejection.

Rationale.

1. **User-corpus realism trumps spec purity.** I write Markdown that contains currency amounts in prose ("It costs $5 and they had $10" is a representative line). (b) silently regresses every such document — `inlineMath{value:"5 and they had "}` lands in the wire, downstream renderers either typeset garbage or report a LaTeX error on the broken span. That is a real-world corpus-level regression on the `v1 ship criterion` input class (GFM blog posts), the exact failure mode grill-0 A5 rejected (c)-strict-opt-in for. Accepting it now reverses A5 silently. Reject (b).

2. **(a) burns a Cycle on a sub-decision IDEA does not motivate.** Forking the inline parser is technically clean (byte-identical remark-math) but reopens ADR-0004 and triggers another grill on fork-target (vendor `inline.go` only vs vendor whole package minus renderers vs hand-roll fresh against `goldmark/parser.InlineParser`). The library's own test surface (`mathjax_test.go`) is five cases — `plain text`, `bold`, `$1+2$` inline, `$$\n1+2\n$$` display, one panic-regression — none of which exercise currency. The library never claimed to implement remark-math; Round-2 Interviewer's "implements the remark-math currency guard verbatim" claim was the assertion error, not a library defect. Forking would mean carrying ~50 LoC of ongoing-drift-vs-upstream vendored Go for a rule the upstream library was never trying to satisfy. Bounded delegation (grill-0 A4) said "library, not hand-rolled"; (a) is hand-rolling-via-fork dressed up. Reject (a) for this Run.

3. **(c) is the most contained remediation.** ~30 LoC inside `translate` (the AST→mdast layer that *already* owns library-behavior compensation per ADR-0004 Decision 5's unclosed-`$$` predicate — see PRD §Implementation Decisions `translate` bullet, sub-point 1). The currency post-pass is sub-point 2 in the exact same compensation pattern: `translate` re-applies the three predicates (whitespace-after-opener, whitespace-before-closer, no-digit-after-closer) against src bytes for each `*ast.InlineMath` and demotes on rejection. Demoted ranges coalesce with adjacent `text` siblings via existing `translate.go:225-231` logic — no new contiguous-text coalescing code. No fork. No ADR-0004 reopen on library identity. No CONTEXT.md rewrite of `remark-math currency rule` — only a softening clause naming the known divergence.

4. **Honest about the (c) cost — `$5 and $x$` divergence is real and fixture-pinned.** The library greedy-matches the first `$...$` it finds (`inline.go:38-52` walks forward to the first closer satisfying the `closure == opener && next != '$'` branch). On input `$5 and $x$`, the library produces one `inlineMath{value:"5 and "}` spanning bytes 0–9, then `text{value:"x"}` then a stray `$`. remark-math, by contrast, rejects the opening `$` (whitespace after = `5` is not a digit-check trigger, but the closing-`$`-followed-by-` ` against ` ` test — wait, re-checking: remark-math's rule is "opening `$` followed by non-whitespace AND closing `$` preceded by non-whitespace AND closing `$` not followed by a digit." Opening `$` followed by `5` (non-whitespace, passes). Closing `$` (at pos 9 in `$5 and `... wait the next `$` after `$5` is at pos 8 in `$5 and $x$` — followed by `x`, preceded by ` ` (space) → **fails** the closing-preceded-by-non-whitespace check). So remark-math rejects the first match, scans onward, finds `$x$` as a valid match → `[text{value:"$5 and "}, inlineMath{value:"x"}]`. Option (c) cannot re-scan the demoted bytes (the library has already greedy-consumed through them); demoting the rejected match converts the whole span back to `text` → `[text{value:"$5 and $x$"}]`. The valid `$x$` is lost. That is a real regression vs remark-math on the specific mixed-prose-plus-math pattern.

   I accept this cost. The divergence is (i) narrow — requires currency-rejected match's span to physically contain a valid later inline-math span; (ii) bounded — the rejected-then-demoted bytes are *not silently dropped*, they survive byte-for-byte in `text.value` per `Text/Code value preservation`, so the downstream consumer still sees `$x$` as literal source; (iii) explicit — a fixture pinning this divergence (PRD adds a fixture asserting `[text{value:"$5 and $x$"}]` on this exact input) makes the loss observable, regression-proof, and documentable. Compared to (b)'s silent corpus-wide currency regression, (c)'s narrow-and-pinned divergence is the better tradeoff.

**A2 (Q2). Skip.** Not (a).

**A3 (Q3). Skip.** Not (b).

---

**CONTEXT.md edit required as part of `to-prd` re-run** (PO authorizes; PRD-stage author applies — same Stage that owns ADR-0004 authoring per grill-0 Round 3).

Current `remark-math currency rule` entry ends: "The chosen goldmark math extension must implement this rule; if it does not, that is an extension-pick blocker, not a rule reopen."

That guard is empirically falsified (litao91 does not implement the rule and we are not switching libraries). Replace with:

> "The chosen goldmark math extension may not implement this rule end-to-end; when it does not, the `translate` layer applies the three predicate checks as a demote-only post-pass against each emitted `inlineMath`, converting rejected ranges back to `text`. This restores user-story-3 fidelity (`$5 and $10` → ordinary prose) at the cost of a narrow divergence from remark-math on inputs where a currency-rejected match's span physically contains a later valid inline-math match (e.g., `$5 and $x$` produces a single `text` node rather than `[text, inlineMath, text]`); divergence is fixture-pinned, see PRD `to-prd` output."

**PO decision**: keeps the rule's wire-output semantics (the three predicates still decide what is and is not an `inlineMath` on the wire) while honestly naming the implementation route (translate post-pass, not library-internal) and the one known divergence. Does *not* rewrite the rule's three predicate checks — those stay verbatim, they remain the spec the wire output is measured against. The "extension-pick blocker" half of the original guard becomes "translate-compensation responsibility" — same load-bearing posture, different layer.

---

**Scope guard.** This Round resolves the two flagged terms only:
- `remark-math currency rule` vocab drift — resolved by the CONTEXT.md edit above (PRD author applies).
- Library pick — `github.com/litao91/goldmark-mathjax` stands; ADR-0004 Decision 3 ("byte-identically") must be rewritten to name the translate-layer post-pass route (`to-prd` author applies the ADR-0004 edit too).

Critic's remaining Round-2 defects (Fixture #5 soft-break shape, `translate` unclosed-`$$` predicate spelled-out, fixture #3/#4 as library-pick blockers, fixture #8 verification) are artifact-shape concerns owned by critic ↔ proposer in the resumed `to-prd` Stage, not this triggered-grill's scope.

Using terms `translate-layer currency post-pass`, `currency-rule demote-only`, `$5 and $x$ divergence fixture` — implementation-detail vocabulary, not on the wire, belongs in ADR-0004 and PRD §Implementation Decisions, **not** in `<product_dir>/CONTEXT.md`. Same posture as `ast.InlineMath`/`ast.Math` per grill-0 Round 2 PO direction.

### VERDICT: continue
