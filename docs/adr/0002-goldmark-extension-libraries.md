# ADR-0002: goldmark extension libraries and the YAML-only frontmatter pin

- Status: Accepted
- Date: 2026-05-23
- Decider: PO (resolved in TDD-stage S03)

## Context

CONTEXT.md and the PRD pin `github.com/yuin/goldmark` as the parser. They also require **GFM** (tables, task lists, strikethrough, autolinks), **footnotes**, and **YAML frontmatter** to be parsed; **TOML frontmatter** is an explicit v1 non-goal. Goldmark itself ships GFM and Footnote as standard extensions under `github.com/yuin/goldmark/extension`. Frontmatter is **not** a standard extension and needs a third-party library or a hand-rolled detector in `parse`.

The S03 slice is the first one to actually construct the goldmark parser, so the extension-library choice is decided and recorded here.

## Decision

1. **GFM** — use `github.com/yuin/goldmark/extension`'s `extension.GFM` value (a meta-extension that bundles `Table`, `Strikethrough`, `Linkify`, `TaskList`). It's the same module as goldmark itself, so there is no extra third-party dependency and no version-coupling risk.

2. **Footnotes** — use `extension.Footnote`, also from `github.com/yuin/goldmark/extension`. Same rationale.

3. **YAML frontmatter** — use `go.abhg.dev/goldmark/frontmatter@v0.3.0` (`Extender`), configured with `Formats: []frontmatter.Format{frontmatter.YAML}`. This restricts the extension to YAML-only and **does not register the TOML format**, so a document starting with `+++` is parsed as body content (the leading `+++` line becomes a paragraph/text per CommonMark), matching the PRD's "TOML frontmatter is out of scope" rule without a runtime check in `parse`.

4. **YAML library** — `gopkg.in/yaml.v3`, transitively pulled by `go.abhg.dev/goldmark/frontmatter`. This is consistent with the grill-transcript note that the YAML library choice is a `parse`-internal implementation detail and is replaceable without affecting any other module.

5. **Frontmatter integration mode** — `Mode: 0` (the zero value). We **do not** ask the extender to set goldmark's document metadata. Instead, `parse.Parse` reads the frontmatter back via `frontmatter.Get(ctx).Decode(&v)` on the parser.Context, where `v any` lets both YAML scalars and YAML maps flow through. This keeps the wire-side frontmatter shape (object vs scalar passthrough per PRD user story 28) owned by the `parse` module rather than by goldmark's metadata cache.

## Consequences

- **Positive.** GFM + Footnotes ride on the same module as goldmark, so a `go get -u github.com/yuin/goldmark@latest` upgrades both at once with no version-skew. The frontmatter dep is small, single-purpose, and actively maintained.
- **Positive (TOML excluded by construction).** The frontmatter library's `Formats` field is the exact knob for "YAML only, no TOML." `+++` blocks are not even attempted as frontmatter; they fall through to the body parser. No runtime "is this TOML?" branch is needed in `parse`.
- **Negative (transitive TOML import).** Pulling `go.abhg.dev/goldmark/frontmatter` pulls `github.com/BurntSushi/toml` transitively because the library's `format.go` references the TOML format at package-init time even though we never construct it. The TOML library is link-time only — at runtime we never invoke its parser — so the cost is a few hundred KB of binary size and one extra entry in `go.sum`. Acceptable; revisit if binary-size budgets ever become a concern.
- **Negative (no central registry).** The choice of "which extensions are enabled" is a single function (`parse.New`) by convention. A future maintainer adding (say) `extension.DefinitionList` must remember to thread it through `parse.New` rather than expect goldmark to auto-detect. Documented in `parse.go`'s package comment.

## Out of scope (post-v1)

- A `--extensions` flag to toggle GFM/footnotes/frontmatter at runtime. The v1 wire contract pins the enabled set; runtime toggling would change the output schema.
- Math (`$...$`, `$$...$$`) extensions. PRD non-goal.
- MDX. PRD non-goal.
- TOML frontmatter. PRD non-goal.
- A streaming parser. ADR-0001 already pins whole-document-in-memory.

## Cross-references

- CONTEXT.md `Distribution` entry — pins goldmark as the parser.
- CONTEXT.md `Markdown (input)` and `Frontmatter` entries — pin the enabled extension set and YAML-only frontmatter.
- PRD "Module sketch" `parse` paragraph — pins the YAML library as a `parse`-internal implementation detail per the grill transcript.
- PRD "Out of Scope" — pins TOML frontmatter, math, and MDX as v1 non-goals.
