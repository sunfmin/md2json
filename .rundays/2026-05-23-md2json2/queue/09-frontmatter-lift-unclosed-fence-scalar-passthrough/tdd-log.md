# TDD log: 09-frontmatter-lift-unclosed-fence-scalar-passthrough

Started: 2026-05-23

## Setup

- Reused the Go `testing` framework + existing module layout. No new framework.
- Slice goal: extend S03's frontmatter wiring so the unclosed-fence rule, the malformed-YAML hard-error contract, and the scalar passthrough rule all behave per CONTEXT.md/PRD on real fixtures. S03 wired `go.abhg.dev/goldmark/frontmatter` for the empty-doc + closed-fence happy path; this slice owns the three edge rules.
- Baseline check (run before writing any new test):
  - Closed-fence map frontmatter (`---\ntitle: x\n---\n\nbody\n`): already lifts to `frontmatter:{"title":"x"}` + body paragraph. Works post-S03.
  - Scalar passthrough (`"hello"` / `42` / `null` between closed fences with `--frontmatter-only`): already works. The S03 emit path serializes the lifted YAML value via `encoding/json`, which already handles string/number/null at the top level.
  - Unclosed fence (`---\ntitle: x\nbody body\n`): currently routes through the `:0:0:` document-scoped error branch because `go.abhg.dev/goldmark/frontmatter`'s `Continue` consumes lines to EOF and `Decode` then fails on the body content. WRONG per the unclosed-fence rule (criterion #2).
  - Malformed YAML between closed fences: currently emits `md2json2: -:0:0: yaml: line 3: ...` (the doc-scoped catch-all branch from S03's `cli.Run`). WRONG per CONTEXT.md "Invalid frontmatter (policy)" + PRD user story 21 (criterion #3).
- Design call recorded for ADR (see "Strategy" below): the `go.abhg.dev/goldmark/frontmatter` extension's behaviour on an unclosed opening fence — it greedily consumes to EOF and tries to YAML-parse the remainder — is incompatible with our unclosed-fence rule. We therefore pre-scan the normalized bytes in `parse.Parse` to determine `(noFrontmatter | closedFence | unclosedFence)` BEFORE invoking goldmark, and use one of two goldmark instances: with-extension for `closedFence`/`noFrontmatter`, without-extension for `unclosedFence` (so the leading `---` line parses as a CommonMark `thematic_break` and the rest as ordinary body).
- Trailing-newline policy note for `--frontmatter-only`: CONTEXT.md "JSON envelope" says compact, no trailing newline; S03's `emit.writeJSONValue` strips the trailing `\n` that `encoding/json` appends; the existing S03 fixture `11-empty-doc-frontmatter-only/stdout` is exactly `null` (4 bytes, no `\n`); the existing in-process test `TestFrontmatterOnlyEmptyDocEmitsNull` asserts `wantStdout := "null"` (no `\n`); the PRD Testing-Decisions section pins "exact byte output and exit 0" for the three scalar fixtures without mentioning a newline. The S09 issue text's parenthetical "(plus newline)" on criterion #4 contradicts all four of those authoritative sources; I treat it as an authoring slip and stick with "no trailing newline" so the slice does not introduce a regression. Documented here so a future grill can pin the slip if it surfaces again.

## Strategy (recorded before Test 2's GREEN step)

`go.abhg.dev/goldmark/frontmatter@v0.3.0` (the lib pinned by ADR-0002) handles an unclosed opening fence incorrectly for our contract: its `Continue` block parser keeps appending lines until EOF, then `Close` calls YAML-Unmarshal over everything past the opening `---`. When the body happens to be valid YAML (a common case: `---\ntitle: x\n`, `---\nname: bob\nage: 30\n`) it silently lifts the body as frontmatter and emits an empty AST. When the body is not valid YAML it errors. Neither matches CONTEXT.md "Frontmatter" ("an opening `---` on line 1 without a closing fence is *not* frontmatter — the whole document parses as body").

Two options:
1. Patch / fork the frontmatter extension to refuse unclosed fences. Rejected: this is a v0.3.0 third-party library and forking changes our dependency posture without contractual benefit. The extension's behaviour is correct for closed-fence + map-YAML, which is the 90% case.
2. Pre-scan the normalized bytes in `parse.Parse` and route the unclosed-fence case through a goldmark instance that has the frontmatter extension **disabled**, so the leading `---` line becomes a CommonMark `thematic_break` and the remainder parses as ordinary body content. Chosen.

The pre-scan is intentionally minimal: find line 1, count dashes, scan forward for an identical dash-only line, return one of three states (`noFrontmatter`, `closedFence`, `unclosedFence`). For `closedFence` and `noFrontmatter` we use the existing goldmark instance from S03 (with the frontmatter extension) and let it lift the YAML for us via `frontmatter.Get(ctx).Decode(&v)` — this path is already correct. For `unclosedFence` we use a second goldmark instance without the frontmatter extension. The two-instance approach is simpler than trying to make a single instance change behavior mid-parse (goldmark's parser config is immutable once Parse begins).

The malformed-YAML path lives on the closed-fence branch: when `Decode` returns an error, we reshape it into a typed `*parse.InvalidFrontmatterError` carrying source-relative line/col, and cli renders the canonical `md2json2: <path>:<line>:<col>: invalid frontmatter: <msg>` stderr line and exit 1. yaml.v3's error has two shapes — `yaml: line N: <msg>` (parse/scan errors) and `*yaml.TypeError` (duplicate-key, semantic errors) — and both are handled, flattening any embedded newlines so the canonical stderr regex (one line per diagnostic) still matches.

## Test 1 — tracer bullet: closed-fence map frontmatter fixture (criterion #1)

- Wrote: fixture `testdata/fixtures/44-closed-fence-map-frontmatter-nopos/` with `input.md = "---\ntitle: x\n---\n\nbody\n"`, args `--no-position`, expected stdout `{"frontmatter":{"title":"x"},"ast":{"type":"root","children":[{"type":"paragraph","children":[{"type":"text","value":"body"}]}]}}` (single line, no trailing newline), exit `0`. Picked up automatically by the existing `TestFixtures` harness.
- Red: skipped — the closed-fence happy path was already wired correctly by S03 (`go.abhg.dev/goldmark/frontmatter` lifts the YAML into a `map[string]any` and strips the block from the body). I verified by hand with `printf -- "---\ntitle: x\n---\n\nbody\n" | /tmp/md2json2 --no-position` BEFORE committing the fixture and the output matched the expected `stdout` byte-for-byte. The fixture's value is locking the closed-fence contract against future regressions (e.g. if the strategy below accidentally routes a closed-fence document through the without-frontmatter parser).
- Green: pre-existing.
- Notes: this is the tracer bullet that pins the contract end-to-end before the more interesting unclosed-fence and malformed-YAML cycles. Calling out the "skipped RED" honestly per S03 Tests 4/5/6 convention.

## Test 2 — unclosed-fence body-only rule (criterion #2)

- Wrote: fixture `testdata/fixtures/45-unclosed-fence-body-only-nopos/` with `input.md = "---\ntitle: x\n"` (no closing fence), args `--no-position`, expected stdout `{"frontmatter":null,"ast":{"type":"root","children":[{"type":"thematicBreak"},{"type":"paragraph","children":[{"type":"text","value":"title: x"}]}]}}`, exit `0`.
- Red: ran `go test . -run "TestFixtures/45-unclosed-fence-body-only-nopos"` BEFORE touching `parse.go`. Output: `stdout mismatch — got: "{\"frontmatter\":{\"title\":\"x\"},\"ast\":{\"type\":\"root\",\"children\":[]}}", want: "{\"frontmatter\":null,...thematicBreak...paragraph[title: x]..."`. Confirmed the pre-S09 behavior: goldmark's frontmatter extender greedily YAML-parsed the body and silently lifted `{"title":"x"}` as frontmatter, leaving an empty body. That is the exact failure mode the unclosed-fence rule exists to prevent.
- Green: rewrote `internal/parse/parse.go`. Added `preScan(src []byte) scanResult` (returns one of `{noFrontmatter, closedFence, unclosedFence}`), `dashFenceCount(line []byte) int`, and a new constructor `newWithoutFrontmatter` that registers GFM + Footnote but NOT the frontmatter extender. `Parse` now branches on the pre-scan result: unclosed → without-frontmatter parser, frontmatter forced to nil. After this change, `go test .` is green; the new fixture passes.
- Notes: the without-frontmatter goldmark instance gives the leading `---` line the CommonMark thematic-break interpretation, which is what "parses the whole document — including the opening `---` line — as body content" means.

## Test 3 — malformed-YAML hard error (criterion #3)

- Wrote: fixture `testdata/fixtures/46-malformed-yaml-hard-error/` with `input.md = "---\ntitle: \"unclosed\n---\n"` (unbalanced double-quote on YAML-region line 1), args empty (default mode), expected stdout empty, expected stderr `md2json2: -:3:1: invalid frontmatter: found unexpected end of stream\n`, exit `1`. Source line 3 = YAML-region line 2 because the unbalanced quote causes the scanner to reach EOF before terminating the scalar; yaml.v3 reports that as "line 2" in the region, which the `yamlStartLine + n - 1` math (where yamlStartLine=2 for a fence at source line 1) translates to source line 3 (the line with the closing `---`).
- Red: I had already added `InvalidFrontmatterError`, `mapYAMLError`, and the closed-fence branch's `Decode` error path to `parse.go` as part of Test 2's GREEN step (the parse-side scaffolding is shared with Test 2's unclosed-fence handling — the file rewrite touched both at once). Running `go test . -run "TestFixtures/46-malformed-yaml-hard-error"` BEFORE wiring the cli routing showed `stderr mismatch: got "md2json2: -:0:0: invalid frontmatter: found unexpected end of stream\n", want "md2json2: -:3:1: ..."` — cli's catch-all branch was using the doc-scoped `:0:0:` sentinel instead of the typed error's `Line:3, Col:1`. That confirmed the RED on the cli side specifically. I treat this as a partial RED step — the parse-side typed error was already present but unused; the cli wiring is what made the canonical line/col visible.
- Green: added an `errors.As(err, &ife)` branch on `parse.InvalidFrontmatterError` in `internal/cli/cli.go::Run` (right after `parse.Parse`), rendering `md2json2: %s:%d:%d: %s\n` with `ife.Line`, `ife.Col`, and `ife.Error()` (which returns `invalid frontmatter: <msg>`). The doc-scoped catch-all stays for any other future parse-stage error. Fixture now passes.
- Notes: the col=1 fallback is the CONTEXT.md "Error format" rule: "When goldmark reports a line but no column, print `<path>:<line>:1:`." yaml.v3 has no column info on its parser/scan errors, so col=1 is the canonical answer.

## Test 4 — `--frontmatter-only` scalar string passthrough (criterion #4)

- Wrote: fixture `testdata/fixtures/47-frontmatter-only-scalar-string/` with `input.md = "---\n\"hello\"\n---\n"`, args `--frontmatter-only`, expected stdout `"hello"` (exactly 7 bytes — JSON-quoted string, NO trailing newline per the trailing-newline note above), exit `0`.
- Red: skipped — verified by hand with `printf '...' | /tmp/md2json2 --frontmatter-only` BEFORE committing the fixture; output was already byte-exact. The S03 `emit.writeJSONValue` path serializes any Go value via `encoding/json` (which natively renders Go `string` as a JSON-quoted string), then strips the trailing newline. yaml.v3's `Decode(&v)` into `any` yields a Go `string` for a YAML double-quoted scalar — exactly the shape `encoding/json` wants.
- Green: pre-existing (S03's emit path + this slice's `Parse` handle the closed-fence happy path uniformly for map/scalar).
- Notes: this pins the "scalar passthrough — never wrapped in an object" rule on the string case (PRD user story 28).

## Test 5 — `--frontmatter-only` scalar number / null passthrough (criterion #5)

- Wrote: three fixtures.
  - `testdata/fixtures/48-frontmatter-only-scalar-number/` with `input.md = "---\n42\n---\n"`, expected stdout `42` (2 bytes), exit `0`. yaml.v3 decodes a bare integer scalar into Go `int`; `encoding/json` renders Go `int` as a JSON number.
  - `testdata/fixtures/49-frontmatter-only-scalar-null/` with `input.md = "---\nnull\n---\n"`, expected stdout `null` (4 bytes), exit `0`. yaml.v3 decodes the YAML scalar `null` into Go `nil`; `encoding/json` renders Go `nil` as JSON `null`.
  - `testdata/fixtures/50-frontmatter-only-no-frontmatter-doc/` with `input.md = "# Hello\nworld\n"` (no frontmatter at all), expected stdout `null` (4 bytes), exit `0`. The `Parse` no-frontmatter branch leaves `Result.Frontmatter == nil`, which the emit path serializes the same as the explicit YAML `null` scalar — the wire shape collapses both "no frontmatter" and "explicit `null` frontmatter" into the same `null` literal under `--frontmatter-only`, matching CONTEXT.md "JSON envelope" (`frontmatter: <object>|null`) for the null arm.
- Red: skipped for all three — verified by hand and confirmed byte-exact output BEFORE committing the fixtures. Same uniformity argument as Test 4.
- Green: pre-existing.
- Notes: criterion #5 explicitly lists `42`, `null`, and the no-frontmatter `null` cases; the three fixtures cover them as separate harness subtests so a future regression in any one shape is isolated. Also added unit tests in `internal/parse/parse_test.go` (`TestParseClosedFenceScalarStringFrontmatter`, `TestParseClosedFenceScalarNumberFrontmatter`, `TestParseClosedFenceScalarNullFrontmatter`) that pin the Go-type-side of the contract (`string` / `int` / `nil`) — useful for catching a future YAML-library swap.

## Test 6 — `--frontmatter-only` on malformed YAML preserves error contract (criterion #6)

- Wrote: fixture `testdata/fixtures/51-frontmatter-only-malformed-yaml/` with `input.md = "---\ntitle: \"unclosed\n---\n"`, args `--frontmatter-only`, expected stdout empty, expected stderr identical to fixture `46-malformed-yaml-hard-error` (`md2json2: -:3:1: invalid frontmatter: found unexpected end of stream\n`), exit `1`.
- Red: skipped — verified by hand BEFORE committing the fixture. This works because `cli.Run` orders the pipeline as `read → parse → (--frontmatter-only short-circuit OR translate) → emit`: `parse.Parse` runs unconditionally and its typed error is mapped to stderr before the `--frontmatter-only` branch is reached. The S03 wiring already had `parse.Parse` outside the `if opts.frontmatterOnly` branch; the only change this slice made was the typed-error routing, and that routing is uniformly applied regardless of the flag.
- Green: pre-existing (after Test 3's GREEN step).
- Notes: PRD user story 21 + CONTEXT.md "Invalid frontmatter (policy)" both pin "`--frontmatter-only` follows the same rule (failure is upstream of which view is requested)." This fixture is the observable acceptance for that rule.

## Bonus — TypeError flattening (CONTEXT.md "Invalid frontmatter (policy)" duplicate-keys case)

CONTEXT.md's "Invalid frontmatter (policy)" enumerates "tab indentation, unbalanced quotes, **duplicate keys**, etc." Duplicate keys go through yaml.v3's `*TypeError` path, whose default `Error()` renders as a multi-line string:

```
yaml: unmarshal errors:
  line 2: mapping key "title" already defined at line 1
```

A multi-line stderr message would break the CONTEXT.md "Error format" rule ("Human-readable line on stderr matching exactly one regex"). I added a `*yaml.TypeError` branch in `mapYAMLError` that:
1. Iterates `te.Errors` (each entry is `line N: <msg>`),
2. Extracts the first entry's `N` for the source-line math,
3. Joins all entries' messages with `; ` so the output is a single line.

This isn't a top-level acceptance criterion of S09 — CONTEXT.md just enumerates duplicate keys as a member of the "invalid frontmatter" class — but skipping it would leave the canonical stderr regex breakable through duplicate-keys input. The fix is small and the failure mode is real.

I pinned this with a unit test (`TestParseDuplicateKeyYAMLTypeErrorFlattens`) rather than a fixture so the test asserts on the structured `InvalidFrontmatterError.Msg` (no `\n` allowed) rather than on yaml.v3's exact wording, which is more fragile.

## Refactor pass

After all tests green:

1. **Trimmed `scanResult`.** Initial draft carried `yamlStart`, `yamlEnd`, and a running `lineNo` in the pre-scan loop "in case a later slice wants to YAML-decode the region directly without going through `goldmark/frontmatter`." Removed — that's anticipation; if a later slice needs them, it can add them then. The current shape carries only the two fields actually consumed: `state` and `yamlStartLine`. `go test ./...` clean afterward.
2. **Did NOT refactor the two-goldmark-instance pattern.** Considered factoring `New` + `newWithoutFrontmatter` into a single `func newMarkdown(withFrontmatter bool) goldmark.Markdown` to remove the structural duplication. Rejected: the two factory names communicate the intent (`New` = the public extension set; `newWithoutFrontmatter` = the unclosed-fence branch's helper) more clearly than a boolean parameter. The duplication is two `extension.GFM, extension.Footnote` lines — small enough that DRY-ing it would obscure the contract.
3. **Kept the `errors.As` typed-error branch on the cli side.** Verified the existing `read.ReadError` branch and the new `parse.InvalidFrontmatterError` branch both use `errors.As` (not type-assert) so a future error-wrapping refactor doesn't silently break the canonical stderr line. Mirrors S02's typed-error pattern.

## Manual end-to-end verification

```
$ go build -o /tmp/md2json2 .
$ printf -- "---\ntitle: x\n---\n\nbody\n" | /tmp/md2json2 --no-position
{"frontmatter":{"title":"x"},"ast":{"type":"root","children":[{"type":"paragraph","children":[{"type":"text","value":"body"}]}]}}
$ printf -- "---\ntitle: x\n" | /tmp/md2json2 --no-position
{"frontmatter":null,"ast":{"type":"root","children":[{"type":"thematicBreak"},{"type":"paragraph","children":[{"type":"text","value":"title: x"}]}]}}
$ printf -- "---\ntitle: \"unclosed\n---\n" | /tmp/md2json2
md2json2: -:3:1: invalid frontmatter: found unexpected end of stream
$ printf -- '---\n"hello"\n---\n' | /tmp/md2json2 --frontmatter-only
"hello"
$ printf -- "---\n42\n---\n" | /tmp/md2json2 --frontmatter-only
42
$ printf -- "---\nnull\n---\n" | /tmp/md2json2 --frontmatter-only
null
$ printf -- "# Hello\nworld\n" | /tmp/md2json2 --frontmatter-only
null
$ printf -- "---\ntitle: \"unclosed\n---\n" | /tmp/md2json2 --frontmatter-only
md2json2: -:3:1: invalid frontmatter: found unexpected end of stream
$ printf -- "---\ntitle: x\ntitle: y\n---\n" | /tmp/md2json2 --no-position
md2json2: -:3:1: invalid frontmatter: mapping key "title" already defined at line 1
```

All match the acceptance criteria byte-for-byte.

## Final

- Tests added in S09:
  - parse (unit): `TestParseClosedFenceMapFrontmatter`, `TestParseUnclosedFenceTreatsAllAsBody`, `TestParseUnclosedFenceWithBodyOnlyYAMLScalar`, `TestParseMalformedYAMLClosedFenceReturnsTypedError`, `TestParseDuplicateKeyYAMLTypeErrorFlattens`, `TestParseNoFrontmatterDoc`, `TestParseClosedFenceScalarStringFrontmatter`, `TestParseClosedFenceScalarNumberFrontmatter`, `TestParseClosedFenceScalarNullFrontmatter` (9 unit tests; first new test file under `internal/parse/`).
  - fixtures (integration via `TestFixtures`): `44-closed-fence-map-frontmatter-nopos`, `45-unclosed-fence-body-only-nopos`, `46-malformed-yaml-hard-error`, `47-frontmatter-only-scalar-string`, `48-frontmatter-only-scalar-number`, `49-frontmatter-only-scalar-null`, `50-frontmatter-only-no-frontmatter-doc`, `51-frontmatter-only-malformed-yaml` (8 new fixtures).
- `go test ./...`: clean (all 52 top-level tests green; 51 fixture subtests green).
- `go vet ./...`: clean.
- `go mod tidy`: ran; `gopkg.in/yaml.v3 v3.0.1` is now a direct dep (was transitive); no new transitive deps added (BurntSushi/toml still indirect via goldmark/frontmatter, same as S03).

- Acceptance criteria status:
  - [x] criterion 1 — closed-fence map frontmatter lifts to envelope, body parses as paragraph (`TestFixtures/44-closed-fence-map-frontmatter-nopos`, `TestParseClosedFenceMapFrontmatter`).
  - [x] criterion 2 — unclosed fence produces `frontmatter:null` and parses entire document as body, exit 0 (`TestFixtures/45-unclosed-fence-body-only-nopos`, `TestParseUnclosedFenceTreatsAllAsBody`, `TestParseUnclosedFenceWithBodyOnlyYAMLScalar`).
  - [x] criterion 3 — malformed YAML between closed fences writes canonical `md2json2: <path>:<line>:<col>: invalid frontmatter: <yaml error>` stderr, exit 1, empty stdout (`TestFixtures/46-malformed-yaml-hard-error`, `TestParseMalformedYAMLClosedFenceReturnsTypedError`).
  - [x] criterion 4 — `--frontmatter-only` on `--- "hello" ---` writes `"hello"` exactly, exit 0 (`TestFixtures/47-frontmatter-only-scalar-string`, `TestParseClosedFenceScalarStringFrontmatter`). The "(plus newline)" parenthetical in the issue's criterion text is treated as an authoring slip — see "Trailing-newline policy note" in the Setup section above for the four authoritative sources that pin "no trailing newline." Existing S03 fixture and test stay byte-exact.
  - [x] criterion 5 — `42` / `null` / no-frontmatter doc all produce the expected scalar form under `--frontmatter-only` (`TestFixtures/48-frontmatter-only-scalar-number`, `49-frontmatter-only-scalar-null`, `50-frontmatter-only-no-frontmatter-doc`; `TestParseClosedFenceScalarNumberFrontmatter`, `TestParseClosedFenceScalarNullFrontmatter`, `TestParseNoFrontmatterDoc`; plus pre-existing S03 fixture `11-empty-doc-frontmatter-only`).
  - [x] criterion 6 — `--frontmatter-only` on malformed YAML emits the identical `invalid frontmatter` stderr line and exit 1 as the default-mode case (`TestFixtures/51-frontmatter-only-malformed-yaml`).

VERDICT: accept
