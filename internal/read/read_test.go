package read

import (
	"bytes"
	"errors"
	"testing"
)

// Unit test (criterion #4a): a leading UTF-8 BOM (0xEF 0xBB 0xBF) is stripped
// from the returned bytes. The returned slice does NOT include the BOM.
func TestLeadingBOMIsStripped(t *testing.T) {
	in := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...)
	got, err := Read(bytes.NewReader(in), "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []byte("hello")
	if !bytes.Equal(got, want) {
		t.Errorf("BOM not stripped\n  got:  %q\n  want: %q", got, want)
	}
}

// Unit test (criterion #4b, CRLF half): every \r\n sequence collapses to a
// single \n byte. The returned slice has no \r bytes left.
func TestCRLFNormalizedToLF(t *testing.T) {
	in := []byte("a\r\nb\r\nc")
	got, err := Read(bytes.NewReader(in), "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []byte("a\nb\nc")
	if !bytes.Equal(got, want) {
		t.Errorf("CRLF not normalized\n  got:  %q\n  want: %q", got, want)
	}
}

// Unit test (criterion #4c): the byte length of the returned slice reflects
// both transforms — BOM bytes gone, each CRLF collapsed to one byte. Input is
// BOM (3 bytes) + "a\r\nb\r\nc" (7 bytes) = 10 bytes; output is "a\nb\nc" =
// 5 bytes (3 BOM bytes removed + 2 CR bytes removed).
func TestByteLengthReflectsBothTransforms(t *testing.T) {
	in := append([]byte{0xEF, 0xBB, 0xBF}, []byte("a\r\nb\r\nc")...)
	if len(in) != 10 {
		t.Fatalf("setup: input length is %d, expected 10", len(in))
	}
	got, err := Read(bytes.NewReader(in), "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("byte length: got %d, want 5 (BOM stripped + 2 CRLF collapsed)", len(got))
	}
	want := []byte("a\nb\nc")
	if !bytes.Equal(got, want) {
		t.Errorf("transformed bytes\n  got:  %q\n  want: %q", got, want)
	}
}

// Unit test (criterion #4d): invalid UTF-8 returns a typed *ReadError with
// the right byte offset, line, and column; the returned slice is nil (the
// caller does not get any partial document on a hard error). Path is threaded
// through verbatim.
func TestInvalidUTF8ReturnsTypedReadError(t *testing.T) {
	// "hi\nworld" then 0xC3 (a 2-byte lead with no valid continuation).
	in := []byte("hi\nworld\xC3\x28")
	got, err := Read(bytes.NewReader(in), "/path/to/file.md")
	if got != nil {
		t.Errorf("on error, returned slice should be nil, got %q", got)
	}
	var re *ReadError
	if !errors.As(err, &re) {
		t.Fatalf("expected *ReadError, got %T: %v", err, err)
	}
	if re.Path != "/path/to/file.md" {
		t.Errorf("Path: got %q, want %q", re.Path, "/path/to/file.md")
	}
	if re.Line != 2 {
		t.Errorf("Line: got %d, want 2", re.Line)
	}
	if re.Col != 6 {
		t.Errorf("Col: got %d, want 6", re.Col)
	}
	if re.Offset != 8 {
		t.Errorf("Offset: got %d, want 8", re.Offset)
	}
	if re.Msg != "invalid utf-8 byte at offset 8" {
		t.Errorf("Msg: got %q, want %q", re.Msg, "invalid utf-8 byte at offset 8")
	}
}

// Unit test (criterion #5): the BOM is stripped only when at the very start.
// A BOM-shaped byte sequence appearing mid-document is valid UTF-8 (the U+FEFF
// zero-width no-break-space code point) and is left alone — both the bytes and
// the count stay intact.
func TestBOMShapedMidDocumentIsLeftAlone(t *testing.T) {
	// "hello" + U+FEFF (3 bytes: 0xEF 0xBB 0xBF) + "world".
	in := append([]byte("hello"), 0xEF, 0xBB, 0xBF)
	in = append(in, []byte("world")...)
	got, err := Read(bytes.NewReader(in), "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, in) {
		t.Errorf("mid-doc BOM-shaped bytes mutated\n  got:  %q\n  want: %q", got, in)
	}
}

// Unit test (criterion #6): a file with no BOM and only LF line endings
// round-trips through Read unchanged byte-for-byte. This is the no-op identity
// case that guarantees the module does not "helpfully" mangle well-formed
// input.
func TestNoBOMLFOnlyRoundTripsByteForByte(t *testing.T) {
	in := []byte("# heading\n\nparagraph with utf-8: héllo wörld\n\n- item 1\n- item 2\n")
	got, err := Read(bytes.NewReader(in), "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, in) {
		t.Errorf("LF-only input mutated\n  got:  %q\n  want: %q", got, in)
	}
}

// Unit test: a bare leading 0xFF byte produces a *ReadError at line 1 col 1
// offset 0. Complements TestInvalidUTF8ReturnsTypedReadError (which covers the
// mid-doc case) by pinning the leading-byte boundary explicitly.
func TestLeadingInvalidByteAtOriginPosition(t *testing.T) {
	in := []byte{0xFF}
	got, err := Read(bytes.NewReader(in), "-")
	if got != nil {
		t.Errorf("on error, returned slice should be nil, got %q", got)
	}
	var re *ReadError
	if !errors.As(err, &re) {
		t.Fatalf("expected *ReadError, got %T: %v", err, err)
	}
	if re.Path != "-" {
		t.Errorf("Path: got %q, want %q", re.Path, "-")
	}
	if re.Line != 1 || re.Col != 1 || re.Offset != 0 {
		t.Errorf("position: got line=%d col=%d offset=%d, want 1,1,0", re.Line, re.Col, re.Offset)
	}
	if re.Msg != "invalid utf-8 byte at offset 0" {
		t.Errorf("Msg: got %q, want %q", re.Msg, "invalid utf-8 byte at offset 0")
	}
}

// Unit test (criterion #4b, bare-\r half): a bare \r (not followed by \n) is
// also rewritten to \n. The S02 pin: bare \r maps to \n, consistent with
// ADR-0001's "normalize CRLF to LF" cross-platform-stability rule (a classic-
// Mac \r-only file produces a multi-line document rather than collapsing into
// a single line that would skew every position.line value).
func TestBareCRNormalizedToLF(t *testing.T) {
	in := []byte("a\rb\rc")
	got, err := Read(bytes.NewReader(in), "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []byte("a\nb\nc")
	if !bytes.Equal(got, want) {
		t.Errorf("bare CR not normalized\n  got:  %q\n  want: %q", got, want)
	}
}
