# TDD log: 03-real-empty-doc-parse-translate-emit

Started: 2026-05-23

## Setup

- Reused S01's test framework (Go `testing` + `go test`) and module layout.
- Slice goal: replace S01's hard-coded envelope with a real `read → parse → translate → emit` pipeline for the empty-document baseline, and add end-to-end BOM/CRLF fixtures (deferred from S02). Block/inline node translation is out of scope here — translate walks the goldmark `Document` node into mdast `root` only.
- New modules:
  - `internal/parse/parse.go` — wraps `github.com/yuin/goldmark` (GFM + frontmatter + footnote extensions registered) and returns the goldmark `*ast.Document` plus a frontmatter value (for the empty-doc case the latter is `nil`).
  - `internal/translate/translate.go` — walks a goldmark root node and produces a Go value tree of mdast nodes. At this slice it only translates `root` (children stay empty); later slices add the rest of the node set.
  - `internal/emit/emit.go` — JSON-encodes the envelope `{"frontmatter": ..., "ast": ...}` per CONTEXT.md key ordering (`type` first, then type-specific, then `children`, then `position`). Compact by default; drops `position` under `--no-position`. `--frontmatter-only` emits just the frontmatter value.
- Dep choices (documented for ADR-0002 below):
  - `github.com/yuin/goldmark` v1.8.2 — required by CONTEXT.md/PRD as the parser.
  - `github.com/yuin/goldmark/extension` — ships GFM (`extension.GFM`) and footnote (`extension.Footnote`) as standard extensions on the goldmark module itself. No third-party GFM lib needed.
  - `go.abhg.dev/goldmark/frontmatter` v0.3.0 — for the frontmatter extension. Configured with `Formats: []frontmatter.Format{frontmatter.YAML}` so TOML frontmatter (a v1 non-goal per PRD "Out of Scope") is **not** recognized. Pulled `github.com/BurntSushi/toml` and `gopkg.in/yaml.v3` as transitive deps; the BurntSushi/toml dep is link-time only — at runtime we never invoke the TOML format.
  - ADR-0002 captures the extension-library choice.

## Test 1 — tracer bullet: empty stdin default-mode emits envelope WITH `position` on root (criterion #2)

- Wrote: `TestEmptyStdinDefaultEmitsEnvelopeWithPosition` in `internal/cli/cli_test.go`. The expected stdout is the full `{...,"position":{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}}` envelope.
- Also re-pinned three S01 tests that previously asserted the no-position envelope on a default invocation: `TestEmptyStdinNoPositionEmitsEnvelope` (renamed from `TestEmptyStdinEmitsHardcodedEnvelope`, now passes `--no-position` explicitly), `TestPositionalFileEmitsEnvelope` (now adds `--no-position`), and `TestStdinSentinelDashBehavesLikeNoPositional` (now adds `--no-position`). Fixture `01-empty-stdin/args` updated to `--no-position` to match its expected stdout. The S01 tests' contracts (FILE accepted, `-` sentinel, no-position flavor) survive; only the envelope shape on default mode changed, and that's exactly what this slice owns.
- Red: `go test ./internal/cli` reported the expected stdout mismatch — S01's hard-coded `emptyEnvelope` had no `position` key, while the test wanted the zero-width position field. Confirmed before any GREEN code was written.
- Green: built `internal/parse/parse.go` (goldmark factory + Parse), `internal/translate/translate.go` (Node value tree + rootPosition handling the empty-source case), `internal/emit/emit.go` (compact envelope encoder with mdast key ordering), and wired `cli.Run` to call read → parse → translate → emit. Removed the hard-coded `emptyEnvelope` constant. `go test ./...` clean.
- Notes: the tracer was deliberately chosen so that the existing S01 hard-coded envelope path would **not** accidentally satisfy it — the missing `position` key on `root` was the diff that forced the real pipeline into existence.

## Test 2 — `--frontmatter-only` on empty doc emits `null` (criterion #3)

- Wrote: `TestFrontmatterOnlyEmptyDocEmitsNull` in `internal/cli/cli_test.go`.
- Red: skipped — by the time the GREEN step of Test 1 finished, the `cli.Run` short-circuit on `opts.frontmatterOnly` was already in place (we wrote it as part of the wiring step because `emit.Options.FrontmatterOnly` is part of the emit module's contract). I verified honestly by re-reading `cli.Run` and confirming the branch routes to `emit.Emit(stdout, nil, nil, {FrontmatterOnly:true})`, which serializes the JSON literal `null` for the nil frontmatter value. Same TDD purity stance as S01 Tests 4/6/7 / S02 Tests 2–11 — keep the test, acknowledge the lack of an explicit RED step. The test pins the contract against future refactors of `cli.Run` (e.g. accidentally routing `--frontmatter-only` through the translate stage anyway).
- Green: pre-existing.
- Notes: this is the v1 "scalar passthrough" rule for the null case; non-null scalars (`"hello"`, `42`, etc.) are pinned by S09 when actual YAML frontmatter parsing comes online.

## Test 3 — translate emits a Go value tree, not a goldmark node (criterion #4)

- Wrote: `TestTranslateEmptyDocProducesGoValueTreeNotGoldmarkNode` + `TestTranslateSingleNewlineRootPosition` in `internal/translate/translate_test.go`.
- Red: skipped — the GREEN step of Test 1 had already built `internal/translate/translate.go` with the `*Node` value tree as the return type. These tests pin the structural contract: the return type is a `*translate.Node` (plain Go struct) that does NOT satisfy `goldmark/ast.Node`. The reflection check would flag any future refactor that accidentally returned a goldmark wrapper.
- Green: pre-existing.
- Notes: the second test (`TestTranslateSingleNewlineRootPosition`) pins the user-story-27 boundary case (single `\n` byte → `root.position.end = {2,1,1}`, `children = []`). Not exercised by S03's CLI fixtures yet but the math is in the codebase, locked by a unit test.

## Test 4 — no module reaches into process globals (criterion #5)

- Wrote: `anti_globals_test.go` at the module root, with `TestNoProcessGlobalsOutsideMain`. It walks every `.go` file under the module root (excluding `main.go`, test files, `testdata/`, and hidden directories like `.rundays`), parses each via `go/parser`, and asserts no `os.Args`/`os.Stdin`/`os.Stdout`/`os.Stderr`/`os.Exit` `SelectorExpr` appears.
- Red: first attempt used a substring scan, which false-positived on the doc-comment in `cli.go` ("it never calls os.Exit"). I treated that as a legitimate RED — the test was too coarse.
- Green: refactored the test to parse the Go syntax with `go/parser` and inspect `*ast.SelectorExpr` nodes. Comments and string literals are stripped at parse time, so doc-strings mentioning `os.Exit` don't trigger. Test now passes on the current codebase.
- Notes: this is the structural acceptance test for "bytes flow as typed values; no globals reached." Catches the obvious failure mode (a deep module sneaking in an `os.Exit(1)` instead of returning an error). Test files and `main.go` are deliberately exempt — tests use `os.MkdirTemp` etc. legitimately, and `main.go` is the one allowed injection point per the PRD.

## Test 5 — BOM-only stdin end-to-end byte-identical to empty-doc (criterion #6)

- Wrote: two fixtures, `04-bom-only-stdin-nopos/` and `05-bom-only-stdin-default/`. Each has `input.md` containing exactly the 3-byte UTF-8 BOM, `args` matching the empty-doc fixtures (`--no-position` and default respectively), and `stdout` byte-identical to fixtures `01-empty-stdin/stdout` and `10-empty-stdin-default/stdout`. The harness's byte-exact comparison enforces the equivalence.
- Red: skipped — the read module already strips the BOM (S02 pinned this with `TestLeadingBOMIsStripped`), and after BOM strip the pipeline sees zero bytes (the empty-doc case). I verified by hand with `printf '\xEF\xBB\xBF' | /tmp/md2json2` (default and `--no-position`) before writing the fixture; both already produced the empty-doc envelope. Test pins the end-to-end contract through the black-box harness (S02 deferred this to S03 explicitly).
- Green: pre-existing.
- Notes: this is the slice that makes the BOM-strip behavior observable end-to-end for the first time. S02's tests pinned the byte-level transform in `read`; S03's fixtures pin the full pipeline's behavior.

## Test 6 — CRLF-equivalent stdin end-to-end byte-identical to LF (criterion #7)

- Wrote: four fixtures: `06-crlf-only-stdin-nopos/`, `07-crlf-only-stdin-default/` (input is `\r\n`), `08-lf-only-stdin-nopos/`, `09-lf-only-stdin-default/` (input is `\n`). Each `--no-position` flavor's stdout is byte-identical to its CRLF/LF counterpart; same for the default flavor. The default-mode stdout encodes the single-newline boundary case (`end:{line:2,column:1,offset:1}`), exercising the rootPosition math from `translate.endPosition`.
- Red: skipped — CRLF→LF normalization is pinned at the byte level by S02's `TestCRLFNormalizedToLF`. End-to-end equivalence falls out of the pipeline because every downstream stage sees the normalized bytes only. I verified by hand with `printf '\r\n' | /tmp/md2json2` vs `printf '\n' | /tmp/md2json2`; both produced byte-identical envelopes in both default and `--no-position` modes. Fixtures pin the contract.
- Green: pre-existing.
- Notes: this is the first slice where the single-newline boundary is observable on the wire (the position rule's first non-trivial extrapolation from the zero-byte baseline). Six fixtures total (BOM × 2 modes + CRLF × 2 modes + LF × 2 modes) make the equivalence relation explicit: BOM-only ≡ empty-doc; CRLF-only ≡ LF-only; both modes independent.

## Refactor pass

After all tests green:

1. **Removed the hard-coded `emptyEnvelope` const from `cli.go`.** It was the S01 tracer-bullet artifact; with the real pipeline in place it has no consumer. `go vet ./...` clean afterward.
2. **Verified `parse.go` is minimal.** Initial draft had a dead `_ = bytes.Equal` line I left as a "reserved for later slices" marker; removed it because reserving for the future violates the "minimal code for this test" rule. If S09 needs `bytes.Equal`, it can add the import then.
3. **Sharpened the anti-globals test from substring scan to `go/parser` AST walk.** This was the most load-bearing refactor — the substring version was a leaky abstraction; the AST version is precise and false-positive-free. Tests stayed green.
4. **Did NOT refactor `translate.rootPosition` further** even though the function is split into `rootPosition` + `endPosition`. The split anticipates S10's broader position-info work, but the current shape is the minimum needed to satisfy the empty-doc and single-newline tests without speculative depth.

## Manual end-to-end verification

```
$ go build -o /tmp/md2json2 .
$ /tmp/md2json2 --no-position < /dev/null | xxd
00000000: 7b22 6672 6f6e 746d 6174 7465 7222 3a6e  {"frontmatter":n
00000010: 756c 6c2c 2261 7374 223a 7b22 7479 7065  ull,"ast":{"type
00000020: 223a 2272 6f6f 7422 2c22 6368 696c 6472  ":"root","childr
00000030: 656e 223a 5b5d 7d7d                      en":[]}}
$ /tmp/md2json2 < /dev/null
{"frontmatter":null,"ast":{"type":"root","children":[],"position":{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}}}
$ printf '\xEF\xBB\xBF' | /tmp/md2json2 --no-position
{"frontmatter":null,"ast":{"type":"root","children":[]}}
$ printf '\r\n' | /tmp/md2json2
{"frontmatter":null,"ast":{"type":"root","children":[],"position":{"start":{"line":1,"column":1,"offset":0},"end":{"line":2,"column":1,"offset":1}}}}
$ /tmp/md2json2 --frontmatter-only < /dev/null
null
```

All match the acceptance criteria byte-for-byte.

## ADR-0002

Captured the goldmark extension-library choice (GFM + Footnote from `goldmark/extension`, frontmatter from `go.abhg.dev/goldmark/frontmatter` configured for YAML-only) plus the `Mode: 0` decision and the transitive-TOML-dep tradeoff in `<product_dir>/docs/adr/0002-goldmark-extension-libraries.md`.

## Final

- Tests added in S03:
  - cli (in-process): `TestEmptyStdinDefaultEmitsEnvelopeWithPosition`, `TestFrontmatterOnlyEmptyDocEmitsNull` (and 3 S01 tests re-pinned to use `--no-position`: `TestEmptyStdinNoPositionEmitsEnvelope` renamed from `TestEmptyStdinEmitsHardcodedEnvelope`, `TestPositionalFileEmitsEnvelope`, `TestStdinSentinelDashBehavesLikeNoPositional`).
  - translate (unit): `TestTranslateEmptyDocProducesGoValueTreeNotGoldmarkNode`, `TestTranslateSingleNewlineRootPosition`.
  - module-root structural: `TestNoProcessGlobalsOutsideMain`.
  - fixtures (integration via `TestFixtures`): `04-bom-only-stdin-nopos`, `05-bom-only-stdin-default`, `06-crlf-only-stdin-nopos`, `07-crlf-only-stdin-default`, `08-lf-only-stdin-nopos`, `09-lf-only-stdin-default`, `10-empty-stdin-default`, `11-empty-doc-frontmatter-only` (8 new fixtures).
- `go test ./...`: clean (all green; 1 fixture-harness `TestMain` build, 11 fixture subtests, plus all unit/in-process tests across cli/read/translate).
- `go vet ./...`: clean.
- `go mod tidy`: ran; go.mod has `github.com/yuin/goldmark v1.8.2` and `go.abhg.dev/goldmark/frontmatter v0.3.0` as direct deps, with `github.com/BurntSushi/toml v1.5.0` and `gopkg.in/yaml.v3 v3.0.1` as transitive deps.

- Acceptance criteria status:
  - [x] criterion 1 — `md2json2 --no-position < empty.md` exact envelope, exit 0 (`TestEmptyStdinNoPositionEmitsEnvelope`, fixture `01-empty-stdin`)
  - [x] criterion 2 — `md2json2 < empty.md` exact envelope with `position` on root, exit 0 (`TestEmptyStdinDefaultEmitsEnvelopeWithPosition`, fixture `10-empty-stdin-default`)
  - [x] criterion 3 — goldmark configured with GFM + frontmatter + footnote extensions (verifiable in `internal/parse/parse.go::New`); `--frontmatter-only < empty.md` emits `null` exit 0 (`TestFrontmatterOnlyEmptyDocEmitsNull`, fixture `11-empty-doc-frontmatter-only`)
  - [x] criterion 4 — translate output is a Go value tree (not goldmark node), with type:"root" ready for emit (`TestTranslateEmptyDocProducesGoValueTreeNotGoldmarkNode`)
  - [x] criterion 5 — bytes flow read → parse → translate → emit; no module reaches into process globals (`TestNoProcessGlobalsOutsideMain` walks every non-test Go file outside `main.go` and asserts no `os.Args`/`os.Stdin`/`os.Stdout`/`os.Stderr`/`os.Exit` references)
  - [x] criterion 6 — BOM-only file end-to-end byte-identical to empty (`TestFixtures/04-bom-only-stdin-nopos` vs `01-empty-stdin`; `TestFixtures/05-bom-only-stdin-default` vs `10-empty-stdin-default`)
  - [x] criterion 7 — CRLF-only file end-to-end byte-identical to LF-only (`TestFixtures/06-crlf-only-stdin-nopos` vs `08-lf-only-stdin-nopos`; `TestFixtures/07-crlf-only-stdin-default` vs `09-lf-only-stdin-default`)

VERDICT: accept

