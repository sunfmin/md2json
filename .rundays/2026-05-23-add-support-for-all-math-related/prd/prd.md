# PRD: add-support-for-all-math-related

Status: ready-for-agent
Created: 2026-05-23
Last revised: 2026-05-23 (Round-3-critique fixes applied: predicate (i) restated CONTEXT.md-verbatim (drop "non-`$`" sub-clause); divergence fixture #4b added for input `$ 5 and $x$`; unclosed-`$$` blank-line-internal body declared out-of-scope; fixture #14 rewritten as behavioral A-vs-B equivalence assertion. Triggered-grill branch (c) translate-layer currency post-pass + ADR-0004 Decision 3 unchanged.)

## Problem

A user piping a GFM blog post through `md2json` today loses every `$x = 5$` and `$$\frac{a}{b}$$` span in the source: math is on ADR-0002's "Out of scope (post-v1)" list, so dollar-sign math is dropped or parsed as ordinary prose. Downstream renderers (KaTeX, MathJax) consuming `md2json`'s JSON envelope have nothing to render. Real-world Pandoc/CommonMark-extra inputs — the GitHub / VSCode / Obsidian common-denominator dialect on which the `v1 ship criterion` is anchored — currently round-trip with math content effectively missing from the AST.

## Solution

Extend the v1 pipeline so dollar-sign math source survives parse and lands on the wire as two new mdast node types: `inlineMath{value, position}` for `$...$` and `math{value, meta, position}` for `$$...$$`. md2json remains **transport-only** — it carries the LaTeX bytes through to the JSON envelope and never invokes a renderer, validates LaTeX, expands macros, or balances braces. The single static Go binary `Wedge` is preserved. The wire-contract delta is +2 node types and +1 optional field (`meta`, always `null` in v1.x); the schema-extension count is closed.

## User Stories

1. As a blog-post author piping GFM through `md2json`, I want `$x = 5$` in my prose to land in the AST as `inlineMath{value: "x = 5"}` so my downstream renderer can typeset it.
2. As the same author, I want `$$\frac{a}{b}$$` on its own paragraph to land as `math{value: "\\frac{a}{b}\n", meta: null}` so display equations render distinctly from inline math.
3. As a writer mentioning money, I want "It costs $5 and they had $10" to remain ordinary prose — no spurious `inlineMath` node — because the **remark-math currency rule** rejects whitespace-after-opening-`$` and digit-after-closing-`$`. The library's inline parser does NOT enforce these predicates (it matches by `$`-run-length equality only — see `probe/goldmark-mathjax/inline.go:24-52`); `translate` enforces the three predicates as a demote-only post-pass against each emitted `*ast.InlineMath`, converting predicate-rejected matches to `text` (per CONTEXT.md `remark-math currency rule` "translate-compensation responsibility" clause).
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
  2. **Currency rule compensation — translate-layer demote-only post-pass.** After goldmark emits the AST and before the goldmark→mdast structural walk produces its final tree, `translate` walks the AST for every `*ast.InlineMath` node and re-applies the three remark-math currency predicates against the original source bytes:
     - (i) **opener-followed-by-non-whitespace** — the byte at `inlineMath.opener-pos + 1` is non-whitespace (verbatim from CONTEXT.md `remark-math currency rule`: "opening `$` must be immediately followed by a non-whitespace character"). No "non-`$`" sub-clause.
     - (ii) **closer-preceded-by-non-whitespace** — the byte at `inlineMath.closer-pos - 1` is non-whitespace.
     - (iii) **closer-not-followed-by-digit** — the byte at `inlineMath.closer-pos + 1` is either EOF or a non-digit.

     On any predicate FAILURE, the post-pass demotes the entire `*ast.InlineMath` to an `*ast.Text` whose `Segment` spans the full original `$...$` range (opening `$`, interior bytes, closing `$` all included). The demoted node is then a plain text run; when `translateChildren` builds the paragraph's mdast children, the existing contiguous-text sibling-coalescing logic at `internal/translate/translate.go:225-231` merges the demoted text with adjacent `text` siblings on offset-contiguity, exactly the same way two ordinary `*ast.Text` siblings are merged. No new coalescing code introduced. Code surface: ~30 LoC inside `translate`.

     The post-pass is **demote-only** — it never re-promotes, never re-scans the demoted bytes for a later valid inline-math match (the library has already greedy-consumed those bytes once; re-parsing inside the demoted range is not available without re-invoking goldmark). This is the load-bearing semantic difference vs. remark-math's recursive scan; the divergence is documented and fixture-pinned (see Testing Decisions fixture #4b — input `$ 5 and $x$` — for the canonical divergence shape; fixture #4a pins the no-divergence-on-this-input convergence trace for input `$5 and $x$`).
- `emit` — adds the two new node types to its `switch n.Type` writer. `inlineMath` serializes `{type, value, position}`; `math` serializes `{type, value, meta, position}` with `meta` always rendered as JSON `null` (never elided). `--no-position` continues to strip `position` uniformly; no per-node-type special-casing.

**Extension library pick.** `github.com/litao91/goldmark-mathjax`, ratified Round 2 of grill-0, recorded in **ADR-0004** (sibling to ADR-0002). ADR-0004 supersedes ADR-0002's "Out of scope (post-v1)" bullet `Math ($...$, $$...$$) extensions. PRD non-goal.` and is the home for the goldmark-side implementation-detail names `ast.InlineMath` / `ast.Math` (which do **not** appear in `<product_dir>/CONTEXT.md` per PO direction). The math extender is appended to `parse.New`'s extension list — **`parse.New` is the single function by convention, with no central registry**, per ADR-0002 §"Negative (no central registry)". No new wiring style is introduced.

**Currency disambiguation — translate-layer post-pass over the library's matching-`$`-run rule.** The litao91 inline parser at `probe/goldmark-mathjax/inline.go:24-52` matches inline math purely by `$`-run-length equality between opener and closer; there is NO whitespace-after-opener check, NO whitespace-before-closer check, NO digit-after-closer check. The **wire-contract currency rule** (CONTEXT.md `remark-math currency rule`) is therefore enforced one layer up: `translate` runs the demote-only currency post-pass (above, sub-point 2) over every `*ast.InlineMath` the library emits, demoting predicate-failing matches back to `text`. The library + post-pass combination delivers the CONTEXT.md `remark-math currency rule` wire output for user story 3 (`It costs $5 and they had $10` → ordinary prose), at the cost of one narrow divergence vs. pure remark-math on inputs whose library greedy-match consumes through a later remark-math-valid inline-math span (post-pass cannot re-scan the demoted range — see fixture #4b for the pinned divergence shape on input `$ 5 and $x$`; fixture #4a pins the closely-related convergence trace on input `$5 and $x$`). This is the resolution recorded in **ADR-0004 Decision 3**.

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

3. **Currency rule — non-math prose (TDD-blocking; post-pass demote enforced).** Input `It costs $5 and they had $10` (28 bytes, no trailing newline) produces, on the **wire after translate's currency post-pass**:
   `root.children = [paragraph.children = [text{value: "It costs $5 and they had $10"}]]`.
   Derivation, step by step against `probe/goldmark-mathjax/inline.go:24-52` and `internal/translate/translate.go:225-231`:
   - **Library inline parse.** Inline parser triggered at orig pos 9 (`$`). opener-loop counts run = 1 (`line[9]='$'`, `line[10]='5'` stops the run). `block.Advance(1)`. Scan forward in the line slice: at offset 15 inside the post-advance slice (orig pos 25, `$`), oldi=15, inner loop reaches offset 16 (orig pos 26, `1`), closure=1==opener, `line[i+1]='0' != '$'` (offset 17 in slice, orig pos 27) → match closes. Child segment covers orig pos 10..25, `value="5 and they had "`. `block.Advance(16)` past orig pos 26. Library emits, in paragraph-child order: `[text{value:"It costs "} @ pos 0..9, *ast.InlineMath{value:"5 and they had ", opener-pos:9, closer-pos:25} @ pos 9..26, text{value:"10"} @ pos 26..28]`.
   - **Translate currency post-pass.** For the emitted `*ast.InlineMath`:
     - Predicate (i) opener-followed-by-non-whitespace: `src[10]='5'` → PASS.
     - Predicate (ii) closer-preceded-by-non-whitespace: `src[24]=' '` (space) → **FAIL**.
     - Predicate (iii) closer-not-followed-by-digit: `src[26]='1'` → **FAIL**.
     Predicate failure on (ii) and (iii); demote. The post-pass replaces the `*ast.InlineMath` with an `*ast.Text` whose `Segment` covers orig pos 9..26 (the full `$5 and they had $` range, opening and closing `$` included). Paragraph children pre-coalesce: `[*ast.Text@0..9, *ast.Text@9..26, *ast.Text@26..28]`.
   - **Translate-children coalesce.** Per `internal/translate/translate.go:225-231`, contiguous-by-offset sibling `text` nodes merge. First merge: end-offset 9 == start-offset 9 → coalesce to `text{value:"It costs $5 and they had $", pos 0..26}`. Second merge: end-offset 26 == start-offset 26 → coalesce to `text{value:"It costs $5 and they had $10", pos 0..28}`.
   - **Final wire shape**: one `paragraph` with one `text` child carrying the full original byte sequence. Zero `inlineMath` nodes. User story 3 satisfied.

4. **Currency rule — adjacent valid math.** Input `Use $x$ and $y$.` produces:
   `root.children = [paragraph.children = [text{value: "Use "}, inlineMath{value: "x"}, text{value: " and "}, inlineMath{value: "y"}, text{value: "."}]]`.
   Derivation: per `inline.go:38-52`, two independent inline-parser invocations (one at each `$` trigger). For `$x$`: opener=1, closer at pos 2 (line[3]=' ' != '$'), child segment = `x`. For `$y$`: opener=1, closer at pos 2 (line[3]='.' != '$'), child segment = `y`. Post-pass predicates on each emitted `*ast.InlineMath`: opener followed by `x`/`y` (PASS), closer preceded by `x`/`y` (PASS), closer followed by ` ` / `.` (no digit, PASS). No demote. Two distinct `inlineMath` nodes survive on the wire, separated by `text` runs.

4a. **Currency post-pass divergence fixture (`$5 and $x$`).** Input `$5 and $x$` (10 bytes, no trailing newline) produces, on the wire after translate's currency post-pass:
   `root.children = [paragraph.children = [inlineMath{value: "5 and $x"}]]`.
   Derivation, traced against `probe/goldmark-mathjax/inline.go:24-52`:
   - **Library inline parse.** Trigger at orig pos 0 (`$`). opener-loop: `line[0]='$'`, `line[1]='5'` stops → opener=1. `block.Advance(1)`. Post-advance slice = `5 and $x$` (9 chars, orig pos 1..10). Scan i=0..8:
     - i=6 `$`: oldi=6, inner-loop `line[6]='$'`, `line[7]='x'` stops → i=7, closure=1==opener. Closer-condition `(i+1=8 >= 9 || line[8] != '$')`: `line[8]='$'`, so `line[i+1] != '$'` is FALSE; 8 < 9. Whole condition FALSE — **first `$` at orig pos 7 does NOT close** (per the library's `line[i+1] != '$'` check at `inline.go:45`, which intentionally yields to a longer-run closer).
     - i=7 `x`: skip.
     - i=8 `$`: oldi=8, inner-loop `line[8]='$'`, i=9 hits `i<len(line)` boundary (len=9) → i=9, closure=1==opener. Closer-condition `(i+1=10 >= 9 || ...)` TRUE → close. Child segment: `segment.WithStop(segment.Start + 9 - 1)` = orig pos 1..9, `value="5 and $x"`. `block.Advance(9)`.
   - Trim-halfspace check (`inline.go:62-82`): first char `src[1]='5'` (not space) → no trim. Library emits one `*ast.InlineMath{value:"5 and $x", opener-pos:0, closer-pos:9}` covering orig pos 0..10. Paragraph has no other children (library consumed the full input).
   - **Translate currency post-pass.** Predicates on the emitted `*ast.InlineMath`:
     - (i) opener-followed-by-non-whitespace: `src[1]='5'` → PASS.
     - (ii) closer-preceded-by-non-whitespace: `src[8]='x'` → PASS.
     - (iii) closer-not-followed-by-digit: orig pos 10 is EOF (no byte) → PASS.
   - **No predicate failure → no demote.** Final wire shape: `[inlineMath{value:"5 and $x"}]`.
   - **Divergence vs. pure remark-math, pinned.** Pure remark-math (which scans `$`-by-`$` looking for a valid opener+closer pair satisfying the three predicates) would, on this same input, find: opener at pos 0 + `5` (PASS opener-pred), candidate closer at pos 7 preceded by ` ` (FAIL closer-preceded-pred) → skip pos 7, next candidate closer at pos 9 preceded by `x` (PASS) and followed by EOF (PASS) → match `inlineMath{value:"5 and $x"}`. **For this specific input, pure remark-math and library+post-pass converge** on `[inlineMath{value:"5 and $x"}]`. The divergence (i.e., where they do NOT converge) is pinned in fixture #4b below.

4b. **Currency post-pass divergence fixture — leading-whitespace opener (`$ 5 and $x$`).** Input `$ 5 and $x$` (11 bytes, no trailing newline) produces, on the wire after translate's currency post-pass:
   `root.children = [paragraph.children = [text{value: "$ 5 and $x$"}]]`.
   Zero `inlineMath` nodes. This is the **fixture-pinned divergence** vs. pure remark-math, which on the same input would emit `[text{value:"$ 5 and "}, inlineMath{value:"x"}]`. PO's branch-(c) ratification (ADR-0004 Decision 3) accepts this divergence as a bounded cost.
   Derivation, traced against `probe/goldmark-mathjax/inline.go:24-52` + `:62-82` and `internal/translate/translate.go:225-231`:
   - **Library inline parse.** Trigger at orig pos 0 (`$`). opener-loop: `line[0]='$'`, `line[1]=' '` stops → opener=1. `block.Advance(1)`. Post-advance slice = ` 5 and $x$` (10 chars, orig pos 1..11, slice indices 0..9 mapping to orig 1..10).
     Inner scan i=0..9 in slice:
     - i=0..6: skip (space, `5`, space, `a`, `n`, `d`, space — no `$`).
     - i=7 `$` (orig pos 8): oldi=7, inner-loop `slice[7]='$'`, `slice[8]='x'` stops → i=8, closure=1==opener. Closer-condition `(i+1=9 >= 10 || slice[9] != '$')`: slice[9]='$' → `!= '$'` is FALSE; 9 < 10. Whole condition FALSE — **first `$` at orig pos 8 does NOT close** (same `line[i+1] != '$'` yield-to-longer-run check at `inline.go:45` that fixture #4a exercises).
     - i=8 `x` (orig pos 9): skip.
     - i=9 `$` (orig pos 10): oldi=9, inner-loop `slice[9]='$'`, i=10 hits `i<len(line)` boundary (len=10) → i=10, closure=1==opener. Closer-condition `(i+1=11 >= 10 || ...)` TRUE → close. Child segment: `segment.WithStop(segment.Start + 10 - 1)` = orig pos 1..10 (Start=1, Stop=10), `value=" 5 and $x"` (9 chars including the leading space and the inner `$`). `block.Advance(10)`. Reader at orig pos 11.
   - **Trim-halfspace check (`inline.go:62-82`).** Node has one child with segment [1, 10). First-side check: `src[segment.Start]=src[1]=' '` (space) → condition `Source()[Start] == ' '` TRUE → outer `!(...)` FALSE → shouldTrimmed stays true. Last-side check: `src[segment.Stop-1]=src[9]='$'` (not space) → condition `Source()[Stop-1] == ' '` FALSE → outer `!(...)` TRUE → shouldTrimmed becomes false. **Trim does NOT fire** (asymmetric: first char is space but last char is `$`). Library emits one `*ast.InlineMath{value:" 5 and $x", opener-pos:0, closer-pos:10}` covering orig pos 0..11. Paragraph has no other children (library greedy-consumed the full input).
   - **Translate currency post-pass (canonical CONTEXT.md predicate (i), Defect-1-fix-respecting).** Predicates on the emitted `*ast.InlineMath`:
     - (i) opener-followed-by-non-whitespace: `src[opener-pos+1]=src[1]=' '` (space, IS whitespace) → **FAIL**.
   - **Predicate (i) failure → demote.** The post-pass replaces the `*ast.InlineMath` with an `*ast.Text` whose `Segment` covers orig pos 0..11 (the full `$ 5 and $x$` range, opening and closing `$` included). Paragraph children pre-coalesce: `[*ast.Text@0..11]` (single demoted node; no other siblings to coalesce against in this input).
   - **Translate-children coalesce.** Per `internal/translate/translate.go:225-231`, no contiguous-by-offset sibling text exists; single text child stands. Final wire shape: `root.children = [paragraph.children = [text{value:"$ 5 and $x$"}]]`. Zero `inlineMath` nodes.
   - **Divergence cited.** Pure remark-math (predicate-aware scan): opener at orig pos 0 followed by ` ` (FAIL predicate (i)) → skip pos 0 as opener-candidate, treat as literal text; next opener-candidate at orig pos 8 followed by `x` (PASS), find closer at orig pos 10 preceded by `x` (PASS) and EOF after (PASS) → match. Pure-remark-math wire: `[text{value:"$ 5 and "}, inlineMath{value:"x"}]`. Library+post-pass wire (this fixture): `[text{value:"$ 5 and $x$"}]`. The library greedy-matches across the would-be valid inner `$x$` span, and the demote-only post-pass cannot recover the inner match (no re-scan of demoted bytes — see ADR-0004 Decision 3). This fixture pins the divergence: any future implementation change that produces the pure-remark-math shape (e.g., recursive-rescan rewrite, library swap, library upgrade adopting predicates) makes this fixture fail explicitly, triggering an ADR-0004 reopen and PO re-ratification of branch (c) vs. an alternative.

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
    Derivation: GFM table cells are inline-content-only (block parser does not fire). The library's inline parser at the cell's `$$x$$` content: per `inline.go:26-28`, opener-count loop runs past both initial `$` chars → opener=2. Per `inline.go:38-52`, scans for closer with `$`-run-length=2 where next char is not `$`; the trailing `$$` at end-of-cell-content matches (closure=2, i+1 OOB → passes the `i+1 >= len(line)` branch). Child segment covers `x`. Library emits `*ast.InlineMath{value:"x", opener-pos:0, closer-pos:3}` over the cell-content bytes (cell content `$$x$$`, 5 bytes, indices 0..4; the closing `$$` run starts at index 3).
    **Survival check under the canonical (CONTEXT.md-verbatim) currency predicates (post-Defect-1 fix):**
    - (i) opener-followed-by-non-whitespace: src[opener-pos+1] = src[1] = `$`. `$` is **not whitespace** → PASS. (Under the Round-2 drifted "non-whitespace-non-`$`" predicate this would have FAILED and demoted to `text{value:"$$x$$"}`, contradicting the pinned tree; the canonical predicate restores fixture survival.)
    - (ii) closer-preceded-by-non-whitespace: src[closer-pos-1] = src[2] = `x` → PASS.
    - (iii) closer-not-followed-by-digit: src[closer-pos+1] = src[4] = `$` (non-digit) → PASS.
    No predicate failure → no demote. Wire output: `[inlineMath{value:"x"}]` as pinned. (Round-1 PRD's "the inline matcher declines on `$$`" claim was WRONG — verified against source, the inline matcher accepts `$$...$$` whenever opener=closer=2; the previous claim assumed a yield-to-block-parser behavior the library does not implement.)

12. **Frontmatter interaction.** Input:
    ```
    ---
    title: t
    ---
    $$\nx\n$$\n
    ```
    produces an envelope with `frontmatter = {title: "t"}` and `ast.children = [math{value: "x\n", meta: null}]`. The frontmatter codepath is untouched.

13. **`--no-position` strips math nodes uniformly.** Input `$x$` under `--no-position` produces a tree whose `inlineMath{value: "x"}` node has **no** `position` field; same input under default flags produces the same node **with** a `position` field. The rule is uniform per CONTEXT.md **Position info**; no per-node-type special-casing.

14. **Library behavior contract test (unclosed-`$$`-at-EOF) — behavioral A-vs-B equivalence.** A focused in-process unit test that parses two inputs through `litao91/goldmark-mathjax` (no `translate`, no `emit`, just goldmark) and asserts the AST-shape invariant the translate-layer unclosed-`$$` predicate relies on (PRD §Unclosed-fence behavior, ADR-0004 Decision 5):
    - Input **A** (unclosed): `$$\nx\n` (5 bytes; `$$`, LF, `x`, LF; no closing fence, EOF after body LF).
    - Input **B** (closed): `$$\nx\n$$\n` (7 bytes; `$$`, LF, `x`, LF, `$$`, LF).

    **Assertion:** both inputs produce a goldmark AST containing exactly one `*ast.Math` (`MathBlock`) node, and for that node, `MathBlock.Lines().Last().Stop` is **identical** on A and B (both equal to the byte offset immediately after the body line's terminating LF — orig pos 5 in A's source, orig pos 5 in B's source as well).

    **Derivation (why the assertion holds).** Per `probe/goldmark-mathjax/block.go:25-43`, `Open` on `$$\n` creates `MathBlock` with `data.indent=0`. Per `block.go:45-65`, `Continue` on the body line `x\n` enters the non-closing branch (no `$$+blank` match): `pos, padding := util.DedentPosition(line, 0, 0)` yields pos=0, padding=0; `seg := text.NewSegmentPadding(segment.Start+0, segment.Stop, 0)` covers the full body line including its trailing LF; `node.Lines().Append(seg)`. So `Lines().Last().Stop` = the body line's `segment.Stop` = orig pos 5 (offset past the body's terminating `\n`).
    - For A: next `PeekLine()` returns nil (EOF) → framework calls `block.Close()` (`block.go:67-69`) which only clears a context key. `MathBlock` survives with `Lines()` covering exactly `x\n`, `Last().Stop=5`.
    - For B: next `Continue` is called with line `$$\n`. `util.IndentWidth(line, 0)` returns w<4; inner `$`-run-count gives length=2; `IsBlank(line[i:])` on the trailing `\n` is TRUE → `Advance(...)` then return `parser.Close` (`block.go:49-57`). The closing fence line is NOT appended to `Lines()` (the return-Close branch returns before the `node.Lines().Append(seg)` at `block.go:62`). `MathBlock` survives with `Lines()` covering exactly `x\n`, `Last().Stop=5`.

    Both A and B leave `MathBlock.Lines().Last().Stop=5` — **behaviorally indistinguishable from the AST alone**. The translate post-pass's closed-vs-unclosed decision therefore must inspect `src[Lines().Last().Stop:]` (the bytes AFTER the recorded body) to find a closing fence, not any field on `MathBlock` itself. This is exactly what PRD §Unclosed-fence behavior + ADR-0004 Decision 5 specify.

    **Why behavioral, not structural.** A prior revision of this fixture asserted "no closed-state field exists on `MathBlock`" via reflection over the external package's struct. That assertion is fragile under upstream evolution (any unrelated field added to `MathBlock` breaks the test even though the behavior the compensation relies on is unchanged). The A-vs-B equivalence assertion is robust: it pins the **load-bearing behavior** ("AST-alone cannot distinguish closed from unclosed at the same body extent") rather than a **structural negation** ("the struct has no closed-state field"). If a future library upgrade adds a closed-state field but keeps `Lines().Last().Stop` identical on A and B, this test still passes and the translate compensation still works. If a future upgrade changes the behavior (e.g., switches A to decline-to-match, or appends the closing fence to `Lines()` in B making A's and B's `Stop` differ), the test fails explicitly and ADR-0004 reopens.

    Cited source: `probe/goldmark-mathjax/block.go:25-43` (Open), `block.go:45-65` (Continue body-line branch), `block.go:49-57` (Continue closing-fence branch), `block.go:67-69` (Close). The test pins library behavior, not wire behavior; fixture #5 holds the wire contract for unclosed-`$$` end-to-end.

**Property tests.** Existing lossiness and `--no-position` property tests extend to the new node types: any goldmark `*ast.InlineMath` / `*ast.Math` reached during tree-walk has a mdast target (lossiness set stays empty for math); any `position`-carrying math node strips under `--no-position` exactly like every other node.

**What is NOT tested.** LaTeX correctness (transport-only — that is the renderer's job). mhchem chemistry validity. Macro expansion. Brace balance. AMS environment structure. `\label`/`\ref` resolution.

## Out of Scope

- **Unclosed `$$` block whose body contains an internal blank line.** Behavior is implementation-defined in v1; the wire shape is whatever the translate post-pass emits and is NOT pinned by a fixture. Rationale: per `probe/goldmark-mathjax/block.go:46-64`, blank lines inside a `MathBlock` are appended to `Lines()` unconditionally — the close-detection branch at `block.go:54` requires a `$`-run of length ≥ 2 first (`length >= 2 && util.IsBlank(line[i:])`); a pure-blank line fails the `length >= 2` precondition and is appended as body. So input like `$$\n\frac{a}{b}\n\nother text\n` produces one `MathBlock` whose `Lines()` spans body bytes across the internal blank, with `Lines().Last().Stop` past `other text\n`. PRD §Unclosed-fence behavior demotes unclosed `$$` to a **single** `paragraph`, but goldmark's prose-parsing of the same source range would split at the internal blank into **two** paragraphs — the singular-paragraph compensation cannot represent this shape faithfully. Out-of-scope-shedding is v1-appropriate: real-world authors very rarely write an unclosed `$$` with an internal blank line, and inventing a multi-paragraph compensation shape would re-open the unclosed-fence-rule design. TDD-time finding (if real-world inputs hit this) → tracking issue + future Run. Cross-ref ADR-0004 Decision 5 (unclosed-`$$` compensation is for the no-internal-blank shape only) and CONTEXT.md `Unclosed-display-math fall-through rule`.
- LaTeX rendering of any kind (LaTeX→MathML, LaTeX→HTML, LaTeX→SVG). md2json is transport-only.
- LaTeX validation (mismatched braces, unknown macros, malformed AMS environments).
- Macro expansion, `\text{}` island handling, equation `\label`/`\ref` resolution.
- Math syntax surfaces other than dollar-sign: bracket form `\(...\)` / `\[...\]`, GitLab/Obsidian fenced ` ```math ``` `, AsciiMath, mhchem-as-separate-syntax, raw `<math>` MathML blocks (the last continues to land as `html{value}` per the existing raw-HTML rule).
- A `--no-math` runtime toggle (consistent with ADR-0002's runtime-extension-toggle posture; pinned in ADR-0004 Decision 6).
- A goldmark attribute-syntax surface (e.g., `{#id .class}` on `$$` blocks) populating `meta` on display math. `meta` stays `null` in v1.x; a future fenced-math Run is what wires it.
- Equation numbering UI / auto-numbering.
- Hand-rolling the math parser. Bounded-delegated to a library (`litao91/goldmark-mathjax`) per grill-0 Round 1 A4 and ADR-0004 Decision 1. The library's inline parser does NOT enforce the remark-math currency rule end-to-end (see ADR-0004 Decision 3); that rule is enforced one layer up by `translate`'s ~30-LoC demote-only currency post-pass, NOT by forking or hand-rolling the inline parser. Library identity stands.
- TOML frontmatter, MDX, streaming parse (unchanged from ADR-0001 / ADR-0002).

## Notes

- **ADR-0004** (`<product_dir>/docs/adr/0004-math-extension-library.md`, authored as a sibling output of this `to-prd` Stage): records `github.com/litao91/goldmark-mathjax` as the math extension library pick, wired exclusively through `parse.New`'s single-function-by-convention extension list, with translate-layer compensations for (1) the library's unclosed-`$$`-at-EOF behavior and (2) the **currency-rule demote-only post-pass** (Decision 3 — branch (c), ratified in the triggered-grill Round 1 A1).
- **ADR-0002** — the math bullet on its "Out of scope (post-v1)" list is superseded by ADR-0004; the other bullets (runtime `--extensions` flag, MDX, TOML, streaming) remain in force. ADR-0002 §"Negative (no central registry)" continues to govern the wiring style for this Run's extension addition.
- **ADR-0001** — input encoding / BOM / CRLF normalization rules apply uniformly to math content; `value` bytes are post-BOM-strip, post-LF-normalize per the existing rule.
- **CONTEXT.md vocabulary** consumed by this PRD (most already pinned by Interviewer's grill-0 glossary updates):
  - `Dollar-sign math (transport-only)`
  - `remark-math currency rule` — the entry's closing sentence has been updated (by the Interviewer prior to this PRD revision) to redirect the rule's enforcement from "extension-pick blocker" to "translate-compensation responsibility"; the three predicate checks themselves remain verbatim and still decide wire `inlineMath` membership.
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

This PRD ends with `VERDICT: accept` (in the Proposer-PRD agent's final message, not in this artifact body). All grill-0 + triggered-grill open questions resolved; fixture set is fully spec'd against the verified library source.
