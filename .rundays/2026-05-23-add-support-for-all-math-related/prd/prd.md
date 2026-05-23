# PRD: add-support-for-all-math-related

Status: ready-for-agent
Created: 2026-05-23

## Problem

A user piping a GFM blog post through `md2json` today loses every `$x = 5$` and `$$\frac{a}{b}$$` span in the source: math is on ADR-0002's "Out of scope (post-v1)" list, so dollar-sign math is dropped or parsed as ordinary prose. Downstream renderers (KaTeX, MathJax) consuming `md2json`'s JSON envelope have nothing to render. Real-world Pandoc/CommonMark-extra inputs — the GitHub / VSCode / Obsidian common-denominator dialect on which the `v1 ship criterion` is anchored — currently round-trip with math content effectively missing from the AST.

## Solution

Extend the v1 pipeline so dollar-sign math source survives parse and lands on the wire as two new mdast node types: `inlineMath{value, position}` for `$...$` and `math{value, meta, position}` for `$$...$$`. md2json remains **transport-only** — it carries the LaTeX bytes through to the JSON envelope and never invokes a renderer, validates LaTeX, expands macros, or balances braces. The single static Go binary `Wedge` is preserved. The wire-contract delta is +2 node types and +1 optional field (`meta`, always `null` in v1.x); the schema-extension count is closed.

## User Stories

1. As a blog-post author piping GFM through `md2json`, I want `$x = 5$` in my prose to land in the AST as `inlineMath{value: "x = 5"}` so my downstream renderer can typeset it.
2. As the same author, I want `$$\frac{a}{b}$$` on its own paragraph to land as `math{value: "\\frac{a}{b}\n", meta: null}` so display equations render distinctly from inline math.
3. As a writer mentioning money, I want "It costs $5 and they had $10" to remain ordinary prose — no spurious `inlineMath` node — because the **remark-math currency rule** rejects whitespace-after-opening-`$` and digit-after-closing-`$`.
4. As a downstream consumer already on the unified/remark ecosystem, I want the node names (`inlineMath`, `math`) and field shapes (`value`, `meta`) to match `remark-math` verbatim so my existing visitors work without a renaming pass.
5. As a downstream renderer, I want `value` to be the literal source bytes between the delimiters — no entity decoding, no whitespace trim, no macro expansion — so I can pass them straight to KaTeX/MathJax which expect raw LaTeX.
6. As a writer who forgets the closing `$$`, I want my document to still parse: the unclosed `$$` and the body bytes that follow emit as `paragraph`/`text` per CommonMark (the **unclosed-display-math fall-through rule**), exit `0`. No hard error, no dropped bytes, no phantom-fence math node.
7. As a writer using math inside lists, blockquotes, footnotes, and table cells, I want inline `$...$` to compose uniformly inside any inline-content container. Block `$$...$$` is recognized at list-item / blockquote line-start, falls through to prose otherwise; block `$$...$$` is never recognized inside a table cell (GFM cells are inline-only).
8. As a user, I want math to be on by default with no flag to remember; the v1 wire contract pins the enabled extension set per ADR-0002, and math joins that pinned set.
9. As a writer with `\ce{H2O}` inside a `$...$`, I want it to ride through inside `value` byte-for-byte — md2json is transport-only, so mhchem, AMS environments, `\text{}` islands, `\label`/`\ref` are all downstream renderer concerns.
10. As a writer with mismatched braces or unknown macros, I want md2json to still exit `0` and emit the math node with the broken LaTeX in `value` — the renderer downstream is the one that reports LaTeX errors.

## Implementation Decisions

**Module touch-set.** Three modules see code changes; no new module is introduced:

- `parse` — registers one additional goldmark extender (`github.com/litao91/goldmark-mathjax`, per ADR-0004) by appending it to the extension list inside `parse.New`. The frontmatter / GFM / footnote wiring is unchanged. The math extender exposes goldmark-native node types (`ast.InlineMath`, `ast.Math` under the litao91 package, per ADR-0004) which `parse` forwards verbatim — `parse` does NOT consume the extension's `Renderer` (md2json never emits HTML; same pattern GFM/footnote/frontmatter already use).
- `translate` — adds two cases to its goldmark-AST→mdast switch: one for the math extension's inline node, mapping to `inlineMath{value, position}`; one for the math extension's block node, mapping to `math{value, meta: null, position}`. The `value` field carries the literal interior bytes per **Text/Code value preservation** (analogous to `code.value`); the `meta` field is always `null` in v1.x and stays in the schema as forward-compatibility for a deferred fenced-math Run (` ```math ... ``` `). `translate` additionally owns one library-behavior compensation: an unclosed-`$$`-at-EOF input causes the library to emit a partial `ast.Math` (the library does not decline-to-match in that case); `translate` detects the missing-closing-fence shape and demotes the node to a `paragraph` containing a single `text` whose `value` is the original `$$`-line and body bytes (per the **unclosed-display-math fall-through rule**). The compensation is deterministic — a single AST-shape check — not heuristic.
- `emit` — adds the two new node types to its `switch n.Type` writer. `inlineMath` serializes `{type, value, position}`; `math` serializes `{type, value, meta, position}` with `meta` always rendered as JSON `null` (never elided). `--no-position` continues to strip `position` uniformly; no per-node-type special-casing.

**Extension library pick.** `github.com/litao91/goldmark-mathjax`, ratified Round 2 of grill-0, recorded in **ADR-0004** (sibling to ADR-0002). ADR-0004 supersedes ADR-0002's "Out of scope (post-v1)" bullet `Math ($...$, $$...$$) extensions. PRD non-goal.` and is the home for the goldmark-side implementation-detail names `ast.InlineMath` / `ast.Math` (which do **not** appear in `<product_dir>/CONTEXT.md` per PO direction). The math extender is appended to `parse.New`'s extension list — **`parse.New` is the single function by convention, with no central registry**, per ADR-0002 §"Negative (no central registry)". No new wiring style is introduced.

**Currency disambiguation = library-implemented.** The chosen extension implements the **remark-math currency rule** byte-identically: opening `$` followed by non-whitespace, closing `$` preceded by non-whitespace AND not followed by a digit; display `$$` has no guard. md2json does NOT re-implement or post-validate this rule — if the library matches, we emit `inlineMath`; if it does not, the `$` bytes fall through to `text.value` per the library's own non-match path. Library fidelity to this rule is a load-bearing pick criterion per ADR-0004 (a future library upgrade that drifts on this rule is a Q4-reopen, not a translate patch).

**Unclosed-fence behavior.** Two mirrored rules, both pinned in CONTEXT.md (`Unclosed-display-math fall-through rule` and the implicit inline mirror inside `remark-math currency rule`):

- Block: an opening `$$` with no matching closing `$$` before EOF emits no `math` node on the wire; the `$$` line and the following bytes emit as a `paragraph` whose single `text.value` is the original opening-`$$`-line and body bytes. The library itself emits a partial `ast.Math` in this case; `translate`'s compensation (above) is what produces the wire-contract `paragraph`/`text` shape.
- Inline: an opening `$` with no closing `$` on the same line/paragraph simply does not match as `inlineMath` (the remark-math rule requires a closing `$` to fire); the `$` and subsequent bytes ride through as `text.value`. No `translate` compensation needed — the library declines to match.

**In-block composition.**

- Inline math inside `tableCell`, `listItem` paragraph, `blockquote` paragraph, `footnoteDefinition` → emits `inlineMath` as a child of the containing paragraph (or directly inside the cell for tables, per mdast's inline-content-direct-in-cell shape). Standard inline-in-block; no special-casing in `translate`.
- Display `$$...$$` inside `listItem` / `blockquote` → matches as `math` when `$$` appears at the start of the list-item / blockquote line; otherwise falls through to prose per the unclosed-fence rule. Arbitrarily-indented `$$` is not recognized (natural consequence of block-parser priority; not a bug).
- Display `$$...$$` inside a `tableCell` → never matches as `math` (GFM table cells are inline-content-only by spec, so the library's block parser does not fire). The inline matcher also declines on `$$` because the character immediately after the opening `$` is another `$` (the library's inline matcher distinguishes `$$` from inline `$...$` and yields). Result: the `$$x$$` bytes inside a table cell land verbatim in `text.value`. Documented as a wire-contract rule, not an accident.

**Lossiness posture unchanged.** The litao91 extension emits exactly two goldmark node types in this Run's scope; both have first-class mdast targets. The silent-drop set for math is empty by construction. CONTEXT.md `Lossiness policy (goldmark → mdast)` does not require an updated dropped-constructs entry for this Run.

**Flags posture unchanged.** No `--no-math` runtime toggle. `parse.New` wires the math extension unconditionally. A user who wants math-off runs a pre-this-Run binary. The `v1 flags` CONTEXT.md enumeration stays at six flags (`-o/--output`, `--pretty`, `--no-position`, `--frontmatter-only`, `-h/--help`, `-V/--version`); ADR-0002's "Out of scope (post-v1)" stance on runtime extension toggles is extended to cover math per ADR-0004 Decision 6.

**Schema delta on the wire (concrete):**

- `mdast node-set v1` gains: `inlineMath{value}`, `math{value, meta}`. Already enumerated in CONTEXT.md (Interviewer pre-landed the glossary edit in grill-0).
- `meta` on `math` is always JSON `null` for v1.x outputs. Forward-compat field, not a live feature.
- `position` is uniform on both new node types per the existing **Position info** uniform-rule.

## Testing Decisions

External-behavior tests, not implementation-detail tests. Fixtures live alongside existing `translate` / `emit` / integration tests; same harness style. Every fixture below is an **exact tree comparison** — no disjunctive ("either/or") acceptance, no "depending on" outcomes. Where a TDD-time check against the live library disagrees with a fixture, the fixture is authoritative (the wire contract is the rule; the library is an implementation pick beneath it) and either `translate` compensates or ADR-0004 is re-opened.

**Mandatory fixture set (TDD-blocking):**

1. **Inline happy path.** Input `$x = 5$` (whole document, no trailing newline) produces:
   `root.children = [paragraph.children = [inlineMath{value: "x = 5"}]]`.
   Every node carries `position` under default flags.

2. **Display happy path.** Input `$$\n\frac{a}{b}\n$$\n` produces:
   `root.children = [math{value: "\\frac{a}{b}\n", meta: null}]`.
   The trailing `\n` on the last content line is preserved per **Text/Code value preservation**'s `code.value` analogy. `meta` serializes as JSON `null` (not elided from the emitted object).

3. **Currency rule — non-math prose.** Input `It costs $5 and they had $10` produces:
   `root.children = [paragraph.children = [text{value: "It costs $5 and they had $10"}]]`.
   Zero `inlineMath` nodes anywhere in the tree. The trailing-digit guard on the closing `$` is the load-bearing rule.

4. **Currency rule — adjacent valid math.** Input `Use $x$ and $y$.` produces:
   `root.children = [paragraph.children = [text{value: "Use "}, inlineMath{value: "x"}, text{value: " and "}, inlineMath{value: "y"}, text{value: "."}]]`.
   Two distinct `inlineMath` nodes separated by `text` runs; the rule fires twice independently.

5. **Unclosed display math at EOF.** Input `$$\n\frac{a}{b}\n` (no closing `$$`, EOF after the body's `\n`) produces:
   `root.children = [paragraph.children = [text{value: "$$\n\\frac{a}{b}\n"}]]`.
   Zero `math` nodes; exit `0`. This is the post-compensation tree — the library emits a partial `ast.Math` in this case (per ADR-0004 Decision 5), and `translate`'s unclosed-`$$` compensation demotes it to the `paragraph`/`text` shape above. Mirror of the existing frontmatter unclosed-fence fixture.

6. **Unclosed inline at end-of-line.** Input `prose $x = 5 still prose` produces:
   `root.children = [paragraph.children = [text{value: "prose $x = 5 still prose"}]]`.
   Zero `inlineMath` nodes. The library's inline matcher declines (no closing `$`); no `translate` compensation involved.

7. **Value preservation.** Input `$$\n\ce{H2O}\n$$\n` produces:
   `root.children = [math{value: "\\ce{H2O}\n", meta: null}]`.
   mhchem source rides through byte-for-byte; md2json does not validate or expand `\ce`.

8. **Mismatched braces — inline.** Input `$\frac{a}{b$` produces:
   `root.children = [paragraph.children = [inlineMath{value: "\\frac{a}{b"}]]`.
   Exit `0`. Rationale: per the **remark-math currency rule**, opening `$` is followed by `\` (non-whitespace, non-`$`) — match opens; closing `$` is preceded by `b` (non-whitespace) and followed by nothing (no digit) — match closes. The unbalanced `{` rides through inside `value`. The broken-LaTeX downstream-renderer error is out of scope per the transport-only posture.

9. **In-block composition — list, blockquote, footnote.** Three sub-fixtures, each container with inline math inside its paragraph:
   - List: input `- prose $x$ more\n` produces a `list` whose `listItem` contains a `paragraph` whose children are `[text{value: "prose "}, inlineMath{value: "x"}, text{value: " more"}]`.
   - Blockquote: input `> prose $x$ more\n` produces a `blockquote` whose `paragraph` children are `[text{value: "prose "}, inlineMath{value: "x"}, text{value: " more"}]`.
   - Footnote: input `[^1]: prose $x$ more\n` produces a `footnoteDefinition{identifier: "1"}` whose `paragraph` children are `[text{value: "prose "}, inlineMath{value: "x"}, text{value: " more"}]`.

10. **In-block composition — display in list / blockquote.** Two sub-fixtures:
    - `$$` at list-item line-start matches as `math`: input `- $$\n  x\n  $$\n` produces a `list` whose `listItem` contains `[math{value: "x\n", meta: null}]` as a direct child.
    - Arbitrarily-indented `$$` falls through to prose: input `    $$x$$\n` (four-space indent, document root) parses as an indented `code` block, not as math — `root.children = [code{lang: null, meta: null, value: "$$x$$\n"}]`. (This pins the natural consequence of indented-code-block priority; no special-casing.)

11. **No block math in tables.** Input is a single-row GFM table with one cell containing `$$x$$`:
    ```
    | a |
    | --- |
    | $$x$$ |
    ```
    produces a `table` whose single `tableCell` has children `[text{value: "$$x$$"}]`. Zero `math` nodes, zero `inlineMath` nodes anywhere under the table. Rationale: GFM table cells are inline-content-only (block parser does not fire); the inline matcher declines on `$$` because the character immediately after the opening `$` is another `$` (the library's inline matcher yields `$$` to the block parser, which is unreachable in inline context, so both decline and the bytes fall through).

12. **Frontmatter interaction.** Input:
    ```
    ---
    title: t
    ---
    $$\nx\n$$\n
    ```
    produces an envelope with `frontmatter = {title: "t"}` and `ast.children = [math{value: "x\n", meta: null}]`. The frontmatter codepath is untouched.

13. **`--no-position` strips math nodes uniformly.** Input `$x$` under `--no-position` produces a tree whose `inlineMath{value: "x"}` node has **no** `position` field; same input under default flags produces the same node **with** a `position` field. The rule is uniform per CONTEXT.md **Position info**; no per-node-type special-casing.

14. **Library behavior contract test (unclosed-`$$`-at-EOF).** A focused unit test asserting `litao91/goldmark-mathjax`'s in-process behavior on input `$$\n\frac{a}{b}\n`: the library's parsed goldmark AST contains an `ast.Math` node whose interior covers the body bytes and which has no closing-fence sentinel (the library does not decline-to-match; it emits a partial block). This is the upstream assumption ADR-0004 Decision 5 and fixture #5 rest on. If this test ever fails (e.g., a future library upgrade switches to "decline to match" or hard-error), the failure mode is explicit: ADR-0004 is reopened. The test pins library behavior, not wire behavior.

**Property tests.** Existing lossiness and `--no-position` property tests extend to the new node types: any goldmark `ast.InlineMath` / `ast.Math` reached during tree-walk has a mdast target (lossiness set stays empty for math); any `position`-carrying math node strips under `--no-position` exactly like every other node.

**What is NOT tested.** LaTeX correctness (transport-only — that is the renderer's job). mhchem chemistry validity. Macro expansion. Brace balance. AMS environment structure. `\label`/`\ref` resolution.

## Out of Scope

- LaTeX rendering of any kind (LaTeX→MathML, LaTeX→HTML, LaTeX→SVG). md2json is transport-only.
- LaTeX validation (mismatched braces, unknown macros, malformed AMS environments).
- Macro expansion, `\text{}` island handling, equation `\label`/`\ref` resolution.
- Math syntax surfaces other than dollar-sign: bracket form `\(...\)` / `\[...\]`, GitLab/Obsidian fenced ` ```math ``` `, AsciiMath, mhchem-as-separate-syntax, raw `<math>` MathML blocks (the last continues to land as `html{value}` per the existing raw-HTML rule).
- A `--no-math` runtime toggle (consistent with ADR-0002's runtime-extension-toggle posture; pinned in ADR-0004 Decision 6).
- A goldmark attribute-syntax surface (e.g., `{#id .class}` on `$$` blocks) populating `meta` on display math. `meta` stays `null` in v1.x; a future fenced-math Run is what wires it.
- Equation numbering UI / auto-numbering.
- Hand-rolling the math parser. Bounded-delegated to a library (`litao91/goldmark-mathjax`) per grill-0 Round 1 A4 and ADR-0004 Decision 1.
- TOML frontmatter, MDX, streaming parse (unchanged from ADR-0001 / ADR-0002).

## Notes

- **ADR-0004** (`<product_dir>/docs/adr/0004-math-extension-library.md`, authored as a sibling output of this `to-prd` Stage): records `github.com/litao91/goldmark-mathjax` as the math extension library pick, wired exclusively through `parse.New`'s single-function-by-convention extension list, with a translate-layer compensation for the library's unclosed-`$$`-at-EOF behavior. Cross-references and supersedes ADR-0002's "Out of scope (post-v1)" bullet `Math ($...$, $$...$$) extensions. PRD non-goal.` Implementation-detail names `ast.InlineMath` / `ast.Math` live in ADR-0004, not in CONTEXT.md.
- **ADR-0002** — the math bullet on its "Out of scope (post-v1)" list is superseded by ADR-0004; the other bullets (runtime `--extensions` flag, MDX, TOML, streaming) remain in force. ADR-0002 §"Negative (no central registry)" continues to govern the wiring style for this Run's extension addition.
- **ADR-0001** — input encoding / BOM / CRLF normalization rules apply uniformly to math content; `value` bytes are post-BOM-strip, post-LF-normalize per the existing rule.
- **CONTEXT.md vocabulary** consumed by this PRD (all already pinned by Interviewer's grill-0 glossary updates; no new term introduced here):
  - `Dollar-sign math (transport-only)`
  - `remark-math currency rule`
  - `inlineMath` node
  - `math` node
  - `Unclosed-display-math fall-through rule`
  - `mdast node-set v1` (the two new entries `inlineMath{value}` and `math{value, meta}` are present)
  - `Markdown (input)` (the v1.x math surface is already named there as in-scope)
  - `Text/Code value preservation` (governs `value` semantics by analogy to `code.value`)
  - `Lossiness policy (goldmark → mdast)` (silent-drop set for math is empty by construction)
  - `Position info` (uniform rule applies to both new node types)
  - `v1 flags` (unchanged — no `--no-math`)
- **Library maintenance risk.** `litao91/goldmark-mathjax`'s last published tag predates current goldmark; the library is maintenance-mode, not abandoned. Fixture #14 is the regression net that pins the library's unclosed-`$$` behavior in-process, so a future upstream change surfaces explicitly rather than silently breaking fixture #5. A future Run may fork the (small) parser surface if upstream goes dark.
- **No new flags.** The `v1 flags` enumeration is unchanged.
