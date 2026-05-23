# S06: math composes inside lists, blockquotes, footnotes, and table cells

Status: ready-for-agent

A writer using inline math inside any inline-content container — list items, blockquote paragraphs, footnote definitions, GFM table cells — sees `$...$` land as a child `inlineMath` of the containing paragraph (or directly inside a table cell, per mdast's cell-inline-content shape). A writer placing `$$...$$` at the start of a list-item or blockquote line sees it match as a `math` node child of the container. GFM table cells are inline-only by spec, so `$$x$$` inside a cell falls to the library's inline parser and matches as `inlineMath{value:"x"}` (opener-count = 2, closer-count = 2 — the inline matcher does fire on `$$...$$` runs). Indented `$$` falls through to the existing indented-code-block rule with no math special-casing.

## Acceptance

- [ ] Input `- prose $x$ more\n` produces a `list` whose `listItem` contains a `paragraph` with children `[text{value:"prose "}, inlineMath{value:"x"}, text{value:" more"}]`.
- [ ] Input `> prose $x$ more\n` produces a `blockquote` whose `paragraph` has those same three children.
- [ ] Input `[^1]: prose $x$ more\n` produces a `footnoteDefinition{identifier:"1"}` whose `paragraph` has those same three children.
- [ ] Input `- $$\n  x\n  $$\n` produces a `list` whose `listItem` has `[math{value:"x\n", meta:null}]` as a direct child.
- [ ] A single-row GFM table whose only cell contains `$$x$$` produces a `table` whose `tableCell` has children `[inlineMath{value:"x"}]`; zero `math` nodes anywhere under the table.
- [ ] Input `    $$x$$\n` (four-space indent at document root) produces `code{lang:null, meta:null, value:"$$x$$\n"}` — indented code wins; no math node.
