# S07: Translate GFM extensions — tables, task lists, strikethrough, autolinks

Status: ready-for-agent

The translate stage covers the GFM-specific node set: `table` carries `align` as a per-column array of `"left" | "right" | "center" | null`, with `tableRow` and `tableCell` children (no per-cell `align`); GFM task list items hoist the checkbox state onto `listItem.checked` as `true` or `false` and drop the goldmark `TaskCheckBox` child node entirely; strikethrough emits as `delete` (containing inline children); bare-URL and `<https://…>` autolinks collapse to mdast `link{url, title: null}` whose single child is a `text` with `value` equal to the URL — goldmark's `AutoLink` type does not appear on the wire.

## Acceptance

- [ ] Fixture: a 3-column GFM table with `:--`, `---`, `--:` alignments produces a `table` with `align: ["left", null, "right"]` and `tableRow`/`tableCell` children; no `tableCell` carries an `align` field.
- [ ] Fixture: `- [x] done\n- [ ] todo\n- plain` produces a `list` with three `listItem` children whose `checked` values are `true`, `false`, `null` in that order; none of them has a checkbox-shaped child node.
- [ ] Fixture: `~~struck~~` inside a paragraph produces a `delete` node containing a `text` child with `value: "struck"`.
- [ ] Fixture: a paragraph containing `<https://example.com>` produces a `link` with `url: "https://example.com"`, `title: null`, and a single `text` child with `value: "https://example.com"`.
- [ ] Fixture: a paragraph containing a bare URL `https://example.com` (GFM autolink) also produces a `link` with the same shape — no `AutoLink` type, no goldmark-native names, on the wire.

## Blocked by

S06 — needs `link`, `listItem`, and the inline-translation scaffolding in place.
