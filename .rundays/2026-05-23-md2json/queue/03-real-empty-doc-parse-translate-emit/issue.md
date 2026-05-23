# S03: Real parse → translate → emit pipeline for the empty-document baseline (with BOM/CRLF end-to-end fixtures)

Status: ready-for-agent

The hard-coded envelope from S01 is replaced by a real pipeline: the read stage hands normalized bytes to a parse stage built on goldmark (with GFM, frontmatter, and footnote extensions enabled at registration time), which returns a goldmark AST; a translate stage walks that AST and produces an mdast-shaped Go value tree rooted at `root`; an emit stage encodes that tree as the JSON envelope and writes it to stdout. For empty input the translate stage produces `root` with empty `children` and the emit stage writes the v1 ship-criterion envelope. No node types beyond `root` are translated in this slice — block and inline nodes are stubbed/skipped — but the wiring of all five modules (`cli` → `read` → `parse` → `translate` → `emit`) is real end-to-end. The `--no-position` flag now does something observable (drops the `position` key on `root`); `--frontmatter-only` short-circuits before `translate` and emits the frontmatter value (which is `null` for the empty document). Because the pipeline now consumes the normalized bytes, BOM-stripped and CRLF-normalized inputs are end-to-end observable for the first time and get fixtures here.

## What to build

`cli` calls `read`, then `parse`, then (unless `--frontmatter-only` short-circuits) `translate`, then `emit`. Errors bubble up as typed values. Each module takes its IO sources/sinks and options as arguments — no module reaches into process globals. `emit` compact mode is the default; key ordering for `root` is `type`, `children`, `position` (the v1 node-set declared order). `--no-position` drops the `position` key. `--frontmatter-only` for the empty document emits the JSON literal `null`.

## Acceptance

- [ ] `md2json --no-position < empty.md` (zero-byte input) writes exactly `{"frontmatter":null,"ast":{"type":"root","children":[]}}` to stdout (no trailing newline) and exits `0`. This is the v1 ship criterion's first half.
- [ ] `md2json < empty.md` (default, no `--no-position`) writes `{"frontmatter":null,"ast":{"type":"root","children":[],"position":{"start":{"line":1,"column":1,"offset":0},"end":{"line":1,"column":1,"offset":0}}}}` to stdout (no trailing newline) and exits `0`. This is the v1 ship criterion's second half.
- [ ] The goldmark parser is constructed with GFM, frontmatter, and footnote extensions registered; running `md2json --frontmatter-only < empty.md` writes `null` to stdout and exits `0`.
- [ ] The translate stage's output for an empty document is a Go value tree (not a goldmark node), with `type: "root"` at the top, ready for JSON encoding by `emit`.
- [ ] Bytes flow `read → parse → translate → emit`; no module reaches into process globals (`os.Args`, `os.Stdin/Stdout/Stderr`, `os.Exit`) — these are injected at the top level only.
- [ ] End-to-end fixture: a file containing exactly a leading UTF-8 BOM (`0xEF 0xBB 0xBF`) and no other bytes produces the same output as the empty-document fixture (default and `--no-position` both byte-identical to their no-BOM counterparts). This proves the BOM is stripped before the pipeline sees the bytes.
- [ ] End-to-end fixture: a file with content separated by CRLF (`\r\n`) produces byte-identical stdout to the LF-equivalent fixture under both default and `--no-position` modes. This proves CRLF normalization happens before the pipeline observes the bytes.

## Blocked by

S02 — needs the read module shipping normalized bytes.
