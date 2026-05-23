# TDD log: 10-position-info-pretty-print-key-ordering

Started: 2026-05-23

## Setup

- Reused the Go `testing` framework + existing module layout. No new framework.
- Slice goal: pin the **uniform position info** contract for v1. Every emitted node carries a meaningful, source-accurate `position` by default (no `(0,0)` placeholders); `--no-position` strips uniformly from root + every descendant; BOM-strip and CRLF-normalization keep `position.offset` stable relative to the normalized document (ADR-0001).
- Audit of current placeholder positions in `internal/translate/translate.go` before any code change (`grep -n "pt.position(0, 0)" internal/translate/translate.go`):
  - line 526 — `translateThematicBreak` (flagged in S07 tdd-log as pragmatic fallback)
  - line 868 — `translateAutoLink` (flagged in S07 tdd-log: AutoLink's inner `*Text` is unexported)
  - line 952 — `translateFootnoteLink` (flagged in S08 tdd-log: `*east.FootnoteLink` carries only `Index`)
- S06 arch-log C8 flagged a separate bug: `translateImage`'s `textChildrenSpan` only walks direct `*ast.Text` children and skips inline containers like `*ast.Emphasis`. The image-node position is wrong for `![an *emph* alt](url)`. Deferred to this slice.
- Baseline: `go test ./...` clean (52 top-level tests + 51 fixture subtests green); `go vet ./...` clean.
- S03 already pins `root.position.end == {2,1,1}` for single-newline input as a translate-unit test (`TestTranslateSingleNewlineRootPosition`); criterion #1 is therefore mostly proven at the translate layer. This slice adds a wire-level fixture so the property holds end-to-end through cli/emit too.


## Test 1 — tracer bullet: single-newline default-mode fixture (criterion #1)

- Wrote: `testdata/fixtures/52-single-newline-default/` with `input.md = "\n"` (one literal newline byte), empty `args` (default mode — position info ON), expected stdout `{"frontmatter":null,"ast":{"type":"root","children":[],"position":{"start":{"line":1,"column":1,"offset":0},"end":{"line":2,"column":1,"offset":1}}}}`, exit `0`, empty stderr. Picks up via the existing `TestFixtures` harness.
- Red: skipped — S03's `rootPosition(pt) → pt.position(0, len(pt.src))` math + `positionTracker.point(1)` on `"\n"` already returns `{2,1,1}`, and S03 pinned this at the translate-unit layer via `TestTranslateSingleNewlineRootPosition`. I ran the new fixture immediately after creating it (`go test -run "TestFixtures/52-single-newline-default" .`) and it passed. The fixture's value is locking the wire-level contract: anyone refactoring `rootPosition` or the emit-side serialization could break the property at the wire boundary while keeping the translate-unit test green. This fixture pins the end-to-end shape (envelope + ast + position keys + values) byte-for-byte.
- Green: pre-existing.
- Notes: this is the tracer bullet — it proves the position-on-root path works end-to-end before tackling the harder "every descendant carries a meaningful position" property.

## Test 2 — property test: every emitted node carries a non-placeholder position (criterion #3)

- Wrote: `TestTranslateEveryEmittedNodeHasNonPlaceholderPosition` in `internal/translate/translate_test.go`. The test walks a translated Node tree and asserts every descendant has a non-nil `Position` whose `Start.Offset` and `End.Offset` are NOT both `0` (the placeholder signature). Root nodes legitimately have `Start.Offset == 0` so they're excluded from the both-zero check; every other node in the corpus is anchored past byte 0 (every fixture starts with `"intro\n\n"` so the interesting node sits at offset 7 or later). The corpus covers 19 node-type / fixture combinations — thematicBreak, autolink (angle + bare), footnote-ref-and-def, image-with-emph-alt, linkReference, imageReference, table, task-list, strikethrough, hard-break, blockquote, code (fenced + indented), inlineCode, html (block + inline), link, image. Helper `walkAndAssertPosition` does the depth-first walk; helper `itoa` (local, not `strconv.Itoa` — translate avoids strconv pulls in non-emit code by convention so far) renders the child index in the path.
- Red: ran the new test BEFORE touching translate.go. Output (excerpted):
  ```
  --- FAIL: TestTranslateEveryEmittedNodeHasNonPlaceholderPosition/thematicBreak
      root/root[1] (type="thematicBreak"): Position is the (0,0)-placeholder
  --- FAIL: TestTranslateEveryEmittedNodeHasNonPlaceholderPosition/autolink-angle
      root/root[1]/paragraph[0] (type="link"): Position is the (0,0)-placeholder
      root/root[1]/paragraph[0]/link[0] (type="text"): Position is the (0,0)-placeholder
  --- FAIL: TestTranslateEveryEmittedNodeHasNonPlaceholderPosition/autolink-bare
      ... same shape ...
  --- FAIL: TestTranslateEveryEmittedNodeHasNonPlaceholderPosition/footnote-ref-and-def
      root/root[1]/paragraph[1] (type="footnoteReference"): Position is the (0,0)-placeholder
  ```
  Exactly the three node types the audit flagged: `thematicBreak`, `link` (collapsed autolink) + its synthetic `text` child, and `footnoteReference`. The other 15 corpus cases all passed — confirming the (0,0) leak is localized to those handlers and isn't a tree-wide rot.
- Green: three changes in `internal/translate/`:
  1. `translateThematicBreak` (translate.go): replaced `pt.position(0, 0)` with a span derived from `tb.Pos()` (the `BaseBlock`'s start offset, which IS populated by goldmark's parser for ThematicBreak even though `Lines()` is empty) plus a forward scan to the next `\n` (or end of source). Now the position covers the entire marker line (`---` / `***` / `___`), which is the natural "thematic break source extent."
  2. `inlineSearchCursor` field + `findInline(needle []byte) (start, end int)` method on `positionTracker` (position.go): a per-Translate-call cursor that locates the next occurrence of a literal byte sequence in `pt.src` starting at the cursor, advances the cursor past the match, and returns the matched range. The in-source-order ordering guarantee comes from goldmark's own inline-parsing contract (inline nodes within a parent block are emitted in source order; document blocks are walked depth-first in source order). The cursor is reset to 0 on every `Translate` call (zero value of the struct field). Empty-needle and not-found cases return a zero-width range at the cursor without advancing — defensive only; the two real callers always pass at least one byte.
  3. `translateAutoLink` + `translateFootnoteLink` (translate.go): both use `pt.findInline(...)` to locate the literal source syntax instead of falling back to `pt.position(0, 0)`. For autolinks the needle is the inner URL (`a.Label(src)` — which equals the inner-Text-segment bytes, not the protocol-prefixed `URL(src)` flavor); the angle-bracket case is detected post-hoc by checking the source bytes immediately before/after the match for `<` / `>` and extending the outer position by 1 byte on each side when present (the synthetic child `text` carries the inner-URL position, NOT the angle-bracket-extended outer position, mirroring how a regular `[text](url)` link's inner `text` child carries only its inner-text segment). For footnote references the needle is `[^<label>]`. After both fixes the property test goes green; ran `go test ./internal/translate/` to confirm.
- Notes:
  - I considered three alternatives for AutoLink position recovery before settling on `findInline`: (a) reflection on AutoLink's unexported `value *Text` field — works but ugly and brittle across goldmark upgrades; (b) thread parent-paragraph source range through translateChildren as a search hint — invasive (touches every translate function signature); (c) post-process via a second walk — pays for a tree we don't need. `findInline` with a per-Translate cursor is the cleanest of the four: localized to positionTracker, no API churn, no reflection.
  - The cursor's monotonicity is a real correctness invariant: a regression that re-walked the tree out of source order (e.g. via a future refactor that sorted children differently) would silently mis-position autolinks and footnote references. The property test doesn't catch out-of-order matches (it only catches the (0,0) placeholder), so I added a more targeted assertion in Test 3 below.

## Test 3 — accuracy of autolink + footnoteReference positions (criterion #3, accuracy half)

- Wrote: `TestTranslateAutoLinkAndFootnotePositionsAreSourceAccurate` with three subtests:
  1. `angle-bracket-twice`: two `<https://x.com>` / `<https://y.com>` autolinks on the same line — pins both individual offsets AND the monotonic-cursor invariant (a regression that resets the cursor between matches, or that searches the whole document each time, would mis-position the second autolink).
  2. `footnote-reference`: `intro text[^a] more` — pins the exact `[^a]` source offset.
  3. `bare-url-linkify`: `see https://example.com here` — pins the linkify case has no angle-bracket extension on the outer position.
- Red: skipped — verified the test passes immediately after writing it (`go test -run TestTranslateAutoLinkAndFootnotePositionsAreSourceAccurate ./internal/translate/` → PASS). The accuracy half of the rule was established in Test 2's GREEN step; this test pins the source byte ranges explicitly so a future refactor breaking the `findInline` monotonic-cursor invariant surfaces with a specific offset mismatch rather than a vague "still non-zero" pass.
- Green: pre-existing (Test 2's translate.go fixes).
- Notes: the `angle-bracket-twice` subtest is the load-bearing one. It would fail with `findInline` that re-searched from offset 0 every time, or with one that incorrectly cached the offset of the first match. The other two subtests pin the per-flavor span shapes (with-delimiters vs without).

## Test 4 — image position must recurse into inline containers (S06 arch-log C8 deferred bug)

- Wrote: `TestTranslateImagePositionWhenAltIsEntirelyInsideInlineContainer` with `src = "![*emph*](https://example.com/x.png)\n"`. The image's alt is ENTIRELY inside an Emphasis container — there is no direct `*ast.Text` child of the Image at all. Pre-S10 `textChildrenSpan` returned `(0, 0)` here (no direct text → empty walk → placeholder span), collapsing the image's position to the placeholder. Post-S10 the helper recurses into inline containers and finds the inner `*ast.Text` segment at [3, 7) (`"emph"`).
- Red: ran the new test BEFORE touching translate.go. Output:
  ```
  --- FAIL: TestTranslateImagePositionWhenAltIsEntirelyInsideInlineContainer
      image position: got [0, 0), want [3, 7) (textChildrenSpan must recurse into inline containers; alt entirely inside an Emphasis must contribute its inner Text segment)
  ```
  Also added `"image-alt-entirely-inside-emph"` to the Test 2 property-test corpus; that subtest also went RED with the same `(0,0)-placeholder` failure, confirming the property test catches this class of bug end-to-end.
- Green: rewrote `textChildrenSpan` in `translate.go` to delegate to a new `walkTextSegments(parent ast.Node, fn func(seg textm.Segment))` helper that recurses into inline containers. The recursion is bounded by the inline-subtree shape (no cycles); the outer helper's contract (return `(0, 0)` for "no Text descendant") is preserved.
- Notes:
  - The pre-S10 `"image-with-emph-alt"` fixture (`![an *emph* alt](url)`) coincidentally produced a CORRECT position despite the bug because the surrounding leading + trailing `*ast.Text` direct children pinned a correct min/max. The boundary case (entirely-inside-inline-container) is the only observable symptom. That's why the bug survived S06 — the existing fixture's contour masked it.
  - `translateCodeSpan` shares the helper. The CodeSpan case never has inline-container children (CommonMark forbids markup inside a code span — goldmark emits only `*ast.Text` inside `*ast.CodeSpan`), so the recursion is a no-op for codespans in practice but the consistency is cheap and survives any future goldmark refactor.

## Test 5 — multibyte fixture exercising code-point column counting (criterion #2)

- Wrote: `testdata/fixtures/53-multibyte-column-counting/` with `input.md = "# 中 hi\n\nbar\n"` (14 bytes — the U+4E2D CJK ideograph is 3 UTF-8 bytes), empty `args`, exit `0`. The expected stdout includes the heading's `text.position.end == {"line":1,"column":7,"offset":8}` — the load-bearing assertion is that `column == 7` (counting CODE POINTS: `#`, ` `, `中`, ` `, `h`, `i` is 6 code points, the next position is column 7) rather than `column == 9` (which would mean we'd been counting bytes — `# ` is 2 bytes, `中` is 3 bytes, ` hi` is 3 bytes = 8 bytes total, next column 9).
- Red: skipped — `positionTracker.point` was implemented for code-point columns in S03/S04 (the `b&0xC0 == 0x80` continuation-byte skip is the relevant line); the fixture's value is locking the property end-to-end so any future regression that re-introduced byte counting (e.g. by removing the continuation-byte check in a refactor) would fail this fixture. Ran `go build -o /tmp/md2json2 . && /tmp/md2json2 < input.md` to capture the expected `stdout` and ran `go test -run "TestFixtures/53-multibyte-column-counting" .` — PASS on first run, confirming the implementation is correct.
- Green: pre-existing (positionTracker code-point column counting from S03).
- Notes: an emoji would have served the same role, but the CJK ideograph is 3 bytes vs an emoji's 4 bytes; either works as a "byte != code-point" probe. CJK is also slightly cheaper to test in a terminal (no font fallback issues when eyeballing the fixture).

## Test 6 — `--no-position` strips uniformly from every node (criterion #4)

- Wrote: `TestEmitNoPositionStripsPositionKeyFromEveryNode` in a new file `internal/translate/no_position_property_test.go` under `package translate_test` (external test package). The external package lets us import `emit` without creating an import cycle (`emit` already imports `translate`, so an internal test can't reciprocate). The test feeds a complex corpus (every-node-type-touched: heading, paragraph, em, strong, list, listItem with task-checkbox, fenced + indented code, html, link, image, blockquote, thematicBreak, hard-break, table, strikethrough, autolink, linkReference, definition, footnote) through `parse → translate → emit` twice — once with `NoPosition=false`, once with `NoPosition=true` — and:
  1. Asserts the no-position output contains ZERO `"position":` substrings.
  2. Asserts the default-mode output contains EXACTLY one `"position":` substring per translated Node.
- Red: skipped — verified the test passes immediately after writing it. The S03 `if !opts.NoPosition && n.Position != nil` gate in `writeNode` is the single seam that owns the "drop position uniformly" rule; the test pins both halves of the contract (translate always attaches Position; emit drops it uniformly under NoPosition).
- Green: pre-existing (S03's gate + every subsequent slice's writeNode case respecting the same gate, which is enforced by the gate's location: the gate is AFTER the per-type switch, so every node funnels through the same drop logic regardless of its Type).
- Notes:
  - The "exactly one `position` per Node" half is the load-bearing assertion. It would catch a future regression where a contributor added a new node type to writeNode's switch but forgot to call back into the position-write path (e.g. by writing the type's body as `return` short-circuiting before the position write). It would also catch a regression where translate stopped attaching Position to some node type (the count would drop below `nodeCount`).
  - Used `bytes.Count` rather than a hand-rolled `countSubstring`; the latter was an early draft that I dropped during the move to the external test package since `bytes.Count` is already in stdlib and avoids the YAGNI of a local copy.

## Test 7 — BOM-prefixed input produces same position.offset values as no-BOM (criterion #5)

- Wrote: two sibling fixtures.
  - `testdata/fixtures/54-bom-prefixed-default/input.md`: 14 bytes, leading `0xEF 0xBB 0xBF` UTF-8 BOM followed by `# Hi\n\nbody\n`. Empty args (default mode — positions ON), exit `0`.
  - `testdata/fixtures/55-no-bom-baseline-default/input.md`: 11 bytes, plain `# Hi\n\nbody\n`. Same args, exit `0`.
  - Both `stdout` files are BYTE-IDENTICAL (`diff` confirms). This is the load-bearing assertion of criterion #5: per ADR-0001 §5, the BOM is stripped silently and `position.offset` values count against the **normalized** (post-BOM-strip) document. The two fixtures' identical stdouts prove the property end-to-end through `read → parse → translate → emit`.
- Red: skipped — `read.Read` already strips the leading BOM (S02 unit tests pin this at the read-module boundary: `TestLeadingBOMIsStripped`). The positionTracker downstream receives the normalized bytes and computes offsets accordingly. The fixture pair locks the WIRE-LEVEL contract that no downstream stage accidentally adds the BOM bytes back into offsets — a regression in cli.Run (e.g. reading raw bytes for some auxiliary path) could have broken this; the byte-identical stdouts guarantee against it.
- Green: pre-existing (S02's BOM strip).
- Notes: the byte-identical-stdout property is the load-bearing one. If a future regression made the BOM contribute to offsets, the BOM fixture's `text.position.offset` values for the heading text would each be 3 bytes higher than the no-BOM fixture, and the harness's byte-exact compare would catch it.

## Test 8 — CRLF input produces same position values as LF (criterion #6)

- Wrote: `testdata/fixtures/56-crlf-input-default/input.md` with `# Hi\r\n\r\nbody\r\n` (14 bytes — three CRLF sequences). Empty args, exit `0`. The expected `stdout` is BYTE-IDENTICAL to `55-no-bom-baseline-default/stdout` (`diff` confirms). This is the load-bearing assertion of criterion #6: per ADR-0001 §6, CRLF is normalized to LF BEFORE parsing, and `position.line`/`position.column`/`position.offset` all reflect the normalized document.
- Red: skipped — `read.Read` already collapses every `\r\n` to a single `\n` (S02 unit test `TestCRLFNormalizedToLF` pins this at the read-module boundary; `TestBareCRNormalizedToLF` extends the rule to bare `\r`). Downstream stages see only LF bytes. The fixture locks the wire-level equivalence so a future regression in cli.Run (e.g. passing raw bytes around the read normalize step for some auxiliary path) would break the byte-exact stdout compare.
- Green: pre-existing (S02's CRLF→LF normalize).
- Notes: the byte-identical-stdout assertion (CRLF fixture vs LF baseline fixture) is the cross-platform-stability guarantee from ADR-0001. A Windows developer authoring CRLF files and a macOS / Linux CI on LF produce identical position values for the same logical document.

## Refactor pass

After all tests green:

1. **Considered inlining `walkTextSegments` back into `textChildrenSpan`.** The two are adjacent in `translate.go` and tightly coupled. Decided to keep them separate: `textChildrenSpan` names the "return a span" intent; `walkTextSegments` names the recursive traversal. Splitting along the visitor-callback seam makes both easier to reason about — `textChildrenSpan` does min/max accumulation; `walkTextSegments` does the recursion. If a future caller needs a different accumulation (e.g. concatenating all the text values), it can reuse `walkTextSegments` without re-implementing the recursion.
2. **Considered factoring `findInline`'s cursor-advance pattern into a `findFromCursor(needle) (start, end int, found bool)` returning a tri-state.** Decided to keep the two-value return: the "not found" case is degenerate (would only happen if goldmark emitted a node whose source bytes don't appear in `pt.src`, which the parser doesn't do); the current "return cursor unchanged on not-found" behavior is the natural conservative default and matches the existing `(0,0)`-on-empty convention used by `childrenSpan` and friends.
3. **Did NOT consolidate the BOM and CRLF fixtures (54 + 56) into a single parameterized fixture.** Considered it — both pin the same property ("input transform doesn't leak into position offsets") against a different transform — but the fixture harness's per-directory structure doesn't support parameterization without changing `integration_test.go`'s `loadFixture`. Keeping them as two directories keeps each transform's contract independently auditable; the failure message will identify which transform broke without requiring a parameterized-test name decode.
4. **Did NOT split `translateAutoLink` into separate `translateAngleBracketAutoLink` / `translateLinkifyAutoLink` helpers.** The angle-bracket detection (checking `src[innerStart-1] == '<'`) is a 3-line post-hoc check; factoring it would force two duplicate copies of the URL/Label/children boilerplate. The current single-handler shape with one branch is more compact and matches the `*ast.AutoLink` single-type goldmark exposes (the angle-bracket-vs-linkify discriminator is `Protocol` in goldmark, but using `Protocol` would be brittle for the bare-URL with explicit protocol case — `<https://...>` and `https://...` both have non-nil `Protocol`. The source-byte check is the right discriminator.).

## Manual end-to-end verification

```
$ go build -o /tmp/md2json2 .
$ printf '\n' | /tmp/md2json2
{"frontmatter":null,"ast":{"type":"root","children":[],"position":{"start":{"line":1,"column":1,"offset":0},"end":{"line":2,"column":1,"offset":1}}}}
$ printf '\xef\xbb\xbf# Hi\n\nbody\n' | /tmp/md2json2 | grep -o '"offset":[0-9]*' | head -5
"offset":0
"offset":2
"offset":2
"offset":4
"offset":4
$ printf '# Hi\n\nbody\n' | /tmp/md2json2 | grep -o '"offset":[0-9]*' | head -5
"offset":0
"offset":2
"offset":2
"offset":4
"offset":4
$ printf '# Hi\r\n\r\nbody\r\n' | /tmp/md2json2 | grep -o '"offset":[0-9]*' | head -5
"offset":0
"offset":2
"offset":2
"offset":4
"offset":4
$ printf 'intro\n\n---\n\nbody\n' | /tmp/md2json2 | grep -A1 '"thematicBreak"'
{"type":"thematicBreak","position":{"start":{"line":3,"column":1,"offset":7},"end":{"line":3,"column":4,"offset":10}}}
$ printf '<https://example.com>\n' | /tmp/md2json2 | grep -o '"link"[^}]*}[^}]*}' | head -1
"link","url":"https://example.com","title":null,"children":[{"type":"text","value":"https://example.com","position":{"start":{"line":1,"column":2,"offset":1
$ printf 'text[^a]\n\n[^a]: body\n' | /tmp/md2json2 | grep -o '"footnoteReference"[^}]*}[^}]*}'
"footnoteReference","identifier":"a","label":"a","position":{"start":{"line":1,"column":5,"offset":4},"end":{"line":1,"column":9,"offset":8}}
```

All three transform-invariance fixtures (single-newline, BOM, CRLF) produce the expected wire shape; thematicBreak now spans `[7, 10)` (the `---` marker line in `intro\n\n---\n\n...`) instead of `(0, 0)`; autolink synthesizes inner-text position `[1, 20)` (covering `https://example.com` without the angle brackets) inside an outer link position spanning the full `<URL>` syntax; footnoteReference now carries `[4, 8)` (the `[^a]` source bytes) instead of `(0, 0)`.

## Final

- Tests added in S10:
  - translate (unit, `package translate`):
    - `TestTranslateEveryEmittedNodeHasNonPlaceholderPosition` — 20 subtests (one per node-type fixture) asserting no `(0,0)` placeholders survive translate.
    - `TestTranslateAutoLinkAndFootnotePositionsAreSourceAccurate` — 3 subtests pinning the actual source byte ranges (the monotonic-cursor invariant).
    - `TestTranslateImagePositionWhenAltIsEntirelyInsideInlineContainer` — pins the S06 arch-log C8 deferred bug.
    - `TestTranslateEmptyDocStillHasZeroWidthRootPosition` — regression guard on S03's empty-doc root contract.
  - translate (cross-package, `package translate_test`):
    - `TestEmitNoPositionStripsPositionKeyFromEveryNode` — pins criterion #4 end-to-end through translate + emit.
  - fixtures (integration via `TestFixtures`):
    - `52-single-newline-default` — criterion #1.
    - `53-multibyte-column-counting` — criterion #2 (multibyte column rule).
    - `54-bom-prefixed-default` + `55-no-bom-baseline-default` — criterion #5 (byte-identical BOM-vs-no-BOM contract).
    - `56-crlf-input-default` — criterion #6 (byte-identical CRLF-vs-LF contract via cross-comparison with `55`).
- Production code changes in `internal/translate/`:
  - `position.go`: added `inlineSearchCursor int` field on `positionTracker`; added `findInline(needle []byte) (start, end int)` method; added `bytes` import.
  - `translate.go`:
    - `translateThematicBreak`: replaced `pt.position(0, 0)` placeholder with `tb.Pos()` + forward-scan to next `\n`.
    - `translateAutoLink`: replaced shared placeholder with `pt.findInline(a.Label(src))` + post-hoc angle-bracket-delimiter detection.
    - `translateFootnoteLink`: replaced placeholder with `pt.findInline([]byte("[^" + label + "]"))`.
    - `textChildrenSpan` + new `walkTextSegments` helper: recurse into inline containers (fix S06 arch-log C8).
- `go test ./...`: clean. 154 tests pass (57 top-level + 97 subtests).
- `go vet ./...`: clean.
- No new module dependencies.

- Acceptance criteria status:
  - [x] criterion 1 — single-newline.md → `root.position.end == {2,1,1}`, `root.children == []`, exit 0 (`TestFixtures/52-single-newline-default` + translate-unit `TestTranslateSingleNewlineRootPosition` from S03 + `TestTranslateEmptyDocStillHasZeroWidthRootPosition`).
  - [x] criterion 2 — multi-line doc with multibyte char counts code-point columns (`TestFixtures/53-multibyte-column-counting`).
  - [x] criterion 3 — every emitted node carries a `position` field by default with no `(0,0)` placeholders (`TestTranslateEveryEmittedNodeHasNonPlaceholderPosition` + `TestTranslateAutoLinkAndFootnotePositionsAreSourceAccurate` + `TestTranslateImagePositionWhenAltIsEntirelyInsideInlineContainer`).
  - [x] criterion 4 — `--no-position` strips uniformly from root + every descendant, no other key/field-presence change (`TestEmitNoPositionStripsPositionKeyFromEveryNode`).
  - [x] criterion 5 — BOM-prefixed input produces `position.offset` values identical to no-BOM input (`TestFixtures/54-bom-prefixed-default` vs `55-no-bom-baseline-default` byte-identical stdouts).
  - [x] criterion 6 — CRLF-only input produces position values identical to LF-equivalent input (`TestFixtures/56-crlf-input-default` vs `55-no-bom-baseline-default` byte-identical stdouts).

VERDICT: accept
