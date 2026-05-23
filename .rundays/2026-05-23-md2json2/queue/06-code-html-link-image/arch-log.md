# Arch log: 06-code-html-link-image

Started: 2026-05-23
Scope: mini
File set (per S06 tdd-log):
- internal/translate/translate.go
- internal/translate/translate_test.go
- internal/emit/emit.go
- testdata/fixtures/26-fenced-code-go-nopos … 33-image-flat-alt-nopos

## Baseline
- Tests: 34 passing, 0 failing (`go test ./...`)
- LOC: translate.go 712, emit.go 316 (total 1028 across the two source files)

## Candidate inventory + scores

Reading CONTEXT.md, S06 tdd-log, the two source files, and the new
translate unit tests; applying CONTEXT.md "mdast node-set v1" as the
naming lens and the four refactor lenses (glossary alignment, module
depth, naming clarity, locality).

### C1 — `translateNode` dispatch (~16 cases). Score: NOT-STRONG.

S05 said "re-evaluate at ~15 cases"; S06 added 7 more (16 total). Still
defensible: the switch IS the goldmark→mdast mapping spec, with
compile-time exhaustiveness; a registry/map of `func(ast.Node) *Node`
would cost the per-case static-type binding (`*ast.Heading` etc.) and
buy nothing — every handler still needs the same closure over `src`
and `pt`. Deletion test: replacing with a map gives a longer file, not
a shorter interface. Reject.

### C2 — `writeNode` per-type switch (~10 cases). Score: NOT-STRONG.

Same shape as C1 on the emit side. The switch IS the canonical
mdast-key-order spec; concentrating it is exactly the locality we
want. Reject.

### C3 — `Node` struct now has 15 fields. Score: NOT-STRONG.

Discriminated-union flat struct, matching the mdast tagged-union
shape. Per-type structs (e.g. `CodeNode`, `LinkNode`) would force an
interface-per-node-type hierarchy, breaking the single `switch n.Type`
property emit relies on for canonical key ordering. The trade S05
already considered; S06 doesn't change the calculus.

### C4 — Promote `nullableString` to a shared util pkg. Score: NOT-STRONG (yet).

`nullableString([]byte) *string` lives in `internal/translate` with 2
callers (`translateLink`, `translateImage`). Two adapters is on the
threshold; promotion would add ceremony (new package, import) for no
locality gain because translate is the only consumer today. S08's
forecast adds `definition.Title` / `linkReference.Label` callers —
that's the right time. Defer.

### C5 — Merge `translateRawHTML` and `translateHTMLBlock`. Score: NOT-STRONG.

Both produce `html{value}`. RawHTML pulls from `r.Segments`; HTMLBlock
pulls from `h.Lines()` + optional `h.ClosureLine`. Different goldmark
shapes, same mdast output — but a merged helper would have to dispatch
on the goldmark type internally, just like the outer switch already
does with compile-time exhaustiveness. Locality lost. Already rejected
in S06 tdd-log Pass 2; concur.

### C6 — Merge `writeJSONValue` and `writeJSONString`. Score: NOT-STRONG.

Both do `enc.SetEscapeHTML(false)` + strip-trailing-newline. Different
input types (`any` vs `string`), different call sites (per-envelope vs
per-field). Merging gives up compile-time string-vs-any safety.
Already rejected in S06 tdd-log Pass 3; concur.

### C7 — Extract `textChildrenSpan(parent ast.Node) (start, end int)` from `translateCodeSpan` and `translateImage`. Score: STRONG.

Both functions contain identical 13-line blocks:

```go
startOff, endOff := -1, -1
for c := <parent>.FirstChild(); c != nil; c = c.NextSibling() {
    if t, ok := c.(*ast.Text); ok {
        seg := t.Segment
        if startOff == -1 || seg.Start < startOff {
            startOff = seg.Start
        }
        if endOff == -1 || seg.Stop > endOff {
            endOff = seg.Stop
        }
    }
}
if startOff == -1 {
    startOff = 0
    endOff = 0
}
```

This is the "min/max byte-offset across direct `*ast.Text` children,
defaulting to (0,0) when none" calculation. Same goldmark shape (both
parents expose inline children via `FirstChild`/`NextSibling`), same
output type (a `(start, end int)` pair). Two real adapters of the same
function — past S05's "two adapters = real seam" threshold.

Deletion test: drop the helper, the 13-line block reappears twice and
will reappear a third time in S08 when `*ast.AutoLink` and the
reference-link family join the same family of "compute span across
text children" inline nodes. Helper earns its keep.

Naming: `textChildrenSpan` matches the existing `childrenSpan` helper
naming (already used by `translateEmphasis`/`translateList`/etc. for
the "min/max across already-translated mdast children" flavor) — the
two helpers are siblings: one walks goldmark, one walks mdast.

Locality + module depth: same five-line implementation block
concentrated behind a 1-line callsite, with the helper's doc comment
naming the convention (`FirstChild`/`NextSibling` walks `*ast.Text`
direct children only, ignoring inline containers — the position lens
is "where do my literal text bytes live", not "what does the inline
subtree look like").

Acting on this one.

### C8 — `translateImage` position-walk only sees direct text children. Score: NOT-STRONG (out of scope).

`translateImage` walks direct `*ast.Text` children for positions but
silently skips `*ast.Emphasis`/etc. containers inside the alt. So
`![an *emph* alt](url)`'s image-node position would not include the
`emph` text's bytes. This is potentially a real bug, but the existing
S06 fixtures all use `--no-position` so the wire effect is unobserved;
fixing it would CHANGE BEHAVIOR (position values that have been zero
become non-zero). Out of scope for arch. Defer to S10 (the position-
info pinning slice — that's the right place to assert image-position
contract).

## Pass 1: extract textChildrenSpan helper from translateCodeSpan + translateImage

- Files touched: internal/translate/translate.go
- Action: add helper `textChildrenSpan(parent ast.Node, src []byte) (start, end int)` near the existing `childrenSpan` helper. Rewrite `translateCodeSpan` and `translateImage` to call it.
- For `translateCodeSpan`, the `value` accumulation still happens inline (it depends on `src`-bytes per segment, not just offsets — different work).
- Helper signature: `textChildrenSpan(parent ast.Node) (start, end int)`. Returns `(0, 0)` when no direct text children. Matches the existing `(0, 0)`-on-empty convention of `childrenSpan` / `blockOffsets` / `paragraphOffsets`.
- Tests after: 34 PASS, 0 FAIL. `go vet` clean.
- Reverted? no.

## Final
- Tests: 34 PASS, 0 FAIL (unchanged from baseline).
- LOC delta: translate.go 712 → 721 (+9); emit.go unchanged (316). Net +9. The helper itself plus its doc comment is larger than the inline-duplicated arithmetic it replaces, but the duplicate raw-arithmetic block (13 lines in each of two sites = 26 lines) is now one named call (`textChildrenSpan(c)` / `textChildrenSpan(i)`) plus a single concentrated definition. This refactor is about LOCALITY and DEPTH, not byte count.
- Most consequential change: concentrated the "min/max byte-offset across direct `*ast.Text` children, defaulting to (0,0)" goldmark-walk pattern behind a single named helper `textChildrenSpan`, eliminating the duplicate block between `translateCodeSpan` and `translateImage`. The helper is a sibling to the existing `childrenSpan` (which walks already-translated mdast children); the pair now names both flavors of "position span from descendants" — one for goldmark inline-leaf positioning, one for mdast already-translated children — clearly. The S06 inline-node family (CodeSpan, Image) becomes one-line callsites; future S08 family members joining the same pattern (`*ast.AutoLink`, etc.) inherit the seam without re-introducing the arithmetic.

VERDICT: accept
