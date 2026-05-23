# S07: mismatched braces and broken LaTeX ride through `value` byte-for-byte

Status: ready-for-agent

A writer with mismatched braces, unknown macros, or otherwise broken LaTeX inside a `$...$` or `$$...$$` span sees the document exit `0` with the broken bytes preserved verbatim inside the math node's `value`. md2json is a transport for math — LaTeX validation is the downstream renderer's responsibility. The math node's `value` is the literal interior bytes as the library exposes them, with no trim, no entity decoding, no brace-balance check, no macro expansion.

## Acceptance

- [ ] Input `$\frac{a}{b$` produces one `paragraph` whose only child is `inlineMath{value:"\\frac{a}{b"}`; exit `0`; the unbalanced `{` rides through inside `value`.
- [ ] Input `$$\n\ce{H2O}\n$$\n` (regression from S03) produces `math{value:"\\ce{H2O}\n", meta:null}`; mhchem source is not validated or expanded.
- [ ] The full PRD fixture set runs as exact tree comparisons (no disjunctive acceptance) under both default flags and `--no-position`, including: inline happy path, display happy path, currency-rule prose, currency-rule adjacent valid math, currency post-pass convergence (`$5 and $x$`), currency post-pass divergence (`$ 5 and $x$`), unclosed display at EOF, unclosed inline at EOL, value preservation (`\ce{H2O}`), mismatched braces inline, in-block composition (list/blockquote/footnote/table-cell), indented `$$` falls to code, frontmatter + display math interaction, `--no-position` strips uniformly.
- [ ] The existing lossiness property test passes with `*ast.InlineMath` / `*ast.Math` in the goldmark-AST tree-walk surface — both have first-class mdast targets, the silent-drop set for math is empty.
- [ ] Exit code is `0` for every input above; stdout carries the JSON envelope, stderr is empty.
