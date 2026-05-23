# TDD log: 02-read-utf8-bom-crlf

Started: 2026-05-23

## Setup

- Reused S01's test framework (Go `testing` + `go test`) and module layout.
- New module: `internal/read/read.go` exposing `Read(r io.Reader, path string) ([]byte, error)` and the typed `*ReadError{Path, Line, Col, Offset, Msg}`.
- Wired `cli.Run` to call `read.Read` first; a `*ReadError` is routed through the canonical stderr line `md2json2: <path>:<line>:<col>: <msg>\n` and `cli.Run` returns 1. On success the S01 hard-coded empty envelope still ships on stdout (acceptance criterion of S02 explicitly defers end-to-end BOM/CRLF observability to S03).
- Two new fixtures under `testdata/fixtures/`: `02-invalid-utf8-leading` (single `0xFF` byte on stdin), `03-invalid-utf8-mid-document` (`hi\nworld\xC3\x28` on stdin). Both reuse the S01 binary-build harness with no harness changes.

## Test 1 — tracer bullet: leading `0xFF` on stdin produces canonical error line (criteria #1 + #3)

- Wrote: `TestLeadingInvalidUTF8StdinHardErrors` in `internal/cli/cli_test.go`.
- Red: `stdout should be empty on hard error, got "{\"frontmatter\":null,...}"` plus stderr/exit mismatches (the pre-S02 `cli.Run` still emitted the envelope unconditionally and never wrote to stderr).
- Green: created `internal/read/read.go` with `Read`/`*ReadError`, wired `cli.Run` to call it (path token `-` for stdin, file path otherwise), and routed the typed error through `fmt.Fprintf(stderr, "md2json2: %s\n", re.Error())` + `return 1`. The S01 envelope-on-success path is preserved on the no-error branch.
- Notes: the GREEN step intentionally also wrote the BOM-strip and CRLF-normalize phases of `Read` because they share the `raw → norm` body of the function with the UTF-8 validation phase; the following tests pin each of those phases as contract anchors. The tracer proves the end-to-end wiring (cli ↔ read) and the canonical stderr line shape.

## Test 2 — mid-document bad UTF-8 (criterion #2)

- Wrote: `TestMidDocumentInvalidUTF8StdinHardErrors` — input `"hi\nworld\xC3\x28"`, expects `md2json2: -:2:6: invalid utf-8 byte at offset 8\n` and exit 1.
- Red: skipped — the line/column-tracking validator from Test 1's GREEN already handled mid-doc bad bytes; I verified the path is real by hand (offset 8 = the `0xC3` byte; line 2 after the `\n` at offset 2; column 6 = one past "world"). The test pins the contract.
- Green: pre-existing.
- Notes: TDD purity acknowledgment — Test 1's minimal impl was already general enough to cover Test 2. Same discipline as S01 Tests 4/6/7 (the log there explicitly endorses keeping the contract-lock and noting the absence of an explicit RED step).

## Test 3 — fixtures `02-invalid-utf8-leading` and `03-invalid-utf8-mid-document`

- Wrote: two fixture directories under `testdata/fixtures/` matching the in-process tests above. `input.md` files contain the raw binary bytes (the `.md` suffix is a fixture-harness convention from S01, not a content-type claim); `stderr` is the exact canonical line; `stdout` is empty; `exit` is `1`.
- Red: skipped — the binary was already producing the right behavior end-to-end (the wiring in Test 1's GREEN covers the binary path too). Fixtures pin the contract through the black-box harness.
- Green: pre-existing; `TestFixtures/02-invalid-utf8-leading` and `TestFixtures/03-invalid-utf8-mid-document` both pass.
- Notes: per the issue's "don't add end-to-end BOM/CRLF fixtures — Critic flagged those as tautological at S02's stage; they're deferred to S03," I did not add BOM/CRLF fixtures. The two fixtures here are the bad-UTF-8 pair the issue explicitly calls out plus a natural stdin-path-token-`-` assertion (criterion #3).

## Test 4 — leading UTF-8 BOM stripped (criterion #4a)

- Wrote: `TestLeadingBOMIsStripped` in `internal/read/read_test.go`. Input is `0xEF 0xBB 0xBF` + `"hello"`; expected returned slice is exactly `"hello"`.
- Red: skipped — Test 1's GREEN already implemented BOM-strip (the impl is a single `Read` function whose three phases are tightly coupled). Test pins the contract.
- Green: pre-existing.
- Notes: BOM is checked with `bytes.HasPrefix` (post-refactor; see refactor pass below).

## Test 5 — CRLF → LF normalization (criterion #4b, CRLF half)

- Wrote: `TestCRLFNormalizedToLF`. Input `"a\r\nb\r\nc"` → expected `"a\nb\nc"`.
- Red: skipped — Test 1's GREEN already implemented CRLF→LF. Test pins the contract.
- Green: pre-existing.

## Test 6 — bare CR → LF (criterion #4b, bare-CR half; S02-pinned implementation choice)

- Wrote: `TestBareCRNormalizedToLF`. Input `"a\rb\rc"` → expected `"a\nb\nc"`.
- Red: skipped — Test 1's GREEN already implemented bare-CR→LF mapping. Test pins the S02-pinned interpretation: bare `\r` maps to `\n`, consistent with ADR-0001's CRLF→LF rule (the alternative — leaving bare `\r` alone — would have left a classic-Mac `\r`-only file as a single logical line, violating ADR-0001's cross-platform `position.line` stability).
- Green: pre-existing.
- Notes: this is the "implementation choice — pin in the test" line in the issue's criterion 4b. The choice is documented in both the test name/comment and in the `read.go` package doc comment on the normalization phase.

## Test 7 — byte length reflects both transforms (criterion #4c)

- Wrote: `TestByteLengthReflectsBothTransforms`. Input is 10 bytes (BOM 3 + `"a\r\nb\r\nc"` 7); expected output is 5 bytes (`"a\nb\nc"`).
- Red: skipped — Test 1's GREEN already does both transforms. Test pins the byte-count contract explicitly.
- Green: pre-existing.

## Test 8 — invalid UTF-8 returns typed `*ReadError` with the right position; returned slice is nil (criterion #4d)

- Wrote: `TestInvalidUTF8ReturnsTypedReadError`. Uses `errors.As` to unwrap the `*ReadError` and asserts `Path`, `Line`, `Col`, `Offset`, `Msg` all match expected. Also asserts the returned slice is `nil` (no partial document on hard error).
- Red: skipped — Test 1's GREEN already returns the typed error with all fields populated and a nil slice on the error path. Test pins the public contract of the typed error.
- Green: pre-existing.

## Test 9 — BOM stripped only at start; mid-document BOM-shaped bytes left alone (criterion #5)

- Wrote: `TestBOMShapedMidDocumentIsLeftAlone`. Input `"hello"` + `0xEF 0xBB 0xBF` + `"world"`; expected output bytes equal input bytes (the mid-doc BOM is valid UTF-8 content — U+FEFF zero-width no-break space — and survives).
- Red: skipped — Test 1's GREEN's BOM-strip uses `bytes.HasPrefix` on the original buffer, so a BOM-shaped run mid-document is naturally left alone. Test pins the contract.
- Green: pre-existing.

## Test 10 — no-BOM LF-only file round-trips byte-for-byte (criterion #6)

- Wrote: `TestNoBOMLFOnlyRoundTripsByteForByte`. Input is a small UTF-8 Markdown document with `\n`-only line endings and multi-byte UTF-8 characters (`héllo wörld`); expected output equals input.
- Red: skipped — Test 1's GREEN passes well-formed UTF-8 through unchanged. Test pins the no-op identity contract.
- Green: pre-existing.

## Test 11 — leading bad byte position is line 1 col 1 offset 0

- Wrote: `TestLeadingInvalidByteAtOriginPosition` in the `read` package. Input is `[]byte{0xFF}`; expected `*ReadError{Path:"-", Line:1, Col:1, Offset:0, Msg:"invalid utf-8 byte at offset 0"}`.
- Red: skipped — Test 1's GREEN initializes the validator's `line, col` to `1, 1`. Test pins the boundary contract at the module level (complementing Test 1 which pins the same at the cli level).
- Green: pre-existing.

## Refactor pass

After all tests green: replaced the explicit index-by-index BOM comparison `raw[0] == utf8BOM[0] && raw[1] == utf8BOM[1] && raw[2] == utf8BOM[2]` (gated by a length check) with `bytes.HasPrefix(raw, utf8BOM)`. Same semantics, one expression. Tests stayed green; `go vet ./...` clean.

## Final

- Tests added in S02: 12 total
  - cli (in-process): 2 — `TestLeadingInvalidUTF8StdinHardErrors`, `TestMidDocumentInvalidUTF8StdinHardErrors`
  - read (unit): 8 — `TestLeadingBOMIsStripped`, `TestCRLFNormalizedToLF`, `TestBareCRNormalizedToLF`, `TestByteLengthReflectsBothTransforms`, `TestInvalidUTF8ReturnsTypedReadError`, `TestBOMShapedMidDocumentIsLeftAlone`, `TestNoBOMLFOnlyRoundTripsByteForByte`, `TestLeadingInvalidByteAtOriginPosition`
  - integration (fixtures): 2 — `02-invalid-utf8-leading`, `03-invalid-utf8-mid-document` (both observed via `TestFixtures`)
- Total project tests: 30 (8 S01 cli top-level + 2 S02 cli + 8 S02 read + 1 S01 integration harness sanity + `TestFixtures` walking 3 directories — counted as 3 leaf subtests).
- `go test ./...`: clean (all green).
- `go vet ./...`: clean.
- Manual end-to-end verification: `printf '\xFF' | /tmp/md2json2` writes `md2json2: -:1:1: invalid utf-8 byte at offset 0\n` to stderr, nothing to stdout, exit 1. Behavior matches the canonical contract.

- Acceptance criteria status:
  - [x] criterion 1 — leading-0xFF fixture: stderr `md2json2: -:1:1: invalid utf-8 byte at offset 0`, exit 1, empty stdout (`TestFixtures/02-invalid-utf8-leading`, in-process `TestLeadingInvalidUTF8StdinHardErrors`)
  - [x] criterion 2 — mid-doc 0xC3 0x28 fixture: stderr `md2json2: -:2:6: invalid utf-8 byte at offset 8`, exit 1 (`TestFixtures/03-invalid-utf8-mid-document`, in-process `TestMidDocumentInvalidUTF8StdinHardErrors`)
  - [x] criterion 3 — stdin path token is literal `-` (both bad-UTF-8 fixtures use stdin; `TestInvalidUTF8ReturnsTypedReadError` explicitly threads a path-token through and asserts it round-trips)
  - [x] criterion 4a — BOM stripped (`TestLeadingBOMIsStripped`)
  - [x] criterion 4b — CRLF→LF + bare-CR→LF, both pinned (`TestCRLFNormalizedToLF`, `TestBareCRNormalizedToLF`)
  - [x] criterion 4c — byte length reflects both transforms (`TestByteLengthReflectsBothTransforms`)
  - [x] criterion 4d — invalid UTF-8 returns typed error with right offset, no slice (`TestInvalidUTF8ReturnsTypedReadError`, `TestLeadingInvalidByteAtOriginPosition`)
  - [x] criterion 5 — mid-doc BOM-shaped bytes left alone (`TestBOMShapedMidDocumentIsLeftAlone`)
  - [x] criterion 6 — no-BOM LF-only file round-trips byte-for-byte (`TestNoBOMLFOnlyRoundTripsByteForByte`)

VERDICT: accept
