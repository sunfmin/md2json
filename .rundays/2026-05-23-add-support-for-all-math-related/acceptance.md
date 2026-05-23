# Acceptance Gate — add-support-for-all-math-related

Run: `2026-05-23-add-support-for-all-math-related`
Product: `/Users/sunfmin/Developments/md2json`
IDEA: "Add support for all math related"

## Summary

IDEA seed ("Add support for all math related") landed as a focused, ratified scope through grill-0 + triggered-grill: dollar-sign math (inline `$...$`, display `$$...$$`), **transport-only** (md2json carries LaTeX bytes; never renders/validates/expands), no `--no-math` toggle, library = `github.com/litao91/goldmark-mathjax` wired through `parse.New`, two translate-layer compensations for library gaps. Bracket form `\(...\)`, fenced ` ```math `, AsciiMath, mhchem-as-separate-syntax, and raw `<math>` MathML were ratified as out-of-scope in grill-0 and stay out.

**Load-bearing find** (recorded in ADR-0004 Decision 3 and PRD §Implementation Decisions): the picked library does **not** implement the remark-math currency rule — its inline parser at `probe/goldmark-mathjax/inline.go:24-52` matches by `$`-run-length equality only, with no whitespace-after-opener / whitespace-before-closer / digit-after-closer checks. PO ratified branch (c) in triggered-grill Round 1 A1: enforce the currency rule one layer up via a ~30-LoC demote-only post-pass in `translate` over each emitted `*ast.InlineMath`. Bounded divergence vs. pure remark-math is fixture-pinned (PRD #4b / wire fixture 72 — input `$ 5 and $x$`); convergence on the closely-related `$5 and $x$` is pinned in #4a / fixture 71. Second compensation (ADR-0004 Decision 5): unclosed `$$` at EOF is not observable from the AST alone (library's `MathBlock` carries no closed-state field), so `translate` inspects source bytes after `Lines().Last().Stop` to decide closed-vs-unclosed and demotes unclosed blocks to a prose paragraph (fixture 73 / PRD #5). Predicate (i) was restated CONTEXT.md-verbatim ("non-whitespace", no "non-`$`" sub-clause) after Round-3 critique flagged drift; fixture 80 (`$$x$$` in a table cell → `inlineMath{value:"x"}`) survives the restored predicate.

Deliverables verified:

- **7 issues all `done`** (`queue/01-wire-math-extension` through `queue/07-mismatched-braces-and-value-preservation`, each with `done` marker + `tdd-log.md` + `arch-log.md`).
- **PRD fixtures #1–#14 (including #4a, #4b) all pinned** as black-box wire fixtures (`testdata/fixtures/62..82`, 21 fixture dirs covering `nopos` / `default` / `pretty` variants) plus Go-layer translate / parse unit tests anchoring each transport rule at the package layer (S07 tdd-log §Test 1, §Test 2).
- **Smoke regression** (`testdata/fixtures/61-smoke-non-math-gfm-blog-post-nopos`) pinned byte-identical wire envelope for the existing GFM prose corpus before vs. after extension wiring (S01 tdd-log §Test 2). Existing fixtures 01..60 remain green; no schema break, no positional shift on non-math inputs.
- **Math always on** — `mathjax.NewMathJax()` appended unconditionally inside `parse.newGoldmarkWith` (`internal/parse/parse.go:111`); no `--no-math` flag added; CONTEXT.md `v1 flags` enumeration unchanged at six flags.
- **Translate-layer compensations** cohabit `internal/translate/compensate_math.go` (currency post-pass + unclosed-`$$` predicate + ASCII byte-class helpers) — concept-locality per ADR-0004's "two library-behavior-specific compensations" framing.
- **Lossiness property test** extended with `inlineMath` / `math` map entries; silent-drop set for math is empty by construction.
- **ADR-0004** (`docs/adr/0004-math-extension-library.md`) records library pick, wiring style, currency-rule routing, unclosed-`$$` predicate, no-runtime-toggle posture, and consequences (positives + four negatives, all fixture-pinned).
- **CONTEXT.md** carries `Dollar-sign math (transport-only)`, `remark-math currency rule` (with "translate-compensation responsibility" clause), `inlineMath` node, `math` node, `Unclosed-display-math fall-through rule`, and the two new entries in `mdast node-set v1`.

IDEA intent ("math support that survives parse and lands on the wire so downstream renderers can typeset it, without garbling existing prose with currency mentions") is met. The narrow remark-math divergence (#4b) is documented, bounded, and a stated-cost of branch (c); a future Run can swap to recursive rescan or library-fork if downstream consumers report regressions.

## Decision

`done`. No gap between IDEA intent and shipped product that justifies new issues in this Run. The single known divergence (#4b leading-whitespace-opener greedy match) is PO-ratified scope, fixture-pinned, and recorded in ADR-0004 Decision 3's "Negative" bullets as a future-Run trigger — not a current-Run gap.

The deferred surfaces (bracket `\(...\)` / `\[...\]`, fenced ` ```math `, AsciiMath, mhchem-as-separate-syntax, raw `<math>` MathML, `--no-math` toggle, unclosed-`$$`-with-internal-blank shape, equation numbering) are explicit grill-0 / PRD §Out of Scope items and are not part of "Add support for all math related" as ratified in this Run.

### VERDICT: done
