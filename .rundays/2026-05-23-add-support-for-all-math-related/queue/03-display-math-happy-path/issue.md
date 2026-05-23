# S03: display `$$...$$` math lands as `math` node end-to-end

Status: ready-for-agent

An author writing `$$\n\frac{a}{b}\n$$` on its own paragraph sees the equation survive on the wire as a top-level `math` node whose `value` is the literal interior bytes between the fences with each content line's trailing `\n` preserved (including the final line's). The `meta` field is always rendered as JSON `null`, never elided from the object. Display math composes inside the JSON envelope alongside frontmatter, and the node carries `position` by default and strips uniformly under `--no-position`.

## Acceptance

- [ ] Input `$$\n\frac{a}{b}\n$$\n` produces an AST whose root has exactly one `math` child with `value: "\\frac{a}{b}\n"` and `meta: null`; exit `0`.
- [ ] The serialized JSON for `math` includes the `meta` key with literal `null` (the key is present, not elided), regardless of `--pretty`.
- [ ] Input `$$\n\ce{H2O}\n$$\n` produces `math{value: "\\ce{H2O}\n", meta: null}` — interior bytes ride through byte-for-byte; mhchem source is not validated, expanded, or normalized.
- [ ] An input combining YAML frontmatter (`---\ntitle: t\n---\n`) followed by `$$\nx\n$$\n` produces an envelope with `frontmatter: {title: "t"}` and AST root children `[math{value: "x\n", meta: null}]`; the frontmatter codepath is unchanged.
- [ ] The same display math input under `--no-position` produces the same `math` node with no `position` field; default invocation produces it with `position`.
