# TDD log: 08-references-definitions-footnotes

Started: 2026-05-23

## Reconnaissance

Verified the goldmark surface area before writing the first test:

- `*ast.Link` carries `Reference *ast.ReferenceLink` (nil for inline links, non-nil
  for reference-style). `ReferenceLink.Type` is `ReferenceLinkFull | Collapsed |
  Shortcut`, `Value` is the label bytes between the brackets that produced the
  reference (the raw — i.e. un-normalized — label text).
- `*ast.Image` reuses the same `baseLink` struct, so it also has `Reference`.
- `*ast.LinkReferenceDefinition` is retained as a sibling of its enclosing
  paragraph (see `parser/link_ref.go` `Transform`: `node.Parent().InsertBefore(...)`).
  No special hook needed — extending `translateNode` with a case is enough.
- Footnote extension (already wired in `parse.New` from S03) inserts
  `*extension/ast.Footnote` blocks into a `*extension/ast.FootnoteList`
  container that the transformer appends to the document root. The list
  wrapper has no mdast analog; we flatten it. Each `Footnote.Ref` is the
  label bytes (`"a"` for `[^a]: ...`). Inline `*extension/ast.FootnoteLink`
  carries only an `Index` (1-based); to recover the mdast identifier we
  walk the FootnoteList and match by Index → Ref. The
  `*extension/ast.FootnoteBacklink` injected at the end of each definition
  is presentation-only; silent-drop per CONTEXT.md "Lossiness policy".

Identifier vs label (mdast):
- `label` = the original raw label as written (preserves case, whitespace).
- `identifier` = the normalized form (CommonMark §4.7: trim, case-fold, collapse
  inner whitespace). goldmark's `util.ToLinkReference` performs exactly this
  normalization.

## Test 1 — full linkReference + definition (acceptance criterion #1)
- Wrote: TestTranslateLinkReferenceFullEmitsLinkReferenceAndDefinition (unit) +
  fixture 39-link-reference-full-with-definition-nopos (wire).
- Red: compile-time RED on the missing Identifier/Label/ReferenceType fields on
  `Node`.
- Green: extended `Node` with the three new string fields; introduced
  `translateLinkReferenceDefinition` (mapping `*ast.LinkReferenceDefinition` →
  `definition`); branched `translateLink` on `Reference != nil` to emit
  `linkReference` instead of `link`; added `identifierFromLabel`
  (delegating to `util.ToLinkReference`) and `referenceTypeToMdast` helpers.
  Emit gained `linkReference` and `definition` cases in the type switch and
  added `definition` (plus future `imageReference`, `footnoteReference`) to
  the leaf set in `isContainer`.
- Notes: goldmark's `Link.Reference *ReferenceLink` already discriminates
  reference-style from inline; no parser customization needed.

## Test 2 — collapsed / shortcut referenceType (acceptance criteria #2, #3)
- Wrote: TestTranslateLinkReferenceCollapsedAndShortcutMapToCorrectReferenceType
  (sub-tests "collapsed" and "shortcut") + fixtures 40-link-reference-collapsed-nopos
  and 41-link-reference-shortcut-nopos.
- Red: n/a — Test 1's implementation already generalized over `ReferenceLinkType`
  via `referenceTypeToMdast`. Wrote the assertions and fixtures anyway so the
  load-bearing "goldmark `ReferenceLinkCollapsed`/`Shortcut` enum → mdast
  `"collapsed"`/`"shortcut"` string" mapping is anchored at both the unit and
  wire boundaries — a future refactor that collapsed the switch arms would
  surface here independently of the fixture suite.
- Green: passed on first run.
- Notes: confirmed identifier/label for collapsed and shortcut come from the
  visible link text (CommonMark §6.6 normalization rule, applied by
  `util.ToLinkReference`).

## Test 3 — imageReference + definition (acceptance criterion #4)
- Wrote: TestTranslateImageReferenceFullEmitsImageReferenceAndDefinition (unit)
  + fixture 42-image-reference-full-nopos (wire).
- Red: assertion-level RED — `translateImage` was still emitting `"image"`
  on a reference-style image because the `Reference != nil` branch hadn't been
  added.
- Green: extended `translateImage` with the same `Reference != nil` branch
  used in `translateLink`, emitting `imageReference` with the same
  identifier/label/referenceType triple. The alt flattens via the existing
  `flattenAltText` helper (S06's "image alt is a flat string" rule applies
  unchanged). Emit got an `imageReference` case mirroring `linkReference`
  plus the `alt` field; `isContainer` already had `imageReference` in the
  leaf set (added defensively at Test 1).
- Notes: `*ast.Image` and `*ast.Link` share `baseLink`, so the same
  `Reference *ReferenceLink` discriminator works for both with no
  additional plumbing.

## Test 4 — footnote reference + definition (acceptance criterion #5)
- Wrote: TestTranslateFootnoteReferenceAndDefinition (unit) + fixture
  43-footnote-reference-and-definition-nopos (wire).
- Red: assertion-level RED — `root.Children` was 1 (just the paragraph), not
  2 (paragraph + footnoteDefinition). The inline `*east.FootnoteLink` and
  the block-level `*east.FootnoteList` both silent-dropped through
  `translateNode`'s default arm because no cases existed.
- Green: introduced two pieces of new translate machinery:
  1. `collectFootnoteLabels` document-level pre-pass populating a new
     `positionTracker.footnoteLabels map[int]string` (goldmark's
     1-based `Footnote.Index` → source `Footnote.Ref` label bytes). This
     lets the inline `FootnoteLink` (which only carries `Index`) recover
     the mdast `identifier`/`label`.
  2. `translateChildren` FootnoteList-flattening fast-path: when a child
     is `*east.FootnoteList`, splice each `*east.Footnote` (translated
     to `footnoteDefinition`) directly into the parent's children list
     and silent-drop the wrapper. The mdast contract has
     `footnoteDefinition` as a top-level sibling, not nested under a
     list container.
- New translate functions: `translateFootnote` (block → footnoteDefinition,
  reusing `translateChildren` for the body so the backlink injected by
  goldmark's transformer silent-drops cleanly through the default arm) and
  `translateFootnoteLink` (inline → footnoteReference, looking up the
  label by Index). Both keep the identifier and label equal — the
  footnote-label-as-identifier convention has no case-folding distinction
  in practice, matching remark's emit.
- Emit gained `footnoteReference` and `footnoteDefinition` cases plus
  `footnoteReference` in the `isContainer` leaf set (footnoteReference is
  an inline leaf — no children on the wire).

## Refactor pass
- Reviewed translate.go for duplication / extraction opportunities. The
  three Identifier/Label/ReferenceType writers in emit are mechanically
  similar but each has different field tuples (linkReference: 3 fields,
  imageReference: 4 incl. alt, definition: 4 incl. url/title-nullable,
  footnoteReference: 2, footnoteDefinition: 2). Compressing them behind
  a shared helper would obscure the canonical mdast key-order contract
  that the per-case writers make obvious, so left in place.
- Considered hoisting the `Reference != nil` branch out of `translateLink`
  / `translateImage` into a shared dispatch, but the two functions emit
  different child shapes (link has translated children, image has flat
  Alt), so the branch reads more cleanly inline. The `identifierFromLabel`
  + `referenceTypeToMdast` helpers are already extracted and cover the
  shared computation.
- The Footnote{Reference,Definition} writers in emit are the simplest of
  the lot and were written compact from the start.

## Final
- Tests added (S08): 4 unit tests (1 of them with 2 subtests covering
  collapsed + shortcut, another with 2 subtests already at S07 for autolinks)
  + 5 black-box fixtures (39 through 43).
- All tests passing: every test in the suite is green
  (`go test ./... -count=1` → all packages PASS; `go vet ./...` clean).
- Coverage: not measured (the repo does not gate on a coverage threshold;
  the wire-level fixture suite plus translate-boundary unit tests already
  pin the load-bearing rules — see the per-test "anchored at the
  translate boundary" notes above).
- Acceptance criteria status:
  - [x] criterion 1 — `[text][id]` + def → `linkReference{full}` + `definition` (fixture 39, TestTranslateLinkReferenceFullEmitsLinkReferenceAndDefinition)
  - [x] criterion 2 — `[text][]` → `linkReference{collapsed}` (fixture 40, TestTranslateLinkReferenceCollapsedAndShortcutMapToCorrectReferenceType/collapsed)
  - [x] criterion 3 — `[text]` → `linkReference{shortcut}` (fixture 41, TestTranslateLinkReferenceCollapsedAndShortcutMapToCorrectReferenceType/shortcut)
  - [x] criterion 4 — `![alt][id]` + def → `imageReference` + `definition` (fixture 42, TestTranslateImageReferenceFullEmitsImageReferenceAndDefinition)
  - [x] criterion 5 — `text[^a]\n\n[^a]: body` → `footnoteReference` + `footnoteDefinition` (fixture 43, TestTranslateFootnoteReferenceAndDefinition)

VERDICT: accept
