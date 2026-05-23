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
<left empty — next Cycle's PO fills>

### VERDICT: continue
