# TDD log: 01-wire-math-extension

Started: 2026-05-23

## Test framework

Existing: Go stdlib `testing` (per-package unit tests + black-box integration tests via `testdata/fixtures/<name>/`).

## Pre-Run baselines captured

- Full suite: `go test ./...` — all packages green.
- Module-level cgo deps: `grep -l cgo $(go list -deps -f '{{.Dir}}' ./...) | grep -v "$GOROOT"` — empty set (zero non-stdlib cgo hits).

## Test 1 — Tracer bullet: parse.New registers math extension

- Wrote: `TestParseRegistersMathExtension` in `internal/parse/parse_test.go`.
  Asserts that `Parse([]byte("$x$"))` produces a goldmark AST containing a
  node whose `Kind().String() == "InlineMath"`. The kind name is the public
  contract of `github.com/litao91/goldmark-mathjax`
  (probe/goldmark-mathjax/block_inline.go:28 — `KindInlineMath = ast.NewNodeKind("InlineMath")`).
  Asserts via kind-string rather than direct type-import to stay resilient
  to library struct-name churn while still pinning the load-bearing
  contract: the math extension IS wired into the standard set, no flag.
  Helper `containsNodeKind` walks the AST with a small recursive
  preorder traversal.
- Red: pre-wiring run — output `parse_test.go:240: Parse("$x$"): no goldmark node of kind "InlineMath" found in the AST; the math extension is not wired into parse.New (S01 acceptance criterion #2)`.
- Green: added `github.com/litao91/goldmark-mathjax` as a direct dep via
  `go get github.com/litao91/goldmark-mathjax`, then appended
  `mathjax.NewMathJax()` to the extension list inside `newGoldmarkWith`
  in `internal/parse/parse.go`. Registration order preserved: GFM →
  Footnote → math → caller-supplied extras (frontmatter is a
  caller-supplied extra in the `New` path; absent in
  `newWithoutFrontmatter`). Capacity hint on the slice bumped from
  `2+len(extras)` to `3+len(extras)`. Comment block on `newGoldmarkWith`
  updated to cite ADR-0004 Decisions 1+2 and to note S02/S03 will add
  the translate-layer mapping.
- Notes: Per ADR-0004 the wiring uses the library's `Renderer` side
  too (since `Extend` calls `m.Renderer().AddOptions(...)`), but md2json
  never invokes goldmark's renderer — the renderer registration is
  inert in this codepath (same pattern as GFM / Footnote / Frontmatter
  per ADR-0002). The wire delta for non-math inputs is zero (no
  `*ast.InlineMath` / `*ast.MathBlock` nodes appear, so the translate
  `default: return nil` arm never fires for math types on the existing
  corpus).

## Test 2 — Smoke fixture: byte-identical envelope before vs after wiring

- Wrote: `testdata/fixtures/61-smoke-non-math-gfm-blog-post-nopos/`.
  Inputs: YAML frontmatter (closed-fence, multi-key, list-value) +
  heading + two paragraphs (with `*em*` and `**strong**`) + unordered
  three-item list + fenced Go code block. Flags: `--no-position`
  (eliminates position-offset noise; the acceptance bullet "byte-identical
  JSON envelope" is about the wire schema and node-tree shape, not about
  source position digits which are tested by the position-suite slices).
  Hooks into the existing `TestFixtures` harness — no harness change.
- Red: n/a. The smoke fixture is a regression guard (acceptance bullet
  #4: "byte-identical JSON envelope before and after this issue"), not
  a behavior-adding test. To pin the "before == after" claim I built
  the binary against a stashed pre-wiring `parse.go`, captured stdout,
  then built against post-wiring `parse.go` and diffed:

  ```
  diff /tmp/pre_wire_out.json /tmp/post_wire_out.json && echo BYTE-IDENTICAL
  BYTE-IDENTICAL
  ```

  The captured envelope was installed as the fixture's `stdout` golden,
  so any future regression that perturbs the non-math wire shape
  (extension priority churn, accidental translate-side coupling, etc.)
  surfaces here.
- Green: post-wiring run of `go test . -run 'TestFixtures/61-smoke'`
  passes against the golden.
- Notes: I considered ALSO adding a position-bearing (default-flags)
  variant of this fixture. Decided against in this slice: the existing
  fixture suite (12-h1-hello-nopos, 18-h1-hello-with-position, plus
  many other `*-nopos` siblings) already covers the position-bearing
  envelope shape on every constituent node type. Adding a second
  smoke variant for this slice would pin position offsets too
  defensively — any benign source-position-tracking change in goldmark
  would force a fixture update without indicating a real regression.
  The `--no-position` smoke envelope is the right granularity for the
  acceptance bullet.

## Acceptance check

- [x] **#1 direct dep**: `go.mod` line 6: `github.com/litao91/goldmark-mathjax v0.0.0-20210217064022-a43cf739a50f`.
- [x] **#2 math extension registered, no flag**: `mathjax.NewMathJax()`
  appended unconditionally in `newGoldmarkWith` (`internal/parse/parse.go`).
  No new flag in `cli`, no environment variable read, no opt-in. Test
  `TestParseRegistersMathExtension` pins this.
- [x] **#3 pre-existing test suite passes unchanged**: `go test ./...`
  green across all six packages (md2json root, internal/cli,
  internal/emit, internal/parse, internal/read, internal/translate).
  No existing test modified — only additions.
- [x] **#4 smoke fixture byte-identical**: `testdata/fixtures/61-smoke-non-math-gfm-blog-post-nopos/`
  golden produced by pre-wiring binary; post-wiring binary matches it
  byte-for-byte (verified via stash + diff; also fixture harness
  green).
- [x] **#5 no new cgo dep**: `grep -l cgo $(go list -deps -f '{{.Dir}}' ./...) | grep -v "$GOROOT"`
  returns empty post-wiring — same as pre-Run baseline (zero non-stdlib
  cgo hits). `go mod tidy` is a no-op after wiring (the `go.mod` +
  `go.sum` adds for `litao91/goldmark-mathjax` are tidy's expected output).

## Final

- Tests added: 2 (one Go unit test in `internal/parse`, one black-box
  integration fixture in `testdata/fixtures/`).
- Tests passing: 2/2 of the new tests + every pre-existing test in the
  suite.
- Files touched:
  - `internal/parse/parse.go` — import `mathjax`, append `mathjax.NewMathJax()` in `newGoldmarkWith`.
  - `internal/parse/parse_test.go` — add `TestParseRegistersMathExtension` + `containsNodeKind` helper + `ast` import.
  - `go.mod`, `go.sum` — `go get github.com/litao91/goldmark-mathjax` + `go mod tidy`.
  - `testdata/fixtures/61-smoke-non-math-gfm-blog-post-nopos/{args,input.md,stdout,stderr,exit}` — smoke fixture.
- Commits: none in this Stage (per Rundays orchestrator-protocol,
  agents do not commit; the parent Stage handler commits when each
  Stage closes).
- VERDICT: accept.
