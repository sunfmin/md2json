package main_test

// Black-box integration-test harness. Builds the md2json2 binary once via
// TestMain, then runs every directory under testdata/fixtures/ as a fixture:
//
//   testdata/fixtures/<name>/
//     args           one line, space-separated argv (excluding argv[0])
//     input.md       optional; if present, fed on stdin
//     stdout         expected stdout (byte-for-byte)
//     stderr         expected stderr (byte-for-byte)
//     exit           expected exit code (one line, integer)
//
// Every comparison is byte-exact. Later slices' fixtures plug into this same
// harness with no changes.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "md2json2-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: mktemp:", err)
		os.Exit(2)
	}
	defer os.RemoveAll(dir)

	binaryPath = filepath.Join(dir, "md2json2")
	// Build with the test invocation's GOOS/GOARCH; no CGO needed.
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: go build:", err)
		os.Exit(2)
	}

	os.Exit(m.Run())
}

// fixture represents one directory under testdata/fixtures/.
type fixture struct {
	name       string
	dir        string
	args       []string
	stdin      []byte
	hasStdin   bool
	wantStdout []byte
	wantStderr []byte
	wantExit   int
}

func loadFixture(t *testing.T, dir string) fixture {
	t.Helper()
	name := filepath.Base(dir)
	fx := fixture{name: name, dir: dir}

	argsBytes, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatalf("fixture %s: read args: %v", name, err)
	}
	// Strip a single trailing newline if present; otherwise treat the whole
	// file as the args line. Use Fields-style splitting on whitespace.
	argsLine := strings.TrimRight(string(argsBytes), "\n")
	if argsLine == "" {
		fx.args = nil
	} else {
		fx.args = strings.Fields(argsLine)
	}

	if b, err := os.ReadFile(filepath.Join(dir, "input.md")); err == nil {
		fx.stdin = b
		fx.hasStdin = true
	} else if !os.IsNotExist(err) {
		t.Fatalf("fixture %s: read input.md: %v", name, err)
	}

	if b, err := os.ReadFile(filepath.Join(dir, "stdout")); err == nil {
		fx.wantStdout = b
	} else if !os.IsNotExist(err) {
		t.Fatalf("fixture %s: read stdout: %v", name, err)
	}

	if b, err := os.ReadFile(filepath.Join(dir, "stderr")); err == nil {
		fx.wantStderr = b
	} else if !os.IsNotExist(err) {
		t.Fatalf("fixture %s: read stderr: %v", name, err)
	}

	exitBytes, err := os.ReadFile(filepath.Join(dir, "exit"))
	if err != nil {
		t.Fatalf("fixture %s: read exit: %v", name, err)
	}
	exitStr := strings.TrimSpace(string(exitBytes))
	exitCode, err := strconv.Atoi(exitStr)
	if err != nil {
		t.Fatalf("fixture %s: parse exit %q: %v", name, exitStr, err)
	}
	fx.wantExit = exitCode

	return fx
}

// runFixture invokes the binary as a black box and returns the captured
// stdout/stderr/exit. It does NOT compare; the comparison happens in the
// caller so that the negative sanity-check test (Test "harness detects a
// single-byte stdout diff") can exercise the comparison path explicitly.
func runFixture(t *testing.T, fx fixture) (stdout, stderr []byte, exit int) {
	t.Helper()
	cmd := exec.Command(binaryPath, fx.args...)
	if fx.hasStdin {
		cmd.Stdin = bytes.NewReader(fx.stdin)
	} else {
		cmd.Stdin = bytes.NewReader(nil)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exit = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exit = exitErr.ExitCode()
		} else {
			t.Fatalf("fixture %s: cmd.Run: %v", fx.name, err)
		}
	}
	return outBuf.Bytes(), errBuf.Bytes(), exit
}

// compareFixture is the pure byte-comparison primitive used by both the live
// harness (assertFixture) and the harness's own sanity-check test. It returns
// a list of human-readable mismatch lines (empty means "everything matches").
// Keeping it pure means we can verify the comparison logic itself without
// reaching into testing-framework internals.
func compareFixture(fx fixture, gotStdout, gotStderr []byte, gotExit int) []string {
	var diffs []string
	if !bytes.Equal(gotStdout, fx.wantStdout) {
		diffs = append(diffs, fmt.Sprintf("stdout mismatch\n  got:  %q\n  want: %q", gotStdout, fx.wantStdout))
	}
	if !bytes.Equal(gotStderr, fx.wantStderr) {
		diffs = append(diffs, fmt.Sprintf("stderr mismatch\n  got:  %q\n  want: %q", gotStderr, fx.wantStderr))
	}
	if gotExit != fx.wantExit {
		diffs = append(diffs, fmt.Sprintf("exit code: got %d, want %d", gotExit, fx.wantExit))
	}
	return diffs
}

func assertFixture(t *testing.T, fx fixture) {
	t.Helper()
	gotStdout, gotStderr, gotExit := runFixture(t, fx)
	for _, d := range compareFixture(fx, gotStdout, gotStderr, gotExit) {
		t.Errorf("fixture %s: %s", fx.name, d)
	}
}

// TestHarnessDetectsSingleByteStdoutDiff is acceptance criterion #8: the
// fixture harness's comparison must be byte-exact, so a one-byte difference
// between actual and expected stdout is detected as a failure. We exercise
// compareFixture directly with a synthetic fixture whose wantStdout differs
// from the "got" stdout by exactly one byte; the comparison must report a
// mismatch. (If the harness ever loosened to a substring or whitespace-
// insensitive compare, this test would catch it.)
func TestHarnessDetectsSingleByteStdoutDiff(t *testing.T) {
	want := []byte(`{"frontmatter":null,"ast":{"type":"root","children":[]}}`)
	// Same length, differing in exactly one byte (the trailing brace flipped
	// from } to ] — a real-world failure mode for a hand-edited fixture).
	got := []byte(`{"frontmatter":null,"ast":{"type":"root","children":[]}]`)
	if len(want) != len(got) {
		t.Fatalf("setup error: want and got should be same length; %d vs %d", len(want), len(got))
	}
	// Count the differing bytes; must be exactly 1.
	diffBytes := 0
	for i := range want {
		if want[i] != got[i] {
			diffBytes++
		}
	}
	if diffBytes != 1 {
		t.Fatalf("setup error: want and got should differ in exactly 1 byte, differ in %d", diffBytes)
	}
	fx := fixture{
		name:       "synthetic-one-byte-diff",
		wantStdout: want,
		wantStderr: nil,
		wantExit:   0,
	}
	diffs := compareFixture(fx, got, nil, 0)
	if len(diffs) == 0 {
		t.Errorf("compareFixture failed to detect a one-byte stdout difference; harness is not byte-exact")
	}
}

// TestFixtures walks testdata/fixtures and runs every fixture directory
// against the built binary. New slices add new directories; the harness
// itself does not change.
func TestFixtures(t *testing.T) {
	root := filepath.Join("testdata", "fixtures")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read fixtures root %s: %v", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		fx := loadFixture(t, dir)
		t.Run(fx.name, func(t *testing.T) {
			assertFixture(t, fx)
		})
	}
}
