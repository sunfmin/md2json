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
<left empty — next Cycle's PO fills>

### VERDICT: continue
