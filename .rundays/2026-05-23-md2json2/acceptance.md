# Acceptance Gate

## Summary

Shipped a working `md2json2` Go CLI implementing the full v1 contract documented in `CONTEXT.md`:

- 13/13 queue issues marked `done` (`01-tracer-bullet-empty-envelope` through `12-release-pipeline-and-go-install`, plus the inserted `10a-pretty-print-and-property-test` slice).
- Production code organized as `main.go` + `internal/{cli,parse,translate,emit,read}` (~2571 LOC), with `cli.Run(argv, stdin, stdout, stderr) int` as the pure testable entry point and `main.go` as the only `os.Exit` site.
- Parser: `github.com/yuin/goldmark` with GFM + Footnote + YAML-frontmatter extensions wired through a centralized `newGoldmarkWith` helper (the final-arch pass materialized ADR-0002 §4's "no central registry" hazard into a private registry).
- Translator: goldmark Go-native AST → mdast-shaped Go structs covering the complete `mdast node-set v1` enumeration (root, paragraph, heading, text, emphasis, strong, delete, inlineCode, code, blockquote, list, listItem, thematicBreak, link, image, linkReference, imageReference, definition, html, table+tableRow+tableCell, footnoteDefinition, footnoteReference, break), with reference-style links preserved (not flattened), GFM task `checked` hoisted onto `listItem`, autolinks collapsed to `link{title:null}`, raw HTML byte-preserved.
- Emitter: deterministic JSON with stable key ordering, compact-by-default, `--pretty` 2-space indent, `--no-position` strip applied uniformly, no trailing newline on stdout.
- Position info: every emitted node (including `root`) carries `position{start,end{line,column,offset}}` by default; column counts UTF-8 code points in the normalized line; offset is byte-offset into the post-BOM-strip + LF-normalized document. Property test (`TestTranslateEveryEmittedNodeHasNonPlaceholderPosition`) closes the (0,0) placeholder leaks flagged in S07/S08.
- Frontmatter: lifted to envelope `frontmatter` field (not an AST node); unclosed-fence rule routes through a without-frontmatter goldmark instance; malformed YAML between closed fences = hard error with canonical `<path>:<line>:<col>: invalid frontmatter: <msg>` stderr + exit 1; scalar/string/number/null passthrough verified.
- Input handling: UTF-8 only, BOM-strip silent, CRLF→LF normalize, invalid UTF-8 = hard error matching the canonical regex.
- CLI: all v1 flags pinned (`-o/--output`, `--pretty`, `--no-position`, `--frontmatter-only`, `-h/--help`, `-V/--version`); exit codes 0/1/2 per spec; canonical stderr regex `^md2json2: ([^:]+):(\d+):(\d+): (.+)$` enforced via an integration property test that scans every fixture.
- Distribution: `internal/cli.Version` is ldflags-stampable; `.github/workflows/release.yml` exists with the five-platform matrix (`darwin/{amd64,arm64}`, `linux/{amd64,arm64}`, `windows/amd64`), `CGO_ENABLED=0`, `-trimpath`, `SHA256SUMS` step, and v1-ship-criterion smoke step on linux/amd64 + darwin/arm64. `go install ./...` path is unit-tested.
- Final-arch baseline: 83 PASS / 0 FAIL across `go test ./...`, `go vet ./...` clean; one refactor (Pass 1: centralize goldmark extension registry) shipped without churning tests.
- Product ADRs 0001 (input encoding), 0002 (goldmark extension libraries), 0003 (release pipeline) written; product `CONTEXT.md` is the load-bearing glossary.

## Decision

The Idea Brief reads in full: "A small CLI that convert markdown to Json." That seed was elaborated by the PO/Interviewer grill into `CONTEXT.md`'s 18-term glossary and a precise v1 ship criterion:

> Running `md2json2 < post.md` on a typical GFM blog post with YAML frontmatter prints, to stdout, a valid JSON document with a top-level `frontmatter` object and an `ast` field conforming to the documented mdast subset, exiting 0. On empty input: `md2json2 --no-position < empty.md` prints exactly `{"frontmatter":null,"ast":{"type":"root","children":[]}}` and exits 0; `md2json2 < empty.md` (default) prints the same envelope with a zero-width `position` field on the `root` and exits 0.

Both empty-input shapes of the ship criterion are pinned as byte-exact fixtures: `testdata/fixtures/10-empty-stdin-default/stdout` (default mode, with zero-width root position) and `testdata/fixtures/01-empty-stdin/stdout` (`--no-position` flavor). The "typical GFM blog post with YAML frontmatter" half is covered by the GFM coverage fixtures across S04–S07 (paragraphs, headings, emphasis/strong, lists, code, html, links, tables, task lists, strikethrough, autolinks) plus the frontmatter-lift fixtures from S09 (closed-fence map, scalar passthrough, unclosed-fence body-only, malformed-YAML hard error).

The CLI contract (stdin-first, single positional `FILE`, `-` sentinel, error-format regex, exit-code map) and the distribution contract (`go install`, five-platform release workflow, statically linked, `SHA256SUMS`) are both fully implemented and test-covered.

The final-arch pass (`final-arch-log.md` VERDICT: accept) walked the whole product_dir with a friction lens, found one Strong item (duplicated goldmark extension list in `parse.go`), shipped the fix, and re-ran tests green. The remaining Worth-Exploring / Speculative items are documented for future maintainers and explicitly judged style-only.

The mdast wire contract is the documented subset — goldmark's internal types are correctly treated as an implementation detail (ADR-0002 §4 + the lossiness policy in `CONTEXT.md`). The "silent drop" lossiness rule is consistently applied (no `unknown` node, no `html` fallback for non-HTML constructs).

No material gap surfaces against either the Idea Brief or `CONTEXT.md`'s v1 ship criterion. The shipped artifact is ship-ready.

**PO decision**: A v1.0.0 GitHub release tag (which would exercise the release workflow end-to-end) is a post-ship CI activity, not part of the build product, and the workflow itself is structurally tested. Treating that as ship-blocking would conflate "the code passes its own acceptance" with "the maintainer has cut a tag" — out of scope for this gate.

**PO decision**: The `10a-pretty-print-and-property-test` mid-stream insertion is a feature of the pipeline working correctly (a downstream slice surfaced a coverage gap, an issue was injected, it shipped, queue closed at 13/13). Not a gap, an artifact of normal Rundays operation.

## VERDICT

VERDICT: done
