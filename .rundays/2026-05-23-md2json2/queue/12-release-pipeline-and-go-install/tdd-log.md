# TDD log: 12-release-pipeline-and-go-install

Started: 2026-05-23

Context note: this slice is a release-pipeline slice, so most acceptance criteria target build artifacts (CI workflow YAML, statically-linked binaries, SHA256SUMS) rather than in-tree code behavior. The TDD red-green loop applies cleanly to the in-tree pieces that the workflow relies on — the build-stamped `Version` string referenced by `-V`, the `go install ./...` smoke path, and structural assertions on the workflow YAML. CI-only acceptance is satisfied by writing the workflow file and asserting (via a Go test that parses the YAML) that the matrix, smoke step, and SHA256SUMS step are present and shaped as the issue requires.

Framework: standard Go `testing` (existing). Workflow YAML is parsed in tests via `gopkg.in/yaml.v3` (already a dep transitively via parse).

## Test 1 — build-stamped Version variable referenced by `-V`

- Wrote: `internal/cli.TestVersionFlagUsesBuildStampedVariable` (asserts the `-V` output contains the value of the package-level `Version` var, and the var defaults to a non-empty placeholder; ldflags-injection contract is the link-time replacement of that var)
- Red: pre-existing `versionText` const is hard-coded; new test temporarily overrides `Version` var and asserts `-V` output reflects it
- Green: introduce `var Version = "dev"` in `internal/cli/version.go`; rewrite `versionText` to be derived at print time from `Version`; test passes
- Notes: keep the surface minimal — one exported `var Version` so the release workflow can do `go build -ldflags="-X github.com/sunfmin/md2json2/internal/cli.Version=$TAG" .`. The default `"dev"` keeps `go build .` and `go install ./...` working with a sensible non-empty value when no ldflags are passed. Acceptance criterion #1 needs the `-V` flag to keep working post-install.

## Test 2 — `go install ./...` from a clean module produces a binary that passes the v1 ship-criterion

- Wrote: `TestGoInstallProducesBinaryPassingShipCriterion` (in `integration_test.go`; uses `go install` with a temp GOPATH/GOBIN, then runs the installed binary against the v1 ship-criterion fixture: `md2json2 --no-position < empty.md` → expected envelope, exit 0)
- Red: at this point the binary should already pass (S12 left it working), so the test was expected to go green immediately on the **first** run. The “red” phase is the gap of having no test that pins the `go install`-installed binary specifically (the existing harness uses `go build -o`, not `go install`). Without this test, the install path is not exercised by `go test ./...`.
- Green: the test runs `go install` into a temp GOBIN; resolves the binary path from `GOBIN/md2json2[.exe]`; feeds empty stdin with `--no-position` and asserts the v1 ship-criterion envelope + exit 0
- Notes: covers acceptance criterion #1. Test is `t.Parallel()`-friendly only in the sense that each invocation gets its own GOBIN; on the host running `go test` this works regardless of the developer's real `$GOBIN`. Network-free (we install our own module by path from the local checkout, so no proxy fetch).

## Test 3 — release workflow YAML exists, is valid YAML, and builds the five-platform matrix

- Wrote: `TestReleaseWorkflowExists`, `TestReleaseWorkflowTriggeredByTag`, `TestReleaseWorkflowMatrixCoversFivePlatforms` in `release_workflow_test.go` (parses `.github/workflows/release.yml` with `yaml.v3` and asserts: file exists; `on.push.tags` matches `v*`; the build job's `strategy.matrix.include` contains exactly the five `(goos, goarch)` pairs from CONTEXT.md "Distribution" / PRD US 32)
- Red: no `.github/workflows/release.yml` yet — `os.ReadFile` returns an error
- Green: write `.github/workflows/release.yml` with `on.push.tags: ['v*']`, a `build` job using a `matrix.include` of the five `(goos, goarch)` pairs, and asset uploads with platform-unambiguous filenames (`md2json2-${{ matrix.goos }}-${{ matrix.goarch }}` with `.exe` for Windows)
- Notes: covers acceptance criterion #2. The test is a structural read of the YAML — it does not run GitHub Actions, but it does enforce that a static reviewer (or a future maintainer) could verify the matrix from the YAML alone.

## Test 4 — static-link enforcement (`CGO_ENABLED=0`) and trimpath on the build step

- Wrote: `TestReleaseWorkflowBuildIsStaticallyLinked` (asserts the build step sets `CGO_ENABLED: 0` and that the `go build` command passes `-trimpath` and ldflags-stamps the version)
- Red: workflow exists from Test 3 but does not yet set `CGO_ENABLED` or `-trimpath`
- Green: extend the build step's `env:` block with `CGO_ENABLED: 0` and the `run:` command with `go build -trimpath -ldflags="-s -w -X github.com/sunfmin/md2json2/internal/cli.Version=${TAG}" -o <name> .`
- Notes: covers acceptance criterion #3. Pure-Go (no cgo callees in goldmark, frontmatter, yaml.v3) means `CGO_ENABLED=0` produces a fully static binary on Linux with the default `internal` linkmode; on macOS and Windows the resulting binary has no external dependencies beyond the OS-supplied libc/syscalls, which is the standard meaning of "no required runtime beyond the OS" for those platforms. Documented in ADR-0003.

## Test 5 — SHA256SUMS step on the release workflow

- Wrote: `TestReleaseWorkflowPublishesSHA256SUMS` (asserts a job step produces a `SHA256SUMS` file covering all five binaries and uploads it as a release asset)
- Red: workflow has no checksum step
- Green: add a `checksums` job (runs after `build`, on `ubuntu-latest`) that downloads all the platform binaries from the previous job's artifact uploads and runs `shasum -a 256 md2json2-* > SHA256SUMS`, then uploads `SHA256SUMS` as a release asset
- Notes: covers acceptance criterion #4. `shasum -a 256` is the portable spelling (works on macOS and Linux); the checksum step always runs on `ubuntu-latest` for consistency. Decision pinned in ADR-0003.

## Test 6 — smoke test step runs v1 ship-criterion against the freshly-built binary on linux/amd64 and darwin/arm64

- Wrote: `TestReleaseWorkflowSmokeTestRunsShipCriterion` (asserts the build job, on linux/amd64 and darwin/arm64 matrix entries, runs a step that pipes empty stdin into the just-built binary with `--no-position` and compares against the v1 ship-criterion envelope; the step exits non-zero if the comparison fails)
- Red: workflow has no smoke step
- Green: add a `Smoke test (v1 ship criterion)` step to the build job that runs on the matrix entries whose runner is a real exec host (linux/amd64 → ubuntu-latest, darwin/arm64 → macos-14). Other matrix entries (linux/arm64, windows/amd64, darwin/amd64) cross-compile only; the criterion explicitly requires smoke on **at least** linux/amd64 and darwin/arm64
- Notes: covers acceptance criterion #5. The smoke step uses `printf '' | ./md2json2 --no-position` and compares against the literal v1 ship-criterion envelope. A `diff` against the expected envelope makes the step fail loudly when the published artifact would not satisfy the v1 ship-criterion.

## ADR-0003 — release-pipeline decisions worth pinning

- New `docs/adr/0003-release-pipeline.md`: pins `CGO_ENABLED=0` + `-trimpath` + ldflags-stamped version as the build mode; `shasum -a 256` as the checksum tool; GitHub-hosted runners (`ubuntu-latest` for linux/amd64 + cross-compiles, `macos-14` for darwin/arm64) as the smoke-test hosts; smoke test is the v1 ship-criterion on at least linux/amd64 and darwin/arm64; release artifacts are named `md2json2-<goos>-<goarch>[.exe]` for unambiguous filenames per acceptance criterion #2.

## Final

- Tests added: 6 (1 in `internal/cli`, 1 in `integration_test.go`, 4 in `release_workflow_test.go`)
- Tests passing: all green
- Acceptance status:
  - [x] criterion 1 — `go install ./...` produces a working binary on `PATH` that passes the v1 ship-criterion (Test 2, plus build-stamped Version via Test 1)
  - [x] criterion 2 — release workflow builds the five platforms (Test 3) with platform-unambiguous asset filenames
  - [x] criterion 3 — statically-linked binaries via `CGO_ENABLED=0` + `-trimpath` (Test 4)
  - [x] criterion 4 — `SHA256SUMS` checksum file published (Test 5)
  - [x] criterion 5 — smoke test on linux/amd64 and darwin/arm64 fails the workflow if the v1 ship-criterion does not hold (Test 6)
