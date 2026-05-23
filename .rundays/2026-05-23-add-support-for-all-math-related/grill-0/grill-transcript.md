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

## Round 2

### Interviewer

Round 1 closed Q1/Q2/Q3/Q5. Q4 was bounded-delegated to me: library (not hand-rolled), wired via `parse.New`, sibling ADR to ADR-0002, preference order `go.abhg.dev/goldmark-mathjax` → `github.com/litao91/goldmark-mathjax`, escalate if no library implements the Q5(a) remark-math currency rule. Five questions below — one library-pick to ratify, four remaining ambiguities that affect the PRD output shape and must land **before** `to-prd` not at TDD time.

**Q4 (resolved-by-Interviewer, awaiting PO ratification). Library pick: `github.com/litao91/goldmark-mathjax`.**

Rationale (one paragraph, as bounded):

Surveyed the four live candidates against the Q1(a) scope and the Q5(a) currency rule.

- `go.abhg.dev/goldmark-mathjax` — does not exist. The `go.abhg.dev` namespace owns `frontmatter` (which we already use per ADR-0002), `anchor`, `toc`, `wikilink`, `mermaid`, `hashtag`, and a handful of others, but **not a math extension**. My Round-1 question listed this as a candidate on the assumption that the namespace's math sibling tracked its frontmatter sibling — it does not. PO's preference-order item (1) is therefore non-applicable, not unselected.
- `github.com/litao91/goldmark-mathjax` — the de-facto goldmark math extension. Parses inline `$...$` and display `$$...$$` (the exact Q1(a) surface), exposes `ast.Math` + `ast.InlineMath` Go-native nodes (which `translate` will consume directly, ignoring its `Renderer` per the ADR-0002 "we ignore its HTML renderer entirely, same pattern as GFM/footnote" posture), and implements the **remark-math currency guard verbatim**: opening `$` must be followed by non-whitespace, closing `$` must be preceded by non-whitespace AND not followed by a digit, display `$$` has no guard. That is byte-identical to Q5(a). Wires through `parse.New` as a single extension value, no extra glue.
- Hand-rolled — defeated on cost: PO's bounded delegation says "library, not hand-rolled" and the litao91 extension covers the scope at the rule level. No escalation triggered.
- "None / post-process `text` nodes" — explicitly rejected in Round 1 Q4 as a layering violation.

Trade-offs to surface (none of these change the pick, but the PRD/TDD must know):
1. `litao91/goldmark-mathjax`'s last published tag is older than goldmark itself — the repo is maintenance-mode, not abandoned, but a Run-the-tests-before-shipping commitment is warranted (the upcoming `tdd` Stage already handles this).
2. The extension's package layout exposes `ast.Math` (block) and `ast.InlineMath` (inline) — naming aligns with mdast's `math`/`inlineMath` 1:1, which makes the `translate` mapping a near-identity rename. Pleasant accident; not the reason for the pick.
3. The extension does **not** support fenced ` ```math` (Q1(c), deferred). If a future Run un-defers (c), we revisit Q4 then; for now this gap is in-scope-of-deferral.

**Q4 ask:** ratify `github.com/litao91/goldmark-mathjax` with the above rationale, or override with a different import path (and justify). Either way, recorded as a new product ADR (sibling to ADR-0002, numbered `0004` by sequence) by the PRD-stage author, cross-referencing ADR-0002's `Math (...) extensions. PRD non-goal.` line as the bullet this Run supersedes.

---

**Q6. `meta` field on the `math` node — what bytes are allowed there, and how do they get there under Q1(a)-only?**

PO landed in Round 1 that `meta` is `null` for `$$...$$` blocks and stays in the schema as a future home for fenced ` ```math <meta>` info strings. Under the current Q1(a)-only scope, the `meta` field is therefore **always `null`** — no surface syntax in scope can populate it. Confirm one of:

- (a) **`meta` is always `null` in v1.x math.** Document this explicitly in the PRD as a forward-compatibility field, not a live feature. TDD fixtures assert `meta === null` on every `math` node. A future fenced-math Run is the one that actually wires `meta`.
- (b) **`meta` accepts an out-of-band annotation** even on `$$...$$` blocks via some yet-undecided source (e.g., goldmark attribute syntax `{#id .class}` after the closing fence). This adds scope and a parse path; I am flagging it only to reject unless PO surfaces a concrete need.

Default ask: (a). Confirm (a) or argue (b).

**Q7. Unclosed display math at EOF — hard error, silent drop, or fall-through to prose?**

The litao91 extension's behavior under `$$\n\frac{a}{b}\n` (no closing `$$`, EOF) needs to be pinned at the wire-contract level, not deferred to "whatever the library does." Three options:

- (a) **Fall-through to prose.** The unmatched `$$` and the body bytes are not recognized as a math block; they emit as ordinary `paragraph`/`text` nodes per CommonMark. This matches the **Frontmatter unclosed-fence rule** (CONTEXT.md `Frontmatter` entry) which says "no close, no block; parse as body content." Consistent posture.
- (b) **Hard error.** Exit `1`, `md2json: <path>:<line>:<col>: unclosed display math` on stderr. Aggressive; surfaces the bug at parse time but breaks every blog post that has a stray `$$` in prose.
- (c) **Silent drop** of the math block. Don't emit a `math` node, don't emit the body bytes either. Violates **Text/Code value preservation**'s "if goldmark emits a byte, it goes on the wire" — rejected on principle.
- (d) **Best-effort `math{value: "<body>"}`** with no closing fence pretended. Pretends a `$$` we didn't see.

Default ask: (a) fall-through to prose, mirroring the frontmatter rule. Confirm (a) or override. Mirror question applies to inline `$...$` with no closing `$` on the same line — the remark-math rule already handles this by "not matching as math" (it requires a closing `$` to fire), but PRD should state this explicitly.

**Q8. Math inside other block constructs — lists, blockquotes, table cells, footnote definitions.**

CommonMark/GFM allow block constructs to nest. The litao91 extension is registered as a block parser (for `$$`) and an inline parser (for `$...$`). Three composition cases need wire-contract clarity:

- **Inline math inside a `tableCell`, `listItem` paragraph, `blockquote` paragraph, `footnoteDefinition`**: should emit `inlineMath` as a child of the containing paragraph, same as everywhere else. Standard inline-in-block composition. Confirm yes.
- **Display `$$...$$` inside a `listItem` or `blockquote`**: does the extension recognize the `$$` open at a non-document-root indentation? `litao91`'s block parser is registered at standard block-priority; in practice this means `$$` works at the document root and at the start of a list item / blockquote line, but **not** with arbitrary indentation. Confirm we accept this — display math inside a deeply nested list is a corner case, and the fall-through (Q7a) means a non-matched `$$` becomes prose, which is the least-surprising outcome.
- **Display `$$...$$` inside a `tableCell`**: GFM table cells are inline-content only — no block constructs inside cells. Therefore `$$...$$` inside a table cell **never** matches as display math; the `$$` bytes go on the wire as `text.value` (or, plausibly, the inline `$x$` matcher consumes part of it as inline math depending on the cell contents). Confirm we accept "no block math inside tables" as the rule and document it in the PRD.

Default ask: confirm inline-in-block composition is uniform, accept the indentation/table-cell limits as natural consequences of block-vs-inline parsing, document in PRD. Override if PO wants a stronger guarantee (which would force hand-rolling).

**Q9. Lossiness policy interaction — does the v1.x math Run change the silent-drop posture?**

CONTEXT.md `Lossiness policy (goldmark → mdast)` says "any goldmark construct that does not map to a node in mdast node-set v1 is dropped silently." Under transport-only math (Q3a), the litao91 extension emits exactly two new goldmark node types (`ast.InlineMath`, `ast.Math`), both of which now have first-class mdast targets (`inlineMath`, `math`). Therefore:

- **Claim:** the silent-drop set for math is **empty** — every math goldmark node has an mdast home, by construction of Q2(i) + Q3(a). No new dropped-constructs entry.
- **Mirror claim:** the schema-extension count is +2 (`inlineMath`, `math`) plus one new optional field on `math` (`meta`); the new ADR (0004) names these explicitly so the PRD can reference them.

Confirm both claims. Override only if PO sees a math goldmark construct I'm missing.

**Q10. CommonMark fallback when the math extension is disabled — does v1.x ship a `--no-math` switch?**

Round 1 Q4 deferred this to "a follow-on ADR." Pinning now because it affects the PRD's flag enumeration (CONTEXT.md `v1 flags` entry):

- (a) **No runtime toggle in v1.x.** Math is always enabled once the Run ships; `parse.New` wires the extension unconditionally. Matches ADR-0002's "the v1 wire contract pins the enabled set; runtime toggling would change the output schema" posture from the "Out of scope (post-v1)" list. A user who wants to disable math runs an older binary.
- (b) **`--no-math` flag** that drops `inlineMath`/`math` recognition for that invocation, falling back to CommonMark prose parsing for `$...$`/`$$...$$`. Adds a flag, adds a code path, adds a TDD axis (test math-on AND math-off).

Default ask: (a), consistent with ADR-0002's negative. If PO picks (b), the new ADR-0004 must spell out the flag's interaction with `--frontmatter-only` and the empty-input acceptance criterion. Confirm (a) or argue (b).

---

If PO ratifies Q4 (or counter-picks) and lands Q6–Q10 as defaults, the load-bearing math vocabulary is closed. `to-prd` has: input dialect, node names + field semantics, transport-only renderer posture, currency rule, library pick + ADR scaffold, `meta`-always-null posture, EOF-unclosed fall-through, in-block composition rules, lossiness-empty claim, no-runtime-toggle posture. Anything PO declines to default on gets one more Round; everything else moves to PRD.

### PO

**A4 (ratify Q4). Ratified: `github.com/litao91/goldmark-mathjax`.** Wire through `parse.New` exclusively (ADR-0002 §"Negative (no central registry)"). Recorded as **ADR-0004** by the `to-prd` author, sibling to ADR-0002, cross-referencing and superseding ADR-0002's "Out of scope (post-v1)" bullet `Math ($...$, $$...$$) extensions. PRD non-goal.` The translate layer consumes `ast.InlineMath` → mdast `inlineMath{value, position}` and `ast.Math` → mdast `math{value, meta:null, position}`; the library's `Renderer` is **not** registered (same pattern GFM/footnote/frontmatter already use — we consume Go-native AST, never goldmark's HTML output).
**PO decision**: Round-1 preference-order item (1) `go.abhg.dev/goldmark-mathjax` is non-existent per Interviewer's namespace audit — not a defeat of my preference, just an empty cell. (2) `github.com/litao91/goldmark-mathjax` was the explicit fallback, covers Q1(a) exactly, implements the Q5(a) remark-math currency rule byte-identically, exposes Go-native `ast.InlineMath`/`ast.Math` whose names align 1:1 with mdast (pleasant accident, not the basis of the pick), and its lack of fenced ` ```math ` support is in-scope-of-deferral per Q1(c). Maintenance-mode tag is a real risk but bounded — `tdd` Stage will run the library's test surface on our fixtures, and the wire-contract surface we consume (two AST node types + their parser) is small enough to fork if upstream goes dark. No escalation trigger met. Pick stands.

**A6. `meta` field.** **(a) `meta` is always `null` in v1.x math.** Document in PRD as a forward-compatibility schema field, not a live feature. TDD fixtures must assert `meta === null` on every emitted `math` node — this is a load-bearing invariant, not a nice-to-have, because the `null` posture is what reserves the field for the deferred fenced-math Run without a wire-contract break. A future Run un-defers Q1(c) and that Run wires `meta` to the info string after ` ```math `.
**PO decision**: option (b) (out-of-band annotation via goldmark `{#id .class}` attribute syntax on `$$` blocks) adds a parse path, a second source of `meta` bytes, and a precedent — "math nodes can be annotated via attributes" — that would then need to apply to every other block node for consistency. That is a v2 attribute-syntax conversation across the entire node-set, not a math-Run scope. Reject (b). The `meta` field stays in the schema (already pinned in Round-1 A2), populated only by future fenced-math.

**A7. Unclosed display math at EOF.** **(a) Fall-through to prose.** Unmatched `$$` and body bytes are not recognized as a math block; they emit as ordinary `paragraph`/`text` nodes per CommonMark, exactly mirroring the **Frontmatter unclosed-fence rule** (CONTEXT.md `Frontmatter`: "no close, no block; parse as body content"). Mirror rule for inline: an opening `$` with no closing `$` before end-of-line/end-of-paragraph simply does not match as math under the remark-math rule (the rule requires a closing `$`); the `$` and subsequent bytes go on the wire as `text.value`. PRD must state both rules explicitly so the TDD fixtures cover the EOF and end-of-line cases.
**PO decision**: (a) is the only option consistent with the existing unclosed-fence posture for frontmatter; introducing a hard-error path for math but not for frontmatter would be a posture inconsistency, and that inconsistency would itself become load-bearing vocabulary the next person has to learn. (b) hard error breaks every blog post with a stray `$$` in prose (the same real-world-corpus argument that killed Q5(c) in Round 1). (c) silent drop violates **Text/Code value preservation** ("if goldmark emits a byte, it goes on the wire"). (d) best-effort with a phantom closing fence pretends bytes we did not see — manufacturing AST content from absence is exactly the failure mode the lossiness/preservation rules are designed to prevent. Behavior of `litao91/goldmark-mathjax` on unclosed `$$` must be verified at TDD time to in fact be (a) — if the library emits a partial `ast.Math` or hard-errors, that is a TDD-blocking finding and the `translate` layer must compensate (either by detecting the unclosed-fence case and demoting to prose, or by reporting back here as a Q4 escalation).

**A8. Math inside other block constructs.** Confirm all three sub-points:
1. **Inline math inside `tableCell`, `listItem` paragraph, `blockquote` paragraph, `footnoteDefinition`** — **yes**, emits `inlineMath` as a child of the containing paragraph (or directly inside the cell for tables, which carry inline content directly per mdast). Standard inline-in-block composition, no special-casing in `translate`.
2. **Display `$$...$$` inside `listItem` / `blockquote`** — **accept the library's natural behavior**: `$$` matches as display math when it appears at the start of a list-item line or a blockquote line, but **not** under arbitrary indentation. Non-matched `$$` falls through to prose per Q7(a), which is the least-surprising outcome.
3. **Display `$$...$$` inside a `tableCell`** — **never matches as display math**; GFM table cells are inline-content-only by spec, so the `$$` bytes either land in `text.value` or get consumed in part by the inline `$...$` matcher (depending on cell contents). Document the rule in PRD as: "block math is not recognized inside table cells; bytes fall through to inline parsing."
**PO decision**: hand-rolling the parser to extend block-math recognition to arbitrarily-indented or table-cell contexts is option-rejected by Round-1 A4's "library, not hand-rolled" bounded delegation, and would also produce wire output that no remark-math consumer expects (since remark-math itself does not support block math inside table cells). Accept the composition limits as natural consequences of the inline-vs-block parser layering; PRD documents them as wire-contract rules so the TDD fixtures cover them affirmatively (not as accidents-of-implementation).

**A9. Lossiness policy interaction.** **Confirm both claims.**
- **Silent-drop set for math is empty.** The litao91 extension emits exactly `ast.InlineMath` and `ast.Math`; Q2(i) gives both a first-class mdast target (`inlineMath`, `math`). No new entry in the dropped-constructs set.
- **Schema extension is +2 node types (`inlineMath`, `math`) plus one new optional field (`meta` on `math`).** ADR-0004 names these explicitly; PRD references ADR-0004 from its `mdast node-set v1` enumeration section (the updated CONTEXT.md enumeration is already in place per Interviewer's Round-2 CONTEXT update).
**PO decision**: nothing in the litao91 extension's published surface emits goldmark node types beyond `ast.InlineMath`/`ast.Math` that I am aware of. If `tdd` Stage discovers an extension-emitted construct without a mapping (e.g., some library-internal annotation node), that is a lossiness-set delta to record in the dropped-constructs entry, not a re-litigation of the silent-drop posture itself.

**A10. `--no-math` runtime toggle.** **(a) No runtime toggle in v1.x.** `parse.New` wires `litao91/goldmark-mathjax` unconditionally. A user who wants math-off runs an older binary (pre-this-Run).
**PO decision**: directly consistent with ADR-0002's "Out of scope (post-v1)" list — `A --extensions flag to toggle GFM/footnotes/frontmatter at runtime. The v1 wire contract pins the enabled set; runtime toggling would change the output schema.` Math is now part of that pinned set. (b) `--no-math` would (i) add a flag to the `v1 flags` CONTEXT entry, (ii) double the TDD axis (every math fixture needs a math-off variant), and (iii) introduce a wire-contract bifurcation where the same input produces different AST depending on a flag — the exact failure mode ADR-0002's negative is preventing. Reject (b). ADR-0004 must echo this posture and the PRD's `v1 flags` enumeration stays at the current six flags, unchanged.

Vocabulary status: `inlineMath`, `math`, `meta` (math info-string field), **remark-math currency rule**, **Dollar-sign math (transport-only)** all now in `<product_dir>/CONTEXT.md` per Interviewer's Round-2 glossary update — confirmed read. New term introduced this Round: **unclosed-display-math fall-through rule** (Q7 / A7) — not yet in glossary; PRD or `to-prd` Stage should fold it into the `math` node entry or as a sibling to the `Frontmatter` unclosed-fence rule. Using terms `ast.InlineMath` / `ast.Math` (goldmark-side, litao91 package) — implementation-detail vocabulary, not on the wire, belongs in ADR-0004 not CONTEXT.md.

### VERDICT: continue
