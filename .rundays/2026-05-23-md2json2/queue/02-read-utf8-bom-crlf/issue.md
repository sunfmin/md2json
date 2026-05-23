# S02: Read module — UTF-8 validation, BOM strip, CRLF normalization

Status: ready-for-agent

The CLI grows a real input-reading stage that implements ADR-0001 in full: it reads the entire document into memory, validates the bytes are UTF-8 (returning a typed error on the first invalid byte, never substituting U+FFFD), strips a single leading UTF-8 BOM if present, and normalizes any CRLF line endings to LF before the bytes reach any downstream stage. Because the JSON envelope on stdout is still hard-coded by S01, the end-to-end observability of BOM strip and CRLF normalization is deferred to S03 (where the real pipeline observes the normalized bytes). What this slice can prove end-to-end is the bad-UTF-8 hard-error path — invalid bytes route through the canonical stderr line and exit `1` because the read stage rejects them before any envelope is emitted. The byte-level BOM/CRLF transforms are proven by **direct unit tests on the read module** in this slice; the end-to-end BOM/CRLF fixtures re-enter the suite in S03 once the real pipeline can observe their effects.

## What to build

The `read` module exposes a single entry point that takes an `io.Reader` and the `<path>` token to use in error messages, and returns either the normalized byte slice or a typed error. The typed error carries the offset of the first invalid byte (`offset 0` for a leading bad byte, the document offset otherwise). The `cli` module routes that error through the canonical stderr line shape `md2json2: <path>:<line>:<col>: invalid utf-8 byte at offset <N>` and exits `1` — `<path>` is the literal `-` for stdin and the file path otherwise. `<line>` and `<col>` track the position of the bad byte in the source: leading bad byte at line 1 column 1; mid-document is the line/column where the first invalid byte sits. The hard-coded envelope from S01 still ships on stdout for any input the read stage accepts (so the BOM/CRLF transforms are not yet end-to-end observable — that is S03's job).

## Acceptance

- [ ] A fixture with a file containing one invalid UTF-8 byte (e.g. `0xFF` standalone) produces stderr line `md2json2: <path>:1:1: invalid utf-8 byte at offset 0`, exit `1`, with nothing on stdout.
- [ ] A fixture with invalid UTF-8 mid-document (valid UTF-8 prefix then `0xC3 0x28`) produces the canonical `invalid utf-8 byte at offset <N>` stderr line where `<N>` is the byte offset of the first bad byte, exit `1`, nothing on stdout.
- [ ] When the input source is stdin, the `<path>` token in any read-stage error line is the literal `-`.
- [ ] Unit tests on the read module directly assert the byte-level transforms: (a) a leading UTF-8 BOM (`0xEF 0xBB 0xBF`) is stripped from the returned bytes — the returned slice does **not** include the BOM; (b) `\r\n` sequences in the input become single `\n` bytes in the returned slice; bare `\r` followed by non-`\n` is normalized to `\n` per ADR-0001's "normalize CRLF to LF" rule (single `\r` mapping is implementation choice — pin in the test whichever ADR-0001-consistent behavior the module adopts); (c) the byte length of the returned slice reflects both transforms (BOM bytes gone, each CRLF collapsed to one byte); (d) invalid UTF-8 returns a typed error with the right byte offset (no slice returned, or returned slice is irrelevant to caller).
- [ ] Unit test: the BOM is stripped only when at the very start of the input; a BOM-shaped byte sequence appearing mid-document is left alone (it is valid UTF-8 inside content).
- [ ] Unit test: a file with no BOM and only LF line endings round-trips through the read module unchanged byte-for-byte.

## Blocked by

S01 — needs the `cli` entry point and the integration-test harness in place.
