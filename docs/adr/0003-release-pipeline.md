# ADR-0003: release pipeline — toolchain, static-link strategy, checksum tool

- Status: Accepted
- Date: 2026-05-23
- Decider: PO (resolved during S13)

## Context

CONTEXT.md "Distribution" + PRD US 31–32 + issue 12 require two install paths for v1: `go install github.com/sunfmin/md2json@latest` (primary) and prebuilt static binaries on GitHub Releases for `darwin/{amd64,arm64}`, `linux/{amd64,arm64}`, `windows/amd64` (secondary). The release pipeline that produces the Releases needs three load-bearing decisions pinned somewhere a future maintainer will find them: how the binaries are statically linked, how the build is reproducible across runners, and which checksum tool emits the manifest.

Constraints from the acceptance criteria of issue 12:
- Each uploaded binary must be statically linked ("no required runtime beyond the OS").
- A `SHA256SUMS` (or equivalent) checksum file ships alongside the binaries.
- A smoke test runs the v1 ship-criterion against the freshly-built binary on at least linux/amd64 and darwin/arm64.
- Asset filenames must name the platform/arch unambiguously.

Constraints from the existing module:
- Pure-Go dependency graph (`goldmark`, `go.abhg.dev/goldmark/frontmatter`, `gopkg.in/yaml.v3`, transitive `github.com/BurntSushi/toml` — all pure Go, no cgo callees).
- `-V` output is the single user-visible knob the release pipeline can stamp at link time (see `internal/cli/version.go`).

## Decision

1. **`CGO_ENABLED=0` + pure-Go deps = statically-linked binary.** Set `CGO_ENABLED=0` on the build step's `env:` block AND in the `go build` command line for visibility. Because every transitive dependency is pure Go, the resulting binary on Linux is fully static under the default `internal` linkmode; on macOS and Windows the binary has no external dependencies beyond the OS-supplied libc/syscalls, which is the standard meaning of "no required runtime beyond the OS" for those platforms. No `-extldflags '-static'` is needed (it would force the external linker path; we want the internal linker so cross-compilation Just Works from a Linux runner targeting macOS without an Apple toolchain).

2. **`-trimpath` + `-ldflags="-s -w"` for reproducible, small binaries.** `-trimpath` strips the host filesystem path from every binary so two CI runs on different runners produce byte-identical binaries (modulo embedded build-time strings like the version stamp). `-s -w` strips the symbol table and DWARF debug info, shrinking the binary by ~30 % without affecting runtime behavior (no panics-with-line-numbers in production use — error messages are constructed by `cli`, not by the Go runtime).

3. **Version stamping via `-ldflags="-X .../cli.Version=$TAG"`.** The release workflow exports `GITHUB_REF_NAME` as `$TAG` and passes it through `-ldflags -X github.com/sunfmin/md2json/internal/cli.Version=$TAG` so the published binary's `-V` output is the same string as the GitHub Release tag. `internal/cli/version.go` defines `Version` as a `var` (not a `const`) so the link-time substitution takes effect; the default value `"dev"` keeps unstamped `go build .` invocations producing a sensible `-V` line.

4. **`shasum -a 256` for the SHA256SUMS manifest.** Both Ubuntu and macOS GitHub-hosted runners ship `shasum` (Perl-based, part of `perl-base` on Linux, system tool on macOS). `sha256sum` is Linux-only (coreutils) and is absent from macOS by default. Standardizing on `shasum -a 256` would let us move the checksum job between runner OSes without rewriting the command; the workflow as committed pins `ubuntu-latest` for the checksums job, but the portability of the tool is still the better default.

5. **Smoke test = v1 ship-criterion on linux/amd64 + darwin/arm64.** The acceptance criterion requires smoke on *at least* these two; we add no others because no other matrix entry has a GitHub-hosted exec host that matches the cross-compile target. `linux/arm64` could run on `ubuntu-24.04-arm` if/when we move off the free tier; `windows/amd64` could run on `windows-latest`. Both are deferred as runner-cost / setup-complexity tradeoffs not justified by the v1 ship criterion.

6. **Asset filename format: `md2json-<goos>-<goarch>[.exe]`.** Names the platform/arch unambiguously and parses trivially from a download URL. The `.exe` suffix only appears on Windows (so a macOS user does not download `md2json-darwin-arm64.exe` by accident from a tab-completed URL).

7. **GitHub-hosted runners.** `ubuntu-latest` for linux/* + windows cross-compile + checksums + release publish; `macos-14` for darwin/* (the only runner family with an Apple Silicon host that can actually exec a darwin/arm64 binary). No self-hosted runners in v1.

## Consequences

- **Positive (no Apple toolchain required for darwin builds when cross-compiling on Linux).** `CGO_ENABLED=0` + internal linker means a Linux runner can produce a darwin binary without `osxcross` or a real macOS host. The current workflow uses `macos-14` for darwin so we *can* smoke-test natively, but if smoke is ever relaxed, the build itself is portable across all runner OSes.
- **Positive (single source of truth for the version string).** The `-V` output, the GitHub Release name, and the git tag all reference the same identifier (the tag) because the workflow is the only thing that stamps `cli.Version`. A `go install ./...@latest` user sees `dev` instead — acceptable, because the install path is meant for the head-of-trunk developer, not for the user who wants a stable tag.
- **Negative (smoke matrix is asymmetric).** linux/arm64 and windows/amd64 are cross-compile-only; a real exec on those platforms only happens when a user downloads the asset. Mitigated by `go vet ./...` + `go test ./...` on linux/amd64 (which already exercises the same code paths) and by the fact that the binaries are pure-Go cross-compiles (no platform-specific build tag in the module, no syscalls that vary by OS — `os.Stdin/Stdout/Stderr` are the only OS-touching surface and Go's stdlib handles them portably).
- **Negative (build is not bit-for-bit reproducible across distinct CI runs of the same tag).** Two CI runs on the same tag may produce binaries whose embedded build IDs differ. Acceptable for v1; if someone needs bit-for-bit reproducibility, they can re-build locally and compare against the SHA256SUMS manifest.
- **TDD implication.** Acceptance is structural: `release_workflow_test.go` parses the YAML and asserts the matrix, the trigger, the static-link flags, the checksum step, and the smoke step are all shaped as the issue requires. The `go install` install path is exercised in `integration_test.go` (`TestGoInstallProducesBinaryPassingShipCriterion`).

## Cross-references

- `.github/workflows/release.yml` — the workflow this ADR pins.
- `internal/cli/version.go` — the package var stamped by ldflags.
- `release_workflow_test.go` — structural assertions on the workflow.
- CONTEXT.md "Distribution" — pins the five-platform set + the `go install` install path.
- PRD US 31–32 — the user-facing install paths the workflow makes real.

## Out of scope (post-v1)

- Homebrew tap (CONTEXT.md "Distribution" defers it).
- Code-signing / notarization for macOS releases (Apple Developer Program enrollment required; not v1).
- Authenticode signing for the Windows binary.
- A reproducible-builds attestation (SLSA-3+) — interesting but heavier than v1's scope.
- An `arm64` Linux smoke test (`ubuntu-24.04-arm` runner) — defer until a real "this binary segfaulted on arm64" report.
- A Windows smoke test (`windows-latest` runner) — defer until a real Windows user reports an issue; the cross-compile + Go's portable stdlib is a strong-enough proxy for v1.
