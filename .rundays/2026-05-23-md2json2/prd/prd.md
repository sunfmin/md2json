# PRD: md2json2 — a small CLI that converts Markdown to JSON

Status: ready-for-agent
Created: 2026-05-23

## Problem

A writer or tool author has a GitHub Flavored Markdown file (often with YAML frontmatter on top — a typical static-site blog post) and needs the document's structure available as JSON so a downstream program can read, transform, lint, or render it. The existing options each force an unwelcome trade: `pandoc -t json` emits its own pandoc-AST shape rather than the mdast schema the rest of the Markdown tooling world has settled on; `remark-parse` and `marked` are the right shape but drag a Node runtime into every install; rolling a one-off Markdown parser in-house re-invents GFM edge cases the user does not want to own. There is no zero-runtime single-binary tool that takes "GFM + YAML frontmatter in, mdast-shaped JSON out" and exits cleanly into a Unix pipeline.

## Solution

`md2json2` is a single-binary Go CLI that reads one Markdown document (from `stdin` or a positional `FILE`) and writes one JSON document to `stdout`. The output is a fixed envelope `{"frontmatter": <object>|null, "ast": <mdast root node>}`: YAML frontmatter (if present) is lifted out as a first-class top-level field, and the body is parsed and emitted as an `mdast`-shaped node tree drawn from a closed, documented v1 node set. The tool is a Unix filter first — compact JSON by default, stdout reserved for the JSON document, errors on stderr with a non-zero exit code, and one consistent stderr regex across every diagnostic. It installs via `go install …@latest` or as a prebuilt static binary from GitHub Releases, with no Node / Python / Haskell runtime to manage.

## User Stories

1. As a blog author with a directory of `.md` posts, I want to run `md2json2 post.md` and get a single-line JSON envelope on stdout, so that I can pipe it into `jq` to extract the title or tags from the frontmatter.
2. As a shell scripter, I want to run `md2json2 < post.md > post.json` (no positional argument), so that the tool behaves as a Unix filter and composes with `cat`, `xargs`, and process substitution.
3. As a CI author processing many files, I want `md2json2 -o out.json post.md` to write directly to a file, so that I do not need to manage shell redirection for every invocation.
4. As a human reading the output during debugging, I want `--pretty` to emit 2-space-indented JSON, so that I can eyeball the structure without piping through `jq .`.
5. As a downstream consumer that only needs metadata, I want `--frontmatter-only` to emit just the frontmatter value (or `null`), so that I can skip the cost of parsing the body when I do not need it.
6. As a downstream consumer that does not need source-mapping, I want `--no-position` to drop every `position` field uniformly, so that my diff/snapshot tests are stable and the JSON is smaller.
7. As a static-site-generator author, I want the JSON to conform to the documented `mdast` node-set v1 (closed enumeration), so that I can write transformers against a stable, public schema rather than against goldmark's Go-native AST.
8. As a tooling author, I want fenced code blocks to surface `lang`, `meta`, and `value` on the `code` node, so that I can route by language without re-parsing the info string.
9. As a tooling author, I want GFM tables to expose per-column alignment on `table.align` (an array of `"left"|"right"|"center"|null`), so that I render columns correctly without inspecting individual cells.
10. As a tooling author, I want GFM task list items to expose `checked: true|false` on `listItem` directly (with `null` for non-task items), so that I can build a to-do view without descending into children to find a checkbox node.
11. As a tooling author, I want reference-style links/images preserved as the `linkReference` / `imageReference` / `definition` triple rather than flattened to inline `link`/`image`, so that I can round-trip the document or surface the definitions separately.
12. As a tooling author, I want autolinks (bare URL or `<https://…>`) collapsed to mdast `link{url, title:null}` with the URL as the child `text.value`, so that one node shape covers all link forms.
13. As a tooling author, I want footnotes to emit as `footnoteDefinition{identifier, label}` and `footnoteReference{identifier, label}`, so that I can resolve references to definitions without re-walking the source.
14. As a tooling author, I want raw HTML (block or inline) preserved verbatim as an `html{value}` node, so that I get the original markup and can decide for myself whether to render or strip it.
15. As an editor-integration author, I want each node (including `root`) to carry a `position: { start, end }` field by default with 1-indexed `line`, 1-indexed `column` counted in UTF-8 code points, and `offset` as a byte offset, so that I can map any AST node back to its source range.
16. As an editor-integration author running on Windows with CRLF-saved files, I want `position.line` and `position.column` to match what my editor shows me regardless of line-ending style, so that error messages and source maps are cross-platform stable.
17. As a programmer driving the tool from a pipeline, I want exit code `0` on success, `1` on parse error, and `2` on usage error (bad flag, missing/unreadable file), with stdout staying clean (no JSON-on-stdout error fallback), so that I can branch on `$?` without inspecting stdout.
18. As a programmer parsing diagnostics, I want every stderr line to match exactly one regex `^md2json2: ([^:]+):(\d+):(\d+): (.+)$`, so that I can `grep`/`sed` paths and positions out without writing a multi-format parser.
19. As a programmer reading diagnostics for a stdin invocation, I want the `<path>` token in the error line to be the literal `-` (matching the CLI's own stdin sentinel), so that the diagnostic token round-trips with the invocation convention.
20. As a programmer running the tool with no input source determined yet (usage error: unknown flag, missing required positional, or unreadable `FILE` before any bytes are read), I want the `<path>` token in the error line to be the literal program name `md2json2` and the position to be the `:0:0:` "no position available" sentinel, so that the canonical stderr regex still matches and I can distinguish pre-input usage errors from document-scoped errors by inspecting `<path>`.
21. As a programmer whose document uses YAML frontmatter, I want a malformed frontmatter block (tab indent, unbalanced quotes, duplicate keys) to fail loudly with `md2json2: <path>:<line>:<col>: invalid frontmatter: <yaml error>` and exit `1`, so that bad metadata never silently disappears into a `frontmatter: null` envelope.
22. As a programmer whose document begins with `---` but has no closing fence, I want the document treated as body-only with `frontmatter: null` and exit `0`, so that an unclosed fence never becomes an implicit "empty frontmatter" or a hard error.
23. As a programmer with a UTF-8 BOM on the front of my file (saved by Notepad or a Windows tool), I want the BOM stripped silently before parsing, so that the first text node does not begin with `﻿`.
24. As a programmer with a file containing invalid UTF-8 bytes, I want the tool to exit `1` with `md2json2: <path>:<line>:<col>: invalid utf-8 byte at offset <N>` on stderr (and never silently substitute U+FFFD), so that a partially-corrupt input never produces a partially-corrupt AST that looks fine.
25. As a programmer running `md2json2 --no-position < empty.md` (zero-byte input), I want exactly `{"frontmatter":null,"ast":{"type":"root","children":[]}}` on stdout and exit `0`, so that the empty-document baseline is byte-for-byte stable.
26. As a programmer running `md2json2 < empty.md` (default, zero-byte input), I want the same envelope but with a zero-width `position` on `root` (`{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}`) and exit `0`, so that the "every node carries `position` by default" rule holds without per-node special-casing.
27. As a programmer running the tool on a single-newline input (one `\n` byte and nothing else), I want the same `root` envelope as the empty-input case but with `root.position.end = {"line":2,"column":1,"offset":1}` (the newline advances `line` and `offset` by one; `root.children` remains `[]` because a blank line produces no block content), so that the position rule extrapolates predictably from the zero-byte baseline.
28. As a programmer using `--frontmatter-only` on a document whose frontmatter is a YAML scalar (e.g. `--- "hello" ---` or `--- 42 ---` or `--- null ---`), I want the output to be the scalar's JSON equivalent at the top level (`"hello"`, `42`, or `null` respectively — not wrapped in an object), so that the flag's contract is uniform: emit exactly the JSON value the YAML denotes.
29. As a downstream consumer composing `--pretty --no-position`, I want the pretty-printer to keep mdast convention for key ordering (`type` first, then type-specific fields in their declared order, then `children`, then `position` when present) and to preserve explicit `null` fields rather than elide them (so the schema shape is uniform whether a field is set or null), so that two documents with the same logical shape produce diffs that only reflect content differences and never key reordering.
30. As a tooling author reading an emitted `image` node, I want `image.alt` to be a flat string (the concatenation of the alt's textual content per the mdast spec), with any non-text inline children silently dropped from the alt rather than nested under it, so that the image node shape matches mdast unconditionally and consumers do not need to flatten the alt themselves.
31. As an installer, I want to run `go install github.com/<owner>/md2json2@latest` and get a working binary on my `PATH`, so that I do not need to deal with Node, Python, or Haskell runtimes.
32. As an installer who cannot run `go install`, I want a prebuilt static binary on the GitHub Releases page for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, and `windows/amd64`, so that I can download and run without a Go toolchain.
33. As a downstream consumer of the JSON envelope's `ast` field, I want every emitted node's `type` to be a member of the **mdast node-set v1** enumeration (no `unknown`, no `html` fallback for non-HTML constructs, no goldmark-native type names), so that I can build a strict whitelist parser against the documented schema and treat any out-of-set type as a tool bug rather than a graceful-degradation signal. *(This is the property-test acceptance criterion for the silent-drop lossiness policy: a property assertion over the `translate` module's output node set rather than an example-based fixture, because under GFM + frontmatter + footnotes the dropped-construct set is intended to be effectively empty.)*

## Implementation Decisions

### Module sketch — deep modules with narrow interfaces

The implementation is decomposed into five modules. Each is a deep module: the public surface is small (one or two entry points), most of the complexity is hidden inside, and the interface rarely changes across the v1 ship criterion and future extensions. All modules take their IO sources and sinks and their argument list explicitly — no module reaches into process-global state (no `os.Args`, no `os.Stdin/Stdout/Stderr`, no `os.Exit`). Process globals are injected at the top level so the entire pipeline is callable as a pure function inside a test.

**`cli` — argument parsing, exit codes, IO wiring.** Owns flag parsing for the **v1 flags** set (`-o/--output`, `--pretty`, `--no-position`, `--frontmatter-only`, `-h/--help`, `-V/--version`), positional-`FILE` handling including the `-` stdin sentinel, opening the input source (a file path or the injected stdin), opening the output sink (a file path from `-o` or the injected stdout), and translating any error returned by lower modules into the canonical **Error format** stderr line plus the right exit code. The module is the single place that knows how to render typed errors from lower modules into stderr text and that decides which exit code each error maps to.

**`read` — bytes in, normalized document out.** Implements ADR-0001 in full: read the whole document into memory (no streaming, no size cap in v1), validate UTF-8 (return an `invalid utf-8 byte at offset <N>` error on the first bad byte — never substitute U+FFFD), strip a leading UTF-8 BOM, and normalize CRLF to LF. All downstream `position.offset` and `position.line` values are relative to the bytes this module returns, not to the raw file. The module knows nothing about Markdown or YAML; it takes the `<path>` token used in error messages as an argument so it can attribute byte-level errors back to the right source.

**`parse` — normalized document → frontmatter value + body AST.** Detects a `---`-fenced YAML frontmatter block at the very top (per the `Frontmatter` rule, including the **unclosed-fence rule** — opening `---` without a closing fence is body-only, not an error and not implicit-EOF frontmatter), YAML-parses the frontmatter block (returning the canonical `invalid frontmatter` error on YAML failure per the **Invalid frontmatter (policy)** rule), strips the frontmatter from the body, and runs goldmark over the remaining bytes with the GFM, footnote, and frontmatter extensions enabled. Returns goldmark's native AST; does not translate. The YAML library choice (`gopkg.in/yaml.v3` vs `goldmark-meta`) is an implementation detail of this module — the grill transcript explicitly marks it as not a contract-level decision — and is replaceable without affecting any other module.

**`translate` — goldmark AST → mdast-shaped node tree.** Takes a goldmark root node plus the normalized source bytes (needed to compute `position.column` / `position.offset` for inline nodes) plus the two translation options (`noPosition`, `frontmatterOnly` — though `frontmatterOnly` short-circuits in `cli` before `translate` is called) and returns the mdast-shaped root node as a plain Go value tree ready for JSON encoding. The module enforces the **mdast node-set v1** enumeration as the **only** types it ever emits and applies the **Lossiness policy (goldmark → mdast)** of silent-drop for any goldmark construct without an mdast equivalent in v1 (no `unknown`, no `html` fallback for non-HTML constructs).

Per-disagreement commitments encoded here:
- Reference-style links/images emit the `linkReference` / `imageReference` / `definition` triple — never flattened to inline `link` / `image`.
- Autolinks (goldmark `AutoLink`) collapse to mdast `link{url, title:null}` with the URL also as the child `text.value`.
- GFM task-list checkbox state is hoisted from goldmark's `TaskCheckBox` child onto `listItem.checked` (`true` / `false`); non-task items get `null`. The child checkbox node is dropped.
- Table alignment is emitted as a per-column array on `table.align`; `tableCell` carries no `align` field. (Goldmark per-cell alignment is collapsed into the column array; the GFM constraint that all cells in a column agree on alignment makes this lossless.)
- Block and inline raw HTML both emit as `html{value}` (mdast does not distinguish).
- Indented code blocks emit `code{lang:null, meta:null, value:<...>}`; fenced code blocks emit `code{lang, meta, value}` from the info string.
- Hard line breaks (trailing two-space, `\`-escaped newline) emit as `break`.
- `image.alt` is a flat string per mdast — the concatenated textual content of goldmark's inline children. Non-text inline children inside an image's alt are silently dropped from the alt (they do **not** survive as alt children, because mdast types `image.alt` as `string`).

`position` is attached to every emitted node — including `root` — unless `noPosition` is set, per the **Position info** rule. The uniform rule holds on the boundary cases: empty input gives `root` a zero-width `{start:{1,1,0}, end:{1,1,0}}` position; a single-newline input gives `root` the position `{start:{1,1,0}, end:{2,1,1}}` (the trailing newline advances `line` to 2 and `offset` to 1; `root.children` is empty because a blank line produces no block content under goldmark).

**`emit` — JSON envelope encoder.** Takes the frontmatter value (or `nil`), the mdast root (or `nil` when `--frontmatter-only` short-circuits), the two emit-time options (`pretty`, `frontmatterOnly`), and the output sink. In default mode emits `{"frontmatter": ..., "ast": ...}` compact (single-line). With `--pretty` emits 2-space-indented JSON. With `--frontmatter-only` emits just the frontmatter value, including the **scalar passthrough rule**: when the YAML frontmatter is a scalar (string / number / boolean / null), `--frontmatter-only` emits the scalar's JSON equivalent at the top level (`"hello"`, `42`, `true`, `null`) — never wrapped in an object. The module is purely a serializer; it never decides what to drop or compute.

**Pretty-print key ordering and null preservation (compose rule).** When `--pretty` is in effect, every emitted object orders its keys in mdast convention: `type` first, then the type-specific fields in their declared order in **mdast node-set v1** (e.g. `depth` before `children` on a `heading`; `url`, `title` before `alt` on an `image`; `ordered`, `start`, `spread` before `children` on a `list`), then `children` if the node has them, then `position` if not stripped. Explicit `null` fields are **preserved** in output, never elided, so the schema is uniform whether a field has a real value or is null (`{"lang":null,"meta":null,...}` on an indented code block, not `{}`). This rule composes with `--no-position` (drop the `position` key only, keep everything else uniform). Compact mode follows the same key order for byte-for-byte stability across pretty/compact rendering of the same logical document.

### Pipeline assembly

`cli` calls `read`, then `parse`, then (unless `--frontmatter-only` short-circuits) `translate`, then `emit`. Errors bubble up as typed values from each module; `cli` is the single place that knows how to render them into the canonical stderr line + exit code mapping.

### Error-format / exit-code mapping (contract)

| Failure source                                  | Stderr line                                                          | Exit |
| ----------------------------------------------- | -------------------------------------------------------------------- | ---- |
| Pre-input usage error (unknown flag, missing positional, unreadable `FILE` before any bytes read) | `md2json2: md2json2:0:0: <usage message>` | `2`  |
| Document-scoped error (goldmark error with no line/column) | `md2json2: <path>:0:0: <message>` | `1`  |
| Invalid UTF-8 byte                              | `md2json2: <path>:<line>:<col>: invalid utf-8 byte at offset <N>`    | `1`  |
| Malformed YAML frontmatter                      | `md2json2: <path>:<line>:<col>: invalid frontmatter: <yaml error>`   | `1`  |
| goldmark unrecoverable parse error              | `md2json2: <path>:<line>:<col>: <message>`                           | `1`  |
| Success                                         | (nothing on stderr; JSON on stdout)                                  | `0`  |

When goldmark reports a line but no column, print `<path>:<line>:1:` (round unknown column up to 1, never 0). When the error is document-scoped with no position, use the `:0:0:` sentinel per CONTEXT.md's `Error format` entry. When the input is stdin (and `read` has been entered), `<path>` is the literal `-`. When the failure occurs before any input source is determined — usage errors raised by `cli` flag parsing or by failing to open a `FILE` for reading — `<path>` is the literal program name `md2json2` (so the line reads `md2json2: md2json2:0:0: ...`). The literal `-` is reserved for "stdin was the chosen source"; the literal `md2json2` is the sentinel for "no source was ever in play." The single regex `^md2json2: ([^:]+):(\d+):(\d+): (.+)$` matches every stderr line; consumers distinguishing pre-input from document-scoped errors inspect `<path>` (`md2json2` vs file path or `-`).

### State the modules do not own

- No module reaches into process globals; `os.Args`, `os.Stdin/Stdout/Stderr`, and `os.Exit` are injected at the top level.
- No module mutates the input bytes after `read` returns; downstream modules only read.
- No module short-circuits the **Lossiness policy** — there is no opt-out flag for surfacing dropped goldmark constructs in v1.

### Reference: ADR-0001

The `read` module's UTF-8 / BOM / CRLF / streaming / size-cap behavior is the implementation of `<product_dir>/docs/adr/0001-input-encoding-and-normalization.md`. Any change to that module's contract must update ADR-0001.

## Testing Decisions

Tests target **external behavior**, not internal module shape: the contract under test is "given this stdin/argv/file fixture, the tool emits this stdout and this stderr and this exit code." This keeps the test suite stable across the inevitable refactors of `translate` and `parse` internals.

### What makes a good test for md2json2

- **Runnable-command + expected-output-pair fixtures**, mirroring the v1 ship criterion's `(invocation, expected stdout, expected exit code)` triple. Each fixture is a directory containing an `input.md` (or `input.bin` for non-UTF-8 cases), an `args` file, an expected `stdout.json`, an expected `stderr.txt`, and an expected `exit` code. The test harness runs the CLI as a black box and compares byte-for-byte.
- **The v1 ship criterion is the headline acceptance test.** It decomposes into two concrete fixtures: (1) `md2json2 --no-position < empty.md` produces exactly `{"frontmatter":null,"ast":{"type":"root","children":[]}}` on stdout and exit `0`; (2) `md2json2 < empty.md` (default) produces the same envelope with a zero-width `position` field on `root` and exit `0`. Both must hold byte-for-byte.
- **One fixture per `mdast` node type in the v1 node set.** A focused 1–3 line Markdown input that produces exactly that node type, so the schema enumeration is testable as a list, not as a holistic assertion.
- **One fixture per per-disagreement commitment**: reference-style not flattened, autolink collapsed to `link`, task checkbox hoisted onto `listItem.checked`, table alignment on `table.align` only, raw HTML preserved as `html{value}`, hard break emits `break`, indented code emits `lang:null, meta:null`, `image.alt` is a flat string with non-text inline children dropped.
- **Property-test fixture for the silent-drop lossiness policy (user story 33).** Generate (or hand-pick) a broad set of GFM + frontmatter + footnote inputs, run them through the pipeline with `--no-position` for byte stability, walk the emitted AST, and assert that every node's `type` is a member of the documented **mdast node-set v1** enumeration. This is the acceptance test for "no goldmark-native types leak"; it is a property assertion over the `translate` module's output rather than an example-based fixture, because under v1's enabled extension set the dropped-construct set is intended to be effectively empty.
- **Scalar-frontmatter fixtures for `--frontmatter-only`.** Three small fixtures cover `--- "hello" ---` (emits `"hello"`), `--- 42 ---` (emits `42`), `--- null ---` (emits `null`), each asserting exact byte output and exit `0`.
- **Pretty-printer compose fixtures.** Fixture A: `--pretty` on a representative document, asserting mdast-convention key order (`type`, type-specific fields, `children`, `position`) and `null` field preservation. Fixture B: `--pretty --no-position` on the same input, asserting the `position` key is uniformly absent and all other key ordering / null preservation behavior is unchanged.
- **Single-newline boundary fixture.** Input is exactly one `\n` byte; assert `root.position.end == {line:2, column:1, offset:1}`, `root.children == []`, exit `0`. Complements the empty-input fixture by showing the position rule extrapolates predictably.
- **ADR-0001 fixture matrix** (called out explicitly in the ADR's TDD implication section): BOM-prefixed input, CRLF-only input, mixed CRLF/LF input, invalid UTF-8 at the leading byte, invalid UTF-8 mid-document, and a handful of large-but-not-pathological inputs. For each, assert both the JSON output (or lack thereof) and the canonical stderr line.
- **One fixture per failure mode** in the error/exit-code table above: pre-input usage error (asserting the `<path>` token is literally `md2json2` and the position is `:0:0:`), invalid UTF-8, malformed frontmatter, unrecoverable goldmark error, document-scoped goldmark error (asserting the `:0:0:` sentinel and a file-path `<path>`). Each asserts the exact stderr line matches the canonical regex and the exit code.
- **One fixture for the unclosed-`---` rule**: input begins with `---` on line 1 but no closing fence; expect `frontmatter: null`, body parses as usual, exit `0`.
- **One fixture for the stdin path token**: invoke with stdin and assert the stderr `<path>` token is the literal `-`, not `stdin` or `<stdin>` or empty.
- **One fixture for the `:1:` column-fallback rule**: simulate a goldmark error with line-only and assert `:1:` column (not `:0:`).

### Modules tested in isolation (in addition to the black-box fixture suite)

- `read` is unit-tested directly for UTF-8 validation / BOM strip / CRLF normalization — easier to assert the byte-level transform there than through the full pipeline.
- `translate` is unit-tested directly for each `mdast` node-type emission, taking a hand-built goldmark AST and asserting the mdast tree shape. This keeps the per-node-type table tractable even as fixtures grow. The node-set-whitelist property test (user story 33's acceptance criterion) also lives here, because it is a property of `translate`'s output specifically.
- `cli`, `parse`, and `emit` are exercised primarily through the fixture suite; they do not benefit from finer-grained unit tests because their behavior is observable end-to-end. The `emit` module's pretty-print key ordering and null-field preservation rule is verified by the pretty-printer compose fixtures rather than a unit test.

### Prior art

The mdast schema itself (documented under unified/remark) is the prior-art reference for translation correctness. The grill transcript explicitly anchors disagreements to mdast wherever goldmark and mdast diverge; tests follow that anchor.

## Out of Scope

The following are explicit v1 non-goals. Each is a "do not let scope creep recover it during TDD" line.

- Math (`$...$`, `$$...$$`). Out of scope.
- MDX. Out of scope.
- TOML frontmatter. YAML only.
- Multi-file / directory / glob input. One file per invocation; users shell-loop for batches.
- Watch mode.
- Config file (no `.md2json2rc`, no env vars in v1).
- `--schema` flag. The schema *is* mdast; the v1 node set is the documentation.
- `--max-size` flag / hard input size cap. Trust the OS / Go allocator; revisit on real OOM reports.
- Encoding auto-detection or `--encoding` flag. UTF-8 only.
- `--lenient-utf8` (U+FFFD-substituting) mode. Invalid UTF-8 is a hard error.
- Homebrew tap. Post-v1.
- Soft-fallback frontmatter envelope (`_raw` / `_error`). Malformed frontmatter is a hard error.
- `{"type":"unknown",...}` escape hatch or `html` fallback for non-HTML constructs. Lossiness policy is silent-drop.
- JSON-on-stdout error envelope. Errors go to stderr; stdout stays clean.
- Streaming parse for unbounded input. Whole-document in memory only.
- Non-text inline children inside an `image.alt`. The alt is a flat string per mdast; the v1 translator drops non-text inlines from the alt rather than nesting them.

## Further Notes

- Cross-reference: `<product_dir>/docs/adr/0001-input-encoding-and-normalization.md` is the authoritative record for the input-encoding / BOM / CRLF / streaming / size-cap decisions implemented in the `read` module. Any divergence between this PRD and ADR-0001 should be resolved in favor of ADR-0001.
- The `mdast` node-set v1 enumeration in `<product_dir>/CONTEXT.md` is the **authoritative schema** for TDD fixtures and downstream consumers. The PRD restates it for context; CONTEXT.md is the source of truth.
- Open edges flagged by the Interviewer Agent in grill-0 Round 3 are **resolved in this PRD**:
  - `--frontmatter-only` on scalar YAML emits the scalar's JSON equivalent at top level (user story 28, `emit` module's scalar-passthrough rule).
  - `--pretty` × `--no-position` interaction is pinned: mdast-convention key ordering, `null` fields preserved (user story 29, the pretty-printer compose rule in `emit`).
  - `image.alt` is a flat string per mdast; non-text inline children inside the alt are silently dropped (user story 30, `translate` per-disagreement commitment).
  - `position.line` for a single-newline input is `{start:{1,1,0}, end:{2,1,1}}` with empty `root.children` (user story 27, `translate` boundary-case commitment).
  - YAML library choice (`gopkg.in/yaml.v3` vs `goldmark-meta`) remains a `parse`-internal implementation detail per the grill transcript, not a contract decision.
- Pre-input `<path>` sentinel: the literal program name `md2json2` is used for usage errors raised before any input source is in play, distinguishing them from document-scoped errors (which use a real file path or the stdin `-` sentinel). The `:0:0:` "no position available" sentinel from CONTEXT.md's `Error format` entry is the only position-pair used for both pre-input and document-scoped no-position errors; there is no second sentinel form.
- Wire contract is **mdast-shaped JSON**, not goldmark-AST-shaped JSON. Every module disagreement is resolved in favor of mdast. Treat goldmark as a parser library, not as a schema.
- The single observable acceptance test for "v1 ships" is the v1 ship criterion verbatim from `<product_dir>/CONTEXT.md`. The PRD adds no additional ship criteria.
