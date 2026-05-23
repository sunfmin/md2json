# ADR-0004: math extension library and the dollar-sign math wire surface

- Status: Accepted-pending-PO-resolution (Decision 3 flagged — see §Open subquestion below)
- Date: 2026-05-23
- Decider: PO (ratified in grill-0 Round 2 of the `add-support-for-all-math-related` Run; Decision 3 re-opened by critic-Round-2 source-verification findings)

## Context

CONTEXT.md `Dollar-sign math (transport-only)`, `remark-math currency rule`, `inlineMath` node, `math` node, and `Unclosed-display-math fall-through rule` pin the wire contract for the v1.x math Run. Two new mdast node types (`inlineMath`, `math`) and one new optional field (`meta` on `math`, always `null` in v1.x) are entering the closed `mdast node-set v1`. goldmark itself does not ship a math extension; ADR-0002's "Out of scope (post-v1)" line `Math ($...$, $$...$$) extensions. PRD non-goal.` is being reopened, and the choice of how `$...$` / `$$...$$` becomes goldmark-side AST needs a single record with rationale.

Bounded options surveyed in grill-0 Round 2:
- `go.abhg.dev/goldmark-mathjax` — does not exist; namespace owns `frontmatter` (per ADR-0002), `anchor`, `toc`, `wikilink`, `mermaid`, `hashtag`, but no math sibling.
- `github.com/litao91/goldmark-mathjax` — the de-facto goldmark math extension.
- Hand-rolled parser registered via `parser.Parser.AddOptions(...)`.
- `translate`-layer post-processing of `text` nodes — rejected in grill Round 1 as a layering violation.

The library source was cloned into `<product_dir>/.rundays/<run_id>/probe/goldmark-mathjax/` for direct inspection during critic-Round-2 revision. Decisions below cite specific file:line ranges in that clone where the library's behavior is the load-bearing observable.

## Decision

1. **Library pick.** `github.com/litao91/goldmark-mathjax`. Wired through `parse.New` as a single extension value, ignoring the library's `Renderer` (md2json never emits HTML; same pattern as GFM / footnote / frontmatter in ADR-0002).

2. **Wiring style.** Per ADR-0002 §"Negative (no central registry)", the enabled extension set is a single function (`parse.New`) by convention. The math extender is appended to that function's extension list. No new wiring mechanism is introduced.

3. **Currency rule fidelity — FLAGGED-PENDING-PO.** Grill-0 Round 2 ratified litao91 on the explicit claim that it "implements the remark-math rule byte-identically." **That claim is false against the verified library source.** Per `probe/goldmark-mathjax/inline.go:38-52`, the inline parser scans char-by-char for matching `$`-run length:
   ```
   if c == '$' {
       oldi := i
       for ; i < len(line) && line[i] == '$'; i++ {
       }
       closure := i - oldi
       if closure == opener && (i+1 >= len(line) || line[i+1] != '$') {
           // ... match closes
       }
   }
   ```
   There is no check for whitespace after the opener (the char at `block.Position()` after `Advance(opener)`), no check for whitespace before the closer (`src[segment.Start + i - closure - 1]`), no check for digit after the closer (`line[i]` after the closing run). Concrete trace: input `It costs $5 and they had $10` produces `inlineMath{value: "5 and they had "}` against litao91. Grill-0 Round 1 A4 PO bounded delegation contained an explicit escalation precondition ("If Interviewer's Round-2 survey turns up that no maintained library implements the Q5(a) rule faithfully, escalate back to me before defaulting to hand-rolled"); the precondition is met. This ADR pins the three resolution paths in the §Open subquestion below; PO resolves before `to-issues` can proceed. The "extension-pick blocker, not a rule reopen" guard in CONTEXT.md `remark-math currency rule` is in direct conflict with this finding — vocab update is part of the resolution.

4. **goldmark-side node names consumed by `translate`.** `*ast.InlineMath` → mdast `inlineMath{value, position}`. `*ast.Math` → mdast `math{value, meta: null, position}`. The 1:1 name alignment between the library's `ast.*` types and the mdast targets is a pleasant accident, not the basis of the pick. These implementation-detail names live in this ADR; they do not appear in CONTEXT.md (which speaks only the wire contract).

5. **Unclosed-`$$`-at-EOF compensation lives in `translate`, not in the library. Predicate pinned to source.** The library's block parser unconditionally creates a `MathBlock` on opening `$$` (per `probe/goldmark-mathjax/block.go:25-43`, no closed-state side-effect) and unconditionally appends body-line segments to its `Lines()` (per `block.go:60-64`). The closing-fence branch at `block.go:49-57` fires ONLY when the current line in `Continue` is `$$+ blank tail`; if EOF is reached mid-block, the framework calls `block.Close()` (line 67-69) which only clears a context key. The `MathBlock` struct at `block_node.go:5-7` embeds `ast.BaseBlock` and adds **zero** fields beyond it — no `Closed bool`, no terminator segment, no unterminated marker. Therefore the unclosed-vs-closed distinction is **not observable from the AST alone**. The `translate` predicate inspects source bytes after `MathBlock.Lines().Last().Stop`: walk forward, skip LF/blank lines, check whether the next non-blank line consists of two-or-more `$` chars followed by a (whitespace-only) tail. If yes → closed (no compensation). If no (or EOF reached) → unclosed (compensation fires). The predicate is a single forward scan over a handful of trailing bytes — deterministic, not heuristic. On unclosed: demote the construct to a `paragraph` whose `text` children mirror goldmark's standard prose-paragraph text-segmentation (one `*ast.Text` segment per source line, segments stop BEFORE the LF, no embedded LF in any text value) — see PRD fixture #5 for the exact tree shape.

6. **No runtime toggle.** Math is enabled unconditionally once this Run ships. The `v1 flags` enumeration in CONTEXT.md is unchanged (six flags). Consistent with ADR-0002's "Out of scope (post-v1)" stance on runtime extension toggles. A user who wants math-off runs a pre-this-Run binary.

## Open subquestion (Decision 3 — grill-blocking)

Three PO-scope resolution paths, each with a concrete fixture #3 target shape, a code surface, and a CONTEXT vocabulary delta:

- **(a) Fork inline.go; vendor as `parse/internal/mathjax/`.** Surface: ~50 LoC. Add three predicate checks before the `closure == opener` branch at `inline.go:45`: (i) `src[startSegment.Start + opener]` is non-whitespace (opener-side rule), (ii) `src[segment.Start + i - closure - 1]` is non-whitespace (closer-preceded rule), (iii) `(i+1 >= len(line)) || !isDigit(line[i+1])` is true alongside the existing `line[i+1] != '$'` check (closer-not-followed-by-digit rule). On any predicate failure, the closer does NOT close; the scanner continues searching for the next valid closer. Delivers byte-identical fidelity to remark-math. Fixture #3 target shape: `paragraph.children = [text{value:"It costs $5 and they had $10"}]` (single text node, library never emits inlineMath for that input). CONTEXT.md unchanged. Cost: ~50 LoC vendored Go, ongoing maintenance against upstream litao91 drift. Contradicts grill A4's "library, not hand-rolled" letter, matches its escalation-precondition spirit.

- **(b) Accept litao91's matching-`$`-run rule; rewrite CONTEXT.md `remark-math currency rule` entry.** Code: zero additional LoC. Vocab: replace `remark-math currency rule` with a new term (`matching-$-run rule` or similar) defining the actual library behavior; remove the `extension-pick blocker` guard. Fixture #3 target shape: `paragraph.children = [text{value:"It costs "}, inlineMath{value:"5 and they had "}, text{value:"10"}]`. Currency prose corpora regress loudly. Contradicts grill A5's "produces AST that looks compatible but disagrees on which spans are math — worst of both worlds" rejection of non-remark-math rules.

- **(c) Translate-layer currency post-pass — demote-only.** `translate` walks each `*ast.InlineMath`, applies the three remark-math predicate checks against src bytes; on rejection, demote to `text` (subsequent contiguous-text coalescing per `internal/translate/translate.go:225-231` merges into surrounding text). Cost: ~30 LoC in translate. Delivers correct output for user story 3's exact input (`$5 ... $10` becomes ordinary text). **Diverges from remark-math** in mixed prose-and-math cases where the library's greedy first match consumes bytes containing a later valid math span; demotion converts the whole greedy match to text, and re-scanning inside the demoted range is not available without re-parsing. Concrete divergence: input `$5 and $x$` — remark-math produces `[text{value:"$5 and "}, inlineMath{value:"x"}]`; option (c) produces `[text{value:"$5 and $x$"}]`. Fixture #3 target shape: `paragraph.children = [text{value:"It costs $5 and they had $10"}]` (the demote-only post-pass converts the library's spurious match to text, which coalesces with surrounding text into a single text node — matches option (a)'s shape for THIS input, but differs on the `$5 and $x$` input). CONTEXT.md `remark-math currency rule` softens to "approximate via demote-only post-pass; rare divergences documented" plus an explicit fixture pinning the `$5 and $x$` divergence.

## Consequences

- **Positive (ecosystem fidelity).** Node names (`inlineMath`, `math`), field names (`value`, `meta`) all match the unified/remark ecosystem byte-identically. Downstream consumers (KaTeX, MathJax, remark-math-aware visitors) work without a renaming or normalization pass. **Currency-rule fidelity is open per Decision 3.**

- **Positive (Wedge preserved).** Transport-only posture means no LaTeX renderer is linked in. The single static Go binary is unchanged; no Node/Python runtime is introduced. All three Decision-3 branches preserve Wedge.

- **Positive (lossiness set unchanged).** The library emits exactly `*ast.InlineMath` and `*ast.Math` within this Run's scope; both have first-class mdast targets. CONTEXT.md `Lossiness policy (goldmark → mdast)` requires no new dropped-constructs entry.

- **Negative (translate carries an unclosed-fence compensation with a src-byte predicate).** Per Decision 5, `translate` owns one library-behavior-specific branch. The predicate is deterministic (a single forward src-byte scan after `Lines().Last().Stop`, not heuristic) and is covered by an explicit TDD fixture (PRD fixture #5 for the wire contract, PRD fixture #14 for the library-behavior assertion). The cost is that a future library upgrade with different unclosed-`$$` behavior is a ADR-0004 reopen, not a silent regression.

- **Negative (Decision 3 introduces translate carries a currency-rule compensation, OR a vendored inline parser fork, OR a CONTEXT vocab regression).** Open per §Open subquestion above.

- **Negative (library maintenance mode).** `litao91/goldmark-mathjax`'s last published tag predates current goldmark. The repo is maintenance-mode, not abandoned. The consumed surface is small (two AST node types and their parsers); a future Run may fork that surface if upstream goes dark. Decision-3 option (a) effectively pre-emptively forks.

- **Negative (no fenced ` ```math `).** The library does not parse fenced math; that surface was deferred in grill-0 Round 1 (Q1c). When a future Run un-defers fenced math, this ADR is revisited.

## Cross-references

- Supersedes ADR-0002 "Out of scope (post-v1)" bullet `Math ($...$, $$...$$) extensions. PRD non-goal.` Other ADR-0002 "Out of scope" bullets (runtime `--extensions` flag, MDX, TOML frontmatter, streaming parser) remain in force.
- CONTEXT.md `Dollar-sign math (transport-only)` — pins transport-only posture this ADR implements.
- CONTEXT.md `remark-math currency rule` — pins the rule Decision 3 now flags as library-violated. The CONTEXT entry's "extension-pick blocker, not a rule reopen" guard is in conflict with the verified library source and is part of the §Open subquestion resolution.
- CONTEXT.md `Unclosed-display-math fall-through rule` — pins the wire behavior Decision 5's translate compensation realizes; the rule's TDD-blocking-finding clause is the precedent for Decision 5's src-byte predicate.
- CONTEXT.md `inlineMath` node and `math` node entries — pin the field semantics this ADR's `translate` mapping targets.
- CONTEXT.md `mdast node-set v1` — already enumerates the two new node types.
- ADR-0001 — input encoding / BOM / CRLF normalization rules apply uniformly to math `value` bytes.
- ADR-0002 §"Negative (no central registry)" — pins the `parse.New`-by-convention wiring style this ADR follows.
- Probe clone: `<product_dir>/.rundays/2026-05-23-add-support-for-all-math-related/probe/goldmark-mathjax/` — `block.go`, `inline.go`, `block_node.go`, `mathjax_test.go`. This is the source-of-truth for every "the library does X" claim in this ADR; future-maintainer reading this ADR can re-verify any Decision against the cited file:line ranges.
