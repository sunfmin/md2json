# S13: `go install` + GitHub Releases static binaries for five platforms

Status: ready-for-agent

The tool ships in both supported install paths from the PRD's Distribution section. `go install github.com/<owner>/md2json@latest` produces a working binary on `PATH`. A GitHub Actions release workflow, triggered by a tagged push, builds static binaries for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, and `windows/amd64`, attaches them to a GitHub Release for that tag, and includes a checksum manifest. A smoke test in CI installs the just-built binary on each runner and runs the v1 ship-criterion fixture against it (`md2json --no-position < empty.md` → expected envelope, exit `0`) so the published artifact is exercised end-to-end before the release goes public.

## Acceptance

- [ ] `go install ./...` (or equivalent module-path install) from a clean checkout produces an `md2json` binary on `PATH` that passes the v1 ship-criterion fixture.
- [ ] A GitHub Actions workflow triggered by a tag matching `v*` builds five binaries (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`) and uploads them as Release assets named so the platform/arch is unambiguous from the filename.
- [ ] Each uploaded binary is statically linked (no required runtime beyond the OS), verifiable by inspecting linkage or running on a clean container/VM with no Go toolchain installed.
- [ ] The release workflow publishes a `SHA256SUMS` (or equivalent) checksum file alongside the binaries.
- [ ] The release workflow runs the v1 ship-criterion fixture against the freshly-built binary on at least the `linux/amd64` and `darwin/arm64` runners before publishing the Release, and fails the workflow if either smoke test fails.

## Blocked by

S12 — needs the full CLI contract (exit codes, error format, all v1 flags) shipping so the released binary passes the v1 ship-criterion fixture.
