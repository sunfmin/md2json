# Arch log: final-arch (full)

Started: 2026-05-24
Scope: full
File set: whole product_dir (`/Users/sunfmin/Developments/md2json`)

## Baseline

- Tests: 102 passing, 0 failing (`go test ./... -count=1`, all 6 packages green: `md2json`, `internal/cli`, `internal/emit`, `internal/parse`, `internal/read`, `internal/translate`). 389 test-run events incl. subtests.
- LOC of in-scope translate.go: 1661 lines. emit.go: 445 lines. parse.go: 346 lines. position.go: 137 lines.
- Fixture count: 82. Lossiness corpus rows: 31.

## Survey

Friction probes across the whole product, lens-by-lens against CONTEXT.md + ADR-0001..0004:

### S07's two deferred candidates

1. **Math-compensation locality.** `internal/translate/translate.go` carries two ADR-0004 library-behavior-specific compensations (Decision 3 currency post-pass, Decision 5 unclosed-`$$` src-byte predicate). They live in two separate regions of the file:
   - Currency post-pass: lines 26-204 (byte-class helpers + `currencyPostPass` + `currencyPredicatesPass` + `inlineMathDelimitedSpan`), placed BEFORE the `Node` type definition (line 268). This breaks the rest-of-file convention where helpers sit AFTER the per-type mappers.
   - Unclosed-`$$` compensation: lines 600-732 (`displayMathClosed` + `demoteUnclosedDisplayMath`), placed adjacent to `translateMath` (line 561) — its only caller.
   - The two compensations are ~400 lines apart despite sharing the byte-class helpers and ADR-0004's "two library-behavior-specific compensations" concept-frame.

   Two-adapter rule: `Translate()` calls `currencyPostPass` (adapter #1); `translateMath()` calls `displayMathClosed` and `demoteUnclosedDisplayMath` (adapter #2). Real seam, not hypothetical.

   Deletion test on a hypothetical `compensate_math.go` file (same package): does complexity reappear? No — the functions remain private-to-package, callers remain `Translate` and `translateMath`. The split is documentation-by-filename + locality improvement. **Strong candidate.**

2. **`lossinessCorpus` row count.** 31 rows; flat `[]struct{name, src string}` slice. Each row is a single-line literal except `big-mixed`. Test functions iterate directly. Probe: does the math additions (rows 30-31) distort the shape? No — they slot in linearly, identical syntax to the GFM/frontmatter neighbors. Grouping by category would add a wrapper struct without test-side leverage. **Score: Speculative, no friction surface.** S07 already scored this correctly; final-arch concurs.

### Whole-product probes this Run touched

3. **`translate.go` size + cohesion (1661 LoC).** The file has 5 logical sections (math-compensation helpers, Node/Position types, Translate entry + dispatch, per-type translateXxx mappers, position math helpers). Sections 2-5 follow a clean "types → entry → per-type → utilities" reading order. Section 1 (math-compensation) is at the top of the file before even the Node type — anomalous. Splitting Section 1 into its own file is exactly Candidate A above. No other section warrants splitting at this scale (Go convention tolerates ~2k-line files when the dispatch + per-type-helper pattern is uniform).

4. **`emit.go` leaf-list pattern (`isContainer`).** Added `inlineMath`, `math` to the leaf list at S02/S03. Now 12 leaves. Negative-list polarity (S02 arch-log noted) matches mdast's container-majority bias. Deletion test: removing `isContainer` forces 19 per-case `,"children":[...]` writes scattered across `writeNode`. Heavy locality loss. **Score: not a candidate.** Healthy.

5. **`parse.go` ADR-0004 Decision 2 "single function by convention".** `newGoldmarkWith` now wires GFM + Footnote + mathjax + (optional) frontmatter — 4 base extensions. ADR-0002's "no central registry" maintainer-hazard is addressed by this single function. Comment block at lines 95-114 names the v1.x math Run additions explicitly. **Score: not a candidate.** Convention holds.

6. **Glossary drift.** Scanned code for forbidden synonyms per CONTEXT.md `_Avoid_` lists:
   - `mathInline`, `mathBlock`, `blockMath`, `displayMath` as mdast-type literals — not present in code (the goldmark-side library type `*mathjax.MathBlock` is a legitimate library reference, called out as such by ADR-0004 Decision 4).
   - `math rendering` / `LaTeX support` — not present.
   - `strikethrough` as wire type — not present (uses `delete`).
   - `goldmark AST` as wire contract phrase — not present (used only to refer to the goldmark-internal type, correctly).

   No flagged terms. Glossary matches code.

7. **Test corpus default vs nopos balance (11 vs 55).** Probe: does the corpus grow `default`+`nopos` pairs without bound? No — only a few rules need both flavors: empty input, frontmatter envelope shape, position-sensitive position-math edge cases. Most node-shape rules use only the `--no-position` flavor (smaller stdout, focuses on tree shape). Position-math rules have their own dedicated default-mode fixtures (18, 53, 56, 59). The two flavors pin different rules; no redundancy. **Score: not a candidate.**

8. **`displayMathClosed` naming polarity.** Returns true when closed; callers use `if !displayMathClosed(...)` to detect the compensation-firing case. Double-negative reading. Alternative: `displayMathIsUnclosed` returning true when compensation fires. Trade-off: aligns with action semantic but flips testing-the-positive convention. **Score: Speculative**, no Strong action — naming-clarity gain is marginal and the existing comment block at line 576 already documents the polarity.

## Candidates

**Strong:**

- A. Split math-compensation helpers from `translate.go` into a new `compensate_math.go` in the same package. Moves `isASCIIWhitespace`, `isASCIIDigit`, `isAllASCIIWhitespace`, `currencyPostPass`, `currencyPredicatesPass`, `displayMathClosed`, `demoteUnclosedDisplayMath`. Keeps `inlineMathDelimitedSpan` (shared with happy-path `translateInlineMath`) and `lineStartOffset` (shared with `blockOffsets`) in `translate.go`. Two adapters → real seam. Concept (ADR-0004's "library-behavior-specific compensations") gets a filename.

**Worth exploring (recorded for future):**

- `displayMathClosed` → `displayMathIsUnclosed` polarity flip. Action-semantic alignment vs convention-of-positive-naming trade. Defer.

**Speculative (recorded, not acted on):**

- Per-row `wants []string` tag on `lossinessCorpus` rows. Same as S07's note. Cost > benefit at current scale.
- Grouping `lossinessCorpus` by GFM/math/frontmatter category. No friction surface; rows absorb additions linearly.
- Renaming `displayMathClosed` (above).

## Passes attempted

### Pass 1: Extract math-compensation helpers into `internal/translate/compensate_math.go`

- Lens: locality (concept-level co-location; ADR-0004's "two library-behavior-specific compensations" gets a named home) + naming clarity (filename = concept).
- Baseline tests before: 102 passing, 0 failing.
- Files touched:
  - `internal/translate/compensate_math.go` (NEW): houses `isASCIIWhitespace`, `isASCIIDigit`, `isAllASCIIWhitespace`, `currencyPostPass`, `currencyPredicatesPass`, `displayMathClosed`, `demoteUnclosedDisplayMath`. Plus a file-header comment explaining the file-split rationale (concept-locality, what stays in `translate.go` and why).
  - `internal/translate/translate.go`: removed the seven function definitions; left in place `inlineMathDelimitedSpan` (still shared with `translateInlineMath`) and `lineStartOffset` (still shared with `blockOffsets`). Updated the comment block at the top of `inlineMathDelimitedSpan` to point at the new file. Updated the `Currency-rule demote-only post-pass` comment in `Translate` to reference the sibling file and replaced the stale `translate.go:226-232` line-range pointer with a content-pointer ("see the `if len(out) > 0 && n.Type == "text"` branch in translateChildren") that won't drift on the next refactor.
- Tests after: 102 passing, 0 failing (`go test ./... -count=1`, all 6 packages green).
- `go vet ./...` clean.
- Reverted? no.
- LoC: `translate.go` went 1661 → 1385 (−276). `compensate_math.go` new at 330. Net +54 (file-header comment block + import duplication).

### Pass 2: Refresh drifted package + dispatch doc-comments

- Lens: naming clarity (package-level description matches reality).
- Baseline tests before: 102 passing.
- Three stage-stamped doc-comments named the wrong development epoch:
  1. Package doc: claimed "At S04 the supported subset is `root`, `paragraph`, `heading{depth}`, `text{value}`, `emphasis`, and `strong`" — false; the full v1 node set is supported as of S10/S07. Rewrote to enumerate the full closed set verbatim from CONTEXT.md `mdast node-set v1`, with a pointer to `translateNode` as canonical dispatch table and to `compensate_math.go` for math-compensation entry points.
  2. `translateNode` doc: claimed "At S06 the recognized set is: Heading, Paragraph, Text, Emphasis (level 1 → emphasis, level 2 → strong), List, ListItem, TextBlock..." — false; the comment missed AutoLink, LinkReferenceDefinition, Table*, Strikethrough, FootnoteLink, math types. Rewrote to point at the switch as the canonical table (avoiding the duplicate-list drift trap) and pinned the two emission rules that lack a switch arm: synthetic `break` (HardLineBreak flag) and emphasis-level discriminator inside `translateEmphasis`.
  3. `Options` doc: claimed "At S04 there are still no per-translate knobs" — accurate but stage-stamped. Dropped the "At S04" prefix; same content.
- Files touched: `internal/translate/translate.go` (three doc-comment blocks).
- Tests after: 102 passing, 0 failing. `go vet` clean.
- Reverted? no.
- LoC delta: small (~+7 due to longer accurate node-set enumeration in package doc, ~−4 in translateNode doc, ~0 in Options doc).

## Final

- Tests: 102 passing, 0 failing (unchanged from baseline). All 6 packages green. `go vet ./...` clean.
- LoC delta on product: roughly +57 (file split adds header comments; doc-refresh adds a few lines of accurate node-set enumeration). Inside `translate.go`: −269 LoC, since the math-compensation block moved out.
- Most consequential change: Pass 1 — the math-compensations file split. ADR-0004's two-compensations concept now has a filename on disk, the byte-class helpers travel with their callers, and the file-header comment block in `compensate_math.go` plus the cross-file pointer block in `translate.go` together name the two cross-file references (`inlineMathDelimitedSpan` shared with the happy-path mapper, `lineStartOffset` shared with `blockOffsets`) so a future maintainer doesn't have to rediscover them. The package-doc and dispatch-doc refresh (Pass 2) closes the stage-stamped drift that had accumulated from S04 onward — the package now self-describes accurately at the v1.x math Run shipping state.
- Glossary alignment (CONTEXT.md): no drift found, no `_Avoid_` synonyms in code. The new file uses ADR-0004's exact phrase "library-behavior-specific compensations" in its header.
- No new ADR. ADR-0004 already pins the design decisions; the file split is a refactor of the implementation organization, not a new architectural commitment. No CONTEXT.md edits — no glossary terms added or shifted; the file split is package-internal.

VERDICT: accept

