# TDD log: 02-inline-math-happy-path

Started: 2026-05-23

## Approach

Strict TDD, tracer-bullet style. Default-path implementation (no `selection.md`
— no prior prototype Round for this issue). Issue acceptance has 5 bullets;
mapped to 4 black-box integration fixtures under `testdata/fixtures/` plus
the byte-exact `stdout` comparison enforced by the existing `TestFixtures`
harness in `integration_test.go`. Each fixture exercises one or more
acceptance bullets:

- **#62 inline-math-happy-path-nopos** — bullets 1 (shape), 3 (no-position
  half), 4 (serialized field set with no `meta`/`data`).
- **#63 inline-math-happy-path-default** — bullets 1 (with `position`
  field), 3 (default-mode-with-position half).
- **#64 inline-math-adjacent-text-siblings-nopos** — bullet 2 (PRD
  fixture #4 shape: `[text, inlineMath, text, inlineMath, text]`).
- **#65 inline-math-in-frontmatter-envelope-nopos** — bullet 5
  (in-frontmatter-envelope survival, PRD fixture #12).

S04 (currency post-pass), S03 (display math), S05 (unclosed-`$$`
compensation) are downstream issues and not in S02's scope. S02's
inputs are chosen so that the library's currency-rule gap is irrelevant
(all `$...$` matches are predicate-passing on the inputs used).

## Test 1: 62-inline-math-happy-path-nopos (tracer bullet)

- Wrote: `testdata/fixtures/62-inline-math-happy-path-nopos/{args,input.md,stdout,stderr,exit}`.
- Input: `$x = 5$\n` under `--no-position`.
- Expected stdout: `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"paragraph","children":[{"type":"inlineMath","value":"x = 5"}]}]}}`.
- Red: pre-implementation run produced `paragraph.children:[]` — the
  `*mathjax.InlineMath` node was silent-dropped by `translateNode`'s
  default arm (no case existed). Confirmed RED.
- Green: added one switch case `*mathjax.InlineMath` → `translateInlineMath`
  in `internal/translate/translate.go` + new `translateInlineMath` helper,
  plus `inlineMath` case in `internal/emit/emit.go` `writeNode` and the
  `isContainer` switch (leaf — no `children` key on the wire).
- Notes:
  - `value` extraction: concatenate the InlineMath's child `*ast.Text`
    segments' bytes verbatim. The library stores interior bytes as one
    `*ast.RawTextSegment` child whose Segment covers between-delimiter
    bytes (post any trim-halfspace).
  - Position derivation: walk leftward from first-child `Segment.Start`
    while `src[i-1] == '$'` to include the opener; walk rightward from
    last-child `Segment.Stop` while `src[i] == '$'` to include the
    closer. The library does not expose opener/closer width directly,
    but the bytes immediately bracketing the interior segment are
    guaranteed-`$`-run by construction of the inline parser
    (`probe/goldmark-mathjax/inline.go:24-52`).

## Test 2: 63-inline-math-happy-path-default

- Wrote: `testdata/fixtures/63-inline-math-happy-path-default/{args,input.md,stdout,stderr,exit}`.
- Input: `$x = 5$\n`, default flags.
- Expected stdout pins the `position` field on the `inlineMath`,
  `paragraph`, and `root` nodes byte-for-byte:
  - `inlineMath`: start `{line:1,column:1,offset:0}`, end `{line:1,column:8,offset:7}`.
  - `paragraph`: same span (the inlineMath is the only inline child, and
    `paragraphOffsets` derives from `lines[0].Start..lines[last].Stop`).
  - `root`: start `{1,1,0}`, end `{line:2,column:1,offset:8}` (end of file
    after the trailing `\n`).
- Red: pre-fixture (before file existed) — N/A; the fixture's RED is
  implicit in its byte-exact compare. Wrote the expected stdout, ran the
  test, it passed first try on the position-derivation math.
- Green: same code as Test 1 — position derivation walked `$` runs
  correctly on first attempt.
- Notes: PRD fixture #1's position derivation matched mine
  (start offset 0 includes opener, end offset 7 includes closer).

## Test 3: 64-inline-math-adjacent-text-siblings-nopos

- Wrote: `testdata/fixtures/64-inline-math-adjacent-text-siblings-nopos/{args,input.md,stdout,stderr,exit}`.
- Input: `Use $x$ and $y$.\n` under `--no-position`.
- Expected paragraph children: `[text "Use ", inlineMath "x", text " and ", inlineMath "y", text "."]`.
- Red/Green: passed first try with the Test 1 implementation. The
  existing contiguous-text coalescing in `translateChildren`
  (`internal/translate/translate.go:225-231`) does NOT coalesce across an
  `inlineMath` sibling — the coalesce check requires both `prev` and `n`
  to be `text`, which is false when `prev` is `inlineMath` or `n` is
  `inlineMath`. So the `text → inlineMath → text` interleaving falls out
  for free.
- Notes: covers PRD fixture #4 (the convergence case for the
  currency-rule post-pass — but S02 doesn't run a post-pass; the inputs
  here are simple enough that the library's match is the wire match).

## Test 4: 65-inline-math-in-frontmatter-envelope-nopos

- Wrote: `testdata/fixtures/65-inline-math-in-frontmatter-envelope-nopos/{args,input.md,stdout,stderr,exit}`.
- Input: a four-line document with closed YAML frontmatter
  (`title: t`) and one paragraph containing `$x = 5$`.
- Expected envelope: `{"frontmatter":{"title":"t"},"ast":{...inlineMath...}}`.
- Red/Green: passed first try. The frontmatter pre-scan + lift in
  `internal/parse/parse.go` is unchanged by S02 — math wiring lives one
  layer up from the YAML lift. PRD fixture #12 derivation.
- Notes: pins bullet #5 (frontmatter co-existence).

## Refactor pass

Implementation surface is small (one new case + one ~50-LoC helper in
`translate`, one new case + one isContainer entry in `emit`). No
duplication emerged with existing translate helpers — the
`children-walk-then-concat` shape is similar to `translateCodeSpan` but
the position derivation is delimiter-driven (walk `$` runs), not
child-span-driven, so extracting a helper would either lose information
or add a parameter that has only one caller. Left as-is.

Final inspection:

- `translate.go` imports added: `mathjax "github.com/litao91/goldmark-mathjax"`.
- `translateNode` switch arm added: `case *mathjax.InlineMath`.
- `translateInlineMath` function added (concatenates child Text segments
  into `value`; walks `$`-runs on both sides of the children's span for
  `position`).
- `emit.go` writeNode arm added: `case "inlineMath"` (writes `,"value":<str>`
  after the `type` field).
- `emit.go` `isContainer` updated: `inlineMath` listed alongside `text`,
  `inlineCode`, etc. (leaf — no `children` key on the wire).

## Library-quirk verification: trim-halfspace probe

Per the issue's note, ran an in-package probe (not pinned as a fixture
since S02's acceptance inputs do not trigger it):

- `$ x $\n` (space-padded) produces `inlineMath{value:"x"}`. The library's
  trim-halfspace pass at `probe/goldmark-mathjax/inline.go:62-82` shifts
  the child Segment by one byte on each side when BOTH ends are spaces.
  My implementation passes that trimmed value through verbatim — which
  agrees with the library's observable behavior but **diverges** from
  CONTEXT.md `inlineMath node`'s "no whitespace trim" clause for this
  specific double-space-padded input.
- `$x = 5$\n` produces `inlineMath{value:"x = 5"}` — no trim (first byte
  is non-space). Matches CONTEXT.md.
- `Use $x$ and $y$.\n` produces the expected 5-sibling shape.

This divergence is bounded and is **NOT** in S02's acceptance set. S04's
currency-rule post-pass and any future "value byte-restoration" pass
would be the place to address it. Pinning this as a fixture now would
either:
(a) accept the library quirk and freeze a fixture that contradicts the
glossary's "no whitespace trim" rule — undesirable; or
(b) write code in S02 to undo the trim (extend value by the original
delimiter-adjacent bytes the library removed), which expands S02's
scope beyond "happy path map" and pre-empts the S04 post-pass design.
Neither is appropriate for S02. Documented here so a future Run can pick
up the loose thread.

## Acceptance check

- [x] **#1** Input `$x = 5$` produces `paragraph.children=[inlineMath{value:"x = 5", position}]`, exit 0 — fixture 63 (with position) + fixture 62 (no-position) byte-exact green.
- [x] **#2** Input `Use $x$ and $y$.` produces `[text "Use ", inlineMath "x", text " and ", inlineMath "y", text "."]` — fixture 64 byte-exact green.
- [x] **#3** Same `$x = 5$` input under `--no-position` strips the `position` field; default keeps it — fixture 62 (stripped) vs fixture 63 (kept). Existing `TestEmitNoPositionStripsPositionKeyFromEveryNode` property test still green; its corpus does not include math, but the per-fixture diff catches any per-node-type regression.
- [x] **#4** `inlineMath` serializes as `{"type":"inlineMath","value":...,"position":...}` — no `meta`, no `data`, no extra fields. Enforced by the byte-exact fixture compare (any extra field would flip the byte stream).
- [x] **#5** `inlineMath` survives unchanged inside the JSON envelope when the input is wrapped in frontmatter; frontmatter object unaffected — fixture 65 byte-exact green.

## Final

- Tests added: 4 (all black-box integration fixtures under `testdata/fixtures/`).
- Tests passing: 4/4 of the new fixtures + every pre-existing test
  in the suite (`go test ./...` green across all six packages).
- Files touched:
  - `internal/translate/translate.go` — import `mathjax`; add
    `case *mathjax.InlineMath` to `translateNode`; add new
    `translateInlineMath` helper.
  - `internal/emit/emit.go` — add `case "inlineMath"` to `writeNode`;
    add `"inlineMath"` to `isContainer`'s leaf-list.
  - `testdata/fixtures/62-inline-math-happy-path-nopos/{args,input.md,stdout,stderr,exit}`.
  - `testdata/fixtures/63-inline-math-happy-path-default/{args,input.md,stdout,stderr,exit}`.
  - `testdata/fixtures/64-inline-math-adjacent-text-siblings-nopos/{args,input.md,stdout,stderr,exit}`.
  - `testdata/fixtures/65-inline-math-in-frontmatter-envelope-nopos/{args,input.md,stdout,stderr,exit}`.
- Commits: none in this Stage (per Rundays orchestrator-protocol).
- No ADRs added: the architectural decisions (library pick, name
  alignment, wiring style) are all already pinned by ADR-0004
  Decision 4. S02 is a straight execution of that ADR.
- VERDICT: accept.
