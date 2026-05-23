# PRD: add-support-for-all-math-related

Status: ready-for-agent
Created: 2026-05-23
Last revised: 2026-05-23 (Round 2 critic remediation; verified against litao91 source)

## Problem

A user piping a GFM blog post through `md2json` today loses every `$x = 5$` and `$$\frac{a}{b}$$` span in the source: math is on ADR-0002's "Out of scope (post-v1)" list, so dollar-sign math is dropped or parsed as ordinary prose. Downstream renderers (KaTeX, MathJax) consuming `md2json`'s JSON envelope have nothing to render. Real-world Pandoc/CommonMark-extra inputs — the GitHub / VSCode / Obsidian common-denominator dialect on which the `v1 ship criterion` is anchored — currently round-trip with math content effectively missing from the AST.

## Solution

Extend the v1 pipeline so dollar-sign math source survives parse and lands on the wire as two new mdast node types: `inlineMath{value, position}` for `$...$` and `math{value, meta, position}` for `$$...$$`. md2json remains **transport-only** — it carries the LaTeX bytes through to the JSON envelope and never invokes a renderer, validates LaTeX, expands macros, or balances braces. The single static Go binary `Wedge` is preserved. The wire-contract delta is +2 node types and +1 optional field (`meta`, always `null` in v1.x); the schema-extension count is closed.

## User Stories

1. As a blog-post author piping GFM through `md2json`, I want `$x = 5$` in my prose to land in the AST as `inlineMath{value: "x = 5"}` so my downstream renderer can typeset it.
2. As the same author, I want `$$\frac{a}{b}$$` on its own paragraph to land as `math{value: "\\frac{a}{b}\n", meta: null}` so display equations render distinctly from inline math.
3. As a writer mentioning money, I want "It costs $5 and they had $10" to remain ordinary prose — no spurious `inlineMath` node — because the **remark-math currency rule** rejects whitespace-after-opening-`$` and digit-after-closing-`$`. **(BLOCKED — see Open Questions §Currency rule fidelity.)**
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
- `translate` — adds two cases to its goldmark-AST→mdast switch: one for the math extension's inline node, mapping to `inlineMath{value, position}`; one for the math extension's block node, mapping to `math{value, meta: null, position}`. The `value` field carries the literal interior bytes per **Text/Code value preservation** (analogous to `code.value`); the `meta` field is always `null` in v1.x and stays in the schema as forward-compatibility for a deferred fenced-math Run (` ```math ... ``` `). `translate` additionally owns **two** library-behavior compensations:
  1. **Unclosed-`$$`-at-EOF compensation** (predicate pinned below in §Unclosed-fence behavior). The library emits a `MathBlock` regardless of whether the closing fence was seen (`MathBlock` carries no closed-state field — see `probe/goldmark-mathjax/block_node.go:5-13` and `block.go:67-69`). `translate` inspects the source bytes immediately after `MathBlock.Lines().Last().Stop` to decide closed-vs-unclosed; if unclosed, demote to a `paragraph` whose `text` children mirror what goldmark prose-paragraph parsing would have produced for the same source range (see fixture #5 for the exact shape).
  2. **Currency rule compensation** — **OPEN: see §Open Questions §Currency rule fidelity below.** The current PRD shape assumes the chosen library implements the remark-math currency rule. Verified-against-source: it does not (see `probe/goldmark-mathjax/inline.go:38-52` — the inline parser scans for matching `$`-run length without consulting whitespace-after-opening, whitespace-before-closing, or digit-after-closing). Routing decision (translate post-pass demotion vs fork inline parser vs accept rule loss) is grill-blocking.
- `emit` — adds the two new node types to its `switch n.Type` writer. `inlineMath` serializes `{type, value, position}`; `math` serializes `{type, value, meta, position}` with `meta` always rendered as JSON `null` (never elided). `--no-position` continues to strip `position` uniformly; no per-node-type special-casing.

**Extension library pick.** `github.com/litao91/goldmark-mathjax`, ratified Round 2 of grill-0, recorded in **ADR-0004** (sibling to ADR-0002). ADR-0004 supersedes ADR-0002's "Out of scope (post-v1)" bullet `Math ($...$, $$...$$) extensions. PRD non-goal.` and is the home for the goldmark-side implementation-detail names `ast.InlineMath` / `ast.Math` (which do **not** appear in `<product_dir>/CONTEXT.md` per PO direction). The math extender is appended to `parse.New`'s extension list — **`parse.New` is the single function by convention, with no central registry**, per ADR-0002 §"Negative (no central registry)". No new wiring style is introduced.

**Currency disambiguation — verified-against-source, not library-implemented (CONTRADICTS grill A4/A5).** The litao91 inline parser at `probe/goldmark-mathjax/inline.go:38-52` matches inline math purely by `$`-run-length equality between opener and closer:
```
for i := 0; i < len(line); i++ {
    c := line[i]
    if c == '$' {
        oldi := i
        for ; i < len(line) && line[i] == '$'; i++ {
        }
        closure := i - oldi
        if closure == opener && (i+1 >= len(line) || line[i+1] != '$') {
            // ... close match
        }
    }
}
```
There is no check for whitespace after the opener, no check for whitespace before the closer, no check for digit after the closer. Concrete consequence: input `It costs $5 and they had $10` parses (against litao91, verified by source trace) as `paragraph.children = [text{value:"It costs "}, inlineMath{value:"5 and they had "}, text{value:"10"}]` — exactly the failure mode grill A5 picked the remark-math rule to prevent. This invalidates ADR-0004 Decision 3's "byte-identically" claim and CONTEXT.md `remark-math currency rule`'s "extension-pick blocker, not a rule reopen" guard. Resolution path requires PO ratification — see §Open Questions §Currency rule fidelity below.

**Unclosed-fence behavior.** Two mirrored rules:

- **Block: unclosed `$$` at EOF — `translate` compensates via a src-tail predicate.** The library's block parser unconditionally emits a `MathBlock` carrying `Lines()` covering body lines (`probe/goldmark-mathjax/block.go:60-64` appends each body-line segment; the closing-fence branch at `block.go:49-57` only fires when a `$$<blank>` line is actually seen). `MathBlock` itself stores no closed/unclosed bit (`probe/goldmark-mathjax/block_node.go:5-7` — `MathBlock` embeds `ast.BaseBlock` and adds zero fields beyond it). **Predicate for `translate`'s closed-vs-unclosed check**: given `last := node.Lines().At(node.Lines().Len()-1).Stop`, walk `src[last:]` skipping LF/blank lines; if the next non-blank line consists of two-or-more `$` chars followed by a (whitespace-only) tail, the block was closed; otherwise it was unclosed. The predicate is a single forward scan over at most a handful of trailing bytes — deterministic, not heuristic. On unclosed: demote to a `paragraph` whose `text` children replay what goldmark prose parsing would have produced for the source range from the opening `$$` to the body's last line. Per goldmark's prose paragraph behavior (one `*ast.Text` segment per source line, no embedded LF in any text value), the wire shape is multiple `text` siblings — see fixture #5 for the exact tree.
- **Inline: unclosed `$` on the same paragraph — library declines, no compensation.** Per `probe/goldmark-mathjax/inline.go:33-37`, when the inline parser advances past the opener `$`-run and finds no closer before line==nil (paragraph end), it returns `ast.NewTextSegment(startSegment.WithStop(startSegment.Start + opener))` — the opening `$` bytes ride through as ordinary text. No `translate` compensation needed; the library's own non-match path is the correct wire behavior.

**In-block composition.**

- Inline math inside `tableCell`, `listItem` paragraph, `blockquote` paragraph, `footnoteDefinition` → emits `inlineMath` as a child of the containing paragraph (or directly inside the cell for tables, per mdast's inline-content-direct-in-cell shape). Standard inline-in-block; no special-casing in `translate`.
- Display `$$...$$` inside `listItem` / `blockquote` → matches as `math` when `$$` appears at the start of the list-item / blockquote line; otherwise falls through to prose per the unclosed-fence rule. Arbitrarily-indented `$$` is not recognized (natural consequence of block-parser priority; not a bug).
- Display `$$...$$` inside a `tableCell` → never matches as `math` (GFM table cells are inline-content-only by spec, so the library's block parser does not fire). For inline `$$x$$` inside a cell, the library's inline parser opens with opener=2 (it counts the `$` run, see `inline.go:26-28`), then scans for a closing `$`-run of length 2 where the char after the run is not `$` (`inline.go:45`). For input `$$x$$` (closure at end of cell, no follower), the closer matches and produces `inlineMath{value: "x"}` — **NOT** `text{value:"$$x$$"}`. Fixture #11 below pins the verified shape.

**Lossiness posture unchanged.** The litao91 extension emits exactly two goldmark node types in this Run's scope (`*ast.InlineMath` and `*ast.Math`); both have first-class mdast targets. The silent-drop set for math is empty by construction. CONTEXT.md `Lossiness policy (goldmark → mdast)` does not require an updated dropped-constructs entry for this Run.

**Flags posture unchanged.** No `--no-math` runtime toggle. `parse.New` wires the math extension unconditionally. A user who wants math-off runs a pre-this-Run binary. The `v1 flags` CONTEXT.md enumeration stays at six flags (`-o/--output`, `--pretty`, `--no-position`, `--frontmatter-only`, `-h/--help`, `-V/--version`); ADR-0002's "Out of scope (post-v1)" stance on runtime extension toggles is extended to cover math per ADR-0004 Decision 6.

**Schema delta on the wire (concrete):**

- `mdast node-set v1` gains: `inlineMath{value}`, `math{value, meta}`. Already enumerated in CONTEXT.md (Interviewer pre-landed the glossary edit in grill-0).
- `meta` on `math` is always JSON `null` for v1.x outputs. Forward-compat field, not a live feature.
- `position` is uniform on both new node types per the existing **Position info** uniform-rule.

## Testing Decisions

External-behavior tests, not implementation-detail tests. Fixtures live alongside existing `translate` / `emit` / integration tests; same harness style. Every fixture below is an **exact tree comparison** — no disjunctive ("either/or") acceptance, no "depending on" outcomes. Where a fixture's derivation depends on litao91's verified behavior, the fixture body cites `probe/goldmark-mathjax/<file>:<lines>` so the wire contract is grounded in the picked library's source.

**Mandatory fixture set (TDD-blocking):**

1. **Inline happy path.** Input `$x = 5$` (whole document, no trailing newline) produces:
   `root.children = [paragraph.children = [inlineMath{value: "x = 5"}]]`.
   Derivation: per `probe/goldmark-mathjax/inline.go:24-52`, opener=1 (single `$` at pos 0), advance, scan for closure with `$`-run-length=1 where next char is not `$`; closure at pos 6 (line[7] out-of-bounds, passes the `i+1 >= len(line)` branch); child segment covers `x = 5`. Trim-halfspace check at lines 62-82: `src[1]='x'` (not space) → no trim. Every node carries `position` under default flags.

2. **Display happy path.** Input `$$\n\frac{a}{b}\n$$\n` produces:
   `root.children = [math{value: "\\frac{a}{b}\n", meta: null}]`.
   Derivation: per `probe/goldmark-mathjax/block.go:25-43`, `Open` sees `$$` (i-pos=2 ≥ 2), creates `MathBlock`. `Continue` on `\frac{a}{b}\n` appends segment (lines 60-64). `Continue` on `$$\n`: scan `$`-run, length=2, `IsBlank(line[i:])` true → `Advance` then return `parser.Close` (lines 49-57). Lines() now holds just the `\frac{a}{b}\n` segment. `value` = literal bytes of that segment, trailing `\n` preserved per **Text/Code value preservation**'s `code.value` analogy. `meta` serializes as JSON `null` (not elided from the emitted object).

3. **Currency rule — non-math prose. SPEC TBD; current library behavior pinned.** Input `It costs $5 and they had $10` against **raw litao91 output (no translate compensation)** produces:
   `root.children = [paragraph.children = [text{value:"It costs "}, inlineMath{value:"5 and they had "}, text{value:"10"}]]`.
   Derivation: per `inline.go:38-52`, the inline parser at `$` (pos 9) matches opener=1, scans forward, finds closing `$` at pos 25 (closure=1, line[26]='1' != '$' → passes the closer-not-followed-by-`$` branch), so the child segment covers `"5 and they had "`. **The library does not consult the currency rule.** The **target wire shape** (zero `inlineMath` under user story 3) requires either a translate-layer demotion post-pass, an inline-parser fork, or a CONTEXT vocabulary update. This fixture is **non-final** until §Open Questions §Currency rule fidelity is resolved by PO — see fixture-shape branches there. Listed here so the deferred decision has a fixture pin.

4. **Currency rule — adjacent valid math.** Input `Use $x$ and $y$.` produces:
   `root.children = [paragraph.children = [text{value: "Use "}, inlineMath{value: "x"}, text{value: " and "}, inlineMath{value: "y"}, text{value: "."}]]`.
   Derivation: per `inline.go:38-52`, two independent inline-parser invocations (one at each `$` trigger). For `$x$`: opener=1, closer at pos 2 (line[3]=' ' != '$'), child segment = `x`. For `$y$`: opener=1, closer at pos 2 (line[3]='.' != '$'), child segment = `y`. Both fire regardless of which currency-rule resolution lands in §Open Questions — the openings/closings here all satisfy the conservative side of every option. Two distinct `inlineMath` nodes separated by `text` runs.

5. **Unclosed display math at EOF.** Input `$$\n\frac{a}{b}\n` (no closing `$$`, EOF after the body's `\n`) produces:
   `root.children = [paragraph.children = [text{value: "$$"}, text{value: "\\frac{a}{b}"}]]`.
   Derivation: per `block.go:25-43`, `Open` consumes `$$\n` and creates `MathBlock`. `Continue` on `\frac{a}{b}\n` appends segment (lines 60-64). EOF reached; framework calls `Close` (lines 67-69, which only nils a context key — `MathBlock` carries no closed-state field per `block_node.go:5-7`). At translate time, the closed-vs-unclosed predicate walks `src[Lines().Last().Stop:]` skipping LF/blanks; finds no `$$<blank>` fence line → **unclosed**. Compensation demotes: emit a `paragraph` whose `text` children mirror goldmark's standard prose-paragraph text-segmentation (one `*ast.Text` per source line, segments stop BEFORE the LF, no embedded LF in any text value — consistent with how multi-line prose paragraphs already shape on the wire). Two text nodes: `"$$"` (the opening line) and `"\\frac{a}{b}"` (the body line). Zero `math` nodes; exit `0`.

6. **Unclosed inline at end-of-line.** Input `prose $x = 5 still prose` produces:
   `root.children = [paragraph.children = [text{value: "prose $x = 5 still prose"}]]`.
   Derivation: per `inline.go:33-37`, when the inline parser advances past the opener `$` and finds no closer before line==nil, it returns a Text segment of just the opener bytes; the rest of the line continues as ordinary paragraph text and (via translate's contiguous-text coalescing — `translate.go:225-231`) merges into a single `text` node. Zero `inlineMath` nodes. No `translate` compensation involved.

7. **Value preservation.** Input `$$\n\ce{H2O}\n$$\n` produces:
   `root.children = [math{value: "\\ce{H2O}\n", meta: null}]`.
   Derivation: identical block-parser flow to fixture #2; the interior bytes `\ce{H2O}\n` ride through `MathBlock.Lines()` byte-for-byte. mhchem source is not validated or expanded by md2json (transport-only).

8. **Mismatched braces — inline.** Input `$\frac{a}{b$` produces:
   `root.children = [paragraph.children = [inlineMath{value: "\\frac{a}{b"}]]`.
   Derivation: per `inline.go:24-52`, opener=1 at pos 0, advance, scan from pos 1: at pos 10 (`$`), oldi=10, i=11, closure=1==opener. `i+1=12 >= len(line)=11` → closes (the `i+1 >= len(line)` branch on line 45 fires). Child segment covers pos 1..10 = `\frac{a}{b`. Trim-halfspace at lines 62-82: `src[1]='\\'` (not space) → no trim. Exit `0`. Unbalanced `{` rides through inside `value` per transport-only posture.

9. **In-block composition — list, blockquote, footnote.** Three sub-fixtures, each container with inline math inside its paragraph:
   - List: input `- prose $x$ more\n` produces a `list` whose `listItem` contains a `paragraph` whose children are `[text{value: "prose "}, inlineMath{value: "x"}, text{value: " more"}]`.
   - Blockquote: input `> prose $x$ more\n` produces a `blockquote` whose `paragraph` children are `[text{value: "prose "}, inlineMath{value: "x"}, text{value: " more"}]`.
   - Footnote: input `[^1]: prose $x$ more\n` produces a `footnoteDefinition{identifier: "1"}` whose `paragraph` children are `[text{value: "prose "}, inlineMath{value: "x"}, text{value: " more"}]`.

10. **In-block composition — display in list / blockquote.** Two sub-fixtures:
    - `$$` at list-item line-start matches as `math`: input `- $$\n  x\n  $$\n` produces a `list` whose `listItem` contains `[math{value: "x\n", meta: null}]` as a direct child.
    - Arbitrarily-indented `$$` falls through to prose: input `    $$x$$\n` (four-space indent, document root) parses as an indented `code` block, not as math — `root.children = [code{lang: null, meta: null, value: "$$x$$\n"}]`. (This pins the natural consequence of indented-code-block priority; no special-casing.)

11. **No block math in tables; inline matcher DOES fire on `$$x$$` inside a cell.** Input is a single-row GFM table with one cell containing `$$x$$`:
    ```
    | a |
    | --- |
    | $$x$$ |
    ```
    produces a `table` whose single `tableCell` has children `[inlineMath{value: "x"}]`. Zero `math` nodes anywhere under the table.
    Derivation: GFM table cells are inline-content-only (block parser does not fire). The library's inline parser at the cell's `$$x$$` content: per `inline.go:26-28`, opener-count loop runs past both initial `$` chars → opener=2. Per `inline.go:38-52`, scans for closer with `$`-run-length=2 where next char is not `$`; the trailing `$$` at end-of-cell-content matches (closure=2, i+1 OOB → passes the `i+1 >= len(line)` branch). Child segment covers `x`. (Round-1 PRD's "the inline matcher declines on `$$`" claim was WRONG — verified against source, the inline matcher accepts `$$...$$` whenever opener=closer=2; the previous claim assumed a yield-to-block-parser behavior the library does not implement.)

12. **Frontmatter interaction.** Input:
    ```
    ---
    title: t
    ---
    $$\nx\n$$\n
    ```
    produces an envelope with `frontmatter = {title: "t"}` and `ast.children = [math{value: "x\n", meta: null}]`. The frontmatter codepath is untouched.

13. **`--no-position` strips math nodes uniformly.** Input `$x$` under `--no-position` produces a tree whose `inlineMath{value: "x"}` node has **no** `position` field; same input under default flags produces the same node **with** a `position` field. The rule is uniform per CONTEXT.md **Position info**; no per-node-type special-casing.

14. **Library behavior contract test (unclosed-`$$`-at-EOF).** A focused unit test asserting `litao91/goldmark-mathjax`'s in-process behavior on input `$$\n\frac{a}{b}\n`: the parsed goldmark AST contains an `ast.Math` node whose `Lines()` segments cover the body bytes `\frac{a}{b}\n`, and no closed-state field exists on the node (verifiable by reflection or by direct field access — `MathBlock` is a struct with only the embedded `ast.BaseBlock`, per `probe/goldmark-mathjax/block_node.go:5-13`). This pins the upstream assumption ADR-0004 Decision 5 and fixture #5's compensation rest on. If this test ever fails (e.g., a future library upgrade adds a closed-state field, or switches to "decline to match" on unclosed input), the failure mode is explicit: ADR-0004 reopens. The test pins library behavior, not wire behavior.

**Property tests.** Existing lossiness and `--no-position` property tests extend to the new node types: any goldmark `*ast.InlineMath` / `*ast.Math` reached during tree-walk has a mdast target (lossiness set stays empty for math); any `position`-carrying math node strips under `--no-position` exactly like every other node.

**What is NOT tested.** LaTeX correctness (transport-only — that is the renderer's job). mhchem chemistry validity. Macro expansion. Brace balance. AMS environment structure. `\label`/`\ref` resolution.

## Out of Scope

- LaTeX rendering of any kind (LaTeX→MathML, LaTeX→HTML, LaTeX→SVG). md2json is transport-only.
- LaTeX validation (mismatched braces, unknown macros, malformed AMS environments).
- Macro expansion, `\text{}` island handling, equation `\label`/`\ref` resolution.
- Math syntax surfaces other than dollar-sign: bracket form `\(...\)` / `\[...\]`, GitLab/Obsidian fenced ` ```math ``` `, AsciiMath, mhchem-as-separate-syntax, raw `<math>` MathML blocks (the last continues to land as `html{value}` per the existing raw-HTML rule).
- A `--no-math` runtime toggle (consistent with ADR-0002's runtime-extension-toggle posture; pinned in ADR-0004 Decision 6).
- A goldmark attribute-syntax surface (e.g., `{#id .class}` on `$$` blocks) populating `meta` on display math. `meta` stays `null` in v1.x; a future fenced-math Run is what wires it.
- Equation numbering UI / auto-numbering.
- Hand-rolling the math parser. **Bounded-delegated to a library (`litao91/goldmark-mathjax`) per grill-0 Round 1 A4 and ADR-0004 Decision 1 — but see §Open Questions §Currency rule fidelity: that bounded delegation has an escalation precondition ("escalate if no library implements the Q5(a) remark-math currency rule") that has now been empirically tripped.**
- TOML frontmatter, MDX, streaming parse (unchanged from ADR-0001 / ADR-0002).

## Open Questions (grill-blocking)

**Currency rule fidelity.** Grill-0 Round 1 A5 ratified the **remark-math currency rule** as load-bearing: "opening `$` must be immediately followed by a non-whitespace character, closing `$` immediately preceded by a non-whitespace character, AND closing `$` must not be immediately followed by a digit." Grill-0 Round 1 A4 PO bounded-delegated Q4 to Interviewer with the explicit escalation precondition: "If Interviewer's Round-2 survey turns up that no maintained library implements the Q5(a) rule faithfully, escalate back to me before defaulting to hand-rolled." Round 2 Interviewer ratified litao91 on the claim that it "implements the remark-math currency guard verbatim." That claim is **false against the actual library source**:

> Per `probe/goldmark-mathjax/inline.go:38-52`, the inline parser matches by `$`-run-length equality only. There is no check for whitespace after the opener, no check for whitespace before the closer, no check for digit after the closer. Concrete trace: input `It costs $5 and they had $10` produces `inlineMath{value: "5 and they had "}` against litao91 — exactly the failure mode grill A5's rule was picked to prevent.

The escalation precondition is met. Three PO-scope resolution paths:

- **(a) Reopen ADR-0004 — switch to a faithful library, fork inline.go, or hand-roll.** No other published goldmark math extension implements the remark-math rule faithfully (Interviewer's Round-2 survey, re-verified). Fork option: the inline parser surface is ~50 LoC at `inline.go:24-84`; we add the three predicate checks (whitespace-after-opener, whitespace-before-closer, digit-after-closer) before the `closure == opener` branch fires, and vendor the forked parser inside `parse/internal/mathjax/`. Delivers byte-identical fidelity to remark-math. Cost: ~50 LoC vendored Go, ongoing maintenance, contradicts grill A4's "library, not hand-rolled" letter (but matches its escalation-precondition spirit).

- **(b) Accept litao91's matching-`$`-run rule; rewrite CONTEXT.md.** Drop the currency rule from CONTEXT.md `remark-math currency rule` entry; replace with a "matching-`$`-run rule" definition. User story 3 becomes a known-loss: `$5 ... $10` garbles. Currency prose corpora regress. Loud loss; contradicts grill A5's "produces AST that looks compatible but disagrees on which spans are math — worst of both worlds" rejection of non-remark-math rules.

- **(c) Translate-layer currency post-pass — demotion-only, partial fidelity.** `translate` walks each `*ast.InlineMath` and applies the three predicate checks against src bytes. On rejection, demote the matched range to `text` (which then coalesces with adjacent text per existing translate logic). Delivers correct output for user story 3's exact input (`$5 ... $10` becomes ordinary text). **Diverges from remark-math** in mixed prose-and-math cases that contain currency-rejected matches consuming bytes that remark-math would have skipped to find a later valid match. Concrete divergence: input `$5 and $x$` — remark-math produces `[text{value:"$5 and "}, inlineMath{value:"x"}]`; option (c) produces `[text{value:"$5 and $x$"}]` (the library's greedy first match consumes through the first valid closer; demotion converts the whole match to text; re-scanning inside the demoted range to find `$x$` is not available without re-parsing). Cost: ~30 LoC in translate. CONTEXT.md `remark-math currency rule` must soften from "byte-identical" to "approximate via demote-only post-pass; rare divergences documented" — a vocab update going through grill.

  Recommendation if PO picks (c): document the divergence as a known-limit in CONTEXT.md and add a fixture pinning the `$5 and $x$` divergence so it does not silently regress.

**This Run cannot proceed past `to-prd` until PO picks (a), (b), or (c).** The fixture set in §Testing Decisions is parametric on the pick — fixture #3 in particular has different target shapes per branch. `to-issues` cannot author tasks against an unresolved fixture #3.

## Notes

- **ADR-0004** (`<product_dir>/docs/adr/0004-math-extension-library.md`, authored as a sibling output of this `to-prd` Stage): records `github.com/litao91/goldmark-mathjax` as the math extension library pick, wired exclusively through `parse.New`'s single-function-by-convention extension list, with translate-layer compensations for (1) the library's unclosed-`$$`-at-EOF behavior and (2) the open currency-rule routing. Decision 3 ("currency rule fidelity") is now flagged-pending PO resolution of §Open Questions §Currency rule fidelity; the ADR records the verified-against-source library behavior and the three resolution branches.
- **ADR-0002** — the math bullet on its "Out of scope (post-v1)" list is superseded by ADR-0004; the other bullets (runtime `--extensions` flag, MDX, TOML, streaming) remain in force. ADR-0002 §"Negative (no central registry)" continues to govern the wiring style for this Run's extension addition.
- **ADR-0001** — input encoding / BOM / CRLF normalization rules apply uniformly to math content; `value` bytes are post-BOM-strip, post-LF-normalize per the existing rule.
- **CONTEXT.md vocabulary** consumed by this PRD (most already pinned by Interviewer's grill-0 glossary updates):
  - `Dollar-sign math (transport-only)`
  - `remark-math currency rule` — **NB: the "extension-pick blocker, not a rule reopen" guard in this entry is now in conflict with the verified library behavior; resolution depends on §Open Questions outcome.**
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
- **Soft-line-break handling note.** Defect #2 of critic Round 2 raised that the demoted-prose shape of fixture #5 must mirror ordinary multi-line-prose paragraph shape. Verified-against-source (`internal/translate/translate.go:225-231`): translate coalesces contiguous-by-offset sibling `text` nodes. Goldmark splits a multi-line prose paragraph into one `*ast.Text` segment per line, with each segment stopping BEFORE the LF (LF is not in any text segment). Result: a two-line prose paragraph emits two `text` siblings with no embedded LF, no synthetic `break` between them (soft-line-breaks generate no node — only hard breaks via `HardLineBreak()` do, per `translate.go:239-251`). Fixture #5's compensated shape now matches this: `[text{value:"$$"}, text{value:"\\frac{a}{b}"}]`, two siblings, no embedded LF.

## VERDICT pointer

This PRD ends with `VERDICT: trigger-grill` (in the Proposer-PRD agent's final message, not in this artifact body). Reason: §Open Questions §Currency rule fidelity requires PO resolution before `to-issues` can author tasks.
