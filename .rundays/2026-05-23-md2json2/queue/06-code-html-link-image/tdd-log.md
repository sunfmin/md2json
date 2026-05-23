# TDD log: 06-code-html-link-image

Started: 2026-05-23

## Setup

- Reuses S01-S05's test framework (Go `testing` + `go test`, plus the
  black-box fixture harness at `integration_test.go`). No new dependencies.
- Slice goal: extend `translate` to walk goldmark's remaining basic CommonMark
  constructs that are not lists/headings/blockquotes/breaks:
  `*ast.FencedCodeBlock`, `*ast.CodeBlock` (indented), `*ast.HTMLBlock`,
  `*ast.RawHTML` (inline), `*ast.CodeSpan`, `*ast.Link`, `*ast.Image`. Extend
  the `Node` struct with the nullable-string fields `Lang *string`,
  `Meta *string`, `Title *string` plus the non-nullable `URL string` and
  `Alt string` fields. Extend `emit.writeNode`'s per-type switch with the new
  canonical-key-order slots: `code` (lang, meta, value), `inlineCode` (value),
  `html` (value), `link` (url, title), `image` (url, title, alt). Add
  `writeJSONNullableString` to concentrate the `*string` → JSON null-or-string
  rendering convention. Extend `isContainer` so `inlineCode`, `code`, `html`,
  and `image` are leaves on the wire (no `children` array). `link` remains
  a container (carries its inline label as children); `image` is a leaf
  because `alt` is a flat string per `image.alt: string` mdast constraint.
- Goldmark mapping facts (read out of
  `~/go/pkg/mod/github.com/yuin/goldmark@v1.8.2/ast/{block,inline}.go`):
  - `*ast.FencedCodeBlock` has `Info *Text` (the info-string text node) and
    `Lines()` for the body. `Language(src)` returns the first space-delimited
    word of the info string; the full info text is at `Info.Segment`.
  - `*ast.CodeBlock` (indented) has only `Lines()` — no language/meta concept.
  - `*ast.HTMLBlock` has `Lines()` for the body lines plus an optional
    `ClosureLine` segment (when goldmark recognized a separate closing line,
    e.g. multi-line `<div>\n...\n</div>` patterns). Single-line `<div>x</div>`
    has no closure; closure must be appended to `value` when present.
  - `*ast.RawHTML` (inline) has `Segments *textm.Segments` — one or more
    contiguous segments holding the raw markup.
  - `*ast.CodeSpan` has `*ast.Text` children whose segments are the literal
    content between the backticks (goldmark has already applied CommonMark's
    one-space-trim rule).
  - `*ast.Link` and `*ast.Image` share `baseLink` with fields
    `Destination []byte` and `Title []byte`. Image children are the inline
    parsed-from-alt nodes (text + emphasis + etc.); the mdast contract is to
    flatten these to a single `alt` string.
- Encoding fact: `encoding/json.Marshal` escapes `<`, `>`, and `&` by default
  (HTML-safe mode). The wire contract for `html.value` is the raw markup
  byte-for-byte per CONTEXT.md "Text/Code value preservation". The existing
  `writeJSONValue` path used `enc.SetEscapeHTML(false)` for the frontmatter
  side; before S06, `writeJSONString` used the default-escape Marshal because
  no value-bearing emitted field had ever contained `<`/`>`/`&`. S06 forces
  the issue (block raw HTML `<div>raw</div>` is the canonical case) — fix is
  to switch `writeJSONString` to a json.Encoder with `SetEscapeHTML(false)`,
  mirroring `writeJSONValue`.
- Shape decision: applied S04/S05's "single-struct `Node` with optional
  fields; nullable mdast fields are Go pointer types" convention. The new
  `Lang *string`, `Meta *string`, `Title *string` fields express the
  CONTEXT.md mdast node-set v1 rule that `code.lang`, `code.meta`,
  `link.title`, `image.title` serialize as JSON `null` when absent — NEVER
  elided, NEVER `""`. `URL string` and `Alt string` are plain strings because
  the mdast contract has them as `string` (not nullable string). New
  `writeJSONNullableString` helper at emit names the convention at the wire
  layer, matching S05's `writeJSONNullableInt`/`writeJSONNullableBool` pair.

## Test 1 — tracer bullet: fenced code block ` ```go\nfunc x(){}\n``` ` (criterion #1)

- Wrote: fixture `testdata/fixtures/26-fenced-code-go-nopos/` with
  `args=--no-position`, `input.md="```go\nfunc x(){}\n```\n"`, `exit=0`,
  empty stderr, and expected stdout byte-exact:
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"code","lang":"go","meta":null,"value":"func x(){}\n"}]}}`
- Red: `go test -run 'TestFixtures/26-' .` failed — stdout was the empty
  envelope `{"frontmatter":null,"ast":{"type":"root","children":[]}}`
  because `translate` at S05 silent-dropped `*ast.FencedCodeBlock` per
  CONTEXT.md "Lossiness policy". This is the tracer — it forces the entire
  FencedCodeBlock → code-with-nullable-lang-and-meta path AND the Node
  shape extensions AND the emit per-type case AND the writeJSONNullableString
  helper into existence in one go.
- Green: extended `internal/translate/translate.go`:
  - `Node` gained `Lang *string`, `Meta *string`, `URL string`,
    `Title *string`, `Alt string` fields. Documented the rationale inline:
    pointer types for nullable mdast fields, applied S05's `*int`/`*bool`
    precedent to `*string`.
  - `translateNode` switch gained seven new cases: `*ast.FencedCodeBlock`,
    `*ast.CodeBlock`, `*ast.HTMLBlock`, `*ast.RawHTML`, `*ast.CodeSpan`,
    `*ast.Link`, `*ast.Image`.
  - `translateFencedCodeBlock`: info-string split on the first space
    yields `lang` (prefix) and `meta` (suffix after the space); empty
    prefix → `lang: nil`, empty suffix → `meta: nil`. `value` is
    `f.Lines().Value(src)` (per CONTEXT.md: literal content between
    fences, including every content line's trailing `\n`).
- And on the emit side `internal/emit/emit.go::writeNode` grew a `code`
  case in the type-specific switch, writing `lang`, `meta`, `value` in
  canonical key order using `writeJSONNullableString` for the two
  nullable fields and `writeJSONString` for `value`. Added the
  `writeJSONNullableString(buf, *string)` helper at top level, mirroring
  the S05 `writeJSONNullableInt`/`writeJSONNullableBool` pair. Extended
  `isContainer` to mark `code` (and the other S06 leaves) as non-container
  so no empty `children` array bleeds onto the wire.
- Notes: this is the tracer — one fixture exercises the entire
  parse→translate(FencedCodeBlock)→emit(code with nullable lang/meta)
  pipeline end-to-end. The "info string has only a language, no meta"
  flavor — Test 2 picks up the meta-split.

## Test 2 — fenced with meta `go runme=true` (criterion #2)

- Wrote: fixture `testdata/fixtures/27-fenced-code-meta-nopos/` with
  `input.md="```go runme=true\nfunc x(){}\n```\n"`. Expected stdout pins
  `"lang":"go","meta":"runme=true"`.
- Red: skipped — Test 1's GREEN already implemented the info-string
  first-space-split. The fixture would PASS on first run with no code
  changes. Verified honestly with `go test -run 'TestFixtures/27-' .`
  — passes immediately. Per the S05 tdd-log precedent, kept the fixture
  as a behavior anchor: a regression that accidentally hardcoded `meta: nil`
  for fenced blocks (e.g. by not splitting on space) would fail here.
- Green: pre-existing.
- Notes: pins the lang/meta split contract; complements Test 1 which
  pinned the lang-only flavor.

## Test 3 — indented code `    abc\n    def\n` (criterion #3)

- Wrote: fixture `testdata/fixtures/28-indented-code-nopos/` with
  `input.md="    abc\n    def\n"`. Expected stdout pins
  `"lang":null,"meta":null,"value":"abc\ndef\n"`.
- Red: skipped — Test 1's GREEN added the `*ast.CodeBlock` dispatch case
  with `translateCodeBlock` (always `Lang: nil, Meta: nil`; `value` from
  `Lines().Value`). The fixture pins the dedented value AND the
  never-elided-null rule. Verified `go test -run 'TestFixtures/28-' .`
  — passes immediately.
- Green: pre-existing.
- Notes: the `null` fields are the load-bearing acceptance criterion —
  `lang` and `meta` must be present as JSON `null`, NEVER elided, NEVER
  `""`. The byte-exact comparison in the fixture harness catches both
  failure modes (omission and empty-string-equivalent).

## Test 4 — inline code `` `x` `` (criterion #4)

- Wrote: fixture `testdata/fixtures/29-inline-code-nopos/` with
  `input.md="a `x` b\n"`. Expected stdout pins
  `paragraph → text("a "), inlineCode("x"), text(" b")`.
- Red: skipped — Test 1's GREEN added the `*ast.CodeSpan` dispatch with
  `translateCodeSpan` (concatenates `*ast.Text` child segment values into
  one `inlineCode.value`). The fixture pins the leaf shape (no `children`
  array) and the value-extraction rule.
- Green: pre-existing.
- Notes: `inlineCode` is a leaf — `isContainer` was extended to mark it
  so. The fixture would fail if a future refactor accidentally treated
  inlineCode as a container with an empty `children: []` array.

## Test 5 — block raw HTML `<div>raw</div>` (criterion #5 part a)

- Wrote: fixture `testdata/fixtures/30-block-html-nopos/` with
  `input.md="<div>raw</div>\n"`. Expected stdout pins
  `"html","value":"<div>raw</div>\n"` (literal `<` and `>`, NOT
  unicode-escaped).
- Red: `go test -run 'TestFixtures/30-' .` failed — stdout had the value
  field as `"<div>raw</div>\n"` because
  `writeJSONString` used `json.Marshal`, which defaults to HTML-safe escape
  mode. The wire contract per CONTEXT.md "Text/Code value preservation"
  says `<`/`>`/`&` flow through literal. This is the first slice where
  any value-bearing emitted field contained `<` or `>` (text values from
  earlier slices were plain ASCII), so the escape behavior had not yet
  bitten.
- Green: switched `writeJSONString` to use a `json.Encoder` with
  `SetEscapeHTML(false)`, mirroring `writeJSONValue`'s frontmatter-side
  treatment. Stripped the trailing `\n` that `json.Encoder` always
  appends, matching the existing pattern in `writeJSONValue`. Added a
  doc comment naming CONTEXT.md "Text/Code value preservation" as the
  rule this change implements. Re-ran the entire test suite: 33 passing
  fixtures (no regressions in the earlier 32 fixtures), all package-level
  tests still green.
- Notes: this is the load-bearing wire-contract change for S06. Without
  it, every downstream consumer would have to know the difference
  between `<` and `<` in `value` fields and re-decode — a wire
  contract the v1 PRD does not establish. The change preserves the
  necessary JSON-string escapes for `"`, `\`, control characters, etc.
  (json.Encoder still emits those properly) while passing literal
  `<`/`>`/`&` through.

## Test 6 — inline raw HTML `<span>x</span>` (criterion #5 part b)

- Wrote: fixture `testdata/fixtures/31-inline-html-nopos/` with
  `input.md="a <span>x</span> b\n"`. Expected stdout pins five children
  in the paragraph: `text("a ")`, `html("<span>")`, `text("x")`,
  `html("</span>")`, `text(" b")`. Same `html` mdast type as the block
  case (mdast does not distinguish block from inline raw HTML).
- Red: skipped — Test 1's GREEN added the `*ast.RawHTML` dispatch with
  `translateRawHTML` (concatenates the `Segments` value as one
  `html.value`). The fixture would PASS on first run; Test 5's
  HTML-escape fix in `writeJSONString` was the prerequisite — pre-fix
  the literal `<span>` would have escaped here too.
- Green: pre-existing (Test 5's `writeJSONString` fix already in place).
- Notes: this fixture confirms acceptance criterion #5's "same mdast
  type" half — inline and block raw HTML both emit `html`. Goldmark
  splits `<span>x</span>` into three inline siblings (open tag,
  inner text, close tag) because the inner `x` is parseable as
  Markdown content; the mdast wire shape reflects that faithfully.

## Test 7 — link `[text](https://example.com "t")` (criterion #6)

- Wrote: fixture `testdata/fixtures/32-link-with-title-nopos/` with
  `input.md="[text](https://example.com \"t\")\n"`. Expected stdout pins
  `link{url:"https://example.com",title:"t",children:[text("text")]}`.
- Red: skipped — Test 1's GREEN added the `*ast.Link` dispatch with
  `translateLink`. `link` is a container (carries its inline label as
  `children`), unlike `image` which is a leaf — `isContainer`'s case
  list reflects this distinction. The fixture pins the URL/title/child-
  text triplet.
- Green: pre-existing.
- Notes: confirms the nullable-title convention at the link level:
  `title: "t"` (non-empty `[]byte` → `*string`) vs `title: null` (empty
  `[]byte` → nil pointer). Test 8's image case exercises the
  `title: null` branch.

## Test 8 — image with non-text inline alt `![an *emph* alt](url)` (criterion #7)

- Wrote: fixture `testdata/fixtures/33-image-flat-alt-nopos/` with
  `input.md="![an *emph* alt](https://example.com/x.png)\n"`. Expected
  stdout pins `image{url:"https://example.com/x.png",title:null,alt:"an emph alt"}`
  — flat alt string, non-text inline structure dropped, no `children`
  array (image is a leaf on the wire because `alt: string` is the mdast
  spec).
- Red: skipped — Test 1's GREEN added the `*ast.Image` dispatch with
  `translateImage` + `flattenAltText`. `flattenAltText` recursively walks
  the goldmark inline subtree under the image, concatenating `*ast.Text`
  leaf segments and silently dropping container delimiters (so the
  `*emph*` emphasis container contributes only its inner text `emph` —
  the `*` delimiters do not appear).
- Green: pre-existing.
- Notes: anchors the mdast `image.alt: string` constraint at the
  translate side. The fixture's byte-exact `"alt":"an emph alt"` (with
  one space on each side of "emph", and NO `*` characters) catches both
  failure modes: (a) including the `*` delimiters from the source
  (would yield `"an *emph* alt"`); (b) emitting a nested-children alt
  shape (would yield extra JSON fields or break the byte-exact compare).
  Title is `null` because the source has no `"..."` title clause —
  exercises the `nullableString` empty-bytes-to-nil branch.

## Translate unit tests (anchoring load-bearing rules at the Go-value-tree boundary)

In `internal/translate/translate_test.go` I added three Go-level unit
tests that exercise the load-bearing translate-side rules at the
package boundary (not via the fixture harness):

- `TestTranslateIndentedCodeBlockHasNilLangAndMeta` — explicit
  acceptance criterion #3: indented code carries `Lang == nil` AND
  `Meta == nil` in the Go value tree (NOT a pointer to `""`). The emit
  module serializes nil as `lang: null`; a pointer-to-empty-string would
  serialize as `lang: ""`, a different wire shape that violates the
  mdast node-set v1 contract. Anchors the convention at the Go layer
  separately from the wire-side assertion in the fixture.
- `TestTranslateFencedCodeBlockPreservesTrailingNewline` — anchors
  CONTEXT.md "Text/Code value preservation"'s canonical example at the
  Go layer: `code.Value == "func x(){}\n"`. A future refactor that
  accidentally `TrimRight`-ed the value (a common over-zealous cleanup)
  would surface here.
- `TestTranslateImageAltFlattensNonTextInlineStructure` — anchors the
  mdast `image.alt: string` flattening rule at the Go layer:
  `img.Alt == "an emph alt"` (NOT `"an *emph* alt"`, NOT a child tree),
  and `len(img.Children) == 0` (image is a leaf, no child tree on the
  Go side either — the alt is the only carrier of the alt content).

These complement the fixture suite by pinning the contract at the
package-public-API layer too. The same regression-coverage pattern as
the four S05 translate unit tests.

## Refactor pass

After all tests green:

1. **Extracted `nullableString([]byte) *string` helper** in
   `internal/translate/translate.go`. Both `translateLink` and
   `translateImage` had the same five-line pattern:
   ```go
   var title *string
   if len(t) > 0 {
       s := string(t)
       title = &s
   }
   ```
   Two adapters of the same shape — past S05's stated "two adapters =
   real seam" threshold. The helper compresses each site to a one-liner
   (`Title: nullableString(l.Title)`) and names the goldmark convention
   that an empty `[]byte` is the "no value" sentinel. Boundary: the
   helper does one thing (`[]byte` → `*string` with empty-as-nil
   semantics); the field name and parent struct stay at the callsite.
   Doc comment names CONTEXT.md and forecasts S08's reuse for
   `definition.Title` / `linkReference.Label`.
   - Tests after: 34 PASS, 0 FAIL (`go test ./...`); `go vet ./...`
     clean.
2. **Did NOT consolidate `translateRawHTML` and `translateHTMLBlock`.**
   They both produce `html{value}` mdast nodes but pull from different
   goldmark structures (`Segments *textm.Segments` for RawHTML;
   `Lines()` + optional `ClosureLine` for HTMLBlock). A merged helper
   would need to dispatch internally on the goldmark type — the
   type-switched outer dispatch already does that with compile-time
   exhaustiveness. Locality lost; rejected.
3. **Did NOT consolidate `writeJSONString`'s new encoder approach with
   `writeJSONValue`'s existing encoder.** Both now do
   `enc.SetEscapeHTML(false)` + strip-trailing-newline, but they differ
   in input type (`string` vs `any`) and call site (per-field vs
   per-envelope). Merging would require a parametric `writeJSON(any)`
   that drops compile-time type safety on the string side. Rejected for
   the same reason S05's arch rejected per-type structs: the seam is
   only conceptual, not in the call shape.
4. **Did NOT factor the "extract span from segments" pattern in
   `translateRawHTML`/`translateCodeSpan`.** Both compute `(startOff,
   endOff)` from segments, but `translateRawHTML` uses `Segments` (a
   `*textm.Segments`) while `translateCodeSpan` walks `*ast.Text`
   children. Same conceptual operation, two different goldmark shapes;
   extracting would add a helper that takes a callback or two helpers
   that wrap the same algorithm — neither wins on locality. Leave
   inline.

## Final

- Tests added in S06:
  - 8 fixtures: `testdata/fixtures/26-fenced-code-go-nopos` through
    `testdata/fixtures/33-image-flat-alt-nopos`. Each pins one
    acceptance criterion with a byte-exact wire-shape assertion under
    `--no-position`.
  - 3 translate unit tests:
    `TestTranslateIndentedCodeBlockHasNilLangAndMeta`,
    `TestTranslateFencedCodeBlockPreservesTrailingNewline`,
    `TestTranslateImageAltFlattensNonTextInlineStructure`.
- `go test ./...`: 34 passing tests, 0 failures (all package-level
  test binaries green). Fixture count rose from 25 to 33; package
  unit-test counts from 7 (translate) + 6 (cli) + 2 (read) + 2 (root)
  to 10 (translate) + 6 (cli) + 2 (read) + 2 (root).
- `go vet ./...`: clean.
- Files touched:
  - `internal/translate/translate.go` (extended: Node fields, dispatch
    switch with 7 new cases, 7 new translate functions
    `translateFencedCodeBlock`/`translateCodeBlock`/`translateHTMLBlock`/
    `translateRawHTML`/`translateCodeSpan`/`translateLink`/
    `translateImage`, plus `flattenAltText` recursion helper and
    `nullableString` shared helper)
  - `internal/translate/translate_test.go` (3 new unit tests appended)
  - `internal/emit/emit.go` (extended `writeNode` type-specific switch
    with `code`/`inlineCode`/`html`/`link`/`image` cases in canonical
    key order; extended `isContainer` to mark `inlineCode`/`code`/`html`/
    `image` as leaves; added `writeJSONNullableString` helper; fixed
    `writeJSONString` to disable HTML escaping per CONTEXT.md "Text/Code
    value preservation")
  - 8 new fixtures under `testdata/fixtures/` (26 through 33)

- Acceptance criteria status:
  - [x] criterion 1 — fenced ` ```go\nfunc x(){}\n``` ` produces
    `code{lang:"go",meta:null,value:"func x(){}\n"}`; trailing newline
    preserved, closing fence not in value (`TestFixtures/26-fenced-code-go-nopos`,
    `TestTranslateFencedCodeBlockPreservesTrailingNewline`)
  - [x] criterion 2 — fenced `go runme=true` info string produces
    `lang:"go",meta:"runme=true"` (`TestFixtures/27-fenced-code-meta-nopos`)
  - [x] criterion 3 — indented `    abc\n    def\n` produces
    `code{lang:null,meta:null,value:"abc\ndef\n"}`; both null fields
    present, never elided (`TestFixtures/28-indented-code-nopos`,
    `TestTranslateIndentedCodeBlockHasNilLangAndMeta`)
  - [x] criterion 4 — inline `` `x` `` produces `inlineCode{value:"x"}`
    as a paragraph child (`TestFixtures/29-inline-code-nopos`)
  - [x] criterion 5 — block `<div>raw</div>` produces an `html` node
    with verbatim markup, AND inline `<span>x</span>` produces `html`
    nodes (same type) inside a paragraph; both with literal `<`/`>`/`&`
    on the wire (`TestFixtures/30-block-html-nopos`,
    `TestFixtures/31-inline-html-nopos`)
  - [x] criterion 6 — `[text](https://example.com "t")` produces
    `link{url:"https://example.com",title:"t"}` with `text("text")`
    child (`TestFixtures/32-link-with-title-nopos`)
  - [x] criterion 7 — `![an *emph* alt](url)` produces
    `image{url,title:null,alt:"an emph alt"}` — flat alt, no nested
    children, no `*` delimiters surviving in the alt
    (`TestFixtures/33-image-flat-alt-nopos`,
    `TestTranslateImageAltFlattensNonTextInlineStructure`)

VERDICT: accept
