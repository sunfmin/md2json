# S06: Translate code, inlineCode, html, link, image

Status: ready-for-agent

The translate stage learns the remaining CommonMark constructs that are not lists or headings: fenced and indented code blocks both emit `code` with `lang`, `meta`, `value` (with `lang: null` and `meta: null` for indented code, populated from the info string for fenced); backtick spans emit `inlineCode{value}`; block and inline raw HTML both collapse to `html{value}` (mdast does not distinguish); inline `[text](url "title")` emits `link{url, title}` with its inline `children`; `![alt](url "title")` emits `image{url, title, alt}` where `alt` is a flat string built by concatenating the textual content of the alt's inline children (non-text inline children are silently dropped from the alt per the mdast `image.alt: string` constraint). Per CONTEXT.md's **Text/Code value preservation** entry, `code.value` is the literal content between the fences (or, for indented code, the dedented content) with every content line's trailing `\n` preserved including the final line's `\n`. `inlineCode.value` is the literal content between the backticks as goldmark resolves it. `html.value` is the raw markup as goldmark provides it.

## Acceptance

- [ ] Fixture: a fenced block ` ```go\nfunc x(){}\n``` ` produces a `code` with `lang: "go"`, `meta: null`, `value: "func x(){}\n"`. The trailing `\n` is preserved because it terminates the final content line; the closing fence is not part of `value`. (Per CONTEXT.md **Text/Code value preservation**.)
- [ ] Fixture: a fenced block with info string `go runme=true` produces `lang: "go"`, `meta: "runme=true"`.
- [ ] Fixture: an indented code block of `    abc\n    def\n` produces a `code` with `lang: null`, `meta: null`, and `value: "abc\ndef\n"` — both null fields present (never elided), and both content lines' trailing `\n` preserved. (Per CONTEXT.md **Text/Code value preservation**.)
- [ ] Fixture: `` `x` `` inside a paragraph produces an `inlineCode` with `value: "x"`.
- [ ] Fixture: a block of `<div>raw</div>` on its own paragraph produces an `html` node with `value` equal to the original markup verbatim; an inline `<span>x</span>` inside a paragraph also produces an `html` node (same type) with the literal markup as `value`. (Per CONTEXT.md **Text/Code value preservation**.)
- [ ] Fixture: `[text](https://example.com "t")` produces a `link` with `url: "https://example.com"`, `title: "t"`, and a `text` child with `value: "text"`.
- [ ] Fixture: `![an *emph* alt](https://example.com/x.png)` produces an `image` with `alt: "an emph alt"` (a flat string with non-text inline content's textual value concatenated, no nested children).

## Blocked by

S05 — needs the translate-stage block/inline scaffolding from S04 and S05.
