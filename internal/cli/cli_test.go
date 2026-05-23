package cli

import (
	"bytes"
	"os"
	"regexp"
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
// `md2json --no-position < empty.md`: no `position` key on root.
const wantEnvelopeNoPosition = `{"frontmatter":null,"ast":{"type":"root","children":[]}}`

// wantEnvelopeDefault is the v1 ship criterion's exact stdout for default
// `md2json < empty.md`: same envelope but with a zero-width `position` on root.
const wantEnvelopeDefault = `{"frontmatter":null,"ast":{"type":"root","children":[],"position":{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}}}`

// S01 Test 1, re-pinned for S03 (criterion #1): empty stdin under
// `--no-position` produces exactly the no-position envelope, exit 0. This is
// the v1 ship criterion's first half.
func TestEmptyStdinNoPositionEmitsEnvelope(t *testing.T) {
	stdout, stderr, exit := run(t, []string{"md2json", "--no-position"}, "")
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
	stdout, stderr, exit := run(t, []string{"md2json"}, "")
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
	stdout, stderr, exit := run(t, []string{"md2json", "--no-position", path}, "")
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
	_, _, exit := run(t, []string{"md2json", path}, "")
	if exit == 0 {
		t.Errorf("missing file should produce non-zero exit, got 0")
	}
}

// Test 4 (criterion #3): the literal "-" positional is the stdin sentinel and
// behaves identically to omitting the positional. Both produce the empty
// envelope from whatever stdin contained. Re-pinned in S03 to use
// --no-position so the byte-exact comparison stays valid.
func TestStdinSentinelDashBehavesLikeNoPositional(t *testing.T) {
	stdout, stderr, exit := run(t, []string{"md2json", "--no-position", "-"}, "")
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
		{"-h", []string{"md2json", "-h"}},
		{"--help", []string{"md2json", "--help"}},
		{"-V", []string{"md2json", "-V"}},
		{"--version", []string{"md2json", "--version"}},
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
	stdout, stderr, exit := run(t, []string{"md2json", "--frontmatter-only"}, "")
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
	stdout, stderr, exit := run(t, []string{"md2json"}, "\xFF")
	wantStderr := "md2json: -:1:1: invalid utf-8 byte at offset 0\n"
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
	stdout, stderr, exit := run(t, []string{"md2json"}, in)
	wantStderr := "md2json: -:2:6: invalid utf-8 byte at offset 8\n"
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
		{"--no-such-flag", []string{"md2json", "--no-such-flag"}},
		{"-z", []string{"md2json", "-z"}},
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

// S12 Test 7 (criterion #7): when an error carries a line but no column
// (`col == 0` while `line > 0`), the canonical stderr line rounds the column
// up to `1` — never `:0:` for a real line. The `:0:0:` sentinel is reserved
// for the document-scoped no-position case where BOTH line and column are
// absent. This is a direct unit test against the rendering helper since the
// rounding-up rule is a property of the rendering path itself.
func TestPositionedErrorRoundsUnknownColumnUpToOne(t *testing.T) {
	cases := []struct {
		name string
		line int
		col  int
		want string
	}{
		{"line-with-col", 5, 3, "md2json: post.md:5:3: oops\n"},
		// line known, col unknown → column rounds up to 1.
		{"line-only-col-zero", 5, 0, "md2json: post.md:5:1: oops\n"},
		// document-scoped no-position sentinel: both 0 → 0:0 preserved.
		{"no-position-sentinel", 0, 0, "md2json: post.md:0:0: oops\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			writePositionedError(&buf, "post.md", c.line, c.col, "oops")
			if buf.String() != c.want {
				t.Errorf("got  %q\nwant %q", buf.String(), c.want)
			}
		})
	}
}

// failingWriter is an io.Writer that returns an error on every Write. Used to
// drive emit.Emit into its error branch so the cli's document-scoped error
// rendering (the `:0:0:` sentinel branch) becomes observable end-to-end.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errIO
}

var errIO = stubError("simulated I/O failure")

type stubError string

func (e stubError) Error() string { return string(e) }

// S12 Test 6 (criterion #6): a document-scoped error (no line/column
// information available) renders as `md2json: <path>:0:0: <msg>` on stderr
// and exits 1. We exercise the path through an io.Writer that fails: emit
// returns an error, cli's `writeDocScopedError` branch fires, and the stderr
// line uses the `:0:0:` sentinel with the stdin path token `-`.
func TestDocumentScopedErrorUsesZeroPositionSentinel(t *testing.T) {
	var errBuf bytes.Buffer
	inBuf := strings.NewReader("")
	exit := Run([]string{"md2json"}, inBuf, failingWriter{}, &errBuf)
	if exit != 1 {
		t.Errorf("exit code: got %d, want 1 (document-scoped error)", exit)
	}
	stderr := errBuf.String()
	canonical := regexp.MustCompile(`^md2json: ([^:]+):(\d+):(\d+): (.+)\n$`)
	m := canonical.FindStringSubmatch(stderr)
	if m == nil {
		t.Fatalf("stderr does not match canonical regex: %q", stderr)
	}
	// Stdin source → path token is `-` (CONTEXT.md "Error format").
	if m[1] != "-" {
		t.Errorf("stdin-source document-scoped error <path>: got %q, want %q", m[1], "-")
	}
	// Document-scoped, no position → `:0:0:` sentinel.
	if m[2] != "0" || m[3] != "0" {
		t.Errorf("document-scoped error position: got %s:%s, want 0:0", m[2], m[3])
	}
}

// S12 Test 5 (criterion #3): `-V` / `--version` writes a version string to
// stdout, exit 0. Both forms produce identical output. The exact version
// string is left to S13's release pipeline; this test only enforces shape
// (contains `md2json`, contains a digit, single line modulo trailing \n).
func TestVersionFlagPrintsVersion(t *testing.T) {
	for _, flag := range []string{"-V", "--version"} {
		t.Run(flag, func(t *testing.T) {
			stdout, stderr, exit := run(t, []string{"md2json", flag}, "")
			if exit != 0 {
				t.Errorf("exit code: got %d, want 0", exit)
			}
			if stderr != "" {
				t.Errorf("stderr should be empty on %s, got %q", flag, stderr)
			}
			if !strings.Contains(stdout, "md2json") {
				t.Errorf("version output should contain `md2json`, got %q", stdout)
			}
			if !regexp.MustCompile(`\d`).MatchString(stdout) {
				t.Errorf("version output should contain a digit, got %q", stdout)
			}
		})
	}
	stdoutShort, _, _ := run(t, []string{"md2json", "-V"}, "")
	stdoutLong, _, _ := run(t, []string{"md2json", "--version"}, "")
	if stdoutShort != stdoutLong {
		t.Errorf("-V and --version produced different output:\n-V: %q\n--version: %q", stdoutShort, stdoutLong)
	}
}

// S12 Test 4 (criterion #2): `-h` / `--help` writes a usage message to stdout
// that names every v1 flag and the positional `FILE` / `-` convention. Exit 0,
// stderr empty. Both forms produce identical output.
func TestHelpFlagPrintsUsageNamingEveryFlag(t *testing.T) {
	want := []string{
		// Every v1 flag name (CONTEXT.md "v1 flags") must appear in the
		// usage text so users can see the complete contract from -h.
		"-o", "--output",
		"--pretty",
		"--no-position",
		"--frontmatter-only",
		"-h", "--help",
		"-V", "--version",
		// The positional FILE / stdin sentinel convention (PRD US 1, 2; CLI
		// contract glossary entry).
		"FILE",
	}
	for _, flag := range []string{"-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			stdout, stderr, exit := run(t, []string{"md2json", flag}, "")
			if exit != 0 {
				t.Errorf("exit code: got %d, want 0", exit)
			}
			if stderr != "" {
				t.Errorf("stderr should be empty on -h/--help, got %q", stderr)
			}
			if stdout == "" {
				t.Fatalf("stdout should not be empty on %s", flag)
			}
			for _, frag := range want {
				if !strings.Contains(stdout, frag) {
					t.Errorf("usage missing %q\nfull stdout:\n%s", frag, stdout)
				}
			}
			// Mention of the `-` stdin sentinel: the CONTEXT.md "CLI
			// contract" entry pins `FILE=-` as the stdin sentinel. The
			// usage text must surface that convention somewhere (so the
			// `-` literal appears).
			if !strings.Contains(stdout, "-") {
				t.Errorf("usage missing the stdin `-` sentinel mention\nfull stdout:\n%s", stdout)
			}
		})
	}
	// Both forms must agree byte-for-byte (one source of truth).
	stdoutShort, _, _ := run(t, []string{"md2json", "-h"}, "")
	stdoutLong, _, _ := run(t, []string{"md2json", "--help"}, "")
	if stdoutShort != stdoutLong {
		t.Errorf("-h and --help produced different output:\n-h: %q\n--help: %q", stdoutShort, stdoutLong)
	}
}

// S12 Test 3 (criterion #1): `-o out.json post.md` writes the JSON envelope
// to `out.json` (creating/truncating as needed), nothing on stdout, exit 0.
// Both the short `-o` and the long `--output` forms must work, and the
// `--output=PATH` equals form must work too. (Existing TestKnownFlagsRecognizedAsNoop
// covered "exit 0"; here we additionally pin the contract that the file
// receives the envelope and stdout stays clean.)
func TestOutputFlagWritesEnvelopeToFile(t *testing.T) {
	tmp := t.TempDir()
	cases := []struct {
		name string
		mk   func(outPath string) []string
	}{
		{"short", func(p string) []string { return []string{"md2json", "--no-position", "-o", p} }},
		{"long", func(p string) []string { return []string{"md2json", "--no-position", "--output", p} }},
		{"long-equals", func(p string) []string { return []string{"md2json", "--no-position", "--output=" + p} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outPath := tmp + "/out-" + c.name + ".json"
			stdout, stderr, exit := run(t, c.mk(outPath), "")
			if exit != 0 {
				t.Errorf("exit code: got %d, want 0; stderr=%q", exit, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty when -o is set, got %q", stdout)
			}
			if stderr != "" {
				t.Errorf("stderr should be empty on success, got %q", stderr)
			}
			got, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("read output file: %v", err)
			}
			if string(got) != wantEnvelopeNoPosition {
				t.Errorf("output file contents mismatch\n  got:  %q\n  want: %q", got, wantEnvelopeNoPosition)
			}
		})
	}
}

// S12 Test 3b (criterion #1, truncate half): `-o` on an existing file
// truncates first; the file does not accumulate prior contents.
func TestOutputFlagTruncatesExistingFile(t *testing.T) {
	tmp := t.TempDir()
	outPath := tmp + "/already.json"
	if err := os.WriteFile(outPath, []byte("STALE BYTES THAT MUST BE REMOVED"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	_, _, exit := run(t, []string{"md2json", "--no-position", "-o", outPath}, "")
	if exit != 0 {
		t.Errorf("exit code: got %d, want 0", exit)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(got) != wantEnvelopeNoPosition {
		t.Errorf("output file should be truncated then rewritten\n  got:  %q\n  want: %q", got, wantEnvelopeNoPosition)
	}
}

// S12 Test 2 (criterion #5 + criterion #9 pre-input half): a missing /
// unreadable FILE (error raised before any bytes are read) is a pre-input
// usage error: `<path>` is the literal `md2json`, position is `:0:0:`, exit
// code is 2, stdout is empty.
func TestMissingFilePreInputUsageError(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/nope.md"
	stdout, stderr, exit := run(t, []string{"md2json", path}, "")
	if stdout != "" {
		t.Errorf("stdout should be empty on usage error, got %q", stdout)
	}
	if exit != 2 {
		t.Errorf("exit code: got %d, want 2", exit)
	}
	canonical := regexp.MustCompile(`^md2json: ([^:]+):(\d+):(\d+): (.+)\n$`)
	m := canonical.FindStringSubmatch(stderr)
	if m == nil {
		t.Fatalf("stderr does not match canonical regex: %q", stderr)
	}
	if m[1] != "md2json" {
		t.Errorf("pre-input usage error <path> token: got %q, want %q", m[1], "md2json")
	}
	if m[2] != "0" || m[3] != "0" {
		t.Errorf("pre-input usage error position: got %s:%s, want 0:0", m[2], m[3])
	}
}

// S12 Test 1 (criterion #4 + criterion #9 pre-input half): an unknown flag is a
// pre-input usage error. The stderr line uses the literal program name
// `md2json` as the `<path>` token and the `:0:0:` sentinel; exit code is 2
// (usage error, distinct from 1 = document-scoped parse error). Stdout stays
// empty so callers branching on `$?` don't see a stray byte.
func TestUnknownFlagPreInputUsageError(t *testing.T) {
	stdout, stderr, exit := run(t, []string{"md2json", "--no-such-flag"}, "")
	if stdout != "" {
		t.Errorf("stdout should be empty on usage error, got %q", stdout)
	}
	if exit != 2 {
		t.Errorf("exit code: got %d, want 2", exit)
	}
	// Stderr must be exactly one line matching the canonical regex with
	// `<path>` = `md2json` and position `0:0`.
	canonical := regexp.MustCompile(`^md2json: ([^:]+):(\d+):(\d+): (.+)\n$`)
	m := canonical.FindStringSubmatch(stderr)
	if m == nil {
		t.Fatalf("stderr does not match canonical regex: %q", stderr)
	}
	if m[1] != "md2json" {
		t.Errorf("pre-input usage error <path> token: got %q, want %q", m[1], "md2json")
	}
	if m[2] != "0" || m[3] != "0" {
		t.Errorf("pre-input usage error position: got %s:%s, want 0:0", m[2], m[3])
	}
	if !strings.Contains(strings.ToLower(m[4]), "no-such-flag") &&
		!strings.Contains(strings.ToLower(m[4]), "unknown") {
		t.Errorf("stderr message should mention the bad flag or be an 'unknown flag' diagnostic, got %q", m[4])
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
		{"no-position", []string{"md2json", "--no-position"}},
		{"pretty", []string{"md2json", "--pretty"}},
		{"frontmatter-only", []string{"md2json", "--frontmatter-only"}},
		{"output-long-space", []string{"md2json", "--output", outPath}},
		{"output-long-equals", []string{"md2json", "--output=" + outPath}},
		{"output-short-space", []string{"md2json", "-o", outPath}},
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
