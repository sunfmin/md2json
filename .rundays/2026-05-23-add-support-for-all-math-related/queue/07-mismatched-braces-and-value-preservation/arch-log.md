# Arch log: 07-mismatched-braces-and-value-preservation

Started: 2026-05-24
Scope: mini
File set (from tdd-log "Final fixture inventory" + "Refactor pass"):

- `testdata/fixtures/82-inline-math-mismatched-braces-rides-through-as-value-nopos/{args,input.md,stdout,stderr,exit}` (new)
- `internal/translate/translate_test.go` — added `TestTranslateInlineMathMismatchedBracesRideThroughAsValue`
- `internal/translate/lossiness_property_test.go` — added two `mdastNodeSetV1` map entries (`math`, `inlineMath`); added two `lossinessCorpus` rows (`inline-math-happy-path`, `display-math-happy-path`); updated the coverage-map doc-comment block

S07 is a consolidation slice — no new translate/parse/emit code. All three S07 inputs land on existing structural patterns.

## Baseline

- Tests: all packages PASS at `go test ./... -count=1` (md2json, internal/cli, internal/emit, internal/parse, internal/read, internal/translate).
- tdd-log roll-up records 102 named subtests PASS, 0 FAIL across the full Run's Stages (S01–S07).
- `go vet ./...` clean per tdd-log.

## Survey

Friction probes against the S07 file set:

1. **fixture 82.** Per-fixture testdata harness shape (args/input.md/stdout/stderr/exit) — identical to ~60 existing fixtures. Name `82-inline-math-mismatched-braces-rides-through-as-value-nopos` aligns with CONTEXT.md `Dollar-sign math (transport-only)` ("transport for math") + `Text/Code value preservation` ("rides through inside value"). No friction.

2. **translate_test.go::TestTranslateInlineMathMismatchedBracesRideThroughAsValue.** Mirrors S02/S03 sibling-test precedent (`TestTranslateInlineMathHappyPath`, `TestTranslateDisplayMathPreservesMhchemValue`): single Go-layer anchor per acceptance bullet exercising a load-bearing transport rule. Comment block names CONTEXT.md `remark-math currency rule` predicates (i)/(ii)/(iii) and cites src offsets. Glossary-aligned. No friction.

3. **lossiness_property_test.go math additions.** Map `mdastNodeSetV1` gained `math` (block half) + `inlineMath` (inline half), placement matches CONTEXT.md `mdast node-set v1` ordering. Corpus `lossinessCorpus` gained `inline-math-happy-path` + `display-math-happy-path`. Coverage-map doc-comment block updated to mirror.

   Probe — does the map+corpus pairing want a clearer abstraction now that math has been added?

   **Deletion test on the doc-comment coverage map block.** Delete it → does complexity reappear? No. `TestLossinessCorpusCoversEveryV1NodeType` already enforces the functional drift mechanically (set-difference of `mdastNodeSetV1` vs `collectTypes(corpus)` observed types). The doc-comment is purely advisory documentation, read once when designing the corpus. Drift between doc-comment and reality is caught by the coverage test, not by the comment.

   **Deletion test on the corpus rows themselves.** Delete `inline-math-happy-path` → coverage test fails with "missing types: [inlineMath]"; delete `mdastNodeSetV1["inlineMath"]` after a wire-side `inlineMath` is emitted → walk test fails with "type `inlineMath` is NOT a member of mdast node-set v1". Both halves earn their keep. The pairing is **deep** (small interface — two literal data structures — large leverage — every emitted type is wire-contract-checked across the full pipeline).

   **Speculative: per-row `wants []string` tag replacing the doc-comment.** Would convert the doc-comment coverage map into struct data so the coverage test could pin per-row expectations (not just union coverage). Score: **Speculative**. Cost = each of ~30 corpus rows gains a new field with mechanical content; the existing union-coverage check already catches every drift the per-row tags would catch (because every node type only needs ONE row that produces it, not a specific row). The proposal would make the interface (corpus row) wider with no new leverage. Rejected on depth grounds.

   **Speculative: split the corpus into "primary coverage" vs "incidental" rows.** No friction surface today — every row already pulls weight. Rejected on absence-of-pain grounds.

   The user's framing — "Or is two entries past two enough to leave as-is?" — answered: **leave as-is**. Two more entries landed cleanly on the existing pattern. The pattern absorbs growth linearly without distorting; that is the signal of a depth-appropriate test surface, not an extraction-opportunity signal.

## Candidates

None scored **Strong**.

Speculative notes (recorded for future Runs, not acted on this pass):

- Per-row `wants []string` on `lossinessCorpus` rows. Score: Speculative. Would replace the doc-comment coverage map with struct data. Cost > benefit at current scale (~30 rows); revisit if the corpus doubles or the doc-comment drifts in practice.

Cross-cutting notes deferred to **full-arch** (out of scope for mini per protocol — mini scope is strictly S07 file set):

- `internal/translate/translate.go` now carries two library-behavior-specific compensations per ADR-0004 (Decision 3 currency post-pass + Decision 5 unclosed-`$$` src-byte predicate). Final-arch should consider locality: do these two compensations share a "math-compensation" seam, or are they incidentally co-located? Deletion test + adapter-count rubric apply.
- `lossinessCorpus` row count is approaching the comfortable upper bound for a flat struct slice. Final-arch may consider whether grouping by GFM-vs-math-vs-frontmatter improves readability, or whether flatness is still the right shape.

Both items explicitly OUT OF SCOPE for this mini pass; flagged for final-arch's broader sweep.

## Passes attempted

None. No Strong candidates → no refactor pass → no test re-run needed beyond the baseline above.

## Final

- Tests: unchanged from baseline (all PASS at `go test ./... -count=1`).
- LOC delta: 0 (no refactor applied).
- Most consequential change: none. S07 was a consolidation slice that added one fixture, one Go-layer test, and two map+corpus entries — all on existing structural patterns. The map+corpus pairing absorbed the math additions linearly without distortion; the deletion test confirms both halves earn their keep and the doc-comment coverage map is advisory-only (drift is caught mechanically by `TestLossinessCorpusCoversEveryV1NodeType`). No strong candidates this pass — legitimate per protocol's "Pick" step.

VERDICT: accept
