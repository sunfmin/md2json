# ADR-0001: v1 input encoding, BOM, line-ending, and size policy

- Status: Accepted
- Date: 2026-05-23
- Decider: PO (resolved in grill-0, Round 2)

## Context

`md2json2` reads a single Markdown document and emits a JSON envelope whose nodes carry `position` (line / column / offset). Several upstream decisions about *how the raw bytes become a parseable, position-stable document* shape every downstream guarantee — the position values, the cross-platform stability of `line`/`column`/`offset`, the error behavior on bad bytes, and the OOM surface. These decisions need to be discoverable to a future maintainer (and to TDD-stage test authors) as a single record with rationale, not scattered across a glossary.

The relevant questions, resolved in grill-0 Round 2:
- Streaming vs whole-document-in-memory?
- Is there a maximum input size?
- Which input encodings are accepted? What happens on invalid bytes?
- Is a leading UTF-8 BOM preserved, stripped, or an error?
- Are CRLF line endings preserved or normalized? What does `position.offset` count against?

## Decision

1. **Whole-document in memory.** v1 does not stream. The full input is read into memory before parsing. goldmark itself does not stream, and the output is a single JSON document, so streaming would add complexity without buying a user-visible property.

2. **No hard size cap.** v1 ships without a `--max-size` flag. The OS / Go allocator are trusted; pathological inputs may OOM, which is acceptable behavior for a single-shot CLI filter. A size cap is deferred until a real "this OOM'd me" report justifies a defensible default.

3. **UTF-8 only.** UTF-16, latin-1, and other encodings are non-goals for v1. Users with non-UTF-8 input run their file through `iconv` first.

4. **Invalid UTF-8 is a hard error.** Invalid UTF-8 bytes cause exit `1` with `md2json2: <path>:<line>:<col>: invalid utf-8 byte at offset <N>` on stderr. **No** silent replacement with U+FFFD — that would let a partially-corrupt file produce a partially-corrupt AST that *looks* fine, which is the worst failure mode for a tool whose job is structured output. This is deliberately stricter than CommonMark, which permits arbitrary bytes.

5. **Leading UTF-8 BOM is stripped silently.** The BOM is a transport-layer artifact, not document content. Stripping is the convention in the Hugo / static-site-generator world. Consequence: `position.offset` values are relative to the **post-BOM-strip** document, not the raw file, and the first node's offset is shifted by `-3` bytes relative to the on-disk byte position when a BOM is present.

6. **CRLF is normalized to LF before parsing.** All `position` fields reflect the normalized document:
   - `position.line` counts logical lines (cross-platform stable).
   - `position.column` counts UTF-8 code points within the normalized line.
   - `position.offset` is a byte offset into the normalized (LF-only, post-BOM-strip) document.

## Consequences

- **Positive (cross-platform stability).** A Windows developer on CRLF and a macOS CI on LF produce identical `position` values for the same logical document. Debugging error messages against editor line numbers Just Works regardless of line-ending style.
- **Positive (fail-fast correctness).** Bad bytes never silently degrade output. The contract "if md2json2 exits 0, the AST is faithful to the input" holds.
- **Positive (small surface).** No encoding-detection flag, no replacement-character flag, no size-cap flag in v1.
- **Negative (round-trip arithmetic).** Tools that want to map `position.offset` back to a raw-file byte offset must re-apply the BOM-strip and CRLF-normalize transforms themselves. This is documented but not provided as an API.
- **Negative (no streaming).** Multi-hundred-MB inputs are out of scope; very large documents may OOM. Acceptable for v1; revisit on real demand.
- **TDD implication.** Test fixtures must cover: BOM-prefixed input, CRLF-only input, mixed CRLF/LF input, invalid-UTF-8 byte sequences (both leading and mid-document), and a small handful of "very large but not pathological" inputs. The acceptance of all five encoding/normalization rules above is a concrete test target.

## Out of scope (post-v1)

- `--max-size` flag.
- Encoding auto-detection or explicit `--encoding` flag.
- A `--lenient-utf8` (U+FFFD-substituting) mode.
- Streaming parse for unbounded input.
