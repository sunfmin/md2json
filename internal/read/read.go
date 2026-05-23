// Package read implements ADR-0001's input-handling contract: read the whole
// document into memory, validate UTF-8 (returning a typed error on the first
// invalid byte — never substitute U+FFFD), strip a leading UTF-8 BOM, and
// normalize CRLF to LF before the bytes reach any downstream stage.
//
// The module knows nothing about Markdown or YAML. It takes the <path> token
// used in stderr error lines as an argument so byte-level errors can be
// attributed back to the right source (a real file path, or the literal "-"
// for stdin).
package read

import (
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"
)

// utf8BOM is the three-byte UTF-8 Byte Order Mark (U+FEFF encoded in UTF-8).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ReadError is the typed error returned for any byte-level rejection in the
// read stage. v1's only such failure is invalid UTF-8; the shape is general so
// the cli layer can format the canonical stderr line uniformly.
//
// Offset is the byte offset of the first invalid byte in the input as the
// read stage sees it (i.e. post-BOM-strip, since the BOM is a transport-layer
// artifact — but for the leading-bad-byte case the BOM has not yet been
// stripped, so Offset 0 is the literal first byte of the input stream).
//
// Line and Col are 1-indexed and reflect the position of the bad byte in the
// source as the user sees it: a bare leading bad byte is at line 1 column 1.
// A bad byte mid-document is at the line of the most recently consumed '\n'
// + 1, and the column-th code point (or byte, for the all-ASCII prefix case)
// since the last '\n'.
//
// Path is the <path> token to embed in the canonical stderr line; the read
// module's caller passes it through verbatim.
type ReadError struct {
	Path   string
	Line   int
	Col    int
	Offset int
	Msg    string
}

func (e *ReadError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Col, e.Msg)
}

// Read consumes r in full into memory and returns the normalized byte slice:
// a leading UTF-8 BOM is stripped, any CRLF sequences are collapsed to LF, and
// any bare CR (not followed by LF) is also rewritten to LF (the implementation
// choice pinned by S02 for "bare \r" — consistent with ADR-0001's
// cross-platform-stable normalized-document rule). On invalid UTF-8, Read
// returns a *ReadError pointing at the first bad byte and a nil slice.
//
// path is the <path> token used in any returned *ReadError; the cli layer
// passes the literal "-" for stdin or the file path otherwise.
func Read(r io.Reader, path string) ([]byte, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Strip leading UTF-8 BOM if present. ADR-0001 §5: "Leading UTF-8 BOM is
	// stripped silently." Only at the very start; a BOM-shaped run mid-doc is
	// valid UTF-8 content and is left alone.
	if bytes.HasPrefix(raw, utf8BOM) {
		raw = raw[len(utf8BOM):]
	}

	// Normalize line endings: \r\n -> \n, bare \r -> \n. ADR-0001 §6 mandates
	// CRLF→LF; bare \r is an S02-pinned extension of the same rule so that a
	// pre-OSX-Mac-style file does not produce a single-line document.
	// We allocate up to len(raw) bytes (the worst case: no transform applied).
	norm := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		if b == '\r' {
			// Look ahead: collapse \r\n to a single \n; bare \r also becomes \n.
			if i+1 < len(raw) && raw[i+1] == '\n' {
				norm = append(norm, '\n')
				i++ // skip the \n we just emitted
				continue
			}
			norm = append(norm, '\n')
			continue
		}
		norm = append(norm, b)
	}

	// Validate UTF-8 on the normalized bytes. We track line/column as we walk
	// so the *ReadError carries the position of the first invalid byte. Note
	// that the offset in the error is the offset into the normalized slice —
	// which is what ADR-0001's "position fields are relative to the normalized
	// document" rule requires for any downstream consumer.
	line, col := 1, 1
	for i := 0; i < len(norm); {
		b := norm[i]
		if b < 0x80 {
			// ASCII fast path.
			if b == '\n' {
				line++
				col = 1
			} else {
				col++
			}
			i++
			continue
		}
		// Multi-byte rune. Decode; on RuneError with size 1, this byte is bad.
		r, size := utf8.DecodeRune(norm[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, &ReadError{
				Path:   path,
				Line:   line,
				Col:    col,
				Offset: i,
				Msg:    fmt.Sprintf("invalid utf-8 byte at offset %d", i),
			}
		}
		// Valid multi-byte rune; advance one code-point column.
		col++
		i += size
	}

	return norm, nil
}
