# Arch log: final (full-scope, Acceptance Gate)

Started: 2026-05-23
Scope: final (whole product_dir)
File set: whole product_dir (internal/cli, internal/parse, internal/translate, internal/emit, internal/read, main.go, anti_globals_test.go, integration_test.go, release_workflow_test.go, plus testdata fixtures)

## Baseline

- `go test ./...` (count=1, no cache): all 6 packages OK
  - 83 PASS, 0 FAIL across `--- (PASS|FAIL)` lines (subtest-aware count)
  - per-package: `github.com/sunfmin/md2json`, `internal/cli`, `internal/emit`, `internal/parse`, `internal/read`, `internal/translate` — all `ok`
- `go vet ./...`: clean (no output)
- Production LOC (non-test, non-fixture):
  - `main.go` 17
  - `internal/cli/cli.go` 306, `internal/cli/version.go` 18
  - `internal/parse/parse.go` 323
  - `internal/translate/translate.go` 1219, `internal/translate/position.go` 137
  - `internal/emit/emit.go` 422
  - `internal/read/read.go` 129
  - Total production: ~2571
- 13/13 issues complete entering this gate.

## Friction scan (whole codebase)

### Strong (act on this pass)

- **S-1. `parse.go` duplicates the GFM+Footnote extension list across `New()` and `newWithoutFrontmatter()`.** A future maintainer who adds (say) `extension.DefinitionList` to the v1 enabled set must remember to thread it through both call sites. The two `goldmark.New(goldmark.WithExtensions(...))` blocks share `extension.GFM, extension.Footnote` verbatim. **Locality** lens: the "v1 enabled extension set minus frontmatter-extender choice" is one concept living in two places. A small private helper concentrating the shared prefix gives the two callers the same baseline by construction. (Note: ADR-0002 §4 calls out "no central registry" and pins this as a future-maintainer hazard — this refactor materializes the registry as a private helper without changing the public surface.)

### Worth exploring (NOT acted on this pass; notes for future)

- **W-1. `translate.positionTracker` is now multi-concern** — line/column tracking + `footnoteLabels` map + `inlineSearchCursor`. The type doc-comment itself flags the trade-off ("Stashing this on positionTracker... avoids a churn in every function signature"). Renaming to `walkState` would match the actual concept ("the per-Translate walk's threading state") but it is **pure rename across ~40 call sites** — style alone by the lens rule. Defer until a fourth piece of walk state arrives, at which point the rename earns its keep by simultaneously deepening the type comment.
- **W-2. `cli.parseArgs` switch is a flat sequence of cases over each flag.** Functional but not deep — every flag is one literal case. A table-driven shape would be tidier but no maintainer is currently bouncing between cases. Speculative.
- **W-3. `translate.Options` is `struct{}` (empty) — exported for forward-compat.** Deletion-test today: caller passes `translate.Options{}` literally; removing the type would simplify the caller surface but break backward compat for any future "this slice needs a translate-side knob" slice. Keep — explicit forward-compat surface, not an accidental shallow module.
- **W-4. `emit.Options.FrontmatterOnly` is set asymmetrically.** The frontmatter-only short-circuit in `cli.Run` builds `emit.Options{NoPosition, FrontmatterOnly: true, Pretty}`; the regular path builds `emit.Options{NoPosition, Pretty}` (FrontmatterOnly implicitly false). The branches are mutually exclusive so the asymmetry is correct, but a single `eopts := emit.Options{...}; if opts.frontmatterOnly { eopts.FrontmatterOnly = true; ... }` shape would be more uniform. Style alone — no real bug surface.

### Speculative (notes only)

- **Sp-1. `cli.preInputPathToken` constant** named for one of one callers. The literal `"md2json"` also appears in the canonical-error regex test fixtures, the integration test (`m[1] != "md2json"`), and the release-workflow YAML (asset name `md2json-...`). A single shared constant across boundaries would over-couple — keep one per language site.
- **Sp-2. `internal/translate/translate.go` is 1219 LOC.** Looks long but every function is small and the per-node-type translator pattern is uniform. Splitting by node category (block / inline / table / footnote) would scatter a coherent dispatch surface. Pass.
- **Sp-3. `writePreInputUsageError` returns `int` while `writePositionedError` / `writeDocScopedError` return nothing.** Intentional — the `int` return enables `return writePreInputUsageError(stderr, msg)` ergonomics at the call sites. The asymmetry is the deeper interface (it owns "pre-input usage error" as a triple of path-token + position + exit-code), not friction.

## Pass 1: Centralize v1 goldmark extension set in `parse`

- **Lens:** Locality. The "v1 enabled extensions minus frontmatter choice" is one concept; previously it lived in two `goldmark.WithExtensions(...)` blocks.
- **Change:** Introduced a private helper `newGoldmarkWith(extras ...goldmark.Extender) goldmark.Markdown` in `internal/parse/parse.go` that owns the shared `extension.GFM, extension.Footnote` prefix. `New()` (with-frontmatter) and `newWithoutFrontmatter()` both delegate to it. A future "add `extension.DefinitionList` to v1" change is now one edit, not two.
- **Behavior contract:** Both call sites continue to produce a `goldmark.Markdown` with EXACTLY the same extension registration order they had before — GFM, then Footnote, then (if applicable) the frontmatter extender. Order matters for goldmark's internal priority resolution; the helper preserves it.
- **Files touched:** `internal/parse/parse.go`.
- **Tests after:** `go test ./...` → all 6 packages OK; `go vet ./...` clean. 83 PASS, 0 FAIL (unchanged from baseline).
- **Reverted?** no.

## Final

- Tests: 83 PASS, 0 FAIL across `go test ./...` (unchanged from baseline). `go vet ./...` clean.
- LOC delta: +8 net in `internal/parse/parse.go` (323 → 331; helper docstring + signature outweigh the two condensed literal blocks; locality > LOC).
- Most consequential change: ADR-0002 §4 named "no central registry" as a future-maintainer hazard. Pass 1 materializes that registry as the private `newGoldmarkWith` helper. The "which extensions are enabled in v1" decision now has one editable site instead of two, so the next-extension-added invariant (both parsers must agree) holds by construction rather than by convention.

VERDICT: accept
