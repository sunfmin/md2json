# TDD log: 03-display-math-happy-path

Started: 2026-05-24

## Approach

Strict TDD, tracer-bullet style. Default-path implementation (no
`selection.md` — no prototype Round for this issue). Issue acceptance has
5 bullets; mapped to 4 black-box integration fixtures plus 2 Go-layer
unit tests under `internal/translate/translate_test.go`:

- **#66 display-math-happy-path-nopos** — bullets 1 (shape: root.children
  = [math{value, meta}]), 2 (meta key present as literal null), 5
  (no-position half).
- **#67 display-math-happy-path-default** — bullet 5 (with-position
  half; pins `position` field exists on `math`, `root` per PRD #2 +
  uniform Position-info rule).
- **#68 display-math-value-preservation-nopos** — bullet 3 (mhchem
  `\ce{H2O}` byte-for-byte preservation; PRD #7).
- **#69 display-math-in-frontmatter-envelope-nopos** — bullet 4
  (frontmatter envelope co-exists with display math; PRD #12).

Mirrors S02's pattern (S02 added `*mathjax.InlineMath`→`inlineMath`;
S03 adds `*mathjax.MathBlock`→`math`). ADR-0004 Decision 4 names are
binding. S04 (currency post-pass), S05 (unclosed-`$$` compensation),
S06 (in-block composition) are downstream issues out of S03 scope.

## Test 1: 66-display-math-happy-path-nopos (tracer bullet)

- Wrote: `testdata/fixtures/66-display-math-happy-path-nopos/{args,input.md,stdout,stderr,exit}`.
- Input: `$$\n\frac{a}{b}\n$$\n` under `--no-position`.
- Expected stdout: `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"math","meta":null,"value":"\\frac{a}{b}\n"}]}}`.
- Red: pre-implementation run produced `root.children:[]` — the
  `*mathjax.MathBlock` node was silent-dropped by `translateNode`'s
  default arm (no case existed). Confirmed RED:
  ```
  got:  "{\"frontmatter\":null,\"ast\":{\"type\":\"root\",\"children\":[]}}"
  want: "{\"frontmatter\":null,\"ast\":{\"type\":\"root\",\"children\":[{...math...}]}}"
  ```
- Green: added one switch case `*mathjax.MathBlock` → `translateMath`
  in `internal/translate/translate.go` + new `translateMath` helper,
  plus `math` case in `internal/emit/emit.go` `writeNode` and the
  `isContainer` switch (leaf — no `children` key on the wire).
- Notes:
  - `value` extraction: `m.Lines().Value(src)` concatenates per-line
    body segments byte-for-byte. The library's `Continue` branch at
    `probe/goldmark-mathjax/block.go:60-64` appends each body line's
    segment INCLUDING its trailing `\n`; the closing-fence branch at
    `block.go:49-57` returns `parser.Close` BEFORE appending the
    closing line, so the closing `$$` is NOT in `value` (mirrors
    `code.value` exclusion of the closing fence per CONTEXT.md
    "Text/Code value preservation").
  - `meta` is `nil` (`*string`) → emit serializes via
    `writeJSONNullableString` as JSON `null`. The field is ALWAYS
    present, never elided, per CONTEXT.md `math node` entry and S03
    acceptance bullet #2.
  - Key order in emit: `{type, meta, value}` (then `position` when
    not stripped). Mirrors the `code` case's `lang, meta, value`
    precedent — nullable metadata fields land before the value-bearing
    field.

## Test 2: 67-display-math-happy-path-default

- Wrote: `testdata/fixtures/67-display-math-happy-path-default/{args,input.md,stdout,stderr,exit}`.
- Input: `$$\n\frac{a}{b}\n$$\n`, default flags.
- Expected stdout pins the `position` field on the `math` and `root`
  nodes byte-for-byte. Note: no surrounding `paragraph` (`$$` is a
  top-level block, not inline content).
  - `math`: start `{line:2,column:1,offset:3}`, end `{line:3,column:1,offset:15}`.
    Span covers only the body line(s) — opening `$$` (offsets 0..2)
    and closing `$$` (offsets 15..17) lie outside. Mirrors
    `translateFencedCodeBlock`'s `blockOffsets`-driven span: fenced
    blocks' opening/closing fences are out-of-span on the same
    precedent.
  - `root`: start `{1,1,0}`, end `{line:4,column:1,offset:18}` (end of
    file after trailing `\n`).
- Red: implicit in byte-exact compare; passed first try on the
  position math (computed against `Lines().At(0).Start=3`,
  `Lines().At(0).Stop=15`, then `blockOffsets` snaps start back to
  the line containing offset 3, which IS already offset 3 — body line
  starts immediately after the opening fence's LF).
- Green: same code as Test 1.
- Notes: position-span derivation choice (body-only, exclude fences)
  follows the established fenced-code-block precedent. Issue bullet
  #5 only requires shape uniformity ("position present by default,
  stripped under `--no-position`"); doesn't pin a specific
  delimiter-inclusion choice for display math. PRD #2 doesn't pin
  position either.

## Test 3: 68-display-math-value-preservation-nopos

- Wrote: `testdata/fixtures/68-display-math-value-preservation-nopos/{args,input.md,stdout,stderr,exit}`.
- Input: `$$\n\ce{H2O}\n$$\n` under `--no-position`.
- Expected: `math{value:"\\ce{H2O}\n", meta:null}`. mhchem source rides
  through verbatim — md2json is transport-only.
- Red/Green: passed first try with the Test 1 implementation
  (`Lines().Value(src)` extracts body bytes verbatim, no
  validation/normalization).
- Notes: covers PRD fixture #7. CONTEXT.md `Dollar-sign math
  (transport-only)` posture — no `\ce` validation, no chemistry
  rendering. The `\ce{H2O}` bytes land in `value` byte-for-byte;
  downstream KaTeX/MathJax+mhchem-plugin is the consumer.

## Test 4: 69-display-math-in-frontmatter-envelope-nopos

- Wrote: `testdata/fixtures/69-display-math-in-frontmatter-envelope-nopos/{args,input.md,stdout,stderr,exit}`.
- Input: four-line document with closed YAML frontmatter
  (`title: t`) followed by `$$\nx\n$$\n`.
- Expected envelope: `{"frontmatter":{"title":"t"},"ast":{...math{value:"x\n",meta:null}...}}`.
- Red/Green: passed first try. The frontmatter pre-scan + lift in
  `internal/parse/parse.go` is unchanged by S03 — math wiring lives
  one layer up from the YAML lift. The math block is recognized at
  document top-level after the frontmatter is stripped from the body.
- Notes: pins bullet #4 (frontmatter co-existence). Covers PRD fixture
  #12.

## Go-layer unit tests

Added two unit tests in `internal/translate/translate_test.go` to anchor
the math translation at the translate layer (mirrors precedent: Go-layer
test for `code.value` trailing-newline preservation, plus per-fixture
byte-exact compare for the wire):

- **`TestTranslateDisplayMathHappyPath`** — asserts the
  `*mathjax.MathBlock`→`math` mapping yields the right Type, Value
  (trailing `\n` preserved, closing fence NOT included), nil Meta
  (signaling JSON null), zero children (leaf node).
- **`TestTranslateDisplayMathPreservesMhchemValue`** — anchors
  CONTEXT.md "Text/Code value preservation" + "Dollar-sign math
  (transport-only)" for the mhchem-inside-`$$` byte-for-byte rule
  at the Go layer.

## Refactor pass

Implementation surface is small (one new case + ~10-LoC helper in
translate, one new case + one isContainer entry in emit). The
`translateMath` helper is structurally similar to
`translateFencedCodeBlock` (both extract `value` via
`Lines().Value(src)` and derive position via `blockOffsets`), but
extracting a shared helper would require parameterizing on Type/Meta
shape; the duplication is two lines and the abstraction would be
more code than it saves. Left as-is.

Final inspection:

- `translate.go` imports unchanged (`mathjax` was already imported
  for S02's `*mathjax.InlineMath` case).
- `translateNode` switch arm added: `case *mathjax.MathBlock`.
- `translateMath` function added (~10 LoC: extract `value` via
  `Lines().Value(src)`, derive position via `blockOffsets`, return
  Node with `Type:"math"`, nil Meta).
- `emit.go` `writeNode` arm added: `case "math"` (writes
  `,"meta":<nullable>,"value":<str>` after `type`).
- `emit.go` `isContainer` updated: `math` listed alongside `text`,
  `inlineCode`, `inlineMath`, etc. (leaf — no `children` key).

## Library-quirk verification

Per the issue's note ("If you find a library quirk... S05's territory
if it's about unclosed, or out-of-scope if it's about
closed-happy-path-edge"):

- Closed `$$...$$` happy path: library behavior matches the PRD's
  derivation exactly. `MathBlock.Lines()` for `$$\n\frac{a}{b}\n$$\n`
  holds one segment covering `\frac{a}{b}\n` (Start=3, Stop=15);
  `Lines().Value(src)` yields the body bytes verbatim with the
  trailing LF preserved. No trim, no normalization.
- Unclosed `$$` (no closing fence): S05's territory — out of S03
  scope. Fixture 14 (PRD library-behavior assertion) is also S05's
  territory.
- No quirks surfaced on the closed-happy-path inputs S03 exercises.

## Acceptance check

- [x] **#1** Input `$$\n\frac{a}{b}\n$$\n` produces AST whose root has
  exactly one `math` child with `value: "\\frac{a}{b}\n"` and
  `meta: null`; exit `0` — fixtures 66 (no-position) + 67
  (with-position) byte-exact green, plus
  `TestTranslateDisplayMathHappyPath` Go-layer anchor green.
- [x] **#2** Serialized JSON for `math` includes the `meta` key with
  literal `null`; key is present, not elided — enforced by the
  byte-exact fixture compare (any elision would flip the byte stream)
  and by `writeJSONNullableString`'s `nil → "null"` rule (any nil
  `*string` Meta serializes as `"meta":null`). Pretty-mode preserves
  the same key/value pair (`json.Indent` is a pure whitespace
  reformat; doesn't elide nulls).
- [x] **#3** Input `$$\n\ce{H2O}\n$$\n` produces
  `math{value:"\\ce{H2O}\n", meta:null}` — fixture 68 byte-exact green;
  `TestTranslateDisplayMathPreservesMhchemValue` Go-layer anchor.
- [x] **#4** Frontmatter + display math envelope: `frontmatter:{title:"t"}`
  + `ast.children=[math{value:"x\n", meta:null}]`; frontmatter codepath
  unchanged — fixture 69 byte-exact green.
- [x] **#5** Same display math input under `--no-position` strips the
  `position` field; default keeps it — fixture 66 (stripped) vs fixture
  67 (kept). Existing `TestEmitNoPositionStripsPositionKeyFromEveryNode`
  property test still green; its corpus does not include display math,
  but the per-fixture diff catches any per-node-type regression.

## Final

- Tests added: 4 black-box integration fixtures + 2 Go-layer unit tests = 6.
- Tests passing: 6/6 of the new tests + every pre-existing test in the
  suite (`go test ./...` green across all six packages).
- Files touched:
  - `internal/translate/translate.go` — add `case *mathjax.MathBlock` to
    `translateNode`; add new `translateMath` helper.
  - `internal/translate/translate_test.go` — add
    `TestTranslateDisplayMathHappyPath` +
    `TestTranslateDisplayMathPreservesMhchemValue`.
  - `internal/emit/emit.go` — add `case "math"` to `writeNode` (writes
    `meta` then `value`); add `"math"` to `isContainer`'s leaf-list.
  - `testdata/fixtures/66-display-math-happy-path-nopos/{args,input.md,stdout,stderr,exit}`.
  - `testdata/fixtures/67-display-math-happy-path-default/{args,input.md,stdout,stderr,exit}`.
  - `testdata/fixtures/68-display-math-value-preservation-nopos/{args,input.md,stdout,stderr,exit}`.
  - `testdata/fixtures/69-display-math-in-frontmatter-envelope-nopos/{args,input.md,stdout,stderr,exit}`.
- Commits: none in this Stage (per Rundays orchestrator-protocol).
- No ADRs added: the architectural decisions (library pick, name
  alignment `*mathjax.MathBlock`→`math`, wiring style) are all already
  pinned by ADR-0004 Decision 4. S03 is a straight execution of that
  ADR — same as S02 was.
- Note on `mdastNodeSetV1` map drift: the `lossiness_property_test.go`
  `mdastNodeSetV1` map does NOT yet include `inlineMath` or `math`
  (pre-existing drift from S02; S02's tdd-log did not sync it either).
  The coverage test still passes because `lossinessCorpus` does not
  exercise math inputs. A future Run (likely S07 or a final-polish
  pass) should sync both the map AND the corpus so the wire-contract
  enumeration is enforced for the math node types too. Logged here
  for traceability; not blocking S03 acceptance.
- VERDICT: accept.
