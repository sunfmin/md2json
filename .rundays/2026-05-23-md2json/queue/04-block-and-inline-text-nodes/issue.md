# S04: Translate paragraphs, headings, text, emphasis, strong

Status: ready-for-agent

The translate stage learns to emit the simplest block-and-inline mdast node types so a hand-written hello-world Markdown round-trips into a meaningful AST. After this slice, a paragraph of text with emphasis or bold spans produces `paragraph` containing `text`, `emphasis`, and `strong` nodes; a heading line produces `heading{depth}` with its inline children. The mdast field-ordering convention (`type` first, then type-specific fields like `depth`, then `children`, then `position`) lands here as the uniform shape for every emitted node, including the empty-document `root` from S03.

## Acceptance

- [ ] Fixture: `# Hello` produces an `ast` whose `root.children` is one `heading` node with `depth: 1` and a single `text` child with `value: "Hello"`. Exit `0`.
- [ ] Fixture: a heading of each depth `##` through `######` produces `heading.depth` values `2` through `6` respectively.
- [ ] Fixture: `*hello*` inside a paragraph produces a `paragraph` containing one `emphasis` node containing one `text` node with `value: "hello"`.
- [ ] Fixture: `**hello**` produces `strong` similarly.
- [ ] Fixture: a paragraph of plain `Hello world.` produces a `paragraph` with one `text` child whose `value` is exactly `Hello world.` (no trailing newline, no leading whitespace).
- [ ] Fixture with multiple consecutive paragraphs produces multiple sibling `paragraph` nodes under `root.children`, in source order.

## Blocked by

S03 — needs the real `read → parse → translate → emit` pipeline in place.
