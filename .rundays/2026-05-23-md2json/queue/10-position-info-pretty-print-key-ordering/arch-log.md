# Arch log: mini

Started: 2026-05-23
Scope: mini
File set (from S10 tdd-log "Files touched"):
- internal/translate/position.go (new `inlineSearchCursor` field +
  `findInline` method; new `bytes` import)
- internal/translate/translate.go (3 placeholder positions fixed —
  thematicBreak / autolink / footnoteLink; new `walkTextSegments`
  recursive helper; `textChildrenSpan` rewired to delegate)
- internal/translate/translate_test.go (4 new tests + corpus extensions)
- internal/translate/no_position_property_test.go (NEW file —
  `package translate_test` external-test-package property test)
- testdata/fixtures/52..56 (5 new wire fixtures)

## Baseline

- Tests: `go test ./... -count=1` clean across all packages (cli, parse,
  read, translate, top-level md2json). 57 top-level + 97 subtests = 154
  pass / 0 fail.
- `go vet ./...`: clean.
- LOC in S10 file set: translate.go 1219, position.go 137,
  translate_test.go 1137, no_position_property_test.go 106
  (2599 across the four Go source files).
- Placeholder-position audit: `grep -n "pt.position(0, 0)"
  internal/translate/translate.go` → 0 matches. All three S07/S08
  placeholders are closed out by S10's findInline + thematicBreak
  forward-scan + S06 arch-log C8 fix.

## Candidate inventory + scores

Reading CONTEXT.md, ADR-0001 + ADR-0002, prior arch-logs S05..S09, the
S10 tdd-log, and the four S10 source files. Applying CONTEXT.md "mdast
node-set v1" as the naming lens and the four refactor lenses (glossary
alignment, module depth, naming clarity, locality).

### C1 — `inlineSearchCursor` field + `findInline` method on positionTracker. Score: NOT-STRONG.

**Glossary alignment**: CONTEXT.md uses "inline" extensively as a
class of mdast nodes; "inline search cursor" + "findInline" name-aligns.
**Module depth**: findInline is small (8 LOC body) and hides three
invariants — bytes.Index lookup, cursor monotonicity, the (cursor,
cursor) zero-width fallback for the degenerate not-found case. The
single-line callsites in translateAutoLink/translateFootnoteLink are
much shorter than the inlined alternative would be (bytes.Index +
cursor advance + clamp + len-check at each callsite). DEEP, not
shallow. **Naming clarity**: "findInline" implies "find the inline node's
source bytes," which is exactly what it does — locate the literal source
syntax of a goldmark inline node that doesn't expose its segment via the
public API (AutoLink's `value *Text` is unexported; FootnoteLink carries
only `Index`). The name distinguishes itself from `bytes.Index`
(stateless, no cursor) and from goldmark's `Segment.Value` (which
returns the bytes for an already-known segment).
**Locality**: the per-Translate cursor lives on positionTracker —
the same struct that already threads through every translate call.
Stashing the cursor anywhere else would force a second piece of
per-Translate state to be threaded.

Deletion test: removing findInline puts a bytes.Index + manual cursor
advance at each of two callsites (translateAutoLink,
translateFootnoteLink) — ~6 lines each, plus the monotonicity invariant
becomes implicit-and-fragile rather than named-and-tested. Reject;
deeper than the surface area suggests.

### C2 — Could `findInline` live elsewhere (e.g. a separate `inlineLocator` struct)? Score: NOT-STRONG.

The cursor uses `pt.src` and `pt.inlineSearchCursor`. Both naturally
belong on positionTracker — `src` for the byte-offset math the cursor
walks, and the cursor itself is per-Translate state (matches
positionTracker's per-Translate lifetime). Splitting them into a
separate inlineLocator struct would force translate.go to thread TWO
per-Translate pointers (`pt *positionTracker` AND `il *inlineLocator`)
through every translate call — friction without locality gain. Already
the existing footnoteLabels map lives on positionTracker for the same
reason (S08 arch-log L1 codified this convention). NOT-STRONG.

### C3 — `walkTextSegments` vs `flattenAltText` structural duplication. Score: STRONG. Acted.

This is the load-bearing candidate this mini-arch surfaces. S10
introduced `walkTextSegments(parent ast.Node, fn func(seg
textm.Segment))` as a recursive walker over goldmark inline subtrees,
yielding each `*ast.Text` descendant's Segment to a visitor callback.
Its sole consumer is `textChildrenSpan` (min/max accumulation).

`flattenAltText(n ast.Node, src []byte) string` predates S10 (S06).
It walks an inline goldmark subtree, accumulating the *bytes* of every
`*ast.Text` descendant into a flat string. Its recursion structure is
STRUCTURALLY IDENTICAL to walkTextSegments:

  - both: `for c := n.FirstChild(); c != nil; c = c.NextSibling()`
  - both: `if t, ok := c.(*ast.Text); ok { … leaf action … continue }`
  - both: `else { recurseIntoContainer(c, …) }`

The only difference is the leaf action: walkTextSegments yields
`t.Segment` to a visitor; flattenAltText concatenates
`string(t.Segment.Value(src))` into the accumulator.

**Refactor lens: locality.** Today the recursion logic lives in TWO
places. A future change to "which goldmark inline containers count as
recursable" (e.g. excluding `*east.FootnoteLink` if it ever started
exposing children) would need to update both walkers in lockstep. The
S06 arch-log C8 bug — `textChildrenSpan` missing recursion into
inline containers, fixed in S10 — is exactly the class of bug this
duplication invites: the two walkers can drift.

**Refactor lens: module depth.** walkTextSegments gains a second
adapter, confirming it as a real seam under the "one adapter =
hypothetical, two adapters = real" rule the methodology pins. Before
this refactor, walkTextSegments is one-adapter (textChildrenSpan only).
After, it is two-adapter (textChildrenSpan + flattenAltText) and the
visitor seam is justified.

**Refactor lens: naming clarity.** Separates "how to traverse text
descendants of an inline subtree" (walkTextSegments) from "what to
accumulate at each leaf" (the visitor body — a span min/max OR a string
concat). The two concerns name themselves.

**Deletion test (after refactor).** If you delete walkTextSegments, the
recursion shape reappears at two callsites. Confirmed two-adapter
seam.

Action: rewrote flattenAltText's body as a walkTextSegments call. The
public name and signature of flattenAltText are unchanged; only the
implementation collapses to a visitor invocation.

## Pass 1: collapse `flattenAltText` onto `walkTextSegments`

- Files touched: internal/translate/translate.go (only).
- Tests after: 154 passing / 0 failing across all packages
  (`go test ./... -count=1`); `go vet ./...` clean.
- Reverted? no.
- LOC delta on translate.go: 0 (1219 → 1219). The body shed 7 lines of
  duplicate recursion, but I added 6 lines of doc comment naming the
  delegation explicitly (so a future reader sees the visitor-seam
  relationship without having to grep). Net wash; the structural win
  is the elimination of duplicate recursion, not the line count.
- The function's doc comment is unchanged; the only structural change
  is the body now reads:
  ```go
  var out string
  walkTextSegments(n, func(seg textm.Segment) {
      out += string(seg.Value(src))
  })
  return out
  ```
  vs the prior local for/if/recurse loop. The (parent ast.Node, src
  []byte) → string contract is preserved byte-for-byte; every existing
  test that exercised `flattenAltText` (Image alt flattening,
  imageReference alt flattening, etc.) stays green.

## Other candidates (logged, not acted)

### C4 — `translateImage` walks its subtree twice (textChildrenSpan + flattenAltText). Score: NOT-STRONG.

Both walks have the same shape but accumulate different products
(byte-offset min/max vs concatenated bytes). A merged
"walk-and-return-(span, alt)" helper would have a 4-value return and
would force every other textChildrenSpan caller (translateCodeSpan)
to ignore the alt component. The two-walk cost is negligible (each
inline subtree is tiny in practice — at most a handful of nodes for an
image alt) and the separation matches the single-responsibility shape
of each helper. Defer indefinitely; revisit only if a third "walk and
accumulate X" emerges.

### C5 — translate_test.go's local `itoa`. Score: NOT-STRONG.

S10 tdd-log notes the convention "translate avoids strconv pulls in
non-emit code by convention so far." The local `itoa` is 20 lines of
test infrastructure used in one error-path. `strconv.Itoa` would be
shorter but the convention is intentional (translate.go itself has no
strconv import; emit and parse do). This is stylistic preference, not
one of the four refactor lenses. NOT-STRONG.

### C6 — Property test reusability for S11's silent-drop property test. Score: NOT-STRONG (yet).

`countNodes` + `emitEnvelope` in no_position_property_test.go are
tightly scoped to position-key counting. A future silent-drop property
test (S11) would count dropped-vs-translated nodes, not position keys,
and would not use emit — so reusing these helpers would either dilute
their names ("countNodes" generalized for what?) or fork them. Either
way the right call is to grow S11's own helpers in the same external-
test package when the time comes; nothing to lift today.

### C7 — translate.go LOC at 1219 (unchanged post Pass 1). Score: NOT-STRONG.

S07 arch-log set the file-split re-evaluation threshold at ~1300 LOC.
S10 closed two deferred bugs (S06 arch-log C8 + S07's three placeholder
positions) so the LOC growth was minimal (+58 in S10). Re-evaluate at
final-arch (whole-codebase scope, runs once before the Acceptance
Gate) — that's the right place for file-boundary moves, not a mini-
arch limited to the S10 file set.

### C8 — `findInline`'s two adapters are at the threshold; can a third caller emerge? Score: NOT-STRONG.

Searched translate.go for other goldmark inline types that don't expose
their segment via the public API. The remaining inline types
(`*ast.Text`, `*ast.Emphasis`, `*ast.Link`, `*ast.Image`,
`*ast.CodeSpan`, `*ast.RawHTML`, `*east.Strikethrough`,
`*east.TaskCheckBox`) all expose either a `Segment` (direct or via
`Lines()`) or a `Segments` (RawHTML), or are containers whose children
carry the segments. The only segment-less inline kinds were AutoLink
and FootnoteLink — both already use findInline. No third caller in
sight; findInline is two-adapter STABLE.

## Final

- Tests: 154 passing (unchanged from baseline) across all packages.
- `go vet ./...`: clean.
- LOC delta on the S10 file set: translate.go +0 (1219 → 1219; 7-line
  body shrink + 6-line doc expansion net to wash); position.go +0;
  *_test.go +0. The win is structural (one recursion implementation,
  not two), not line-count.
- Most consequential change: `flattenAltText` now delegates its
  inline-subtree recursion to `walkTextSegments`. The two-adapter rule
  on walkTextSegments is now satisfied, confirming the visitor seam
  S10 introduced is real (rather than hypothetical). The structural-
  duplication risk between the two recursive walkers — the same class
  of risk that produced the S06 arch-log C8 deferred bug
  (textChildrenSpan missing recursion into inline containers) — is
  retired. Future changes to "what counts as a recursable inline
  container" update one place.

VERDICT: accept
