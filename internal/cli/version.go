package cli

// Version is the build-stamped version string surfaced by the `-V` /
// `--version` flag. It is a package-level `var` (not a const) so the release
// workflow can override it at link time:
//
//	go build -ldflags="-X github.com/sunfmin/md2json/internal/cli.Version=$TAG" .
//
// For local development and unstamped `go install ./...@latest` invocations
// the default `"dev"` value is what users see. The release workflow
// (.github/workflows/release.yml) stamps the real semver tag (e.g. `v1.2.3`)
// into this var so the published binary's `-V` output is the same string as
// the GitHub Release name. Pinned in ADR-0003.
//
// Kept in its own file so the build-stamp surface is one symbol in one place,
// and the release-pipeline maintainer can change the default or the format
// without re-reading the rest of `cli`.
var Version = "dev"
