# Arch log: mini

Started: 2026-05-23
Scope: mini
File set (from S05 tdd-log "Files touched"):
- internal/translate/translate.go (extended: Node fields, dispatch, hard-break post-step, 5 new translate funcs)
- internal/translate/translate_test.go (4 new unit tests appended)
- internal/emit/emit.go (extended writeNode switch with list/listItem cases; isContainer leaves +thematicBreak/break)
- 7 new fixtures under testdata/fixtures/ (19..25)

## Baseline

- `go test ./... -v`: 31 PASS, 0 FAIL across 5 packages (root, internal/cli, internal/read, internal/translate; internal/emit + internal/parse have no test files but build clean). TestFixtures alone has 25 sub-fixtures all green.
- `go vet ./...`: clean (per tdd-log).
- Total LOC in scope: 447 (translate.go) + 296 (translate_test.go) + 80 (position.go, unchanged at S05) + 234 (emit.go) = 1057 lines.

## Candidate inventory

Reviewed against the four lenses (CONTEXT.md glossary alignment, module depth,
naming clarity, locality), informed by the S05 tdd-log's own refactor pass
notes and S01/S03/S04 arch precedents:

### C1: translate.translateNode switch growing (9 goldmark cases at S05)
- Score: **Speculative**
- Rationale: dispatch is exhaustive and compile-checked across `*ast.Heading`,
  `*ast.Paragraph`, `*ast.Text`, `*ast.Emphasis`, `*ast.List`, `*ast.ListItem`,
  `*ast.TextBlock`, `*ast.Blockquote`, `*ast.ThematicBreak`. Deletion test for a
  `map[reflect.Type]func(...) *Node` registry: callsites would still need the
  per-type translator functions, the map lookup would replace the type-switch
  with a reflect.TypeOf indirection, and we'd lose Go's compile-time
  exhaustiveness on the type-switch (a goldmark version bump that adds a new
  Node kind would not surface at build time anymore). Same conclusion S04 arch
  reached at 6 cases; ratio of code-saved to safety-lost remains a bad trade
  at 9. Re-evaluate around ~15 cases or when adding a node type genuinely
  becomes a multi-edit grind beyond "add case + add func".

### C2: emit.writeNode type-specific switch (heading/text/list/listItem at S05)
- Score: **Speculative**
- Rationale: 4 cases, ~50 lines. The switch is the single decision point for
  mdast canonical-key-order serialization. Splitting per-type field-writers
  (`writeListFields`, `writeListItemFields`, ...) would consolidate each
  case's body but each helper would have one caller — hypothetical seams.
  Apply the deletion test: removing the helpers, each function body
  reappears once inline. No locality win.

### C3: Hard-break post-step lives inline in translateChildren
- Score: **Worth exploring (not acted on)**
- Rationale: the synthetic `break` insertion is tightly coupled to the
  *goldmark* node (`c.(*ast.Text).HardLineBreak()`, `t.Segment.Stop`, peeking
  at `c.NextSibling()`). Extracting to a `maybeAppendHardBreak(c ast.Node,
  out []*Node, pt *positionTracker) []*Node` helper would gain 6 fewer lines
  in `translateChildren` but the helper would need the same `*ast.Text`
  unwrap and segment math. The hard-break and text-coalesce post-steps both
  live in `translateChildren` precisely because both depend on goldmark
  Node identity (not just the translated `*Node` result) — extracting one
  but not both would split the goldmark-coupled logic across two places,
  worsening locality. Leave inline.

### C4: Field naming vs CONTEXT.md "mdast node-set v1" glossary
- Score: **not a candidate (already aligned)**
- Rationale: every new field on `Node` matches CONTEXT.md verbatim:
  `Ordered` ↔ `list.ordered`, `Start *int` ↔ `list.start` (nullable),
  `Spread bool` ↔ `list.spread` and `listItem.spread`,
  `Checked *bool` ↔ `listItem.checked` (nullable). No `_Avoid_` synonyms in
  use. The leaf classification of `thematicBreak` and `break` matches the
  mdast node-set v1 leaf set. Internal helper names (`translateList`,
  `translateListItem`, `translateTextBlock`, `translateBlockquote`,
  `translateThematicBreak`, `childrenSpan`) are descriptive and don't clash.

### C5: Per-type structs instead of single-`Node`-with-optional-fields
- Score: **Speculative** (already explicitly rejected in S05 tdd-log refactor)
- Rationale: S05's tdd-log refactor pass weighed and rejected this for the same
  reasons S04's arch did. Single-struct + tag-by-Type is the simpler emit
  form; per-type subtypes would require either reflection or a visitor in
  writeNode. Re-confirming the prior decision; not a refactor candidate.

### C6: writeNode bool-rendering repetition (`ordered`, `spread`-on-list, `spread`-on-listItem)
- Score: **Strong** (small, but real)
- Rationale: three call-sites in `writeNode` use the same 5-line if/else to
  serialize a Go `bool` as JSON `true`/`false`:
  ```go
  if n.Ordered { buf.WriteString("true") } else { buf.WriteString("false") }
  ```
  Three adapters of the same shape — past the "two adapters = real seam"
  threshold. Extracting `writeJSONBool(buf, bool)` compresses each to a
  one-liner and names the rendering convention. Boundary: the helper does
  one thing (Go bool → ASCII `true`/`false` literal, no separator); the
  separator-and-key prefix stays at the callsite where the canonical
  key-order context lives. This is a small consolidation, not a depth coup —
  but it removes 12 lines of identical boilerplate and is purely
  syntactic so the test-suite risk is minimal.

### C7: writeNode nullable-pointer field rendering (`start *int`, `checked *bool`)
- Score: **Strong** (names a CONTEXT.md-load-bearing convention)
- Rationale: CONTEXT.md and the S05 tdd-log both name the "pointer types for
  nullable mdast fields" convention as load-bearing — it's the entire reason
  Node carries `Start *int` instead of `int` and `Checked *bool` instead of
  `bool`. Currently the wire-rendering of this convention lives inline in
  `writeNode` in TWO different syntactic forms:
  - `start *int` → if/else on nil writing "null" or `strconv.Itoa(*Start)`
    (4 lines, in `case "list"`)
  - `checked *bool` → 3-arm switch on nil/*true/*false writing
    "null"/"true"/"false" (7 lines, in `case "listItem"`)
  Two adapters of the same conceptual operation (nullable-pointer → JSON
  value-or-null) — real seam. Extracting `writeJSONNullableInt(buf, *int)`
  and `writeJSONNullableBool(buf, *bool)` names the convention at the
  emit layer and concentrates the "nil → `null`, not omitted, not zero-
  rendered" rule in one named function per type. CONTEXT.md upcoming nodes
  (`code{lang,meta}`, `link{title}`, `image{title,alt}`, `table{align}`)
  will all need this same rule on additional pointer types in S06–S08, so
  having the convention already named at the emit layer is locality work
  paid forward, not premature abstraction.

## Pass 1: Extract writeJSONBool helper in emit.go

- Files touched: internal/emit/emit.go
- Diff summary:
  - Added top-level `writeJSONBool(buf *bytes.Buffer, v bool)` helper.
  - Replaced the three if-bool-else-false sequences in `writeNode`
    (`ordered` field on list, `spread` field on list, `spread` field on
    listItem) with a single call each.
- Tests after: 31 PASS, 0 FAIL (`go test ./...`); `go vet ./...` clean.
- Reverted? no.

## Pass 2: Extract writeJSONNullableInt and writeJSONNullableBool helpers in emit.go

- Files touched: internal/emit/emit.go
- Diff summary:
  - Added top-level `writeJSONNullableInt(buf *bytes.Buffer, p *int)` —
    writes `null` for `p == nil`, else `strconv.Itoa(*p)`.
  - Added top-level `writeJSONNullableBool(buf *bytes.Buffer, p *bool)` —
    writes `null` for `p == nil`, `true`/`false` otherwise.
  - Replaced the inline if-nil-else-itoa for `start` in `case "list"` with
    `writeJSONNullableInt(buf, n.Start)`.
  - Replaced the 3-arm switch for `checked` in `case "listItem"` with
    `writeJSONNullableBool(buf, n.Checked)`.
  - Updated doc comments to name "pointer types for nullable mdast
    fields" as the CONTEXT.md-load-bearing convention these helpers
    implement.
- Tests after: 31 PASS, 0 FAIL (`go test ./...`); `go vet ./...` clean.
- Reverted? no.

## Final

- Tests: 31 passing (unchanged from baseline), 0 failing. `go vet ./...` clean.
- LOC delta: net +14 in internal/emit/emit.go (234 → 248). The writeNode
  body shrank by ~23 lines (list case 18→6, listItem case 15→4), and the
  three new top-level helpers — with substantial doc comments naming the
  CONTEXT.md "pointer types for nullable mdast fields" convention they
  implement — added ~37 lines. Net is a slight line increase; the win is
  in concentration and naming, not raw line count.
- Most consequential change: the "pointer types for nullable mdast
  fields" convention from CONTEXT.md (and S05's tdd-log refactor notes)
  now has a named code location at the emit layer. Previously the same
  convention was open-coded in two different syntactic forms (a 4-line
  if/else for `*int`, a 7-line switch for `*bool`) inside writeNode's
  per-type cases. Now `writeJSONNullableInt`/`writeJSONNullableBool`
  name the rendering rule once; future nullable fields landing in S06
  (`code.lang`, `code.meta` per CONTEXT.md mdast node-set v1) reuse the
  same named helpers rather than open-coding the third syntactic form.
  The smaller `writeJSONBool` extraction is a side effect — it removes
  12 lines of identical bool-rendering boilerplate but is stylistic
  consolidation, not a CONTEXT.md-anchored win on its own.

VERDICT: accept
