package cli

import (
	"strings"
	"testing"
)

// S13 Test 1 (acceptance criterion #1 — the `-V` output of a `go install`-ed
// binary must reflect a version string that the release pipeline can stamp at
// link time). The contract is: package-level `var Version` is the single
// source of truth for the `-V` / `--version` output, so the release workflow
// can do
//
//	go build -ldflags="-X github.com/sunfmin/md2json/internal/cli.Version=$TAG" .
//
// to bake the tag (e.g. `v1.2.3`) into the published binary without touching
// source. The default value is a non-empty placeholder so a plain `go build .`
// or `go install ./...@latest` still produces a sensible `-V` line.
//
// This test exercises both halves:
//  1. The default `Version` value appears in the `-V` output verbatim.
//  2. Overriding `Version` at test time (the same mechanism `-ldflags -X`
//     uses at link time) changes the `-V` output. Restoring the original
//     value in `t.Cleanup` keeps the test suite hermetic.
func TestVersionFlagUsesBuildStampedVariable(t *testing.T) {
	// Default value half: the `-V` output must include the current `Version`.
	stdout, stderr, exit := run(t, []string{"md2json", "-V"}, "")
	if exit != 0 {
		t.Fatalf("exit code: got %d, want 0", exit)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty on -V, got %q", stderr)
	}
	if Version == "" {
		t.Fatalf("Version package var should default to a non-empty placeholder; got empty string")
	}
	if !strings.Contains(stdout, Version) {
		t.Errorf("default -V output should contain Version=%q, got %q", Version, stdout)
	}

	// Override half: stamping Version at "link time" (here simulated by
	// directly assigning the package var) changes the -V output. This is the
	// exact substitution `go build -ldflags="-X .../cli.Version=v1.2.3"`
	// applies in the release workflow.
	orig := Version
	t.Cleanup(func() { Version = orig })
	const stamped = "v9.9.9-release-pipeline-test"
	Version = stamped
	stdout2, stderr2, exit2 := run(t, []string{"md2json", "-V"}, "")
	if exit2 != 0 {
		t.Fatalf("exit code (stamped): got %d, want 0", exit2)
	}
	if stderr2 != "" {
		t.Errorf("stderr should be empty on -V (stamped), got %q", stderr2)
	}
	if !strings.Contains(stdout2, stamped) {
		t.Errorf("stamped -V output should contain Version=%q, got %q", stamped, stdout2)
	}
}
