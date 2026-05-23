# TDD log: 10a-pretty-print-and-property-test

Started: 2026-05-23

## Setup

- Reused the Go `testing` framework + existing module layout. No new framework.
- Slice goal: pin the `--pretty` 2-space-indented form (composing cleanly with `--no-position` and preserving every explicit `null` field), and land the silent-drop property test (US33) that asserts every emitted `type` is a member of CONTEXT.md's **mdast node-set v1**.
- Baseline before any code change: `go test ./...` clean (57 top-level + 97 subtests from S10); `go vet ./...` clean. `--pretty` was a recognized but no-op flag (S01); compact key ordering already implemented per-type in `internal/emit/emit.go::writeNode` (S05–S09); null-field preservation already enforced via the never-elide pointer-field writers (`writeJSONNullableBool`, `writeJSONNullableString`); `--no-position` already gated uniformly at the end of `writeNode`.
- Pretty-mode strategy choice: **post-process the compact byte stream with `encoding/json.Indent`**, vs. a per-type indented writer. The issue spec leans toward simpler. I chose `json.Indent` for three reasons:
  1. It guarantees by construction that compact and pretty paths produce the same token order (criterion #3's "byte-stable up to whitespace" property is trivially true — the pretty bytes literally ARE the compact bytes plus inserted whitespace).
  2. The per-type `writeNode` switch (S05–S09) already establishes the canonical key order; routing pretty through a second walker would duplicate that order-defining logic and create two places where a new node type's field-order must be specified.
  3. `json.Indent` preserves explicit `null` values verbatim (it is a pure whitespace re-formatter), so criterion #4's never-elide rule is preserved automatically.
  The alternative (a per-type indented writer parameterized by depth) would have been ~50 lines of changes spread across every case in the switch, with no observable benefit. Rejected.

## Test 1 — tracer: `--pretty` fixture for a representative document (criterion #1, indentation half)

- Wrote: `testdata/fixtures/57-pretty-title-and-bold/` with `input.md = "# Title\n\nBody with **bold**.\n"`, `args = "--pretty --no-position"`, and a hand-written 2-space-indented `stdout` covering the envelope + root + heading + paragraph + text + strong + text + strong/text + text shape (key order: `type` first, then `depth` on heading, then `children`, no `position`).
- Red: ran `go test -run "TestFixtures/57-pretty-title-and-bold" .` BEFORE wiring `--pretty` through to emit. Failure (excerpted):
  ```
  stdout mismatch
    got:  "{\"frontmatter\":null,\"ast\":...}" (compact — current --pretty is no-op)
    want: "{\n  \"frontmatter\": null,\n  \"ast\": {\n  ..."  (2-space pretty)
  ```
  Confirmed `--pretty` was a no-op and the fixture is RED.
- Green: three changes.
  1. `internal/emit/emit.go::Options`: added `Pretty bool` field.
  2. `internal/emit/emit.go::Emit`: after the compact bytes are built (via the existing per-type writeNode switch), if `opts.Pretty` then re-format with `json.Indent(&indented, buf.Bytes(), "", "  ")` and write `indented.Bytes()` to `w` instead of `buf.Bytes()`. The whole compact-build path is untouched — pretty is a strict post-processing step.
  3. `internal/cli/cli.go::Run`: thread `opts.pretty` through to both `emit.Options` shapes (the FrontmatterOnly short-circuit AND the default-envelope path). Two-line change.
  Ran the fixture again — GREEN.
- Notes: the tracer proves the end-to-end path (cli flag → emit option → json.Indent → stdout) works before tackling the harder per-criterion fixtures. The `json.Indent` approach was vindicated immediately: zero per-type changes needed in writeNode, and the existing compact byte stream's key order survived intact.

## Test 2 — fixture: pretty + indented code preserves null lang/meta (criterion #1, null half)

- Wrote: `testdata/fixtures/58-pretty-indented-code-null-lang-meta/` with `input.md = "    abc\n    def\n"` (indented code block), `args = "--pretty --no-position"`. Expected `stdout` includes `"lang": null,` and `"meta": null,` on the emitted `code` node — pinning the never-elide rule under pretty mode for the indented-code case CONTEXT.md "mdast node-set v1" calls out explicitly.
- Red: skipped — the fixture would pass on the GREEN state from Test 1 (json.Indent preserves null fields by construction, and writeJSONNullableString already writes `null` for the indented-code lang/meta case per S06). Ran the fixture to confirm: GREEN on first attempt.
- Notes: this fixture's value is locking the wire-level contract that no future refactor (e.g. a "skip null fields" optimization) silently elides nulls. A regression of that flavor would break the byte-exact compare here AND in `TestNullFieldsPreservedInBothModes` (Test 4 below).

## Test 3 — fixture: pretty + position composes (criterion #1's `"position":{...}` form)

- Wrote: `testdata/fixtures/59-pretty-indented-code-with-position/` with the same `input.md` as fixture 58 but `args = "--pretty"` (positions ON). Expected `stdout` includes `"position": { "start": {...}, "end": {...} }` on the `code` node AND the `root`, with the same 2-space indentation as the rest of the tree. This is the literal form of criterion #1's example: `{"type":"code","lang":null,"meta":null,"value":"...","position":{...}}` rendered in pretty mode.
- Red: skipped — same construction as Test 2; json.Indent preserves position objects byte-stably.
- Notes: paired with fixture 58 (no-position) and the unit test `TestPrettyComposesWithNoPosition`, this fixture closes the three-way matrix (pretty-with-position, pretty-without-position, compact-with-position) at the wire level.

## Test 4 — unit: compact ↔ pretty byte-stability (criterion #3) + null preservation in both modes (criterion #4) + pretty composes with no-position (criterion #2)

- Wrote: `internal/emit/emit_test.go` (new file, package `emit_test` external so it can import parse + translate without cycles). Three tests over a single `representativeCorpus` covering heading, paragraph, em, strong, list, listItem (non-task), fenced code with lang, indented code (null lang/meta), and a link with no title (null title):
  1. `TestPrettyAndCompactAreByteStableUpToWhitespace` — runs the corpus through emit twice, strips structural whitespace (space/tab/LF/CR, with `"..."` literal awareness) from the pretty output, asserts equality with compact. Also asserts `len(pretty) > len(compact)` so the property is non-trivial (a no-op pretty would trivially pass the stripping check).
  2. `TestNullFieldsPreservedInBothModes` — substring-counts `"checked":null` / `"checked": null`, `"lang":null` / `"lang": null`, `"meta":null` / `"meta": null`, `"title":null` / `"title": null` in compact and pretty modes against expected occurrence counts (1 / 1 / 2 / 1 — meta:null appears twice because the corpus has both an indented and a fenced code block, and the fenced one's `meta` is also null).
  3. `TestPrettyComposesWithNoPosition` — asserts `bytes.Count(pretty, []byte(\"position\")) == 0` under `--pretty --no-position`, then strips whitespace from the pretty output AND re-indents the result with `json.Indent`, asserting the round-trip equals the original pretty output (catches a regression where the no-position gate left a dangling comma).
- The `stripJSONWhitespace` helper handles `"..."` literals with `\"` escape awareness so a hard-break inside a string value wouldn't get stripped. The helper is the only non-trivial piece in this file; everything else is property assertions over `bytes.Count` / `bytes.Equal`.
- Red: ran the tests BEFORE wiring `--pretty` through. The byte-stability test would have failed on the no-op `--pretty` because pretty == compact (both compact bytes), so `len(pretty) > len(compact)` would fail; the null-preservation test would have passed (compact-mode preservation was already enforced); the pretty-composes-with-no-position round-trip test would have failed because compact-bytes don't round-trip through `json.Indent` back to themselves. Actually I wrote these tests AFTER Test 1's green, so the wiring was already in place and they all passed first run. The negative-direction control for the lossiness property test (Test 6 below) is the explicit "RED-then-GREEN" demonstration I added in lieu of running these in their pre-impl RED state.
- Green: same `internal/emit/emit.go::Emit` change from Test 1; no further production changes.
- Notes: I considered consolidating the three tests into one parameterized "pretty contract" test. Decided to keep them split: each name describes one S10a acceptance criterion (byte-stability, null preservation, compose-with-no-position), so a future failure points at exactly which criterion regressed.

## Test 5 — property: every emitted `type` is in mdast node-set v1 (criteria #5 + #6, US33)

- Wrote: `internal/translate/lossiness_property_test.go` (new file, package `translate_test`, sibling to the existing `no_position_property_test.go`). Three load-bearing pieces:
  1. `mdastNodeSetV1` — `map[string]bool` listing all 25 v1 node types in the same order as CONTEXT.md's "mdast node-set v1" entry (14 block types + 11 inline types).
  2. `lossinessCorpus` — 30 hand-curated inputs, each named for the node type or extension it exercises (`thematic-break`, `task-list-mixed-checked`, `gfm-table`, `footnote-ref-and-def`, `link-reference-full/collapsed/shortcut`, `image-reference-full`, etc.), plus a `big-mixed` mega-input exercising every node type in one document and a `closed-fence-frontmatter` exercising the frontmatter-lift path.
  3. Three tests:
     a. `TestEveryEmittedTypeIsInMdastNodeSetV1` — per-input subtest: parse → translate → emit `--no-position`, re-parse the JSON, walk the tree, assert every observed `type` is in `mdastNodeSetV1`. On failure the diagnostic includes the fixture name, the AST path to the offending node, and the offending `type` string.
     b. `TestLossinessCorpusCoversEveryV1NodeType` — the orthogonal coverage guard: across the union of all 30 corpus inputs' emitted types, every member of `mdastNodeSetV1` MUST appear at least once. Without this guard, a regression in the corpus (someone deletes the only fixture exercising `footnoteDefinition`) would silently mask a real wire-contract regression in that type.
     c. `TestEveryEmittedTypeIsInMdastNodeSetV1DetectsOutOfSetTypes` — the negative-direction control: feeds a synthetic AST containing `type: "goldmarkAutoLink"` (a plausible leakage shape) into `walkTypes` via a recorder sink and asserts the walker reported the violation. Pinning the failure-direction means the property test cannot silently always-pass due to a bug in the walker itself.
  - The walker (`walkTypes`) is parameterized over an `errorReporter` interface so both `*testing.T` (production) and a private `recorderT` (the negative control) can drive it. The recorder pattern was the cleanest way to test the failure direction without forking a copy of `walkTypes` or relying on `t.Run` subtest-failure-flipping (which doesn't compose with the parent test's PASS/FAIL).
- Red: I wrote the test against the current GREEN state — translate already silent-drops everything not in v1 (CONTEXT.md "Lossiness policy" enforced by `translateNode`'s `default: return nil`), so the test passed on first run. To verify the test would catch a real regression, I performed an explicit negative control AFTER the GREEN state by commenting out `"paragraph": true,` in `mdastNodeSetV1`. Output (excerpted):
  ```
  --- FAIL: TestEveryEmittedTypeIsInMdastNodeSetV1/plain-paragraph
      fixture plain-paragraph, path ast/children[0]: emitted `type` "paragraph" is NOT a member of mdast node-set v1; either translate leaked a goldmark-native type, or the v1 enumeration needs to be extended
  --- FAIL: TestEveryEmittedTypeIsInMdastNodeSetV1/unordered-list
      fixture unordered-list, path ast/children[0]/children[0]/children[0]: emitted `type` "paragraph" is NOT...
  ```
  The diagnostic format gives fixture-name + AST-path + offending-type — exactly the three pieces of info S10a acceptance criterion #6 calls for ("identifies the offending type string AND the input fixture that produced it"). Restored the line; suite went green again.
- Green: no production changes needed — translate's silent-drop policy was already correct from S04 onward (`translateNode`'s `default: return nil`); the property test is purely a regression-guard.
- Notes:
  - The corpus deliberately exercises GFM extras (task list, table, strikethrough, autolink-angle, autolink-bare) AND every reference-style variant (link-reference-full/collapsed/shortcut, image-reference-full). Coverage of the v1 set is enforced by `TestLossinessCorpusCoversEveryV1NodeType`, so a future contributor adding a new fixture or removing an existing one will know immediately if coverage drops.
  - I considered making the walker recurse only on the `children` key rather than all map keys. Decided against: the slightly broader recursion catches hypothetical leaks via other keys (e.g. an `alt` field accidentally containing a node-shaped sub-object), and the false-positive cost is zero because no v1 mdast node ever has nested type-bearing structure outside `children`.
  - The `walkTypes` recursion sorts map keys before iterating so AST paths in diagnostics are deterministic — a Go map's iteration order is randomized per process, and a non-deterministic path would make a failing-test diff between runs unreproducible.
  - Local `itoa` helper instead of `strconv.Itoa`: matches the S10 tdd-log's stated convention of avoiding `strconv` pulls in non-emit translate code (the convention is documented in S10's Test 2 notes; the helper is six lines for path-index rendering only).

## Test 6 — repeat: verify all existing fixtures + tests survive

- Wrote: nothing new — this is the cross-cutting regression check.
- Red: skipped — `go test -count=1 ./...` after every GREEN step. Confirms no fixture or existing test regressed under the `--pretty` wiring change. Both `Pretty` defaulting to `false` and the no-op short-circuit (`if opts.Pretty { json.Indent(...) }`) mean every existing `emit.Options{NoPosition: ...}` call site behaves identically.
- Green: `go test -count=1 ./...` — 63 top-level + 138 subtests, all PASS. `go vet ./...` clean.

## Refactor pass

After all tests green:

1. **Considered factoring `stripJSONWhitespace` out into a shared test utility** (e.g. `internal/emit/testutil.go`). Decided to keep it local to `emit_test.go`: it has exactly one caller pair, and a "test utility" cross-package would invite reuse for cases that don't need the `"..."`-aware variant. The helper's ~25 lines are cheap enough to keep inline.
2. **Considered moving the property test's corpus into a separate file** to keep `lossiness_property_test.go` shorter. Decided against: a corpus in a sibling file would split the "what we test" from the "how we test", making the test harder to reason about. The 30 corpus entries plus the `coverage map` comment block are the readable spec of the slice.
3. **Considered exposing `mdastNodeSetV1` as a public constant in `internal/translate`**. Decided against: it's a test-only artifact and exporting it would invite production code to import the set as the source of truth, which would invert the dependency direction (translate.go's per-type switch IS the source of truth; the property test mirrors it). Keeping the set in `package translate_test` makes the mirror relationship visible.
4. **Considered using a `for typeName := range mdastNodeSetV1 { ... }` `t.Run` loop for the coverage test** instead of a single failure that lists missing types. Decided against: a single failure listing all missing types at once gives the corpus designer one diagnostic to act on; per-type subtests would explode the failure surface and make the "the corpus is missing N types" property harder to read.
5. **Did NOT extract `--pretty` rendering into a separate `pretty.go` file** within `internal/emit`. The `json.Indent` call is one line in `Emit`; a separate file would be over-engineering. If a future slice adds a per-type indented writer (e.g. for streaming output where post-processing isn't viable), THAT would justify a `pretty.go` split — but the current slice's strategy is post-process, and the strategy lives next to `Emit`.

## Manual end-to-end verification

```
$ go build -o /tmp/md2json2 .
$ printf '# Title\n\nBody with **bold**.\n' | /tmp/md2json2 --pretty --no-position | head -8
{
  "frontmatter": null,
  "ast": {
    "type": "root",
    "children": [
      {
        "type": "heading",
        "depth": 1,
$ printf '    abc\n    def\n' | /tmp/md2json2 --pretty --no-position | grep -E '"(lang|meta)":'
        "lang": null,
        "meta": null,
$ printf '- plain\n' | /tmp/md2json2 --pretty --no-position | grep '"checked"'
            "checked": null,
$ printf '[no-title](https://example.com)\n' | /tmp/md2json2 --pretty --no-position | grep '"title"'
            "title": null,
$ printf 'foo\n' | /tmp/md2json2 --pretty --no-position > /tmp/pretty.json
$ printf 'foo\n' | /tmp/md2json2 --no-position > /tmp/compact.json
$ tr -d ' \n\t' < /tmp/pretty.json > /tmp/pretty-stripped.json
$ diff /tmp/compact.json /tmp/pretty-stripped.json && echo MATCH
MATCH
```

The CLI invocation produces the expected pretty form; all four never-elide null cases (`lang`/`meta`/`checked`/`title`) survive pretty mode; the compact-vs-pretty byte-stability property holds end-to-end through the binary's stdout.

## Final

- Tests added in S10a:
  - **emit** (unit, `package emit_test`):
    - `TestPrettyAndCompactAreByteStableUpToWhitespace` — pins criterion #3 cross-input.
    - `TestNullFieldsPreservedInBothModes` — 8 subtests (checked / lang / meta / title × compact / pretty), pinning criterion #4.
    - `TestPrettyComposesWithNoPosition` — pins criterion #2 + round-trip well-formedness.
  - **translate** (cross-package, `package translate_test`):
    - `TestEveryEmittedTypeIsInMdastNodeSetV1` — 30 per-input subtests, pinning criteria #5 + #6 (US33).
    - `TestLossinessCorpusCoversEveryV1NodeType` — pins the corpus-coverage half of US33 (corpus exercises every member of the v1 set).
    - `TestEveryEmittedTypeIsInMdastNodeSetV1DetectsOutOfSetTypes` — negative control for the walker, ensures the property test can never silently always-pass.
  - **fixtures** (integration via `TestFixtures`):
    - `57-pretty-title-and-bold` — criterion #1 + #2 (`--pretty --no-position` on a representative document, key order pinned byte-exact).
    - `58-pretty-indented-code-null-lang-meta` — criterion #1 (null lang/meta preserved verbatim in pretty mode under `--no-position`).
    - `59-pretty-indented-code-with-position` — criterion #1's explicit `"position":{...}` form (pretty WITH position on indented code).
- Production code changes:
  - `internal/emit/emit.go`:
    - Added `Pretty bool` to `Options`.
    - In `Emit`, after building the compact byte stream, if `opts.Pretty` then post-process with `json.Indent` (2-space indent) and write the indented bytes.
  - `internal/cli/cli.go`:
    - Threaded `opts.pretty` through to both `emit.Options` shapes (FrontmatterOnly short-circuit + default-envelope path). Two-line change.
- `go test -count=1 ./...`: clean. 63 top-level + 138 subtests, 0 failures.
- `go vet ./...`: clean.
- No new module dependencies. `encoding/json` was already imported by emit (for `writeJSONValue` / `writeJSONString`); the `json.Indent` call adds no new import.

- Acceptance criteria status:
  - [x] criterion 1 — `--pretty` emits 2-space indented JSON with mdast-convention key order; indented code preserves `lang: null`/`meta: null` verbatim (`TestFixtures/57-pretty-title-and-bold` + `TestFixtures/58-pretty-indented-code-null-lang-meta` + `TestFixtures/59-pretty-indented-code-with-position`).
  - [x] criterion 2 — `--pretty --no-position` composes cleanly; same key order minus the position key (`TestFixtures/57` + `TestFixtures/58` + `TestPrettyComposesWithNoPosition` round-trip).
  - [x] criterion 3 — compact and pretty are byte-stable up to whitespace (`TestPrettyAndCompactAreByteStableUpToWhitespace`).
  - [x] criterion 4 — every emitted object preserves explicit `null` fields in both modes (`TestNullFieldsPreservedInBothModes` — 8 subtests covering `checked` / `lang` / `meta` / `title` × compact / pretty; plus `TestFixtures/58` byte-exact lock).
  - [x] criterion 5 — every emitted node's `type` is in the v1 enumeration across the corpus (`TestEveryEmittedTypeIsInMdastNodeSetV1` — 30 subtests; coverage guarded by `TestLossinessCorpusCoversEveryV1NodeType`).
  - [x] criterion 6 — the property test fails clearly, naming the offending `type` AND the input fixture (verified by the negative control of temporarily removing `"paragraph": true` from `mdastNodeSetV1` and observing the per-subtest diagnostic; the `TestEveryEmittedTypeIsInMdastNodeSetV1DetectsOutOfSetTypes` test pins the failure-direction).

VERDICT: accept
