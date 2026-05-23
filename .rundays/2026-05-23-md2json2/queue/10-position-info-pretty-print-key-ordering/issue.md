# S10: Uniform position info on every node + `--no-position` strips it

Status: ready-for-agent

Every emitted node — including `root` — carries `position: { start: {line, column, offset}, end: {line, column, offset} }` by default, with `line` and `column` 1-indexed, `column` counted in UTF-8 code points within the normalized line, and `offset` a byte offset into the normalized (post-BOM-strip, LF-only) document. `--no-position` drops the `position` key uniformly from every node with no other shape change. This slice pins the position contract; pretty-print formatting, mdast key ordering, null-field preservation, and the silent-drop property test live in S11.

Per CONTEXT.md's **Position info** entry, the uniform rule holds with no per-node special-casing: empty input gives `root` a zero-width `{start:{1,1,0}, end:{1,1,0}}` position (already proven in S03); a single-newline input gives `root` the position `{start:{1,1,0}, end:{2,1,1}}` with empty `children` (the trailing newline advances `line` to 2 and `offset` to 1; a blank line produces no block content under goldmark).

## Acceptance

- [ ] Fixture: `md2json2 < single-newline.md` (input is exactly one `\n` byte) produces `root.position.end == {"line":2,"column":1,"offset":1}`, `root.children == []`, exit `0`.
- [ ] Fixture: a multi-line document's `heading` and inline `text` nodes carry `position` whose `line`, `column`, `offset` match the normalized-document source range; column counts UTF-8 code points (not bytes) — verifiable with a fixture containing a multibyte character (e.g. an emoji or a CJK ideograph) before the asserted node so a byte-count column would disagree with a code-point column.
- [ ] Fixture: every emitted node — `root`, every block child, every inline child — carries a `position` field by default (no exceptions, no per-node opt-out).
- [ ] Fixture: `md2json2 --no-position` on any non-empty fixture produces output identical to the default mode but with the `position` key absent from **every** node (root and all descendants); no other key ordering or field-presence behavior changes.
- [ ] Fixture: BOM-prefixed input produces `position.offset` values relative to the post-BOM document — the first inline node's `offset` is unchanged whether or not the source had a BOM (per ADR-0001's "offsets count against the normalized document").
- [ ] Fixture: CRLF-only input produces the same `position.line` / `position.column` / `position.offset` values as the LF-equivalent input (per ADR-0001's "position fields reflect the normalized document").

## Blocked by

S09 — needs the full translate-stage node-set and the frontmatter lift in place so position fixtures cover the realistic input set.
