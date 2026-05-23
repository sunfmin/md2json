# S02: inline `$...$` math lands as `inlineMath` node end-to-end

Status: ready-for-agent

A blog-post author writing `$x = 5$` in prose sees the math span survive on the wire as an `inlineMath` node whose `value` is the literal source bytes between the `$` delimiters, byte-for-byte, with no whitespace trim and no entity decoding. The node carries a `position` field by default and is stripped to no `position` under `--no-position`, uniformly with every other node. Adjacent prose remains as `text` nodes either side of the math span.

## Acceptance

- [ ] Input `$x = 5$` produces an envelope whose AST root has one `paragraph` child containing one `inlineMath` node with `value: "x = 5"` and a `position` field; exit `0`.
- [ ] Input `Use $x$ and $y$.` produces a paragraph whose children are `text{value:"Use "}`, `inlineMath{value:"x"}`, `text{value:" and "}`, `inlineMath{value:"y"}`, `text{value:"."}` in that order.
- [ ] The same `$x = 5$` input under `--no-position` produces the same `inlineMath{value:"x = 5"}` node with no `position` field; default invocation produces the same node with `position`.
- [ ] `inlineMath` is serialized as `{"type":"inlineMath","value":...,"position":...}` (no extra fields, no `meta`, no `data`).
- [ ] `inlineMath` survives unchanged inside the JSON envelope when the input is also wrapped in frontmatter; the frontmatter object is unaffected.
