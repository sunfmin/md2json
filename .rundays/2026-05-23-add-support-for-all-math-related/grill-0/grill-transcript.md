# grill-0 transcript — add-support-for-all-math-related

Idea Brief: `/Users/sunfmin/Developments/md2json/.rundays/2026-05-23-add-support-for-all-math-related/IDEA.md` — literally one line, "Add support for all math related." Prior v1 of md2json shipped with math as an explicit non-goal (see ADR-0002 "Out of scope" + CONTEXT.md `Markdown (input)` entry). This Run is additive: extend the GFM+frontmatter+footnotes pipeline so math source survives parse and lands as one or more mdast node types on the wire.

Grounding (already pinned, do not re-litigate unless math forces it):
- Parser = `github.com/yuin/goldmark`, single-pass, whole-document-in-memory, UTF-8-only, CRLF→LF, BOM-stripped (ADR-0001).
- Wire contract = mdast-shaped JSON, closed `mdast node-set v1`, silent-drop for unmapped goldmark constructs (CONTEXT.md).
- Extension wiring lives in `parse.New` as the single source of truth (ADR-0002 §"Negative (no central registry)").

## Round 1

### Interviewer

"All math related" is a *want*, not a spec. md2json v1 shipped math as a non-goal on purpose; turning it on adds at least one new goldmark extension, expands the closed `mdast node-set v1` (a wire-contract change, not a refactor), and re-opens the lossiness policy for any math sub-construct we don't model. Five questions to land scope before we can even draft a PRD. Answer in order; defer-with-reason is OK, "we'll see" is not.

**Q1. Input dialect — which markdown-math syntaxes are in scope for this Run?**
Pick a subset, do not say "all":
- (a) **Pandoc/CommonMark-extra style**: inline `$x$` (with the usual "no space after opening `$`, no digit immediately after closing `$`" disambiguation against currency) and display `$$...$$` on its own paragraph.
- (b) **Bracket style**: inline `\(...\)` and display `\[...\]` (LaTeX-faithful, no currency ambiguity, less common in markdown ecosystems).
- (c) **GitLab/Obsidian-style fenced math**: ` ```math ... ``` ` info-string-based display math.
- (d) **AsciiMath** (`` `am: ...` `` or similar) — different surface syntax entirely.
- (e) **mhchem** / **chemfig** — chemistry macros that *ride inside* a math block, not a separate syntax.
- (f) **MathML** raw-input passthrough (`<math>...</math>` HTML blocks).

Concretely: name the syntaxes by letter. Default ask is (a) only — the GitHub/VSCode/Obsidian common-denominator. Anything beyond that needs a one-sentence justification or it gets deferred to a v1.y.

**Q2. Output node shape — which mdast node types do you want emitted, and what fields do they carry?**
mdast's *de facto* community schema for math (from `remark-math`) is two node types:
- `inlineMath{value, position}` — the literal source between the inline delimiters, delimiters stripped.
- `math{value, meta, position}` — block/display math; `meta` is the info-string after the opening fence for fenced-math, `null` for `$$...$$`.

Options to pick from:
- (i) **Adopt remark-math node names verbatim** (`inlineMath` + `math`). Pros: zero friction for downstream consumers already on the unified ecosystem. Cons: the block type is named `math`, which is mildly confusing next to `inlineMath` (no `blockMath`).
- (ii) **Rename for symmetry** (`mathInline` + `mathBlock`, or `inlineMath` + `displayMath`). Pros: less ambiguous to a human reader. Cons: breaks compatibility with any consumer that already consumes remark-math output.
- (iii) **A single `math{display: "inline"|"block", value, meta}` node**. Pros: one node type, easier to switch on. Cons: differs from every existing mdast-math implementation; downstream tooling expecting separate types breaks.

Default ask is (i). Confirm or override.

What's the **value field semantics** — literal source between delimiters, byte-for-byte, no LaTeX-side normalization (we don't parse the LaTeX, we transport it)? Same rule as `code.value`? Confirm yes/no.

**Q3. Is md2json *rendering* math, or just *transporting* it?**
This is the single biggest scope-shaper, so I want it on the record explicitly.
- (a) **Transport-only.** md2json carries the math source through to a `value` field. Downstream (a KaTeX/MathJax renderer at the consuming end) decides how to render. md2json **never** invokes a LaTeX→MathML or LaTeX→HTML converter, never validates the LaTeX, never reports "this `\notacommand` is not a real macro." Mismatched braces in the LaTeX are the downstream renderer's problem.
- (b) **Render-to-MathML.** md2json shells out to / links against a LaTeX-to-MathML converter (e.g., `mathjax-node`, `katex`, `tex2mathml`) and emits a rendered MathML string alongside (or instead of) the source.
- (c) **Render-to-something-else** (HTML, SVG, …).

The whole "single static Go binary, no Node/Python runtime" wedge (CONTEXT.md `Wedge`) collapses if the answer is (b) or (c) and the renderer is a Node thing. If you want (b), name a **pure-Go** LaTeX-to-MathML library and accept the binary-size hit; otherwise the default ask is (a). Confirm (a) or argue for (b)/(c) with a named library.

**Q4. Which goldmark math extension are we wiring in, and what's its scope?**
goldmark itself does not ship a math extension. The candidates I'm aware of:
- `github.com/litao91/goldmark-mathjax` — adds `$...$` and `$$...$$`, emits MathJax-flavored HTML on `Render`. We'd consume its AST nodes (not its HTML) and translate to mdast in `translate`.
- `github.com/yuin/goldmark-meta`'s siblings, e.g. `go.abhg.dev/goldmark/mathjax` — similar surface.
- **Hand-rolled inline/block parser** registered into goldmark via `parser.Parser.AddOptions(...)`. More work, no extra dependency, gives us full control over the currency-disambiguation rules in Q1.
- **None** — implement math at the `translate` layer by post-processing `text` nodes that contain `$...$` runs. (I'm flagging this only to reject it: it's a layering violation and it interacts badly with `position.offset`.)

Pick a library by import path, or pick "hand-rolled." For (a)-only scope in Q1, a library is probably fine; for (a)+(c) fenced math, the library may not cover the fenced case and we end up hand-rolling anyway. If you don't know, say "Interviewer picks" and I'll pick one in Round 2 with a one-paragraph rationale you can ratify or override.

**Q5. The currency / dollar-sign ambiguity rule — pin it now, not at TDD time.**
Inline `$...$` is the source of every bug report in every math-enabled markdown parser, because plain prose like "It costs $5 and they had $10" looks like inline math. There is no universally-agreed rule. The three live options:
- (a) **`remark-math`'s rule**: inline math requires a non-whitespace character immediately after the opening `$` and immediately before the closing `$`, and the closing `$` must not be immediately followed by a digit. So `$5 and $10$` is *not* math; `$x = 5$` is. Display `$$...$$` has no such guard.
- (b) **Pandoc's rule**: same as (a), roughly; minor differences around backslash-escapes.
- (c) **Strict opt-in**: require backslash-escaped `\$` to mean "literal dollar sign in prose," treat every other `$...$` as math. Breaks every existing blog post that talks about money.
- (d) **No inline `$...$` at all** — only display `$$...$$` and/or bracket forms. Eliminates the ambiguity by amputation.

Default ask is (a). Confirm (a) or pick one of (b)/(c)/(d) with a justification.

---

Deferred-but-on-the-radar (do **not** answer this Round unless your Q1–Q5 answers force them):
- Equation numbering / `\label` / `\ref` cross-references — almost certainly out of scope for v1.x math; transport-only means we don't resolve refs.
- AMS environments (`align`, `gather`, `cases`) — these are LaTeX *inside* a display math block; transport-only means we carry the source byte-for-byte, no per-environment AST.
- Math-mode `\text{...}` islands — same story; transport-only.
- Lossiness policy for math sub-constructs we can't represent — N/A under transport-only (Q3a).
- `--no-math` flag to disable the extension at runtime — defer to Q4 / a follow-on ADR.
- Updates to `mdast node-set v1` enumeration in CONTEXT.md — pending Q2.

### PO

**A1. Input dialect.** **(a) only** for this Run — Pandoc/CommonMark-extra dollar style: inline `$x$` with the standard disambiguation guard, display `$$...$$` as its own paragraph.
**PO decision**: IDEA is one line ("Add support for all math related") — I am bounding "all" to the GitHub/VSCode/Obsidian common-denominator that 95% of GFM blog posts in the wild actually use. (b) bracket form is LaTeX-native but ecologically rare in markdown; defer. (c) fenced ` ```math ``` ` is GitLab/Obsidian-specific, adds a second surface area and a second parser path — defer to a follow-on Run once (a) is shipping. (d) AsciiMath, (e) mhchem/chemfig, (f) raw `<math>` MathML — explicit non-goals this Run. mhchem in particular "rides inside" a math block under transport-only (Q3a), so it falls out for free if a user writes `$\ce{H2O}$` — the source survives byte-for-byte and the downstream renderer handles it; no separate work here. Raw `<math>...</math>` continues to land as `html{value}` per the existing raw-HTML rule, no change.

**A2. Output node shape.** **(i) Adopt remark-math node names verbatim** — `inlineMath{value, position}` and `math{value, meta, position}`.
**PO decision**: IDEA silent on this; default ask wins because the entire `mdast node-set v1` already commits to "the unified/remark ecosystem's mdast" as the wire contract (CONTEXT.md `AST (output) / mdast` entry). Inventing our own names (option ii) for cosmetic symmetry would fork the contract on its first extension — exactly the failure mode CONTEXT's `Avoid_: "goldmark AST"` line is guarding against. Single-node-with-discriminator (option iii) is even worse — every existing remark-math consumer breaks. Accept the mild `math` vs `inlineMath` asymmetry as the cost of ecosystem fidelity. `meta` is `null` for `$$...$$` (no info string on dollar-fenced display math); the field stays in the schema so a future fenced-math Run (Q1c deferred) has a home for the info string without a schema break.

**value semantics: yes, byte-for-byte literal between delimiters, same rule as `code.value`.** Delimiters themselves are stripped (they are the syntactic marker, not content); whitespace and all interior bytes pass through untouched, no LaTeX-side normalization, no macro expansion, no brace-balance check. CONTEXT.md `Text/Code value preservation` is the governing principle — extended to math by analogy. Concretely: source `$x = 5$` produces `inlineMath{value: "x = 5"}`; source `$$\n\frac{a}{b}\n$$` produces `math{value: "\\frac{a}{b}\n", meta: null}` (interior content between fences, trailing `\n` of last content line preserved exactly like `code.value` for fenced blocks).
**PO decision**: applying the existing `code.value` rule to math closes the question without introducing a new normalization policy — consistent with the "if goldmark emits a byte, it goes on the wire" posture.

**A3. Render vs transport.** **(a) Transport-only.** md2json carries the math source byte-for-byte to the `value` field. No LaTeX-to-MathML, no LaTeX-to-HTML, no LaTeX-to-SVG, no LaTeX validation, no macro expansion, no brace-balance check. Mismatched braces, unknown macros, malformed AMS environments — all downstream-renderer concerns.
**PO decision**: IDEA silent, but the `Wedge` entry in CONTEXT.md ("single static Go binary, no Node/Python/Haskell runtime") is load-bearing and would collapse under (b)/(c) with any of the realistic renderers (`mathjax-node`, `katex`, `tex2mathml` are all Node). I am not aware of a pure-Go LaTeX-to-MathML library mature enough to ship in v1.x, and even if one existed, embedding it would inflate binary size, add a maintenance surface, and conflate two responsibilities (parse-markdown-structure vs render-math-content) the wire contract has kept cleanly separated. Transport-only also makes the deferred-radar items (equation numbering, AMS envs, `\text{}` islands, math-side lossiness policy) genuinely N/A rather than "deferred but lurking" — they all reduce to "bytes in `value`, downstream's problem." This is the decision the Wedge requires.

**A4. goldmark math extension.** **Interviewer picks** in Round 2, with a one-paragraph rationale, but constrained to: **a library extension (not hand-rolled), import path pinned in an ADR sibling to ADR-0002, wired through `parse.New` exclusively** (ADR-0002 §"Negative (no central registry)" is the governing convention). Preference order if Interviewer has no strong opinion: (1) `go.abhg.dev/goldmark-mathjax` if it covers `$...$` and `$$...$$` with the remark-math currency rule out of the box, (2) `github.com/litao91/goldmark-mathjax` as fallback. We consume AST nodes from whichever library lands and translate to mdast `inlineMath`/`math` in `translate` — we ignore its HTML renderer entirely, same pattern as we use for GFM/footnote extensions today.
**PO decision**: scope is (a) only per Q1, so a library is in fact sufficient — the fenced-math case that would have forced hand-rolling is deferred. Hand-rolling buys full control over the currency rule (Q5) but at the cost of carrying a parser; libraries that already implement remark-math-compatible rules get us there cheaper. The "Interviewer picks in Round 2" delegation is a *bounded* delegation — library, not hand-rolled; wired in `parse.New`; recorded in a new ADR — not a punt. If Interviewer's Round-2 survey turns up that no maintained library implements the Q5(a) rule faithfully, escalate back to me before defaulting to hand-rolled.

**A5. Currency / dollar-sign ambiguity rule.** **(a) `remark-math`'s rule**: inline math requires a non-whitespace character immediately after the opening `$` and immediately before the closing `$`, AND the closing `$` must not be immediately followed by a digit. Display `$$...$$` has no such guard.
**PO decision**: IDEA silent; (a) wins because it is the rule the rest of the mdast ecosystem already implements (Q2 picked remark-math node names — picking a different *parse* rule than remark-math while emitting remark-math *nodes* would be the worst of both worlds, producing AST that looks compatible but disagrees on which spans are math). (b) Pandoc is "roughly the same" with edge-case drift — not worth the divergence. (c) strict opt-in via `\$` is correct in the abstract but breaks every existing blog post that mentions money — a real-world regression on the input corpus md2json targets (GFM blog posts per `v1 ship criterion`). (d) amputation of inline `$...$` directly contradicts Q1(a), which IS the scope. So (a). The chosen goldmark extension in Q4 must implement this rule; if it does not, that is a Q4-blocking finding, not a Q5 reopen.

Using terms `inlineMath`, `math` (block/display), and `meta` (math info-string field) — not yet in `<product_dir>/CONTEXT.md` glossary. To be added under the **mdast node-set v1** enumeration as part of the PRD output of this Run (the `Deferred-but-on-the-radar` item "Updates to `mdast node-set v1` enumeration in CONTEXT.md — pending Q2" is now unblocked). Also note for the PRD author: ADR-0002's "Out of scope (post-v1)" line `Math ($...$, $$...$$) extensions. PRD non-goal.` is the explicit prior decision this Run reverses; the new ADR (sibling, ADR-0004 by numbering) must cross-reference and supersede that bullet.

### VERDICT: continue
