# S11: `--pretty`, mdast key ordering, null-field preservation, and the silent-drop property test

Status: ready-for-agent

The emit stage gains its v1 formatting contract: `--pretty` switches to 2-space-indented JSON; in both compact and pretty modes, keys are emitted in mdast convention (`type` first, then type-specific fields in their declared order from the v1 node set — e.g. `depth` before `children` on `heading`; `url`, `title` before `alt` on `image`; `ordered`, `start`, `spread` before `children` on `list` — then `children`, then `position` when present). Explicit `null` fields are preserved (never elided), so the wire shape is uniform whether a field has a value or is null. The slice also lands the **silent-drop lossiness policy** property test (US33 acceptance): across a corpus of GFM + frontmatter + footnote inputs, every emitted node's `type` is a member of the documented **mdast node-set v1** enumeration — a property assertion over `translate`'s output, gated independently from the formatting work so a failure in one does not mask a failure in the other.

## What to build

`emit` switches indentation style on `--pretty`. The same emitter is used for both compact (no indentation) and pretty (2-space indent) modes; both paths walk the value tree in the same key order so the byte stream is structurally identical up to whitespace. The key ordering is enforced by emitting fields in a fixed declared order per node type (not by relying on Go's `encoding/json` map iteration). The property test lives next to the `translate` module's tests — it generates or hand-picks a representative GFM + frontmatter + footnote corpus, runs each input through the full pipeline with `--no-position` for byte stability, walks the emitted AST, and asserts every node's `type` is in the v1 enumeration.

## Acceptance

- [ ] Fixture: `md2json2 --pretty` on a representative document emits 2-space-indented JSON whose keys appear in the mdast-convention order described above; an indented code block emits `{"type":"code","lang":null,"meta":null,"value":"...","position":{...}}` with both `null` fields preserved verbatim.
- [ ] Fixture: `md2json2 --pretty --no-position` on the same representative document emits the same key order minus the `position` key uniformly — pretty mode composes with no-position cleanly.
- [ ] Fixture: compact output (default) uses the same key ordering as pretty output, so the two are byte-stable up to whitespace (a test that strips whitespace from both and compares verifies this).
- [ ] Fixture: every emitted object preserves explicit `null` fields in both compact and pretty modes — `listItem.checked` is `null` for non-task items (never elided); `code.lang` and `code.meta` are `null` for indented code (never elided); `link.title` is `null` when omitted at source (never elided).
- [ ] Property test on the translate module: across a hand-curated corpus of GFM + frontmatter + footnote inputs covering every node in the v1 node set plus a sampling of GFM extension constructs, every emitted node's `type` is a member of the **mdast node-set v1** enumeration — no goldmark-native type names, no `unknown`, no `html` fallback for non-HTML constructs. (US33 acceptance.)
- [ ] The property test fails clearly when an out-of-set type is observed: the failure message identifies the offending `type` string and the input fixture that produced it.

## Blocked by

S10 — needs the position field on every node so the pretty-print key-order assertions can include `position` as the final key.
