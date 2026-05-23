# S01: wire goldmark-mathjax extension into parse without behavior change

Status: ready-for-agent

A user piping a non-math GFM document through md2json gets the same JSON envelope as before this Run — same node types, same `value` bytes, same `position` fields, exit `0`. The math extension is now compiled into the binary and loaded by the parser, but no input in the existing prose / GFM / frontmatter / footnote / table / code-block test corpus produces a `inlineMath` or `math` node on the wire. The single static Go binary still ships with no Node/Python runtime.

## Acceptance

- [ ] `github.com/litao91/goldmark-mathjax` appears as a direct dependency of the module.
- [ ] The parser registers the math extension as part of its standard extension set; no new flag, no opt-in, no environment variable.
- [ ] The full pre-existing test suite (prose, GFM tables, lists, blockquotes, footnotes, frontmatter, code blocks, raw HTML, `--no-position`, `--frontmatter-only`, `--pretty`, exit codes, error format) passes unchanged with the extension loaded.
- [ ] A smoke fixture confirms a non-math GFM blog post (heading + paragraphs + list + fenced code + frontmatter) emits a byte-identical JSON envelope before and after this issue.
- [ ] `go mod tidy` resolves without introducing any new `cgo`-requiring dependency (verifiable by `grep -l cgo $(go list -deps -f '{{.Dir}}' ./...)` returning no new hits beyond the pre-Run baseline).
