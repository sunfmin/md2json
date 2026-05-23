# TDD log: 07-gfm-tables-tasks-strikethrough-autolinks

Started: 2026-05-23

## Setup

- Reuses S01–S06's test framework: Go `testing` + `go test`, plus the
  black-box fixture harness at `integration_test.go` that runs every
  directory under `testdata/fixtures/` as a single fixture. No new
  dependencies — `extension.GFM` is already wired into `parse.New()` from
  S01 and exposes all four GFM constructs this slice covers (tables, task
  lists, strikethrough, GFM autolink/linkify). The `<...>` autolink form is
  CommonMark's, also covered by goldmark's core parser since S01.
- Slice goal: extend `translate` to walk the four remaining GFM-extension
  goldmark nodes that S06 didn't touch:
  - `*east.Table`, `*east.TableHeader`, `*east.TableRow`, `*east.TableCell`
    (from `github.com/yuin/goldmark/extension/ast`) → mdast `table`/`tableRow`/`tableCell`.
    `table.Alignments` (a `[]east.Alignment` per-column) flattens onto
    `table.align: ["left"|"right"|"center"|null, ...]` on the wire;
    `tableCell` carries no per-cell align (mdast deviates from goldmark here).
  - `*east.TaskCheckBox` (the GFM task-checkbox inline node) → silently
    dropped from output, but its `IsChecked` boolean is HOISTED onto its
    grand-parent `listItem.checked`. The hoisting is the load-bearing
    rule; without it, the checkbox would appear as a stray text child or
    a `[ ]` literal in the paragraph.
  - `*east.Strikethrough` → mdast `delete` (container, inline children).
  - `*ast.AutoLink` (from goldmark core; both `<https://…>` and bare-URL
    linkify lower into this node) → mdast `link{url, title:null,
    children:[text(value:URL)]}` — collapsing the distinct goldmark type
    to the mdast `link` type per CONTEXT.md "mdast node-set v1"
    autolink rule ("autolinks collapse to mdast `link{url, title:null}`…
    goldmark's distinct `AutoLink` type is an implementation detail not
    exposed on the wire").
- Goldmark mapping facts (read out of
  `~/go/pkg/mod/github.com/yuin/goldmark@v1.8.2/{ast,extension/ast}/`):
  - `extension/ast.Table` has `Alignments []Alignment`; `Alignment`'s
    `String()` returns `"left"|"right"|"center"|"none"`. mdast's contract
    uses `null` (not the string `"none"`) for unaligned columns —
    translate must convert `AlignNone` to a `nil` slot in `[]*string`.
  - `Table`'s first child is a `TableHeader` (one row of cells); subsequent
    children are `TableRow`s. mdast does NOT distinguish header from data
    rows (a header is still a `tableRow` whose parent indicates it is the
    header). For v1, the simplest faithful mdast shape is: header becomes
    the first `tableRow` child of `table`; data rows follow. mdast/remark's
    own emitter does this — the header-vs-row distinction lives in
    document position, not type.
  - `extension/ast.TableCell` has `Alignment Alignment`, but mdast deviates
    here: per CONTEXT.md mdast node-set v1, `tableCell` carries no `align`
    field — alignment is a per-column property of `table`, not per-cell.
    So translate intentionally drops `TableCell.Alignment` on the wire.
  - `extension/ast.TaskCheckBox` is inserted as the first inline child of
    the `*ast.TextBlock` (the wrapper inside the `*ast.ListItem`). Its
    `IsChecked` bool is the source of truth. The parser advances past
    `[x] ` / `[ ] ` so the surrounding text of the line is the body text
    only — there is no `[x]` literal in any sibling `*ast.Text` segment.
    To hoist: in `translateListItem`, peek at the first grand-child
    (TextBlock's first child) and, if it's a `*east.TaskCheckBox`, set
    `Checked` to its `IsChecked`; the TaskCheckBox itself returns `nil`
    from `translateNode` (silent drop).
  - `extension/ast.Strikethrough` extends `BaseInline` with no extra
    state; its children are the inline content between the `~~`/`~~`
    delimiters. Translate maps it to `delete{children}`. Position spans
    the source bytes including the four `~` delimiters (Level == 2 for
    goldmark, but Strikethrough is not Emphasis; we compute the span by
    extending the children-span by 2 on each side, mirroring the strong
    expansion at translateEmphasis but with a fixed delimiter length).
  - `ast.AutoLink` (core goldmark) has `AutoLinkType` (URL or Email),
    `Protocol []byte`, and a private `value *Text`. The public `URL(src)`
    method handles the protocol-prefixing case (linkify's `www.` →
    `http://...` prefix); for `<https://example.com>` and bare-URL
    linkify, `URL(src)` returns the literal URL string. No children on
    the goldmark side — translate manufactures a single child `text` node
    with `value` equal to the URL.
- Encoding fact: the existing `writeJSONNullableString` helper (S06)
  handles the `*string` → null-or-quoted convention. For the new
  `table.align: [...]` field (a `[]*string`) we need a new
  `writeJSONNullableStringSlice` helper that brackets the array and
  writes `null` for nil slots, quoted-string for non-nil — the per-slot
  rule matches `writeJSONNullableString`, just iterated.
- Shape decision: applied S04/S05/S06's "pointer-to-T for nullable
  per-element" convention to the new `Align` field. Considered
  `[]string` with sentinel `""`, but rejected because:
  (1) S05/S06 already chose `*int`/`*bool`/`*string` for the nullable
  flavor at the SCALAR boundary, and the per-element rule is the same
  rule applied per slot;
  (2) sentinel `""` is the very thing CONTEXT.md `writeJSONNullableString`
  explicitly distinguishes from `null` ("`null` means \"no language/title/
  meta provided\", `\"\"` would mean \"empty…\""). `align: ""` is not a
  thing in mdast's spec; only `null` for unaligned. So `[]*string` it is.

## Test 1 — tracer bullet: 3-column GFM table with mixed alignments (criterion #1)

- Wrote: fixture `testdata/fixtures/34-table-three-col-mixed-align-nopos/`
  with `args=--no-position`, `exit=0`, empty stderr, and
  `input.md`:
  ```
  | a | b | c |
  |:--|---|--:|
  | 1 | 2 | 3 |
  ```
  Expected stdout (byte-exact):
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"table","align":["left",null,"right"],"children":[{"type":"tableRow","children":[{"type":"tableCell","children":[{"type":"text","value":"a"}]},{"type":"tableCell","children":[{"type":"text","value":"b"}]},{"type":"tableCell","children":[{"type":"text","value":"c"}]}]},{"type":"tableRow","children":[{"type":"tableCell","children":[{"type":"text","value":"1"}]},{"type":"tableCell","children":[{"type":"text","value":"2"}]},{"type":"tableCell","children":[{"type":"text","value":"3"}]}]}]}]}}`
- Red: `go test -run 'TestFixtures/34-' .` returned an empty-envelope mismatch
  — `translate` at S06 silent-dropped `*east.Table` per CONTEXT.md
  "Lossiness policy", so the fixture's stdout came back as
  `{"frontmatter":null,"ast":{"type":"root","children":[]}}`. This tracer
  forces the entire goldmark-extension/ast import, the `Align []*string`
  Node-field extension, the four new translate cases (table, tableHeader,
  tableRow, tableCell), and the new emit slot (table key order, the
  `writeJSONNullableStringSlice` helper) into existence in one cycle.
- Green: extended `internal/translate/translate.go`:
  - Imported `east "github.com/yuin/goldmark/extension/ast"`.
  - `Node` gained `Align []*string`. Documented the per-element-nullable
    rationale inline: applied S05/S06's pointer-to-T-for-nullable
    convention from the scalar to the per-slot case; the `[]string` with
    `""` sentinel was ruled out because `""` is a load-bearing distinct
    value on the wire per `writeJSONNullableString`'s rule.
  - `translateNode` switch gained four cases: `*east.Table`,
    `*east.TableHeader` (mapped to translateTableRow — header is a
    `tableRow` in mdast, the header/data distinction is positional),
    `*east.TableRow`, `*east.TableCell`.
  - Implementations: `translateTable` (children-span position, calls
    `alignmentsToMdast` for `Align`), `translateTableRow` (children-span
    position; signature takes `ast.Node` so it can serve both
    `*east.TableHeader` and `*east.TableRow`), `translateTableCell`
    (children-span position; **drops** `Alignment` per CONTEXT.md
    mdast node-set v1 `tableCell` "no align field" rule),
    `alignmentsToMdast` (maps `AlignLeft/Right/Center` to `*"left"/
    *"right"/*"center"`, `AlignNone` to nil slot).
  - Then in `internal/emit/emit.go`:
  - Added a `case "table":` branch to `writeNode`'s per-type switch,
    emitting `,"align":` followed by the new `writeJSONNullableStringSlice`
    helper output. No case is needed for `tableRow` or `tableCell` because
    they carry only `type` + `children` + `position` — the container
    machinery already handles those (both are containers; isContainer's
    default arm covers them).
  - Added `writeJSONNullableStringSlice(buf, []*string)` — brackets the
    array and writes `null` for nil slots, quoted string for non-nil,
    `,`-separated. Empty input → `[]`. The per-slot rule is delegated
    to `writeJSONNullableString` so the "null vs string" rule remains in
    one named place.
- Refactor: none in this cycle — the new code's shape mirrors the existing
  S06 `translateLink`/`translateImage` + `writeJSONNullableString`
  template, so no extraction was warranted yet.
- Verify: `go test ./...` all green (translate's prior tests, all fixture
  tests including 34, anti-globals, CLI, read unaffected).
- Notes: empty-stderr fixture file is intentionally zero-byte (no trailing
  newline) — matches the existing fixture convention used at S04–S06.

## Test 2 — task list with mixed checked states (criterion #2)

- Wrote: fixture `testdata/fixtures/35-task-list-mixed-checked-nopos/`
  with `args=--no-position`, `exit=0`, empty stderr, and `input.md`:
  ```
  - [x] done
  - [ ] todo
  - plain
  ```
  Expected stdout (byte-exact) — listItem.checked is true/false/null in
  order, no checkbox-shaped child node remains:
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"list","ordered":false,"start":null,"spread":false,"children":[{"type":"listItem","spread":false,"checked":true,"children":[{"type":"paragraph","children":[{"type":"text","value":"done"}]}]},{"type":"listItem","spread":false,"checked":false,"children":[{"type":"paragraph","children":[{"type":"text","value":"todo"}]}]},{"type":"listItem","spread":false,"checked":null,"children":[{"type":"paragraph","children":[{"type":"text","value":"plain"}]}]}]}]}}`
- Red: `go test -run 'TestFixtures/35-' .` failed — every listItem came back
  with `"checked":null` because (a) `translateNode`'s default arm silently
  drops `*east.TaskCheckBox` per "Lossiness policy" (correct — the
  checkbox is NEVER an output node), but (b) the listItem translator never
  read the `IsChecked` flag off the dropped goldmark node. The text body
  for the first two items came through correctly ("done" and "todo") —
  goldmark's tasklist parser advances past the `[x] `/`[ ] ` prefix in
  `block.Advance(m[1])` so the surrounding Text segments hold the body
  only; no `[x]` literal needs to be scrubbed.
- Green: rewrote `translateListItem` in
  `internal/translate/translate.go` to call a new helper
  `extractTaskCheckboxChecked(li)` BEFORE `translateChildren`. The
  helper walks the listItem's first child (goldmark's TextBlock for
  tight items / Paragraph for loose items), checks whether THAT
  container's first inline child is a `*east.TaskCheckBox`, and if so
  returns `*cb.IsChecked` wrapped as `*bool`. The TaskCheckBox itself
  continues to silent-drop through the translateNode default arm, so
  no second emit-path is needed.
  Documented the "only the first inline child of the first container
  child" scoping rule on the helper: matches what the tasklist parser
  itself enforces (it only inserts a TaskCheckBox there, and only when
  the container has no other children yet) and prevents a later refactor
  from accidentally scanning the whole item body for `[x]`-shaped inline
  text.
- Refactor: none — the helper is a 9-line pure function on the goldmark
  side; the original `translateListItem` shape (build children, build
  Node) is unchanged apart from the `Checked` source.
- Verify: `go test ./...` green. Fixture 19 (S05's `- a\n- b\n` plain
  list) and the translate-package unit test
  `TestTranslateListItemNonTaskCarriesNilChecked` both still pass — the
  `Checked: nil` non-task path is exercised by the third item in this
  fixture too, so the regression bar is doubled.

## Test 3 — strikethrough inside a paragraph (criterion #3)

- Wrote: fixture `testdata/fixtures/36-strikethrough-nopos/` with
  `args=--no-position`, `exit=0`, empty stderr, and `input.md=~~struck~~\n`.
  Expected stdout (byte-exact):
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"paragraph","children":[{"type":"delete","children":[{"type":"text","value":"struck"}]}]}]}}`
- Red: `go test -run 'TestFixtures/36-' .` failed. The observed failure
  was particularly informative: stdout had
  `"children":[{"type":"paragraph","children":[]}]` — i.e. the entire
  Strikethrough subtree was lost, NOT just the wrap. Cause: when
  `translateNode` returns nil for a goldmark container, `translateChildren`
  skips it via `continue`, taking the dropped node's children with it.
  The "Lossiness policy" rule is correct for goldmark constructs with no
  mdast home, but Strikethrough DOES have a home (`delete`), so the fix
  is to add a translate case — both restoring the wrap AND its children.
- Green: added `translateStrikethrough` to
  `internal/translate/translate.go`:
  - `translateNode` switch gained `*east.Strikethrough` case.
  - Implementation mirrors `translateEmphasis`'s delimiter-extension
    trick: children-span minus 2 on each side accounts for the
    `~~`/`~~` markers. The mdast type name is `delete` (matches
    CONTEXT.md mdast node-set v1 "delete (GFM strikethrough)") — NOT
    `strikethrough` on the wire; pinned this in the function doc-comment
    so a future refactor doesn't rename it back to the goldmark term.
  - No emit change needed: `delete` is a generic container, so
    isContainer's default arm handles it (no per-type case in
    writeNode's switch); the `type` + `children` + `position` shape is
    the default.
- Refactor: none — the delimiter-extension idiom is identical to
  translateEmphasis's, but they're 3-line bodies; extracting a shared
  `withDelimiter(children, delim)` helper would be premature here. Will
  revisit during the broader S07 refactor pass at end of slice.
- Verify: `go test ./...` green.

## Test 4 — angle-bracket autolink `<https://example.com>` (criterion #4)

- Wrote: fixture `testdata/fixtures/37-autolink-angle-bracket-nopos/` with
  `args=--no-position`, `exit=0`, empty stderr,
  `input.md=<https://example.com>\n`. Expected stdout (byte-exact):
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"paragraph","children":[{"type":"link","url":"https://example.com","title":null,"children":[{"type":"text","value":"https://example.com"}]}]}]}}`
- Red: `go test -run 'TestFixtures/37-' .` failed — paragraph came back
  with empty children (Lossiness-policy silent-drop of `*ast.AutoLink`).
- Green: extended `internal/translate/translate.go`:
  - `translateNode` switch gained a `*ast.AutoLink` case.
  - `translateAutoLink` synthesizes a single `text` child whose `value`
    is the URL string (via goldmark's public `AutoLink.URL(src)`
    getter; this handles both the literal-URL case for `<https://…>` and
    the `http://`-prefix injection for GFM linkify's `www.`-prefixed
    form). The mdast `link` carries `url:URL`, `title:nil` (autolinks
    have no title per the CONTEXT.md autolink rule), and the single
    `text` child.
  - Position: attached `(0, 0)` for both the `link` and its synthetic
    `text` child. Goldmark's `*ast.AutoLink` is a BaseInline with no
    public segment getter; the v1 fixtures here use `--no-position`
    anyway. S10 (the position-info pinning slice) will revisit if a
    default-mode autolink fixture is added.
- Refactor: none. The `link{url, title:nil, children:[text(URL)]}` shape
  could potentially share construction with `translateLink`, but
  `translateLink` consumes goldmark's already-built `children` while
  `translateAutoLink` SYNTHESIZES the child. The shared shape is only
  three lines of struct construction; extracting it would be premature.
- Verify: `go test ./...` green.

## Test 5 — bare-URL GFM linkify `https://example.com` (criterion #5)

- Wrote: fixture `testdata/fixtures/38-autolink-bare-url-nopos/` with
  `args=--no-position`, `exit=0`, empty stderr,
  `input.md=https://example.com\n`. Expected stdout is byte-identical to
  fixture 37's:
  `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"paragraph","children":[{"type":"link","url":"https://example.com","title":null,"children":[{"type":"text","value":"https://example.com"}]}]}]}}`
- Red: n/a in this cycle — the fixture passed immediately after Test 4's
  Green landed. This is expected: goldmark's GFM linkify extension lowers
  bare URLs (with explicit `http://`/`https://`/`ftp://` prefix or the
  `www.` shorthand) into the SAME `*ast.AutoLink` node type that the
  CommonMark `<...>` parser produces. One translate case covers both
  paths.
  The fixture is therefore a **regression-prevention assertion** for
  acceptance criterion #5: any future change that breaks the bare-URL
  path without breaking the angle-bracket path (e.g., a translate
  refactor that special-cased `Protocol != nil`) would fail here
  independently. Strict TDD discipline holds because the failure mode
  the test catches is real and orthogonal: before Test 4 landed,
  fixtures 37 AND 38 both failed; after Test 4, both pass; and the
  fixture provides a separate observable that would catch a one-sided
  regression.
- Green: no code change. The `*ast.AutoLink` case from Test 4 already
  handles this input shape.
- Refactor: none.
- Verify: `go test ./...` green.

## Test 6 — translate-package unit anchors for S07 load-bearing rules

After the five wire-fixture cycles landed, added five focused unit tests
in `internal/translate/translate_test.go` mirroring the S05/S06 pattern of
pinning load-bearing rules at the Go-tree boundary so a translate-internal
refactor surfaces at unit-test level (not only at the wire fixture level):

- `TestTranslateTableAlignNoneMapsToNilSlot` — pins
  `alignmentsToMdast`'s `default: nil` arm: `AlignNone` maps to a
  nil slot in `Align []*string`, NOT to a `*"none"` pointer.
  Anchors acceptance criterion #1.
- `TestTranslateTableCellHasNoAlignField` — pins the mdast-deviation
  rule: `tableCell` carries no per-cell alignment field; the Node
  struct has no per-cell field at all, and translate is responsible
  for NEVER setting `cell.Align`. Walks every cell of the 3×2 fixture
  and asserts `Align == nil`. Anchors acceptance criterion #1.
- `TestTranslateTaskCheckboxHoistedAndDropped` — pins BOTH halves of
  the GFM task-list rule: the IsChecked bool is hoisted to
  `listItem.Checked` (*true / *false / nil for the three items), AND
  no checkbox-shaped descendant survives anywhere in any item's
  subtree (uses `assertNoCheckboxNodeAnywhere`, a recursive walker
  looking for `Type` values like "taskCheckBox" / "checkbox" — these
  must remain absent forever). Anchors acceptance criterion #2.
- `TestTranslateStrikethroughTypeIsDeleteNotStrikethrough` — pins the
  mdast type name `delete` (matches CONTEXT.md / remark, NOT
  goldmark's `Strikethrough` term). Anchors acceptance criterion #3.
- `TestTranslateAutoLinkCollapsesToLinkWithTextChild` — table-driven,
  covers both autolink shapes (angle-bracket and bare-URL linkify) in
  one test; verifies `Type == "link"`, `URL == "https://example.com"`,
  `Title == nil`, and the single synthetic text child carries the URL
  as its value with `ValuePresent: true`. Anchors acceptance criteria
  #4 AND #5.

All five tests pass first run after the prior cycles. They are NOT
fresh red-green cycles (the production code was already complete) —
they are regression-prevention anchors at the translate-package
boundary, following the same pattern S05/S06 used for `Checked: nil`,
`Lang: nil`, `image.Alt` flattening. Without them, a future refactor
that, say, renamed `delete` to `strikethrough` would only surface at
the wire fixture level (a noisy diff far from the bug); with them,
the failure points exactly at the translate-side mutation.

## Final

- Tests added: 5 fixtures (34, 35, 36, 37, 38) + 5 translate-package
  unit tests (TestTranslateTableAlignNoneMapsToNilSlot,
  TestTranslateTableCellHasNoAlignField,
  TestTranslateTaskCheckboxHoistedAndDropped,
  TestTranslateStrikethroughTypeIsDeleteNotStrikethrough,
  TestTranslateAutoLinkCollapsesToLinkWithTextChild — the last is
  table-driven with two sub-cases, but counts as one TEST under
  `go test`'s top-level naming). Total: 10 new TDD assertions for
  S07's five acceptance criteria.
- Tests passing: 39/39 (across all packages — `main`, `internal/cli`,
  `internal/read`, `internal/translate`, plus the integration
  fixtures harness; `internal/emit` and `internal/parse` remain
  test-file-free since they are tested via the integration harness
  and the translate-side unit tests).
- Coverage: not measured for S07 (the slice's discipline is fixture-
  per-acceptance-criterion + translate-side load-bearing anchors,
  not statement-coverage chasing; matches S05/S06 convention).
- go vet ./...: clean.
- Acceptance criteria status:
  - [x] criterion 1 (3-col table with `:--|---|--:` produces
    `table.align: ["left", null, "right"]`, with `tableRow`/`tableCell`
    children, no `align` on `tableCell`) — fixture 34 +
    `TestTranslateTableAlignNoneMapsToNilSlot` +
    `TestTranslateTableCellHasNoAlignField`.
  - [x] criterion 2 (`- [x] done\n- [ ] todo\n- plain` produces
    `checked: true/false/null` in order; no checkbox-shaped child
    survives) — fixture 35 +
    `TestTranslateTaskCheckboxHoistedAndDropped`.
  - [x] criterion 3 (`~~struck~~` inside a paragraph produces a
    `delete` node containing `text{value:"struck"}`) — fixture 36 +
    `TestTranslateStrikethroughTypeIsDeleteNotStrikethrough`.
  - [x] criterion 4 (`<https://example.com>` produces a `link` with
    `url:"https://example.com"`, `title:null`, single text child with
    URL as value) — fixture 37 +
    `TestTranslateAutoLinkCollapsesToLinkWithTextChild/angle-bracket`.
  - [x] criterion 5 (bare URL `https://example.com` produces the
    same link shape — no AutoLink type on the wire, no goldmark-
    native names) — fixture 38 +
    `TestTranslateAutoLinkCollapsesToLinkWithTextChild/bare-url-linkify`.

VERDICT: accept
