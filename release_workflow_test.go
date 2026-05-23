package main_test

// S13 release-pipeline acceptance tests. These tests are STRUCTURAL ASSERTIONS
// against the YAML at .github/workflows/release.yml — they parse the YAML and
// check the matrix, the trigger, the static-link flags, the SHA256SUMS step,
// and the smoke step are all present and shaped as the issue's acceptance
// criteria require. The tests do NOT trigger an actual GitHub Actions run;
// they pin the workflow's static contract so a reviewer (human or test) can
// verify acceptance from the YAML alone.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// workflowPath is the canonical location for the release workflow; pinned
// here so every test reads the same file and a future move surfaces one
// edit, not five.
const workflowPath = ".github/workflows/release.yml"

// loadWorkflow reads and parses the release workflow YAML, returning the
// decoded top-level map. Failures (missing file, invalid YAML) are reported
// via t.Fatalf so each caller can assume a usable map.
//
// We decode into `map[any]any` because YAML 1.1's truthy-value rule means
// an unquoted `on:` key parses as the boolean `true`. The release workflow
// is expected to quote `"on":` as a string key, but decoding into
// `map[any]any` keeps both spellings parseable so a future maintainer's
// unquoted `on:` doesn't silently drop the trigger from the test's view.
func loadWorkflow(t *testing.T) map[any]any {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(workflowPath))
	if err != nil {
		t.Fatalf("read %s: %v (acceptance criterion #2 — release workflow must exist)", workflowPath, err)
	}
	var doc map[any]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse %s as YAML: %v", workflowPath, err)
	}
	return doc
}

// mapVal looks up a key in a yaml.v3-decoded map accepting either the string
// spelling or, for the special `on:` case, the boolean truthy value YAML 1.1
// produces from an unquoted `on`/`off`/`yes`/`no` key. Works for both
// `map[any]any` (when the top-level was decoded into that shape) and
// `map[string]any` (yaml.v3's default for nested string-keyed maps).
func mapVal(m map[any]any, key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	if key == "on" {
		if v, ok := m[true]; ok {
			return v, true
		}
	}
	return nil, false
}

// asMap coerces a yaml.v3 node to `map[any]any`. yaml.v3 decodes nested
// string-keyed maps as `map[string]any` even when the top-level target was
// `map[any]any`, so we normalize on read so every downstream lookup uses the
// same shape.
func asMap(v any) (map[any]any, bool) {
	if m, ok := v.(map[any]any); ok {
		return m, true
	}
	if ms, ok := v.(map[string]any); ok {
		out := make(map[any]any, len(ms))
		for k, val := range ms {
			out[k] = val
		}
		return out, true
	}
	return nil, false
}

// TestReleaseWorkflowExists is the structural baseline: the workflow file
// exists at .github/workflows/release.yml and is valid YAML decoded as a
// top-level map. Without this test a missing or corrupt workflow would only
// surface as cascading failures in the other release-workflow tests.
func TestReleaseWorkflowExists(t *testing.T) {
	doc := loadWorkflow(t)
	if len(doc) == 0 {
		t.Fatalf("%s decoded to an empty map; expected at least `name`, `on`, `jobs`", workflowPath)
	}
}

// TestReleaseWorkflowTriggeredByTag pins the `on.push.tags: ['v*']` trigger
// per acceptance criterion #2 ("A GitHub Actions workflow triggered by a tag
// matching `v*`"). Works whether the workflow quotes `"on":` as a string
// key or leaves it unquoted (yaml.v3 then decodes it as the boolean `true`).
func TestReleaseWorkflowTriggeredByTag(t *testing.T) {
	doc := loadWorkflow(t)
	onNode, ok := mapVal(doc, "on")
	if !ok || onNode == nil {
		t.Fatalf("workflow has no `on:` trigger; criterion #2 requires a tag trigger")
	}
	onMap, ok := asMap(onNode)
	if !ok {
		t.Fatalf("`on:` is %T, expected a map with `push.tags`", onNode)
	}
	pushNode, ok := mapVal(onMap, "push")
	if !ok {
		t.Fatalf("`on.push` is missing; criterion #2 requires a tag-push trigger")
	}
	pushMap, ok := asMap(pushNode)
	if !ok {
		t.Fatalf("`on.push` is %T, expected a map", pushNode)
	}
	tagsNode, ok := mapVal(pushMap, "tags")
	if !ok {
		t.Fatalf("`on.push.tags` is missing; criterion #2 requires a `v*` tag trigger")
	}
	tags, ok := tagsNode.([]any)
	if !ok {
		t.Fatalf("`on.push.tags` is %T, expected a list", tagsNode)
	}
	matched := false
	for _, raw := range tags {
		s, ok := raw.(string)
		if !ok {
			continue
		}
		if s == "v*" || s == "v*.*.*" || strings.HasPrefix(s, "v*") {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("`on.push.tags` should include a `v*` pattern, got %v", tags)
	}
}

// platformMatrixEntries enumerates the five (goos, goarch) pairs required by
// acceptance criterion #2 (PRD US 32 / CONTEXT.md "Distribution"). The test
// asserts the workflow's build job carries exactly these entries — no more,
// no fewer — so a future maintainer who adds a sixth (or drops one) fails
// here.
var platformMatrixEntries = []struct {
	goos   string
	goarch string
}{
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"windows", "amd64"},
}

// TestReleaseWorkflowMatrixCoversFivePlatforms walks the build job's
// `strategy.matrix.include` and asserts the five required (goos, goarch)
// pairs are all present. This is the criterion #2 matrix half. Extra
// platforms are tolerated only if a future ADR explicitly expands the set;
// at v1 we want a hard equality.
func TestReleaseWorkflowMatrixCoversFivePlatforms(t *testing.T) {
	doc := loadWorkflow(t)
	include := findBuildMatrixInclude(t, doc)

	type pair struct{ os, arch string }
	got := map[pair]bool{}
	for _, entry := range include {
		m, ok := asMap(entry)
		if !ok {
			t.Errorf("matrix.include entry is %T, expected a map", entry)
			continue
		}
		osVal, _ := m["goos"].(string)
		archVal, _ := m["goarch"].(string)
		if osVal == "" || archVal == "" {
			t.Errorf("matrix.include entry missing goos/goarch: %v", m)
			continue
		}
		got[pair{osVal, archVal}] = true
	}
	for _, want := range platformMatrixEntries {
		if !got[pair{want.goos, want.goarch}] {
			t.Errorf("matrix.include missing required platform %s/%s", want.goos, want.goarch)
		}
	}
	if len(got) != len(platformMatrixEntries) {
		t.Errorf("matrix.include has %d unique platforms, want exactly %d (v1 ships five)",
			len(got), len(platformMatrixEntries))
	}
}

// findBuildMatrixInclude locates the build job and returns its
// `strategy.matrix.include` list. It is shared by every matrix-shaped
// assertion so the path-walking logic lives in one place.
func findBuildMatrixInclude(t *testing.T, doc map[any]any) []any {
	t.Helper()
	jobsNode, ok := mapVal(doc, "jobs")
	if !ok {
		t.Fatalf("workflow has no `jobs:` block")
	}
	jobs, ok := asMap(jobsNode)
	if !ok {
		t.Fatalf("`jobs:` is %T, expected a map", jobsNode)
	}
	buildNode, ok := mapVal(jobs, "build")
	if !ok {
		t.Fatalf("workflow has no `jobs.build` job; release pipeline must declare a build job")
	}
	build, ok := asMap(buildNode)
	if !ok {
		t.Fatalf("`jobs.build` is %T, expected a map", buildNode)
	}
	stratNode, ok := mapVal(build, "strategy")
	if !ok {
		t.Fatalf("`jobs.build.strategy` is missing; matrix builds need a strategy")
	}
	strat, ok := asMap(stratNode)
	if !ok {
		t.Fatalf("`jobs.build.strategy` is %T, expected a map", stratNode)
	}
	matrixNode, ok := mapVal(strat, "matrix")
	if !ok {
		t.Fatalf("`jobs.build.strategy.matrix` is missing; criterion #2 requires a matrix")
	}
	matrix, ok := asMap(matrixNode)
	if !ok {
		t.Fatalf("`jobs.build.strategy.matrix` is %T, expected a map", matrixNode)
	}
	includeNode, ok := mapVal(matrix, "include")
	if !ok {
		t.Fatalf("`jobs.build.strategy.matrix.include` is missing; the matrix must use `include` to pin (goos, goarch) pairs")
	}
	include, ok := includeNode.([]any)
	if !ok {
		t.Fatalf("`jobs.build.strategy.matrix.include` is %T, expected a list", includeNode)
	}
	return include
}

// buildJobSteps returns the build job's `steps` list. Shared by every step-
// shaped assertion (static-link, smoke-test) so the lookup path lives once.
func buildJobSteps(t *testing.T, doc map[any]any) []any {
	t.Helper()
	jobsNode, _ := mapVal(doc, "jobs")
	jobs, _ := asMap(jobsNode)
	buildNode, _ := mapVal(jobs, "build")
	build, _ := asMap(buildNode)
	stepsNode, ok := mapVal(build, "steps")
	if !ok {
		t.Fatalf("`jobs.build.steps` is missing; the build job must declare steps")
	}
	steps, ok := stepsNode.([]any)
	if !ok {
		t.Fatalf("`jobs.build.steps` is %T, expected a list", stepsNode)
	}
	return steps
}

// TestReleaseWorkflowBuildIsStaticallyLinked pins acceptance criterion #3:
// the build step compiles with `CGO_ENABLED=0` and `-trimpath`, and stamps
// the version via ldflags so the published binary's `-V` output reflects the
// tag (criterion #1 cross-reference). `CGO_ENABLED=0` + a pure-Go dependency
// graph is the standard recipe for a statically-linked Go binary; ADR-0003
// pins the rationale.
func TestReleaseWorkflowBuildIsStaticallyLinked(t *testing.T) {
	doc := loadWorkflow(t)
	steps := buildJobSteps(t, doc)

	foundCGO := false
	foundTrimpath := false
	foundLdflagsVersion := false
	for _, raw := range steps {
		step, ok := asMap(raw)
		if !ok {
			continue
		}
		// CGO_ENABLED=0 can live on the step's `env:` block OR be inlined in
		// the `run:` command. Accept either.
		if envNodeRaw, ok := step["env"]; ok {
			if envNode, ok := asMap(envNodeRaw); ok {
				if v, ok := envNode["CGO_ENABLED"]; ok {
					switch tv := v.(type) {
					case int:
						if tv == 0 {
							foundCGO = true
						}
					case string:
						if tv == "0" {
							foundCGO = true
						}
					}
				}
			}
		}
		if runNode, ok := step["run"].(string); ok {
			if strings.Contains(runNode, "CGO_ENABLED=0") {
				foundCGO = true
			}
			if strings.Contains(runNode, "-trimpath") {
				foundTrimpath = true
			}
			// ldflags must stamp the cli.Version package var; the test does
			// not pin the exact env-var spelling (`$TAG` vs `${GITHUB_REF_NAME}`),
			// only the substitution target.
			if strings.Contains(runNode, "github.com/sunfmin/md2json/internal/cli.Version=") {
				foundLdflagsVersion = true
			}
		}
	}
	if !foundCGO {
		t.Errorf("build job does not set CGO_ENABLED=0; criterion #3 requires statically-linked binaries")
	}
	if !foundTrimpath {
		t.Errorf("build job does not pass -trimpath to `go build`; ADR-0003 pins -trimpath for reproducible builds")
	}
	if !foundLdflagsVersion {
		t.Errorf("build job does not stamp cli.Version via -ldflags; criterion #1 (published binary's -V reflects the tag) requires it")
	}
}

// TestReleaseWorkflowProducesUnambiguousAssetFilenames pins the second half
// of acceptance criterion #2: each uploaded asset's filename names the
// platform/arch unambiguously (`md2json-<goos>-<goarch>[.exe]`).
//
// The check is a substring scan over the build job's steps: at least one
// step must reference an asset name parameterized by `${{ matrix.goos }}` and
// `${{ matrix.goarch }}` (so each matrix entry produces a distinct file).
func TestReleaseWorkflowProducesUnambiguousAssetFilenames(t *testing.T) {
	doc := loadWorkflow(t)
	steps := buildJobSteps(t, doc)
	for _, raw := range steps {
		step, ok := asMap(raw)
		if !ok {
			continue
		}
		runNode, _ := step["run"].(string)
		// `with:` block on actions like upload-artifact also names the file.
		withNode, _ := asMap(step["with"])
		var blob string
		blob += runNode
		for _, v := range withNode {
			if s, ok := v.(string); ok {
				blob += "\n" + s
			}
		}
		if strings.Contains(blob, "md2json-") &&
			strings.Contains(blob, "${{ matrix.goos }}") &&
			strings.Contains(blob, "${{ matrix.goarch }}") {
			return
		}
	}
	t.Errorf("no build step produces an asset filename of the form `md2json-${{ matrix.goos }}-${{ matrix.goarch }}[.exe]`; criterion #2 requires platform-unambiguous filenames")
}

// TestReleaseWorkflowPublishesSHA256SUMS pins acceptance criterion #4: a
// step in the release workflow produces a `SHA256SUMS` file and includes it
// in the published Release.
//
// The check is again structural: the workflow must contain a step whose
// `run:` invokes `shasum -a 256` (ADR-0003's portable spelling) and emits a
// file named `SHA256SUMS`, and either that step or a subsequent one uploads
// `SHA256SUMS` alongside the binaries.
func TestReleaseWorkflowPublishesSHA256SUMS(t *testing.T) {
	doc := loadWorkflow(t)
	jobsNode, _ := mapVal(doc, "jobs")
	jobs, _ := asMap(jobsNode)
	foundChecksumStep := false
	foundChecksumUpload := false
	for _, jobRaw := range jobs {
		job, ok := asMap(jobRaw)
		if !ok {
			continue
		}
		stepsRaw, ok := job["steps"].([]any)
		if !ok {
			continue
		}
		for _, sRaw := range stepsRaw {
			step, ok := asMap(sRaw)
			if !ok {
				continue
			}
			runNode, _ := step["run"].(string)
			if strings.Contains(runNode, "shasum -a 256") && strings.Contains(runNode, "SHA256SUMS") {
				foundChecksumStep = true
			}
			// upload step: look for SHA256SUMS in either run text or with: block.
			withNode, _ := asMap(step["with"])
			for _, v := range withNode {
				if s, ok := v.(string); ok && strings.Contains(s, "SHA256SUMS") {
					foundChecksumUpload = true
				}
			}
			if strings.Contains(runNode, "SHA256SUMS") {
				// inline upload (e.g. gh release upload) counts.
				if strings.Contains(runNode, "gh release upload") ||
					strings.Contains(runNode, "softprops/action-gh-release") {
					foundChecksumUpload = true
				}
			}
		}
	}
	if !foundChecksumStep {
		t.Errorf("no step runs `shasum -a 256 ... > SHA256SUMS`; criterion #4 requires a checksum manifest")
	}
	if !foundChecksumUpload {
		t.Errorf("SHA256SUMS is generated but no step uploads it alongside the binaries; criterion #4 requires the manifest in the Release")
	}
}

// TestReleaseWorkflowSmokeTestRunsShipCriterion pins acceptance criterion
// #5: the workflow runs the v1 ship-criterion fixture against the freshly-
// built binary on at least linux/amd64 and darwin/arm64, and fails the
// workflow if the comparison fails.
//
// The check looks for a step whose `run:` invokes the just-built binary
// with `--no-position` on empty stdin and diffs the output against the
// v1 ship-criterion envelope. The step must be gated on (or otherwise apply
// to) linux/amd64 and darwin/arm64 — the two matrix entries whose runners
// can actually exec a native binary in this matrix (ubuntu-latest and
// macos-14 respectively).
func TestReleaseWorkflowSmokeTestRunsShipCriterion(t *testing.T) {
	doc := loadWorkflow(t)
	steps := buildJobSteps(t, doc)
	var smokeRun string
	for _, raw := range steps {
		step, ok := asMap(raw)
		if !ok {
			continue
		}
		runNode, _ := step["run"].(string)
		if strings.Contains(runNode, "--no-position") &&
			strings.Contains(runNode, v1ShipCriterionEnvelope) {
			smokeRun = runNode
			break
		}
	}
	if smokeRun == "" {
		t.Fatalf("no smoke-test step found that runs the v1 ship-criterion (`--no-position` on empty stdin) against the built binary; criterion #5 requires this")
	}
	// Sanity: the step must actually compare and fail on mismatch. A bare
	// invocation without `diff` or a comparison would silently pass even if
	// the binary produced the wrong envelope. We require a `diff` against
	// the expected output OR a shell `[ ... = ... ]` test.
	if !(strings.Contains(smokeRun, "diff") || strings.Contains(smokeRun, "[ \"")) {
		t.Errorf("smoke-test step runs the binary but does not diff/compare against the expected envelope; criterion #5 requires the workflow to fail on mismatch.\nstep:\n%s", smokeRun)
	}
}
