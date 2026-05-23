# Arch log: 03-display-math-happy-path

Started: 2026-05-24
Scope: mini
File set (per tdd-log):
- `internal/translate/translate.go` — added `case *mathjax.MathBlock` + `translateMath` helper
- `internal/translate/translate_test.go` — added two Go-layer anchors
- `internal/emit/emit.go` — added `case "math"` to `writeNode`; added `"math"` to `isContainer` leaf-list
- 4 new fixtures under `testdata/fixtures/66..69` (read-only data — no arch concerns)

## Baseline
- Tests: all 6 packages green (`md2json`, `cli`, `emit`, `parse`, `read`, `translate`).
  Root package fresh run: 1.749s. Five internal packages: cached green.
- LOC of in-scope translate.go: 1329 lines. emit.go: 446 lines.

## Survey

### Candidate A: deletion test on `translateMath`

Imagined deletion: inline the ~10-line body into `translateNode`'s `case *mathjax.MathBlock` arm.

Result: 10 lines of `Lines().Value(src)` + `blockOffsets` + Node-literal land in the dispatch switch. Every other arm delegates to a same-named helper (translateHeading, translateParagraph, ..., translateFencedCodeBlock, translateCodeBlock, translateHTMLBlock, translateInlineMath, etc.) — consistent shape. Inlining breaks the pattern for one node type.

Interface = `(m, src, pt) -> *Node`. Implementation = value extraction via `Lines().Value(src)`, position via `blockOffsets`, plus the `meta: nil` (signals JSON null) convention. Interface < implementation = depth (small, but present — the helper hides the `Lines().Value(src)` + `blockOffsets` pair behind a typed signature). Naming aligns with CONTEXT.md `math node` glossary entry and ADR-0004 Decision 4 (`*mathjax.MathBlock` → `math`).

**Verdict: pass. Keep as-is.** Score: not a refactor candidate.

### Candidate B (re-evaluation of S02's deferred Candidate C): "value-concat shape across three callers"

S02's arch-log flagged the value-concat pattern in `translateCodeSpan` + `translateInlineMath` as Worth-exploring, predicting `translateMath` in S03 would become the third caller and tip the rule-of-three.

**Re-evaluation: S02's prediction is factually wrong.** `translateMath` uses a different value-extraction primitive entirely:

- `translateCodeSpan` (inline, line 769-784): iterates `*ast.Text` children, concatenates `src[seg.Start:seg.Stop]` for each.
- `translateInlineMath` (inline, line 406-441): same shape — iterate `*ast.Text` children, concatenate segments.
- `translateMath` (block, line 371-381): `string(m.Lines().Value(src))` — single library call, no child walk.

The shape divergence is structural to goldmark: `*mathjax.MathBlock` is a block node with body-line segments populated in `Lines()`; `*ast.CodeSpan` and `*mathjax.InlineMath` are inline containers whose interior bytes are exposed as `*ast.Text` children. The two extraction primitives are not interchangeable — block nodes have no `*ast.Text` children to iterate, and inline nodes have no `Lines()` segments.

So the inline-side `concatTextSegments` helper still has only TWO callers (translateCodeSpan + translateInlineMath), and the block-side `Lines().Value(src)` shape has FOUR callers (translateMath, translateFencedCodeBlock, translateCodeBlock, translateHTMLBlock — see grep at survey time).

The block-side shape (`Lines().Value(src)`) is already a one-call primitive — the four callers are NOT a refactor candidate; they're four callers of a library API.

The inline-side shape stays at two callers. Per S02's own honest framing ("at THREE callers, this becomes Strong"), the threshold isn't crossed. Plus the difference between the two inline callers' surrounding helpers (`textChildrenSpan` recurse vs `$`-run walk) means the shared shape is only the value loop, not the helper as a whole — extracting saves ~5 lines net across 2 sites, ~10 LoC, and concentrates the rule that's already named in CONTEXT.md "Text/Code value preservation" (a rule, not a function).

**Verdict: pass. Two callers is permission, not mandate. Wait for the genuine third inline caller (if one ever arrives — bracket-form `\(...\)` is an explicit v1 non-goal per CONTEXT.md, so it's unlikely soon).** Score: not a refactor candidate.

### Candidate C: deletion test on `isContainer` math entries

`emit.go::isContainer` now lists `"inlineMath", "math"` alongside `"text", "inlineCode", ...` (line 318). Both math types are leaves on the wire per CONTEXT.md `inlineMath node` (`inlineMath{value, position}` — no children) and `math node` (`math{value, meta, position}` — no children).

Imagined deletion: remove the math entries from the leaf-list. Result: `writeNode` would emit `"children":[]` after the `value`/`meta` fields, contradicting the mdast contract that math nodes are leaves. The list earns its keep — same as it did for `inlineCode` and `code`.

The negative-list polarity (enumerate LEAVES; default to container) was already vindicated by S02's Candidate B analysis; S03 adds one more leaf to the negative list (`"math"`), consistent with the bias.

**Verdict: pass. Keep as-is.** Score: not a refactor candidate.

### Candidate D: `translateMath` vs `translateFencedCodeBlock` structural near-twin

Both helpers:
- Call `Lines().Value(src)` for `value`.
- Call `blockOffsets(...Lines(), src)` for position span.
- Emit a leaf-shaped Node (no Children) with `ValuePresent: true`.

Differences:
- `translateFencedCodeBlock` parses an info string into `Lang` + `Meta` (~20 lines).
- `translateMath` always sets `Meta` to nil (1 line implicit) — CONTEXT.md `math node` "for `$$...$$` it is always `null`".

S03's tdd-log already considered this ("would require parameterizing on Type/Meta shape; the duplication is two lines and the abstraction would be more code than it saves"). Re-checking under the deletion test: imagine a `translateBlockValue(...) (value string, startOff, endOff int)` shared primitive consumed by both. That's two lines (value + offsets) shared, four callers if you count CodeBlock + HTMLBlock too — but each caller does something materially different after the shared two lines (FencedCodeBlock parses info; CodeBlock hardcodes lang/meta to nil; HTMLBlock appends ClosureLine bytes; MathBlock sets Meta nil). The shared prefix is two library calls.

**Deletion test on a hypothetical `translateBlockValue` helper**: removing it puts back `lines := ...; value := string(lines.Value(src)); startOff, endOff := blockOffsets(lines, src)` in 4 places. ~3 lines × 4 sites = 12 LoC. The helper would save ~8 net (minus its own signature/body), but at the cost of making each caller's value-and-position derivation indirect — readers would have to chase one more hop to verify each block-type's position-rule against the CONTEXT.md "Position info" glossary entry. The current shape already shows the `Lines()` → `Value` + `blockOffsets` pair side-by-side at each call site, which makes per-type review trivial.

**Verdict: pass. Keep as-is.** Score: Speculative at best. The four block-extraction sites are not duplication — they're four legitimate consumers of a small library-level pattern, each with distinct surrounding logic.

### Candidate E: position-span derivation `Lines() + blockOffsets` pattern

Same observation as D, viewed at the position-math layer rather than the value layer. `blockOffsets` itself IS the named seam ("blockOffsets returns the byte-offset range a heading-style block spans in source" — line 1278-1291). Five callers (Heading, FencedCodeBlock, CodeBlock, HTMLBlock, LinkReferenceDefinition, MathBlock — `grep -n "blockOffsets("` shows 6 if you count Heading too).

The helper exists, is deep (interface = `(lines, src) -> (start, end)`; impl wraps the line-start scan via `lineStartOffset`), and S03 reuses it correctly. No refactor needed.

**Verdict: pass. The seam is already named and used correctly.** Score: not a refactor candidate.

### Candidate F: `lossiness_property_test.go` `mdastNodeSetV1` map missing `inlineMath` / `math`

Pre-existing drift from S02; S03's tdd-log noted it. The map at `internal/translate/lossiness_property_test.go:45-73` enumerates 23 of the 25 mdast node types in CONTEXT.md "mdast node-set v1"; `inlineMath` and `math` are absent.

Effect: `TestEveryEmittedTypeIsInMdastNodeSetV1` would NOT catch a math node leaking with the wrong type name (e.g. `mathInline`, `blockMath`) — the property test would silently pass because the leaked type is also absent from the map, but the closed-enumeration check (`!mdastNodeSetV1[typeStr]`) would still flag it (the map check is the OUT-of-set direction, which works regardless). HOWEVER, `TestLossinessCorpusCoversEveryV1NodeType` (the corpus-coverage twin) ALSO does not flag the gap, because that test iterates the map keys — and `inlineMath`/`math` are not keys, so they're not required to be exercised by the corpus.

Net: the wire-contract enforcement for math node types is currently absent from the property test layer. A real regression (translate emits a goldmark-native type name for math) would still be caught (`!mdastNodeSetV1["someGoldmarkType"]` triggers Errorf), so the wire safety is preserved. The gap is in the POSITIVE-direction enforcement: nothing currently fails if the math fixtures stop emitting `inlineMath`/`math` entirely.

**Why not a 1-line fix:** Adding the two map entries alone (1 line each) would make `TestLossinessCorpusCoversEveryV1NodeType` start FAILING because `lossinessCorpus` (lines 121-171) does not include a `$...$` or `$$...$$` input. Closing the gap fully requires (a) two map entries AND (b) two new corpus entries to exercise them. (b) is adding test coverage — straight into the "Must not: Add tests. New behavior = out of scope." rule.

**Verdict: defer.** Note for future arch pass (likely the same "S07 or final-polish" pass S03's tdd-log already flagged it for). Not safe to fold in under mini-arch's scope.

## Pass count

Strong candidates this pass: zero.

No refactor was applied. Per protocol: "No Strong → 'no strong candidates this pass' + VERDICT: accept. Legitimate."

## Glossary alignment check (no-drift sweep)

Load-bearing terms from CONTEXT.md that appear in the in-scope S03 code:
- `math node` → code uses `"math"` literal + `translateMath` helper + `case "math"` in writeNode + `"math"` in isContainer leaf-list. Aligned.
- `Text/Code value preservation` → `translateMath` does verbatim `Lines().Value(src)` extraction, no trim/normalize; mirrors `translateFencedCodeBlock` and `translateCodeBlock`. Aligned.
- `Position info` → derived via `pt.position(blockOffsets(lines, src))`, body-only span (excludes opening / closing `$$` fences), mirrors `translateFencedCodeBlock` precedent. Aligned.
- `mdast node-set v1` → `math{value, meta, position}` (no `children`, no extra fields) — wire shape matches the enumeration entry verbatim. Aligned.
- `Lossiness policy (goldmark → mdast)` → `translateNode`'s default arm still returns nil (silent drop); `*mathjax.MathBlock` is now a recognized case alongside S02's `*mathjax.InlineMath`. Aligned.
- `Dollar-sign math (transport-only)` → no validation, no rendering, no macro expansion in `translateMath`; bytes flow through `Lines().Value(src)` verbatim. Aligned.

`_Avoid_` synonyms checked against the in-scope code:
- `mathBlock` — not in code as an mdast type literal. (`*mathjax.MathBlock` is the goldmark-side library type per ADR-0004; that's a legitimate library-type reference, not a wire-contract drift.) Good.
- `blockMath`, `displayMath` — not in code. Good.
- `math rendering`, `LaTeX support` — not in code. Good.
- `goldmark AST` (as a wire-contract phrase) — not in code. Good.

No drift. No load-bearing term missing from CONTEXT.md.

## Final

- Tests: all green (unchanged from baseline).
- LOC delta: 0.
- Most consequential change: none — survey-only pass. S03's implementation (one switch arm + ~10-LoC helper in translate, one switch arm + one leaf-list entry in emit) is already shaped consistently with the rest of translate/emit. The S02 hypothesis that `translateMath` would become a third inline-value-concat caller turned out to be wrong: block-math uses `Lines().Value(src)` (block-family primitive), not child-`*ast.Text`-segment concat (inline-family primitive). The inline-side value-concat shape stays at 2 callers (translateCodeSpan + translateInlineMath) — under the rule-of-three threshold S02 set, and the surrounding helpers diverge enough (textChildrenSpan-recurse vs $-run walk) that the shared shape is only the inner loop, not the helper as a whole. Naming aligns with CONTEXT.md `math node` and ADR-0004 Decision 4. Deletion test on `translateMath` fails (the helper earns its keep — keeps the dispatch switch homogeneous with the rest of the per-type helper pattern).

Deferred notes for the next arch pass (likely after issue 04+ or in final-arch):
- Candidate F (`mdastNodeSetV1` map missing `inlineMath`/`math`) requires expanding the lossiness corpus too — falls under the "Must not: Add tests" prohibition for mini-arch. The S07-or-final-polish pass S03's tdd-log already flagged for this is the right home.
- The inline-side value-concat shared shape stays Worth-exploring for a hypothetical future third inline caller; none currently planned (bracket-form `\(...\)` is an explicit v1 non-goal).

VERDICT: accept
