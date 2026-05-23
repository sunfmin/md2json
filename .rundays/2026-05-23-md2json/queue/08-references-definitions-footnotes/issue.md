# S08: Preserve reference-style links/images, definitions, and footnotes

Status: ready-for-agent

The translate stage stops flattening reference-style links and images into inline `link`/`image`: a `[text][id]` form emits `linkReference{identifier, label, referenceType}`, an `![alt][id]` form emits `imageReference{identifier, label, referenceType}`, and the trailing `[id]: url "title"` line emits a sibling `definition{identifier, label, url, title}` node — the triple is preserved on the wire so consumers can round-trip the document or surface the definitions separately. Footnotes (goldmark's footnote extension is already registered from S03) emit as `footnoteDefinition{identifier, label}` for the definition block and `footnoteReference{identifier, label}` for the inline marker; references and definitions share `identifier` so consumers can resolve them without re-walking the source.

## Acceptance

- [ ] Fixture: `[text][id]\n\n[id]: https://example.com "t"` produces a `linkReference` (with `identifier: "id"`, `label: "id"`, `referenceType` set per mdast — `"full"`, `"collapsed"`, or `"shortcut"`) followed by a sibling `definition` with `identifier: "id"`, `url: "https://example.com"`, `title: "t"`. No inline `link` is emitted.
- [ ] Fixture: `[text][]` (collapsed form) with a matching definition produces `linkReference.referenceType: "collapsed"`.
- [ ] Fixture: `[text]` (shortcut form) with a matching definition produces `linkReference.referenceType: "shortcut"`.
- [ ] Fixture: `![alt][id]` plus its definition produces an `imageReference` paired with a `definition`; no inline `image` is emitted.
- [ ] Fixture: `text[^a]\n\n[^a]: footnote body` produces a `footnoteReference` with `identifier: "a"` inline in the paragraph and a sibling `footnoteDefinition` with `identifier: "a"` containing the body as its children.

## Blocked by

S07 — needs the GFM extension translations in place so the reference/footnote slice doesn't have to also resolve table/task-list ambiguities.
