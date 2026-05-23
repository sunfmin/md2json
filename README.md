# md2json

A Unix-filter CLI that parses a single Markdown document (CommonMark + GFM, optional YAML frontmatter) and writes a JSON envelope to stdout:

```json
{ "frontmatter": { ... }, "ast": { "type": "root", "children": [ ... ] } }
```

The `ast` is an [mdast](https://github.com/syntax-tree/mdast)-compatible subset. Parsing is delegated to [goldmark](https://github.com/yuin/goldmark) with its official GFM, frontmatter, and footnote extensions.

Errors go to stderr with a single canonical regex `^md2json: <path>:<line>:<col>: <msg>$` and a non-zero exit code; nothing is written to stdout on failure.

## Install

### Homebrew (macOS / Linux)

```sh
brew install sunfmin/tap/md2json
```

(Or `brew tap sunfmin/tap && brew install md2json`.)

The tap lives at [sunfmin/homebrew-tap](https://github.com/sunfmin/homebrew-tap) and installs prebuilt binaries from this repo's GitHub Releases.

### `go install`

```sh
go install github.com/sunfmin/md2json@latest
```

### Prebuilt binaries

Each tagged release publishes static binaries for `darwin/{amd64,arm64}`, `linux/{amd64,arm64}`, and `windows/amd64`, plus a `SHA256SUMS` manifest. See [the latest release](https://github.com/sunfmin/md2json/releases/latest).

## Usage

```sh
md2json post.md          # parse a file
md2json < post.md        # parse stdin
md2json -                # explicit stdin sentinel
md2json --no-position    # strip mdast `position` fields
md2json --frontmatter-only post.md   # emit only the YAML frontmatter
md2json -V               # version
md2json -h               # full flag/usage help
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | success — JSON envelope on stdout |
| `1` | input error (invalid UTF-8, malformed frontmatter, parse failure) |
| `2` | usage error (unknown flag, bad argv) |

## License

No license yet — treat as "all rights reserved" until one is added.
