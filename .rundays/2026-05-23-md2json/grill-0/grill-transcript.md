## Round 1

### Interviewer

The Idea Brief is a single sentence: "A small CLI that convert markdown to Json." Almost every load-bearing noun is undefined. Before I can hand anything to `to-prd`, I need concrete answers on the following. Please be specific — pick one option per question, or describe a precise alternative; do not list possibilities.

1. **Markdown flavor and surface.** Which dialect must the parser accept as input — strict CommonMark, GitHub Flavored Markdown (tables, task lists, strikethrough, autolinks), Markdown with YAML frontmatter, MDX, something else? If multiple, name the single one that MUST work on day one and any that are explicit non-goals. Also: are fenced code blocks, raw HTML, math (`$...$`), and footnotes in scope or out?

2. **JSON output shape.** What is the JSON the tool emits? Pick one and describe its structure with a tiny example:
   (a) a full AST (every node typed — `heading`, `paragraph`, `list`, `code`, etc., with children) — and if so, which existing schema, e.g. `mdast`/unified, or a custom one?
   (b) a flattened outline (headings + their text, nested by level)?
   (c) frontmatter only (parse YAML/TOML at the top, ignore the body)?
   (d) something domain-specific (e.g. extract every link, or every code block)?
   For a 3-line input `# Title\n\nHello **world**.\n`, paste the exact JSON you expect out.

3. **CLI contract.** Concretely: what is the invocation? `md2json input.md` writing to stdout? `md2json < input.md > out.json` (pure stdin/stdout filter)? `md2json -o out.json input.md`? Does it accept multiple files, a directory, or only one? What's the exit-code contract on parse failure — non-zero with stderr message, or zero with `{"error": ...}` on stdout? Any flags planned for v1 (e.g. `--pretty`, `--schema`, `--frontmatter-only`)?

4. **Language, runtime, distribution.** What is `md2json` implemented in — Go, Rust, Node/TypeScript, Python, something else? How does a user install it — `go install`, `npm i -g`, `brew`, a single static binary, `cargo install`? This determines which markdown parser library is available and shapes the whole codebase, so it is not a deferrable detail.

5. **Why "md2json" — and what is the success criterion?** The name implies a predecessor (`md2json`). Is this a rewrite of something specific, and if so, what does v1 of md2json do that `md2json` (or any existing tool like `pandoc -t json`, `remark-parse`, `marked`) does not? What single observable outcome would let you say "v1 ships"? One concrete sentence, not a feature list.

### PO

1. **Markdown flavor and surface.** Day-one input is **GitHub Flavored Markdown (GFM) with YAML frontmatter**. That means CommonMark plus tables, task lists, strikethrough, and autolinks; YAML frontmatter (delimited by `---` at the top) is parsed and surfaced as a first-class field on the output. Fenced code blocks (with info string / language) are **in scope**. Footnotes are **in scope**. Raw inline/block HTML is preserved as an `html` node verbatim — we do not sanitize or strip it, but we also do not parse into it. Math (`$...$`, `$$...$$`) and MDX are **explicit non-goals for v1**. TOML frontmatter is a non-goal for v1.
    **PO decision**: GFM is the de facto modern default and is what users hitting a tool named `md2json` will reach for; YAML frontmatter is the single most common "the body alone is not enough" need (blog posts, static-site content); math/MDX add a parser-library tax disproportionate to v1 value.

2. **JSON output shape.** Option **(a) — a typed AST, using the `mdast` (unified/remark) node shape**, with frontmatter lifted out into a top-level `frontmatter` field for ergonomics. The top-level envelope is:
    ```json
    { "frontmatter": { ... } | null, "ast": <mdast root node> }
    ```
    For the input `# Title\n\nHello **world**.\n` (no frontmatter), the exact emitted JSON (pretty form shown for legibility; default output is compact, see Q3) is:
    ```json
    {
      "frontmatter": null,
      "ast": {
        "type": "root",
        "children": [
          {
            "type": "heading",
            "depth": 1,
            "children": [
              { "type": "text", "value": "Title" }
            ]
          },
          {
            "type": "paragraph",
            "children": [
              { "type": "text", "value": "Hello " },
              {
                "type": "strong",
                "children": [
                  { "type": "text", "value": "world" }
                ]
              },
              { "type": "text", "value": "." }
            ]
          }
        ]
      }
    }
    ```
    Position info (`position: { start, end }`) is included on every node by default; a `--no-position` flag can strip it (see Q3).
    **PO decision**: `mdast` is the most-tooled, best-documented Markdown AST in the ecosystem — picking it gives downstream consumers a stable, documented schema and lets us defer to an existing parser library rather than inventing a node taxonomy. Outline/frontmatter-only/link-extraction are lossy; the full AST is the superset from which any of those views can be derived by a one-liner.

3. **CLI contract.** Unix-filter-first:
    - **Default invocation**: `md2json [FILE]`. If `FILE` is given, read from that path; if omitted (or `-`), read from stdin. JSON is always written to **stdout**. Errors go to **stderr** with a non-zero exit code.
    - **One file at a time** for v1. Multiple files / directory / glob input are **non-goals for v1** (users can shell-loop; we will not invent a multi-doc envelope yet).
    - **Exit codes**: `0` success; `1` parse error (malformed input we genuinely cannot recover from — note that GFM is famously permissive, so this is rare); `2` usage error (bad flag, missing file); `64`+ reserved.
    - **Error format**: human-readable line on stderr (`md2json: <path>:<line>:<col>: <message>`). No `{"error": ...}` JSON-on-stdout fallback — stdout stays clean for piping.
    - **v1 flags**:
        - `-o, --output <FILE>` — write to file instead of stdout.
        - `--pretty` — pretty-print JSON (2-space indent). Default is compact (single-line) for pipe friendliness.
        - `--no-position` — drop `position` fields from nodes.
        - `--frontmatter-only` — emit only the `frontmatter` field's value (or `null`), skip parsing the body. Useful and cheap because we already lifted it.
        - `-h, --help`, `-V, --version` — standard.
    - **Not in v1**: `--schema` (the schema *is* mdast — we document the version we target), watch mode, config file.
    **PO decision**: Unix filter (`md2json < in.md > out.json`) is the default ergonomics most CLI users expect from a `*2*` tool; positional `FILE` is a convenience that does not break it. Single-file-only keeps the output contract trivially one-JSON-document-per-invocation, which composes cleanly with `xargs`/`find` for multi-file needs without us inventing a multi-doc schema.

4. **Language, runtime, distribution.** Implementation language: **Go**. Markdown parser library: **`github.com/yuin/goldmark`** (CommonMark-compliant, actively maintained, GFM extensions available as official extension packages, has a documented AST we can walk and serialize to the mdast-shaped JSON in Q2). YAML frontmatter via **`gopkg.in/yaml.v3`** (or `goldmark-meta` if it gives us the right hook; PO open to either as an implementation detail).
    Distribution:
    - Primary install: **`go install github.com/<owner>/md2json@latest`**.
    - Secondary: **prebuilt single static binaries** published as GitHub Release assets for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`.
    - Homebrew tap is a post-v1 nice-to-have, not v1.
    **PO decision**: Go gives us (i) a single static binary with zero runtime dependency — best CLI ergonomics, no Node/Python install gymnastics; (ii) `goldmark` is the strongest GFM parser in the Go ecosystem and is what the official `gohugo` static-site generator uses, so its GFM behavior is battle-tested; (iii) `go install` + GitHub Release binaries cover ~all install paths without the maintenance overhead of a Homebrew formula on day one.
    Note: the mdast-shaped JSON in Q2 is the *output contract* — we are translating goldmark's internal AST into the documented mdast node shape on emit, so users get a schema documented elsewhere rather than goldmark's Go-flavored AST.

5. **Why "md2json" — and what is the success criterion?** This is **not a rewrite of any specific predecessor**; the `2` suffix is a name-disambiguation choice because `md2json` is a crowded name on npm/PyPI/GitHub and we want a clean, unambiguous binary name. What `md2json` does that `pandoc -t json`, `remark-parse` (via Node), and `marked` do not: **emits an mdast-shaped JSON for a GFM-with-frontmatter input as a single static Go binary, with no Node/Python/Haskell runtime required** (pandoc needs Haskell-runtime-free binaries but emits its own pandoc-AST shape, not mdast; remark/marked require Node).
    **Single-sentence v1 ship criterion**: *Running `md2json < post.md` on a typical GFM blog post with YAML frontmatter prints, to stdout, a valid JSON document with a top-level `frontmatter` object and an `ast` field conforming to the documented mdast subset, exiting 0; running it on the empty input prints `{"frontmatter":null,"ast":{"type":"root","children":[]}}` and exits 0.*
    **PO decision**: Anchoring "ship" to a single observable command + output pair (rather than a feature checklist) gives `to-prd` a concrete acceptance test it can decompose into issues; mdast-as-output + Go-single-binary is the niche-shaped wedge that justifies a new tool versus the existing options.

### VERDICT: continue

## Round 2

### Interviewer

Round 1 nailed the big shape (GFM + YAML frontmatter → mdast-shaped JSON envelope, Go + goldmark, Unix-filter CLI, single-binary distribution, single-sentence ship criterion). I've written those into `CONTEXT.md`. Five edges remain where `to-prd` cannot draft without an explicit decision — please pick one option per question.

1. **Invalid / malformed YAML frontmatter — exit-1 or warn-and-degrade?** The input has `---` fences at the top, but the YAML between them does not parse (tab indentation, unbalanced quotes, duplicate keys, etc.). Three options: (a) **hard error**: write `md2json: <path>:<line>:<col>: invalid frontmatter: <yaml error>` to stderr, exit `1`, no stdout. (b) **soft fallback**: emit `{"frontmatter": null, "ast": <body parsed as usual, including the `---` lines as a thematic break / paragraph>}`, exit `0`, optional stderr warning. (c) **soft fallback with raw**: emit `{"frontmatter": {"_raw": "<original yaml string>", "_error": "<msg>"}, "ast": ...}`, exit `0`. Pick one, and confirm whether `--frontmatter-only` follows the same rule. Also: when the document opens with `---` but never closes, is that frontmatter-with-no-body, or no-frontmatter-with-a-thematic-break? Pick one.

2. **Error-format `:line:col:` when goldmark gives no column (and the stdin path token).** Many goldmark error sites do not carry a column — only a line — and some are document-scoped (no position at all). Two sub-questions: (i) what do we print when `col` is unknown — `<path>:<line>:` (omit `:col:` entirely), `<path>:<line>:0:`, or `<path>:<line>:1:`? (ii) When input is stdin, is `<path>` the literal string `stdin`, `-`, the empty string (`md2json: :3:5: ...`), or `<stdin>`? Pin both so the format is one regex, not three.

3. **`position` field shape and the empty-input acceptance JSON.** You wrote the ship criterion as `{"frontmatter":null,"ast":{"type":"root","children":[]}}` — note no `position` on the root. Two questions: (i) does the root node carry a `position` (and on empty input, what is it — `{start:{line:1,column:1,offset:0}, end:{line:1,column:1,offset:0}}`)? (ii) The acceptance string in the ship criterion: does it hold **always**, or only **under `--no-position`**? If it must hold by default, then root-on-empty-input has no `position` (i.e. `position` is omitted on the root specifically when children are empty), which is a real edge. Pick: default-omits-position-on-root-when-empty, default-includes-zero-width-position-on-root, or the acceptance string is `--no-position`-conditional.

4. **goldmark → mdast translation: which mdast node types are in v1, and what's the lossiness policy for nodes goldmark produces that mdast does not name (or vice versa)?** Concretely please confirm the in-scope mdast types: `root`, `paragraph`, `heading{depth}`, `text{value}`, `emphasis`, `strong`, `delete` (GFM strikethrough), `inlineCode`, `code{lang, meta}`, `blockquote`, `list{ordered, start, spread}`, `listItem{checked|null, spread}` (task-list checkbox via `checked`), `thematicBreak`, `link{url, title}`, `image{url, title, alt}`, `linkReference`/`imageReference`/`definition` (or are reference-style links flattened to inline `link`?), `html{value}`, `table{align[]}` + `tableRow` + `tableCell`, `footnoteDefinition`/`footnoteReference` (GFM-style), `break` (hard break). For each pair where goldmark and mdast disagree (e.g. goldmark's autolink vs mdast `link`, task-list checkbox state, table cell alignment per-cell vs per-column), commit to which side we emit. **And**: any goldmark node we do not have an mdast equivalent for — drop silently, emit as `html`, or emit a `{"type":"unknown", "raw":"..."}` escape hatch? Pick one global policy.

5. **Input size, encoding, BOM, line endings.** v1 reads the whole document into memory (no streaming) — confirm or override. (i) Is there a max input size that triggers an error (e.g. 100 MB), or do we trust the OS? (ii) Required input encoding: UTF-8 only, or also accept UTF-16 / latin-1? Behavior on invalid UTF-8 bytes — exit 1, or replace with U+FFFD and continue? (iii) Leading UTF-8 BOM: strip silently, preserve in first text node, or error? (iv) CRLF vs LF line endings: normalize to LF before parsing (so `position` offsets are LF-relative), or preserve and let column counts reflect CRLF? These four are the kind of detail a TDD stage will absolutely write a test against, so we need them committed in the PRD.

### PO

1. **Invalid / malformed YAML frontmatter.** Pick **(a) hard error**. If the input opens with a `---` fence and we therefore commit to "this document has frontmatter," then malformed YAML between the fences is exactly the "malformed structured input" case where fail-fast is right: write `md2json: <path>:<line>:<col>: invalid frontmatter: <yaml error>` to stderr, exit `1`, nothing on stdout. `--frontmatter-only` follows the **same rule** — same stderr line, same exit `1`, no stdout — because the failure is upstream of which view we asked for. **Unclosed-`---` case**: if the document opens with `---` on line 1 but the closing `---` never appears, that is **not** frontmatter — it is a body that begins with a thematic break (and possibly a setext heading underline, depending on what follows). The closing fence is mandatory; without it we never entered frontmatter mode. So no frontmatter error fires; the document parses as body-only, `frontmatter: null`, exit `0`.
    **PO decision**: matches Round 1's fail-fast-on-malformed-structured-input posture; a soft fallback with `_raw`/`_error` invents a second envelope schema the consumer must defensively check, contradicting "the schema is mdast, document the version." The unclosed-fence rule mirrors how `goldmark-meta` and Hugo treat it, and avoids us having to invent "implicitly closed at EOF" semantics.

2. **Error-format `:line:col:` shape.**
    (i) When `col` is unknown, print **`<path>:<line>:1:`** — always emit three colon-separated numeric-ish fields so the format is exactly one regex: `^md2json: ([^:]+):(\d+):(\d+): (.+)$`. We round an unknown column **up to 1**, not 0, because lines are 1-indexed everywhere else in the output and `0` would falsely imply "byte before the line start." When position is entirely document-scoped (no line either), use `<path>:0:0:` as the sentinel — the same regex still matches, and `0:0` is unambiguously "no position available" by convention.
    (ii) When input is stdin, `<path>` is the literal string **`-`**. Not `stdin`, not `<stdin>`, not empty. Rationale: `-` is the convention the CLI itself already uses for "read stdin" (`md2json -` is equivalent to no positional arg), so the same token round-trips between invocation and diagnostic. `<stdin>` with angle brackets confuses shell users who try to grep/sed the path. Update CONTEXT.md's `Error format` entry next Cycle to drop the "or the literal `stdin`" alternative — `-` is the single canonical form.
    **PO decision**: one regex over the entire stderr surface is worth the small ugliness of `:1:` and `:0:0:` sentinels; this is precisely the kind of detail TDD will write a fixture against, so it pays to pin it now. Picking `-` over `stdin` keeps the diagnostic token identical to the CLI input convention.

3. **`position` on root + empty-input acceptance JSON.**
    (i) The root node carries a `position` field by default, **including on empty input**, with the zero-width value `{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}`. No special-casing — the rule "every node has `position` unless `--no-position`" holds uniformly.
    (ii) Therefore the ship-criterion string `{"frontmatter":null,"ast":{"type":"root","children":[]}}` holds **only under `--no-position`** (or under any future "no-position by default" reversal, which we are not doing for v1). Update the v1 ship criterion in CONTEXT.md next Cycle to read: *"Running `md2json --no-position < empty.md` prints exactly `{"frontmatter":null,"ast":{"type":"root","children":[]}}` and exits 0; running `md2json < empty.md` (default) prints the same envelope with a zero-width `position` field on the root and exits 0."*
    **PO decision**: uniform "always has `position`" is far simpler to implement, document, and test than "has `position` except when…"; the cost is that the literal acceptance string needs the `--no-position` qualifier, which is a one-flag delta and worth the schema regularity. A consumer that wants the terse form can ask for it explicitly.

4. **goldmark → mdast node-type whitelist + lossiness policy.** Confirm the v1 mdast node set as you listed it, with the following pin-downs:
    - **Confirmed in-scope, exactly as named**: `root`, `paragraph`, `heading{depth}` (depth 1–6), `text{value}`, `emphasis`, `strong`, `delete` (GFM strikethrough), `inlineCode{value}`, `code{lang, meta, value}` (fenced and indented; `lang` and `meta` are `null` for indented), `blockquote`, `list{ordered, start, spread}`, `listItem{checked, spread}` (`checked` is `true`/`false` for GFM task items, `null` otherwise), `thematicBreak`, `link{url, title}`, `image{url, title, alt}`, `html{value}` (both block and inline raw HTML emit as `html` — mdast does not distinguish), `table{align}` + `tableRow` + `tableCell`, `footnoteDefinition{identifier, label}` + `footnoteReference{identifier, label}`, `break` (hard line break, i.e. trailing two-space or `\`-escaped newline).
    - **Reference-style links/images**: emit the mdast triple `linkReference{identifier, label, referenceType}` / `imageReference{identifier, label, referenceType}` / `definition{identifier, label, url, title}`. Do **not** flatten to inline `link`/`image`. Rationale: flattening loses the definition's source position and the ability to round-trip; downstream consumers (and any future serializer-back-to-markdown) need the reference form preserved.
    - **GFM autolinks** (e.g. bare `https://…` or `<https://…>`): emit as **`link{url, title:null}`** with the URL also as the child `text` node's `value`, matching `mdast`'s standard autolink representation. Goldmark's distinct autolink node collapses into mdast `link` on translation. This is a one-side commitment (we follow mdast); goldmark's `AutoLink` type is an implementation detail.
    - **Table cell alignment**: mdast carries `align` as an array on the `table` node (per-column, one of `"left"|"right"|"center"|null`), **not** per-cell. We follow **mdast**: alignment is per-column on `table.align`; `tableCell` itself carries no `align` field. Goldmark's per-cell alignment info is collapsed to the column array on translation (all cells in a column are required by GFM to agree on alignment, so this is lossless in practice).
    - **Task-list checkbox**: lives on `listItem.checked` (`true`/`false`/`null`), not as a child node. Goldmark exposes it as a `TaskCheckBox` child; we hoist that into `listItem.checked` and drop the child node. Following mdast.
    - **Frontmatter**: not an mdast node at all. Lifted into the envelope's `frontmatter` field (per CONTEXT.md). The AST starts at `root`'s first non-frontmatter child.
    - **YAML/HTML/other front-of-document blocks goldmark may emit as nodes we have not whitelisted**: never appear in the AST because the frontmatter strip happens before AST translation.
    - **Global "no mdast equivalent" policy**: **drop silently** and log nothing. No `{"type":"unknown",...}` escape hatch, no `html` fallback dump. Rationale: the wire contract is "documented mdast subset"; emitting an undocumented type would violate that contract and force every downstream consumer to defensively handle it. The set of goldmark constructs without an mdast equivalent in our enabled extension set is effectively empty (we are deliberately enabling only GFM + frontmatter + footnotes, all of which map cleanly). If a future extension introduces a real gap, that is a v2 schema-extension conversation, not a v1 escape hatch.
    **PO decision**: prefer mdast on every disagreement because mdast is the documented wire contract — goldmark is implementation. Silent-drop over `unknown`-fallback because v1's surface should be small and contract-stable; an escape hatch invites consumers to depend on it and turns "v1.x adds a real node type" into a breaking change. Reference-style preservation is the one place we explicitly resist flattening, because the v1 ship criterion is "valid mdast" and mdast specifies the reference triple. Marking this whole node-type set as the v1 **mdast subset**, to be enumerated as a list in the PRD; the PRD/Critic should treat this enumeration as the authoritative schema for TDD test fixtures.

5. **Input size, encoding, BOM, line endings.**
    - **Streaming**: confirm — v1 reads the **whole document into memory**, no streaming. Goldmark itself does not stream, and the JSON output is a single document.
    - (i) **Max input size**: **no hard cap in v1**; trust the OS / Go's allocator. Rationale: a hard cap is a v1 line in the sand we cannot pick well without usage data, and an OOM under truly pathological input is acceptable behavior for a single-shot CLI filter. Document this as "v1 is suitable for documents up to ~tens of MB; multi-hundred-MB inputs are out of scope." **PO decision**: deferring a size-cap flag to post-v1 because picking a number now is guesswork — wait for a real "this OOM'd me" report before adding `--max-size`.
    - (ii) **Required input encoding**: **UTF-8 only**. UTF-16 / latin-1 are **non-goals for v1** — users with non-UTF-8 input run it through `iconv` first. **Behavior on invalid UTF-8 bytes**: **exit 1** with `md2json: <path>:<line>:<col>: invalid utf-8 byte at offset <N>` on stderr. We do **not** silently replace with U+FFFD — that is data corruption masquerading as success, and the fail-fast-on-malformed-structured-input rule applies. (CommonMark itself technically allows arbitrary bytes; we are choosing to be stricter than the spec for v1 ergonomics.)
    **PO decision**: UTF-8-only is the modern CLI default; replacing with U+FFFD would let a partially-corrupt file produce a partially-corrupt AST that looks fine, which is the worst kind of bug for a tool whose job is structured output.
    - (iii) **Leading UTF-8 BOM**: **strip silently** before any further processing. The BOM is a transport-layer artifact, not document content; if we preserved it in the first text node, every downstream consumer would have to special-case it. Stripping is what goldmark callers in the Hugo/static-site-generator world conventionally do. The stripped byte does shift `position.offset` for the first node by `-3` relative to the raw file — document this in the PRD as "byte offsets are relative to the post-BOM-strip document."
    - (iv) **CRLF vs LF**: **normalize CRLF → LF before parsing**. `position` line counts therefore reflect logical lines (which is what users expect: line 3 in their editor is line 3 in our error message regardless of line-ending style); `position.column` counts UTF-8 code points within the normalized line; `position.offset` is the byte offset into the **normalized** (LF-only) document, not the raw file. Document the normalized-offset rule in the PRD. **PO decision**: normalizing keeps `position` cross-platform-stable, which matters for users on Windows whose editors save CRLF — without normalization a Mac CI and a Windows dev would disagree on column counts, which is a debugging nightmare.

### VERDICT: continue

## Round 3

### Interviewer

Closing out grill-0. Across two Rounds PO has pinned: GFM + YAML frontmatter as the input dialect, an mdast-shaped JSON envelope as the wire contract, Go + goldmark as the implementation, Unix-filter ergonomics with a single canonical stderr regex, fail-fast-on-malformed-structured-input as the global posture (invalid frontmatter, invalid UTF-8, both exit 1), a closed enumerated **mdast node-set v1** with explicit per-disagreement commitments (reference-style preserved, autolink collapsed to `link`, table alignment per-column on `table.align`, task checkbox hoisted onto `listItem.checked`), silent-drop as the lossiness policy (no `unknown` escape hatch), and the full input-handling stack (whole-doc in memory, no size cap, UTF-8-only with no U+FFFD substitution, BOM stripped silently, CRLF normalized to LF before parsing, `position.offset` relative to the normalized document). All Round 2 commitments are reconciled into `CONTEXT.md` inline — the `Frontmatter`, `Position info`, `CLI contract`, `Error format`, and `v1 ship criterion` entries are updated, and new entries are added for `Invalid frontmatter (policy)`, `mdast node-set v1`, `Lossiness policy (goldmark → mdast)`, and `Input handling`. The input-encoding/normalization decisions are architectural enough to deserve a discoverable record, so they are also pinned as `docs/adr/0001-input-encoding-and-normalization.md`.

`CONTEXT.md` is sufficient for `to-prd` to draft a PRD. Open edges I am deliberately *not* re-grilling — `to-prd` can either decide them as a PRD detail or fire `trigger-grill` if it hits real ambiguity:

- the exact shape `--frontmatter-only` emits when frontmatter is a YAML scalar (string/number/null) rather than a mapping;
- the interaction between `--pretty` and `--no-position` (both flags compose, but the exact pretty-printer behavior — key ordering, whether `null` fields are elided — is a PRD-level call);
- the YAML library choice (`gopkg.in/yaml.v3` vs `goldmark-meta`) — PO marked this as an implementation detail in Round 1, which I am respecting;
- `position.line` semantics for an input that is exactly one trailing newline vs zero bytes (the empty-input case is pinned; "newline only" is not explicitly pinned but is recoverable from the uniform position rule);
- whether `image.alt` is the raw inline text or a flattened string (mdast says string; goldmark exposes inline children — a translation detail).

None of these block PRD drafting; all of them have an obvious "PO would say X if asked" default rooted in commitments already made.

### PO

### VERDICT: accept
