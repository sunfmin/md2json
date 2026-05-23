# S05: Translate lists, blockquote, thematicBreak, hard break

Status: ready-for-agent

The translate stage gains the remaining basic block-level nodes plus the hard line break: `list` with its `ordered`, `start`, `spread` fields; `listItem` with `spread` (and `checked: null` for non-task items — task hoisting comes later); `blockquote`; `thematicBreak`; and `break` for trailing-two-space or backslash-escaped newlines inside a paragraph. Reference- and image-reference handling is deferred to a later slice; this slice covers only the unambiguous block-level shapes plus hard breaks. Per CONTEXT.md's **Text/Code value preservation** entry, `text.value` is the literal textual run from the normalized source with no extra trimming applied by `translate` — the hard-break case `line1<two-spaces>\nline2` produces two `text` nodes whose values are exactly `"line1"` and `"line2"` because the two trailing spaces are consumed by the `break` boundary and the leading-of-line2 has no whitespace.

## Acceptance

- [ ] Fixture: `- a\n- b` produces a `list` with `ordered: false`, `start: null`, and two `listItem` children, each containing a `paragraph` with a `text` whose `value` is exactly `"a"` and `"b"` respectively.
- [ ] Fixture: `1. a\n2. b` produces a `list` with `ordered: true` and `start: 1`; an ordered list starting at `3.` produces `start: 3`.
- [ ] Fixture: `> quoted` produces a `blockquote` containing a `paragraph` with a `text` child whose `value` is exactly `"quoted"`.
- [ ] Fixture: `---` on its own line produces a `thematicBreak` (asserts the case where `---` is a horizontal rule, distinct from frontmatter).
- [ ] Fixture: a paragraph containing `line1<two-spaces>\nline2` produces a `paragraph` with three children: `text` with `value: "line1"`, then `break`, then `text` with `value: "line2"`. The trailing two spaces are consumed by the `break`; no whitespace survives at the end of `"line1"` or the start of `"line2"`. (Per CONTEXT.md **Text/Code value preservation**.)
- [ ] Fixture: a paragraph containing `line1\\\nline2` (backslash-escaped newline) produces the same `text`, `break`, `text` shape with values `"line1"` and `"line2"`.
- [ ] Fixture: every non-task `listItem` produced in this slice's fixtures carries `checked: null` (never elided, never omitted).

## Blocked by

S04 — needs the translate scaffolding and paragraph/text shape in place.
