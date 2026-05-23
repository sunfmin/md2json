# S05: unclosed-`$$`-at-EOF compensates to paragraph/text via src-byte predicate

Status: ready-for-agent

A writer who opens `$$` and reaches EOF without a closing `$$` sees the document still parse: no hard error, no dropped bytes, no phantom-fence math node. The opening `$$` line and the body bytes that follow emit as a single `paragraph` whose `text` children mirror goldmark's standard prose-paragraph segmentation — one `text` per source line, segments stop before the LF, no embedded LF inside any text value. The compensation decision is taken at the translate layer, since the library cannot distinguish closed from unclosed display math from its AST alone. Inline `$` with no closer on the same paragraph is the library's own non-match path and needs no translate-side compensation.

## Acceptance

- [ ] Input `$$\n\frac{a}{b}\n` (no closing fence, EOF after body LF) produces one `paragraph` whose children are `[text{value:"$$"}, text{value:"\\frac{a}{b}"}]`; zero `math` nodes; exit `0`.
- [ ] Input `$$\nx\n$$\n` continues to produce one `math{value:"x\n", meta:null}` (closed case, S03 regression held).
- [ ] Input `prose $x = 5 still prose` (unclosed inline) produces one `paragraph` whose only child is `text{value:"prose $x = 5 still prose"}` after sibling-coalescing; zero `inlineMath` nodes; no translate-side compensation triggered.
- [ ] A focused in-process unit test parses two inputs (`$$\nx\n` unclosed, `$$\nx\n$$\n` closed) through the library alone and asserts both produce one `MathBlock` whose `Lines().Last().Stop` is identical (byte offset past the body line's terminating LF) — the AST-alone-cannot-distinguish-closed-from-unclosed invariant the translate predicate relies on (PRD fixture #14, library-contract pin).
