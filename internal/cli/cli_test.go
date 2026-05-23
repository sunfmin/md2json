package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// run is a small test helper that wraps cli.Run with byte buffers for the
// three IO streams, mirroring the way main.go will call into cli.Run with
// the process globals. Tests assert on the buffers' bytes and the returned
// exit code; cli.Run never calls os.Exit (per the PRD's "no module reaches
// into process globals" rule).
func run(t *testing.T, argv []string, stdin string) (stdout, stderr string, exit int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	inBuf := strings.NewReader(stdin)
	exit = Run(argv, inBuf, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), exit
}

// wantEnvelopeNoPosition is the v1 ship criterion's exact stdout for
// `md2json2 --no-position < empty.md`: no `position` key on root.
const wantEnvelopeNoPosition = `{"frontmatter":null,"ast":{"type":"root","children":[]}}`

// wantEnvelopeDefault is the v1 ship criterion's exact stdout for default
// `md2json2 < empty.md`: same envelope but with a zero-width `position` on root.
const wantEnvelopeDefault = `{"frontmatter":null,"ast":{"type":"root","children":[],"position":{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}}}`

// S01 Test 1, re-pinned for S03 (criterion #1): empty stdin under
// `--no-position` produces exactly the no-position envelope, exit 0. This is
// the v1 ship criterion's first half.
func TestEmptyStdinNoPositionEmitsEnvelope(t *testing.T) {
	stdout, stderr, exit := run(t, []string{"md2json2", "--no-position"}, "")
	if stdout != wantEnvelopeNoPosition {
		t.Errorf("stdout mismatch\n  got:  %q\n  want: %q", stdout, wantEnvelopeNoPosition)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
	if exit != 0 {
		t.Errorf("exit code: got %d, want 0", exit)
	}
}

// S03 Test 1 (tracer bullet, criterion #2): empty stdin in DEFAULT mode (no
// flags) produces the envelope WITH a zero-width `position` on `root`. This is
// the v1 ship criterion's second half and is what drives the real pipeline
// (parse → translate → emit) into existence: S01's hard-coded envelope had no
// `position`, so this test is RED until translate/emit produce real position
// info on the root node.
func TestEmptyStdinDefaultEmitsEnvelopeWithPosition(t *testing.T) {
	stdout, stderr, exit := run(t, []string{"md2json2"}, "")
	if stdout != wantEnvelopeDefault {
		t.Errorf("stdout mismatch\n  got:  %q\n  want: %q", stdout, wantEnvelopeDefault)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
	if exit != 0 {
		t.Errorf("exit code: got %d, want 0", exit)
	}
}

// Test 3 (criterion #2): a positional FILE argument is accepted and its bytes
// are read off disk. At S01 we still emit the hard-coded envelope regardless
// of the file's contents.
func TestPositionalFileEmitsEnvelope(t *testing.T) {
	// Create a real readable file via testing.TempDir so the disk-read code
	// path is exercised. Contents are deliberately empty: S01's contract here
	// is only "positional FILE is accepted, bytes are read off disk." S04
	// re-pinned this to use an empty file because the translate stage now
	// produces real children from non-empty content; any non-empty fixture
	// would force this test to assert on heading/paragraph shape, which is
	// owned by S04's own fixtures, not by this test.
	tmp := t.TempDir()
	path := tmp + "/any.md"
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	// Re-pinned in S03: pass --no-position so the envelope check stays byte-
	// exact against the no-position constant. S01's contract here is only
	// "positional FILE is accepted, bytes are read"; the envelope shape is now
	// owned by S03.
	stdout, stderr, exit := run(t, []string{"md2json2", "--no-position", path}, "")
	if stdout != wantEnvelopeNoPosition {
		t.Errorf("stdout mismatch\n  got:  %q\n  want: %q", stdout, wantEnvelopeNoPosition)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
	if exit != 0 {
		t.Errorf("exit code: got %d, want 0", exit)
	}
}

// Test 3b (criterion #2, observability half): a missing FILE is observable as
// a non-zero exit code. The criterion explicitly allows the error path to be
// unpolished at S01 — later slices pin the exact stderr line and exit code 2.
// What S01 must guarantee is that the file is actually opened (not just
// shrugged off), which is detectable as "missing file != exit 0".
func TestMissingPositionalFileObservable(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/does-not-exist.md"
	_, _, exit := run(t, []string{"md2json2", path}, "")
	if exit == 0 {
		t.Errorf("missing file should produce non-zero exit, got 0")
	}
}

// Test 4 (criterion #3): the literal "-" positional is the stdin sentinel and
// behaves identically to omitting the positional. Both produce the empty
// envelope from whatever stdin contained. Re-pinned in S03 to use
// --no-position so the byte-exact comparison stays valid.
func TestStdinSentinelDashBehavesLikeNoPositional(t *testing.T) {
	stdout, stderr, exit := run(t, []string{"md2json2", "--no-position", "-"}, "")
	if stdout != wantEnvelopeNoPosition {
		t.Errorf("stdout mismatch\n  got:  %q\n  want: %q", stdout, wantEnvelopeNoPosition)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
	if exit != 0 {
		t.Errorf("exit code: got %d, want 0", exit)
	}
}

// Test 6 (criterion #5): -h and -V exit 0 with placeholder output. The exact
// bytes are pinned by later slices; S01's contract is only that they do not
// fail.
func TestHelpAndVersionExitZero(t *testing.T) {
	cases := []struct {
		name string
		argv []string
	}{
		{"-h", []string{"md2json2", "-h"}},
		{"--help", []string{"md2json2", "--help"}},
		{"-V", []string{"md2json2", "-V"}},
		{"--version", []string{"md2json2", "--version"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, exit := run(t, c.argv, "")
			if exit != 0 {
				t.Errorf("exit code: got %d, want 0", exit)
			}
		})
	}
}

// S03 Test 2 (criterion #3): --frontmatter-only on an empty document emits
// just the JSON literal `null` (no envelope, no trailing newline), exit 0.
// The flag short-circuits BEFORE translate per the PRD pipeline rule. This
// pins the scalar-passthrough rule for the null case; later slices add the
// non-null scalar cases (string/number/bool) under S09.
func TestFrontmatterOnlyEmptyDocEmitsNull(t *testing.T) {
	stdout, stderr, exit := run(t, []string{"md2json2", "--frontmatter-only"}, "")
	wantStdout := "null"
	if stdout != wantStdout {
		t.Errorf("stdout mismatch\n  got:  %q\n  want: %q", stdout, wantStdout)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
	if exit != 0 {
		t.Errorf("exit code: got %d, want 0", exit)
	}
}

// S02 Test 1 (tracer bullet, criteria #1 + #3): end-to-end bad-UTF-8 path.
// A leading 0xFF byte on stdin produces exit 1, the canonical stderr line with
// the literal "-" path token (criterion #3), and nothing on stdout. This drives
// the introduction of the read module + the cli wiring that routes a typed
// read-stage error into the canonical stderr line.
func TestLeadingInvalidUTF8StdinHardErrors(t *testing.T) {
	stdout, stderr, exit := run(t, []string{"md2json2"}, "\xFF")
	wantStderr := "md2json2: -:1:1: invalid utf-8 byte at offset 0\n"
	if stdout != "" {
		t.Errorf("stdout should be empty on hard error, got %q", stdout)
	}
	if stderr != wantStderr {
		t.Errorf("stderr mismatch\n  got:  %q\n  want: %q", stderr, wantStderr)
	}
	if exit != 1 {
		t.Errorf("exit code: got %d, want 1", exit)
	}
}

// S02 Test 2 (criterion #2): a mid-document bad-UTF-8 sequence (valid prefix
// "hi\nworld" then 0xC3 0x28 — 0xC3 introduces a 2-byte sequence but 0x28 is
// not a continuation byte) produces the canonical stderr line with the byte
// offset of the first bad byte (8), line 2, column 6 (one past "world").
func TestMidDocumentInvalidUTF8StdinHardErrors(t *testing.T) {
	in := "hi\nworld\xC3\x28"
	stdout, stderr, exit := run(t, []string{"md2json2"}, in)
	wantStderr := "md2json2: -:2:6: invalid utf-8 byte at offset 8\n"
	if stdout != "" {
		t.Errorf("stdout should be empty on hard error, got %q", stdout)
	}
	if stderr != wantStderr {
		t.Errorf("stderr mismatch\n  got:  %q\n  want: %q", stderr, wantStderr)
	}
	if exit != 1 {
		t.Errorf("exit code: got %d, want 1", exit)
	}
}

// Test 7 (criterion #6): an unknown flag exits non-zero. The exact code (2)
// and the canonical stderr regex are pinned by S11; S01 only needs the
// rejection signal.
func TestUnknownFlagExitsNonZero(t *testing.T) {
	cases := []struct {
		name string
		argv []string
	}{
		{"--no-such-flag", []string{"md2json2", "--no-such-flag"}},
		{"-z", []string{"md2json2", "-z"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, exit := run(t, c.argv, "")
			if exit == 0 {
				t.Errorf("unknown flag should produce non-zero exit, got 0")
			}
		})
	}
}

// Test 5 (criterion #4): each known v1 flag is recognized as a no-op at this
// stage and exits 0. Covered: --no-position, --pretty, --frontmatter-only, and
// -o/--output (value-bound). The behavior of each is pinned by later slices;
// S01 only needs to ensure passing any of them does not fail.
func TestKnownFlagsRecognizedAsNoop(t *testing.T) {
	tmp := t.TempDir()
	outPath := tmp + "/out.json"
	cases := []struct {
		name string
		argv []string
	}{
		{"no-position", []string{"md2json2", "--no-position"}},
		{"pretty", []string{"md2json2", "--pretty"}},
		{"frontmatter-only", []string{"md2json2", "--frontmatter-only"}},
		{"output-long-space", []string{"md2json2", "--output", outPath}},
		{"output-long-equals", []string{"md2json2", "--output=" + outPath}},
		{"output-short-space", []string{"md2json2", "-o", outPath}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, exit := run(t, c.argv, "")
			if exit != 0 {
				t.Errorf("exit code: got %d, want 0; stderr=%q", exit, stderr)
			}
		})
	}
}
