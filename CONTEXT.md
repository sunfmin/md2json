# md2json — product glossary

_Load-bearing terms for the md2json CLI. Populated inline as the Interviewer/PO grill resolves each term. Format: bold term, one or two sentence definition, optional `_Avoid_:` line for confusable synonyms._

**Markdown (input)**:
The v1 accepted input dialect is **GitHub Flavored Markdown (GFM)** — CommonMark plus tables, task lists, strikethrough, and autolinks — with **YAML frontmatter** (delimited by leading `---` lines). Fenced code blocks (with info string) and footnotes are in scope; raw HTML is preserved verbatim as `html` nodes (not parsed into). The v1.x math Run extends this with Pandoc/CommonMark-extra dollar-sign math (inline `$...$`, display `$$...$$`); bracket form `\(...\)`/`\[...\]`, fenced ` ```math ```, AsciiMath, mhchem-as-separate-syntax, and raw `<math>` MathML remain non-goals.
_Avoid_: "Markdown" unqualified (ambiguous between CommonMark/GFM/MDX). MDX and TOML frontmatter remain explicit v1 non-goals.

**Frontmatter**:
A YAML block at the top of the input file fenced by `---` on its own lines before any other content. Parsed and surfaced as a first-class top-level field on the JSON envelope, **not** as a node inside the AST. v1 supports YAML only. **Unclosed-fence rule**: a document that opens with `---` on line 1 but never closes the fence is *not* frontmatter — it parses as body-only with `frontmatter: null`, exit `0`. The closing `---` is mandatory to enter frontmatter mode.
_Avoid_: meta-block, header, preamble.

**Invalid frontmatter (policy)**:
Hard error. When the document opens with a closed `---` fence but the YAML between the fences does not parse (tab indentation, unbalanced quotes, duplicate keys, etc.), write `md2json: <path>:<line>:<col>: invalid frontmatter: <yaml error>` to stderr, exit `1`, nothing on stdout. `--frontmatter-only` follows the same rule (failure is upstream of which view is requested). Matches the global fail-fast-on-malformed-structured-input posture; no `_raw`/`_error` soft-fallback envelope is emitted.

**AST (output) / mdast**:
The JSON node tree shape emitted under the envelope's `ast` field. Conforms to **`mdast`** (unified/remark's Markdown AST), e.g. `root` → `heading{depth, children}` → `text{value}`, `paragraph`, `strong`, `emphasis`, `list`, `code`, `html`, etc. Internally the tool parses with `goldmark` and translates `goldmark`'s Go-native AST into mdast-shaped JSON on emit — the wire contract is mdast, not goldmark's internal types.
_Avoid_: "goldmark AST" (that is an implementation detail, not the contract).

**Position info**:
Each emitted node carries a `position: { start: {line, column, offset}, end: {line, column, offset} }` field by default, derived from goldmark source spans. Stripped by the `--no-position` flag. **Uniform rule**: every node (including `root`) carries `position` unless `--no-position`; no per-node special-casing. On empty input the `root` node carries a zero-width position `{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}`. `line` and `column` are 1-indexed; `column` counts UTF-8 code points within the (normalized) line; `offset` is a byte offset into the **normalized** (post-BOM-strip, LF-only) document, not the raw file.

**JSON envelope**:
The top-level shape of every successful invocation's stdout: `{"frontmatter": <object>|null, "ast": <mdast root node>}`. Single JSON document per invocation. Compact (single-line) by default; `--pretty` switches to 2-space-indented form.

**CLI contract**:
Unix-filter-first invocation `md2json [FILE]`. Reads from `FILE` if given, else from stdin (`FILE=-` is the explicit stdin sentinel and is equivalent to omitting the positional). Always writes the JSON envelope to **stdout**; errors go to **stderr** with a non-zero exit code. One file per invocation in v1 — no directory/glob/multi-file mode.
_Avoid_: "input mode" (ambiguous); always speak in terms of stdin vs positional FILE.

**v1 flags**:
`-o, --output <FILE>` (write to file instead of stdout); `--pretty` (2-space indent); `--no-position` (drop `position` from every node); `--frontmatter-only` (emit just the frontmatter value, or `null`, skipping body parse); `-h, --help`; `-V, --version`. No `--schema`, no watch mode, no config file in v1.

**Exit codes**:
`0` success; `1` parse error (input the parser genuinely cannot recover from); `2` usage error (bad flag, missing/unreadable file); `64`+ reserved.
_Avoid_: success-with-error-body-on-stdout — stdout stays clean for piping.

**Error format**:
Human-readable line on stderr matching exactly one regex: `^md2json: ([^:]+):(\d+):(\d+): (.+)$`. `<path>` is the literal `-` when reading from stdin (not `stdin`, not `<stdin>`, not empty) — round-trips with the CLI's own stdin sentinel. When goldmark reports a line but no column, print `<path>:<line>:1:` (round unknown column **up to 1**, never `0`, since lines/columns are 1-indexed elsewhere). When the error is document-scoped with no position at all, use the sentinel `<path>:0:0:` — the same regex still matches and `0:0` conventionally means "no position available." No JSON-on-stdout error fallback.
_Avoid_: `stdin` / `<stdin>` as the path token; `:0:` for unknown column; omitting the column field entirely.

**Distribution**:
Implemented in **Go**, parser **`github.com/yuin/goldmark`** (with its official GFM, frontmatter, footnote extensions as applicable). Primary install: `go install github.com/<owner>/md2json@latest`. Secondary: prebuilt static binaries on GitHub Releases for `darwin/{amd64,arm64}`, `linux/{amd64,arm64}`, `windows/amd64`. Homebrew tap is post-v1.

**v1 ship criterion**:
Running `md2json < post.md` on a typical GFM blog post with YAML frontmatter prints, to stdout, a valid JSON document with a top-level `frontmatter` object and an `ast` field conforming to the documented mdast subset (see **mdast node-set v1**), exiting `0`. On empty input: `md2json --no-position < empty.md` prints exactly `{"frontmatter":null,"ast":{"type":"root","children":[]}}` and exits `0`; `md2json < empty.md` (default) prints the same envelope with a zero-width `position` field on the `root` (`{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}`) and exits `0`. The single observable acceptance test for v1.

**Dollar-sign math (transport-only)**:
The v1.x math Run accepts Pandoc/CommonMark-extra dollar-sign math: inline `$...$` (with the **remark-math currency rule**, below) and display `$$...$$` on its own paragraph. md2json is a **transport** for math — it carries the source bytes to the wire under an `inlineMath`/`math` node's `value` field and **never** invokes a LaTeX→MathML/HTML/SVG renderer, never validates LaTeX, never expands macros, never balances braces. Mismatched braces, unknown macros, mhchem (`\ce{...}`), AMS environments (`align`, `gather`, `cases`), `\text{...}` islands, equation `\label`/`\ref` — all reduce to "bytes in `value`, downstream's problem." Preserves the **Wedge** (single static Go binary, no Node/Python runtime).
_Avoid_: "math rendering," "LaTeX support" (md2json does neither).

**remark-math currency rule**:
The dollar-sign disambiguation rule used to decide whether `$...$` is inline math or prose-mentioning-money. **Inline:** opening `$` must be immediately followed by a **non-whitespace** character, the closing `$` must be immediately **preceded** by a non-whitespace character, AND the closing `$` must **not** be immediately followed by a digit. So `$5 and $10$` is *not* math (whitespace after opening `$`, digit after closing `$`); `$x = 5$` is. **Display `$$...$$` has no such guard.** This is the rule the `remark-math` ecosystem implements; chosen because `mdast node-set v1` is already remark-shaped (Q2 picked `inlineMath`/`math` verbatim), so the parse rule must match the node shape's ecosystem. The chosen goldmark math extension must implement this rule; if it does not, that is an extension-pick blocker, not a rule reopen.
_Avoid_: "Pandoc rule" (close but edge-case-divergent), "strict `\$` opt-in" (breaks prose mentioning money).

**`inlineMath` node**:
mdast node type for inline dollar-sign math, `inlineMath{value, position}`. `value` is the literal source between the inline `$` delimiters, **byte-for-byte**, delimiters stripped — same rule as `code.value` / `inlineCode.value` (governed by **Text/Code value preservation**). No LaTeX-side normalization, no entity decoding, no whitespace trim, no macro expansion. Example: source `$x = 5$` produces `inlineMath{value: "x = 5"}`.
_Avoid_: `mathInline`, single-`math`-node-with-`display`-discriminator (both fork the remark-math contract).

**`math` node**:
mdast node type for block/display dollar-sign math, `math{value, meta, position}`. `value` is the literal interior bytes between the `$$` fences (with each content line's trailing `\n` preserved, including the final line's — analogous to `code.value` for fenced code blocks). `meta` is the info-string after the opening fence for fenced math (e.g., a future ` ```math <meta> ` block); for `$$...$$` it is always `null`. The field stays in the schema so a future fenced-math Run has a home for the info string without a schema break. Example: source `$$\n\frac{a}{b}\n$$` produces `math{value: "\\frac{a}{b}\n", meta: null}`.
_Avoid_: `blockMath`, `mathBlock`, `displayMath` (all break ecosystem compatibility).

**mdast node-set v1**:
The closed, enumerated set of mdast node types the v1 emitter is allowed to produce. This is the authoritative schema for TDD fixtures and downstream consumers:
- `root`
- `paragraph`
- `heading{depth}` (depth 1–6)
- `text{value}`
- `emphasis`, `strong`, `delete` (GFM strikethrough)
- `inlineCode{value}`
- `code{lang, meta, value}` (fenced and indented; `lang` and `meta` are `null` for indented)
- `blockquote`
- `list{ordered, start, spread}`
- `listItem{checked, spread}` — `checked` is `true`/`false` for GFM task items, `null` otherwise (the task checkbox is hoisted onto `listItem`, not emitted as a child node)
- `thematicBreak`
- `link{url, title}`, `image{url, title, alt}`
- `linkReference{identifier, label, referenceType}`, `imageReference{identifier, label, referenceType}`, `definition{identifier, label, url, title}` — reference-style links/images are **preserved**, not flattened to inline `link`/`image`
- `html{value}` — both block and inline raw HTML emit as `html` (mdast does not distinguish)
- `table{align}` + `tableRow` + `tableCell` — `align` is a per-column array on `table` of `"left"|"right"|"center"|null`; `tableCell` carries no `align` field (follow mdast, not goldmark per-cell)
- `footnoteDefinition{identifier, label}`, `footnoteReference{identifier, label}`
- `break` (hard line break — trailing two-space or `\`-escaped newline)
- `inlineMath{value}` — inline dollar-sign math (v1.x math Run); transport-only, see **`inlineMath` node** entry
- `math{value, meta}` — block/display dollar-sign math (v1.x math Run); transport-only, `meta` is `null` for `$$...$$`, see **`math` node** entry
Frontmatter is **not** an mdast node; it is lifted into the envelope's `frontmatter` field before AST translation. Autolinks (bare URL or `<https://…>`) collapse to mdast `link{url, title:null}` with the URL as the child `text.value`; goldmark's distinct `AutoLink` type is an implementation detail not exposed on the wire.
_Avoid_: "the goldmark node set" — goldmark's internal types are not the contract; treat this enumeration as the contract.

**Text/Code value preservation**:
Raw source bytes flow through to `value`-bearing nodes byte-for-byte as goldmark exposes them on its native AST — the `translate` module does not re-decode, re-escape, or trim. Specifically:
- `text.value` is the literal textual run from the normalized (post-BOM, post-LF) source: no leading/trailing whitespace stripped, no entity decoding applied beyond what goldmark itself has already resolved on the goldmark node. For the hard-break case `line1<two-spaces>\nline2`, the two surrounding `text` nodes carry `value: "line1"` and `value: "line2"` respectively — the trailing two spaces are consumed by the `break` node boundary, the leading-of-line2 whitespace is none, and no source whitespace survives inside the text values themselves.
- `inlineCode.value` is the literal content between the backticks per CommonMark (one space of trim on each side only when both sides have a leading/trailing space and the run is non-empty — this is CommonMark's rule and goldmark already applies it; `translate` does not re-apply or re-trim).
- `code.value` (fenced and indented) is the literal content **between** the fences (or, for indented code, the dedented content), with every content line's trailing `\n` preserved including the final line's `\n`. Concretely: a fenced block ` ```go\nfunc x(){}\n``` ` has `code.value == "func x(){}\n"` — the closing fence is not part of `value`, but the `\n` that terminates the last content line is. An indented code block `    abc\n    def\n` has `code.value == "abc\ndef\n"`.
- `html.value` is the raw markup as goldmark provides it — no entity expansion, no whitespace normalization, no tag-name lowercasing.
_Avoid_: trimming, normalizing, or re-escaping `value`-bearing fields in `translate`. If goldmark emits a byte, it goes on the wire.

**Lossiness policy (goldmark → mdast)**:
**Silent drop.** Any goldmark construct that does not map to a node in **mdast node-set v1** is dropped from the output with no log line, no `html` fallback dump, and no `{"type":"unknown",...}` escape hatch. Rationale: the wire contract is "documented mdast subset"; an undocumented escape type would force every downstream consumer to defensively handle it and would turn "v1.x adds a real node type" into a breaking change. The set of dropped constructs under the v1 enabled-extension set (GFM + frontmatter + footnotes) is intended to be effectively empty; a real gap is a v2 schema-extension conversation.
_Avoid_: "graceful degradation," `unknown` node, `html` fallback for non-HTML constructs.

**Input handling**:
v1 reads the **whole document into memory** (no streaming; goldmark itself does not stream, and the output is a single JSON document). **No hard size cap** in v1 — trust the OS / Go allocator; suitable for documents up to ~tens of MB, multi-hundred-MB inputs out of scope; a `--max-size` flag is deferred. **Encoding: UTF-8 only.** UTF-16 / latin-1 are non-goals; users with non-UTF-8 input run it through `iconv` first. Invalid UTF-8 bytes are a **hard error**: `md2json: <path>:<line>:<col>: invalid utf-8 byte at offset <N>` on stderr, exit `1` (no silent U+FFFD substitution — that would be data corruption masquerading as success). **Leading UTF-8 BOM**: stripped silently before any further processing; `position.offset` values are therefore relative to the post-BOM-strip document, not the raw file. **Line endings**: CRLF is normalized to LF **before** parsing; `position.line` reflects logical lines (cross-platform stable), `position.column` counts UTF-8 code points in the normalized line, `position.offset` is a byte offset into the normalized (LF-only) document.
_Avoid_: "lenient encoding," "auto-detect encoding," U+FFFD replacement.

**Wedge (why this exists)**:
Emits an **mdast-shaped** JSON for **GFM + YAML frontmatter** input as a **single static Go binary**, with no Node/Python/Haskell runtime required. Differentiates from `pandoc -t json` (pandoc-AST, not mdast), `remark-parse` (needs Node), and `marked` (needs Node). Not a rewrite of any specific predecessor — the `2` suffix is name-disambiguation against the crowded `md2json` package namespace.
