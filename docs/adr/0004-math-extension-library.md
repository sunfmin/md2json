# ADR-0004: math extension library and the dollar-sign math wire surface

- Status: Accepted
- Date: 2026-05-23
- Decider: PO (ratified in grill-0 Round 2 of the `add-support-for-all-math-related` Run)

## Context

CONTEXT.md `Dollar-sign math (transport-only)`, `remark-math currency rule`, `inlineMath` node, `math` node, and `Unclosed-display-math fall-through rule` pin the wire contract for the v1.x math Run. Two new mdast node types (`inlineMath`, `math`) and one new optional field (`meta` on `math`, always `null` in v1.x) are entering the closed `mdast node-set v1`. goldmark itself does not ship a math extension; ADR-0002's "Out of scope (post-v1)" line `Math ($...$, $$...$$) extensions. PRD non-goal.` is being reopened, and the choice of how `$...$` / `$$...$$` becomes goldmark-side AST needs a single record with rationale.

Bounded options surveyed in grill-0 Round 2:
- `go.abhg.dev/goldmark-mathjax` — does not exist; namespace owns `frontmatter` (per ADR-0002), `anchor`, `toc`, `wikilink`, `mermaid`, `hashtag`, but no math sibling.
- `github.com/litao91/goldmark-mathjax` — the de-facto goldmark math extension.
- Hand-rolled parser registered via `parser.Parser.AddOptions(...)`.
- `translate`-layer post-processing of `text` nodes — rejected in grill Round 1 as a layering violation.

## Decision

1. **Library pick.** `github.com/litao91/goldmark-mathjax`. Wired through `parse.New` as a single extension value, ignoring the library's `Renderer` (md2json never emits HTML; same pattern as GFM / footnote / frontmatter in ADR-0002).

2. **Wiring style.** Per ADR-0002 §"Negative (no central registry)", the enabled extension set is a single function (`parse.New`) by convention. The math extender is appended to that function's extension list. No new wiring mechanism is introduced.

3. **Currency rule fidelity is the load-bearing pick criterion.** `litao91/goldmark-mathjax`'s inline matcher implements the remark-math rule byte-identically: opening `$` followed by non-whitespace, closing `$` preceded by non-whitespace AND not followed by a digit; display `$$` has no guard. A future upstream change that drifts on this rule re-opens this ADR; it is not a `translate`-layer patch.

4. **goldmark-side node names consumed by `translate`.** `ast.InlineMath` → mdast `inlineMath{value, position}`. `ast.Math` → mdast `math{value, meta: null, position}`. The 1:1 name alignment between the library's `ast.*` types and the mdast targets is a pleasant accident, not the basis of the pick. These implementation-detail names live in this ADR; they do not appear in CONTEXT.md (which speaks only the wire contract).

5. **Unclosed-`$$`-at-EOF compensation lives in `translate`, not in the library.** The library's block parser on an opening `$$` with no closing `$$` before EOF emits a partial `ast.Math` whose interior is the scanned body (it does not decline-to-match; it does not hard-error). The `translate` layer detects this case (block math reaching EOF without a closing-fence sentinel) and demotes the construct to a `paragraph` containing a single `text` node whose `value` is the original `$$`-line plus the body bytes — exactly the wire contract pinned in CONTEXT.md `Unclosed-display-math fall-through rule`. The library is an implementation pick beneath the wire rule; the rule wins on disagreement.

6. **No runtime toggle.** Math is enabled unconditionally once this Run ships. The `v1 flags` enumeration in CONTEXT.md is unchanged (six flags). Consistent with ADR-0002's "Out of scope (post-v1)" stance on runtime extension toggles. A user who wants math-off runs a pre-this-Run binary.

## Consequences

- **Positive (ecosystem fidelity).** Node names (`inlineMath`, `math`), field names (`value`, `meta`), and currency-disambiguation rule all match the unified/remark ecosystem byte-identically. Downstream consumers (KaTeX, MathJax, remark-math-aware visitors) work without a renaming or normalization pass.

- **Positive (Wedge preserved).** Transport-only posture means no LaTeX renderer is linked in. The single static Go binary is unchanged; no Node/Python runtime is introduced.

- **Positive (lossiness set unchanged).** The library emits exactly `ast.InlineMath` and `ast.Math` within this Run's scope; both have first-class mdast targets. CONTEXT.md `Lossiness policy (goldmark → mdast)` requires no new dropped-constructs entry.

- **Negative (translate carries an unclosed-fence compensation).** Per Decision 5, `translate` owns one library-behavior-specific branch. The branch is deterministic (a single AST-shape check, not heuristic) and is covered by an explicit TDD fixture. The cost is that a future library upgrade with different unclosed-`$$` behavior is a Q4-reopen, not a silent regression.

- **Negative (library maintenance mode).** `litao91/goldmark-mathjax`'s last published tag predates current goldmark. The repo is maintenance-mode, not abandoned. The consumed surface is small (two AST node types and their parsers); a future Run may fork that surface if upstream goes dark.

- **Negative (no fenced ` ```math `).** The library does not parse fenced math; that surface was deferred in grill-0 Round 1 (Q1c). When a future Run un-defers fenced math, this ADR is revisited.

## Cross-references

- Supersedes ADR-0002 "Out of scope (post-v1)" bullet `Math ($...$, $$...$$) extensions. PRD non-goal.` Other ADR-0002 "Out of scope" bullets (runtime `--extensions` flag, MDX, TOML frontmatter, streaming parser) remain in force.
- CONTEXT.md `Dollar-sign math (transport-only)` — pins transport-only posture this ADR implements.
- CONTEXT.md `remark-math currency rule` — pins the rule this ADR makes the library's load-bearing criterion.
- CONTEXT.md `Unclosed-display-math fall-through rule` — pins the wire behavior Decision 5's translate compensation realizes.
- CONTEXT.md `inlineMath` node and `math` node entries — pin the field semantics this ADR's `translate` mapping targets.
- CONTEXT.md `mdast node-set v1` — already enumerates the two new node types.
- ADR-0001 — input encoding / BOM / CRLF normalization rules apply uniformly to math `value` bytes.
- ADR-0002 §"Negative (no central registry)" — pins the `parse.New`-by-convention wiring style this ADR follows.
