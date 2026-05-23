# Arch log: 02-inline-math-happy-path

Started: 2026-05-23
Scope: mini
File set (per tdd-log):
- `internal/translate/translate.go` — added `case *mathjax.InlineMath` + `translateInlineMath` helper
- `internal/emit/emit.go` — added `case "inlineMath"` to `writeNode`; added `"inlineMath"` to `isContainer` leaf-list
- 4 new fixtures under `testdata/fixtures/62..65` (read-only data — no arch concerns)

## Baseline
- Tests: all 6 packages green (`md2json`, `cli`, `emit`, `parse`, `read`, `translate`)
- Per-package: cached; root package 1.723s real run.

## Survey

### Candidate A: deletion test on `translateInlineMath`

Imagined deletion: inline the ~30-line body into `translateNode`'s `case *mathjax.InlineMath` arm.

Result: 30 lines of position-derivation + child-walk land in the dispatch switch. Every other arm in the switch (`translateHeading`, `translateParagraph`, `translateText`, `translateEmphasis`, ..., `translateFootnoteLink`) delegates to a same-named helper — consistent shape. Inlining would break that pattern for one node type and bury delimiter-walk position math inside the dispatcher.

The helper's interface = `(im, src, pt) -> *Node` (~3 inputs, 1 output). The implementation does two distinct things: (1) verbatim concat of child `*ast.Text` segments per CONTEXT.md "Text/Code value preservation", (2) `$`-run walk on both sides of the children's span to recover the source range goldmark-mathjax doesn't expose. Interface < implementation = depth, not shallowness.

Naming aligns with CONTEXT.md `inlineMath node` glossary entry and the goldmark-side `*mathjax.InlineMath` type — ADR-0004 Decision 4's 1:1 name alignment.

**Verdict: pass. Keep as-is.** Score: not a refactor candidate.

### Candidate B: emit.go leaf-list pattern in `isContainer`

Pattern: enumerate the leaf node types (`text`, `inlineCode`, `inlineMath`, `code`, `html`, `image`, `thematicBreak`, `break`, `imageReference`, `definition`, `footnoteReference`); everything else is a container. Default = container.

Shallow-module smell check: does adding a new node type touch this in lockstep with `writeNode`'s switch? Yes — each new leaf type costs one line here AND one or more lines in `writeNode`. But the two concerns are distinct:
- `writeNode`'s switch encodes per-type FIELDS and KEY ORDER (the load-bearing wire-shape contract).
- `isContainer`'s list encodes the schema-level "carries `children[]` or not" property.

Merging into a single switch would push `// no children` comments into per-case noise inside writeNode's body, and would make the writeNode switch carry both "what fields go here" and "do I get a children key" — currently writeNode trusts isContainer to answer the second question after the field-writing block returns.

mdast's bias: most node types ARE containers (`root`, `paragraph`, `heading`, `emphasis`, `strong`, `delete`, `list`, `listItem`, `blockquote`, `link`, `linkReference`, `table`, `tableRow`, `tableCell`, `footnoteDefinition`). Enumerating LEAVES is the shorter list — the negative-list shape is the right polarity for the bias.

**Verdict: pass. Healthy pattern, not a shallow-module smell.** Score: not a refactor candidate.

### Candidate C: shared value-concat shape between `translateCodeSpan` and `translateInlineMath` (Worth exploring, not Strong)

Both helpers walk child `*ast.Text` siblings and concat segment bytes into `value`:

```go
for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
    if t, ok := c.(*ast.Text); ok {
        seg := t.Segment
        value += string(src[seg.Start:seg.Stop])
    }
}
```

Two callers = "real seam" by the rule of thumb. A `concatTextSegments(parent ast.Node, src []byte) string` helper would save ~5 lines per call site (~10 lines total) and concentrate the "verbatim concat of inline `*ast.Text` segments" rule named once.

Why not Strong:
- LoC saved is small (~10 net).
- The two callers differ in their position math (`textChildrenSpan` recurse vs `$`-run walk) — the shared shape is just the value loop, not the surrounding helper.
- The CONTEXT.md term that would name the helper is `Text/Code value preservation` — that's already documented as a rule, not a function.
- A future block `translateMath` for `$$...$$` will need the same shape; at THREE callers, this becomes Strong. Better to wait for the third caller (issue 03) and pull the helper then with the full requirement set visible.

**Verdict: defer.** Score: Worth exploring (not Strong). Note here for the issue-03 arch pass to revisit.

### Candidate D: `$`-run walk inside `translateInlineMath` (Speculative)

The two symmetric loops:
```go
for startOff > 0 && src[startOff-1] == '$' { startOff-- }
for endOff < len(src) && src[endOff] == '$' { endOff++ }
```

could be `expandAcrossRun(src, startOff, endOff, '$') (int, int)`. One caller today. Block math (`translateMath` in issue 03) will likely have its own delimiter math but it's `$$`-run-vs-`$`-run, not byte-identical (the library exposes block-fence info differently — see ADR-0004 Decision 5). The block path may not need this helper at all.

**Verdict: defer.** Score: Speculative (one caller, no guaranteed second). Skip.

## Pass count

Strong candidates this pass: zero.

No refactor was applied. Per protocol: "No Strong → 'no strong candidates this pass' + VERDICT: accept. Legitimate."

## Glossary alignment check (no-drift sweep)

Load-bearing terms from CONTEXT.md that appear in the in-scope code:
- `inlineMath node` → code uses `"inlineMath"` literal + `translateInlineMath` helper. Aligned.
- `Text/Code value preservation` → `translateInlineMath` does verbatim segment concat, no trim/normalize. Aligned.
- `Position info` → derived via `pt.position(startOff, endOff)`, consistent with other helpers. Aligned.
- `mdast node-set v1` → `inlineMath{value, position}` (no `meta`, no `data`, no `children`) — wire shape matches the enumeration. Aligned.
- `Lossiness policy (goldmark → mdast)` → `translateNode`'s default arm still returns nil (silent drop); `*mathjax.InlineMath` is now a recognized case. Aligned.

`_Avoid_` synonyms checked:
- `mathInline` — not in code. Good.
- `mathBlock`, `blockMath`, `displayMath` — not in code (none are in scope this issue). Good.
- "goldmark AST" — not in code as a wire contract. Good.

No drift. No load-bearing term missing from CONTEXT.md.

## Final

- Tests: all green (unchanged from baseline).
- LOC delta: 0.
- Most consequential change: none — survey-only pass. The S02 implementation (one switch arm + one ~30-LoC helper in `translate`, one switch arm + one leaf-list entry in `emit`) is already shaped consistently with the rest of the translate/emit pair. Naming aligns with CONTEXT.md `inlineMath node` and ADR-0004 Decision 4. Deletion test on `translateInlineMath` fails (the helper earns its keep). emit.go's leaf-list pattern is healthy — different concern from writeNode's key-order switch, and the negative-list polarity matches mdast's container-majority bias.

Deferred notes for the next arch pass (likely after issue 03 lands block math):
- Candidate C (shared `concatTextSegments` between `translateCodeSpan` + `translateInlineMath` + future `translateMath`) becomes Strong when the third caller arrives.
- Candidate D (`expandAcrossRun` for `$`-run delimiter walks) stays Speculative unless block math shows it has a second use.

VERDICT: accept
