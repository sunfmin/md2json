# Arch log: 10a-pretty-print-and-property-test

Started: 2026-05-23
Scope: mini
File set (from tdd-log.md):
- `internal/emit/emit.go` (production: `Pretty bool` field + `json.Indent` post-process branch)
- `internal/cli/cli.go` (production: `opts.pretty` threaded through both `emit.Options` shapes)
- `internal/emit/emit_test.go` (new unit tests — `package emit_test`)
- `internal/translate/lossiness_property_test.go` (new property test — `package translate_test`)
- `testdata/fixtures/57-pretty-title-and-bold/` (fixture data — not a refactor target)
- `testdata/fixtures/58-pretty-indented-code-null-lang-meta/` (fixture data — not a refactor target)
- `testdata/fixtures/59-pretty-indented-code-with-position/` (fixture data — not a refactor target)

## Baseline

- `go test ./... -count=1`: 63 top-level + 138 subtests, 0 failing. All 6 packages pass.
- `go vet ./...`: clean.
- LOC across refactor-eligible files in scope: 1309 lines (emit.go 422 + emit_test.go 234 + cli.go 225 + lossiness_property_test.go 428).

## Exploration

Read product CONTEXT.md (load-bearing terms: JSON envelope, mdast node-set v1, Lossiness policy, pretty, compact, silent drop, never-elide). Read ADR-0001 (input encoding) and ADR-0002 (goldmark extension libraries). Read S10a tdd-log in full — including its own "Refactor pass" section, which already considered and recorded reasoned decisions on 5 candidate refactors:

1. Extract `stripJSONWhitespace` to shared test util — rejected (single caller pair).
2. Move `lossinessCorpus` to a sibling file — rejected (split spec from test).
3. Promote `mdastNodeSetV1` to a public `translate` constant — rejected (would invert dep direction; the per-type `switch` in `translate.go` is the source of truth, the test mirrors).
4. Convert coverage test to `t.Run` per type — rejected (a single multi-type failure is more actionable for the corpus designer).
5. Split pretty rendering into a separate `pretty.go` — rejected (one-line `json.Indent` call; over-engineering).

These five are already settled; reopening them would re-litigate without new information.

### Friction scan (lens: glossary alignment / module depth / naming clarity / locality)

- **Glossary alignment.** Every load-bearing term used in the touched files matches CONTEXT.md verbatim: "JSON envelope," "mdast node-set v1," "Lossiness policy," "silent drop," "compact," "pretty," "never-elide." No `_Avoid_` synonyms detected. No "data," "header," "preamble," "goldmark AST," "unknown node," "graceful degradation" in the touched code.
- **Module depth.** `Emit` is deep: callers pass `(w, frontmatter, root, opts)` and get correct mdast-JSON-with-key-ordering / null-preservation / position-gating / pretty post-process. Three `Options` fields drive four behaviors. Strong leverage at a small interface. `writeNode` concentrates the per-type mdast key-order switch (the v1 spec) in one decision point. `walkTypes` is parameterized over an `errorReporter` interface — two real adapters (`*testing.T` for the property half, `recorderT` for the negative-direction control) — a genuine seam, not a hypothetical one.
- **Naming clarity.** `Emit` / `writeNode` / `writePosition` / `writeJSONNullableBool` etc. are unambiguous. `mdastNodeSetV1` mirrors CONTEXT.md's glossary entry name exactly. `lossinessCorpus` reflects CONTEXT.md's "Lossiness policy" term. `representativeCorpus` is appropriately scoped (the emit-side property test fixture). No overloaded or `_Avoid_`-synonym names found.
- **Locality.** Null-elide policy concentrated in `writeJSONNullable{Bool,String,Int,StringSlice}` (deletion test: removing them scatters `if p == nil { write "null" }` across every per-type case in `writeNode`; they earn their keep). Canonical stderr template concentrated in `writePositionedError` (deletion test: removing it scatters the `^md2json: %s:%d:%d: %s\n` format across every error branch in `Run`; earns keep). Pretty post-process is a single contiguous branch at the end of `Emit` with comments naming the design tradeoff.

### Deletion-test pass on suspected shallow modules

- `writeDocScopedError` — one-line delegate to `writePositionedError(line=0, col=0)`. Suspected shallow. Deletion test: callers would write `writePositionedError(stderr, pathToken, 0, 0, err.Error())` literally. The 0,0 magic-number pair is a CONTEXT.md-glossary-load-bearing sentinel ("Error format" entry: *"use the sentinel `<path>:0:0:` — `0:0` conventionally means 'no position available.'"*). Keeping the named wrapper preserves the "document-scoped" glossary term at every call site. Earns its keep — naming-clarity lens.
- `emitOnce` helper in `emit_test.go` — wraps `parse.Parse` + `translate.Translate` + `emit.Emit` + error-assert. Three callers in the same file. Deletion would scatter ~9 lines × 3 of `parse/translate/emit/err-check` boilerplate across each test. Earns keep — locality lens.
- `extractAST` / `collectTypes` / `walkTypes` in `lossiness_property_test.go` — three small tree-traversal helpers. Two callers each (`TestEveryEmittedTypeIsInMdastNodeSetV1`, `TestLossinessCorpusCoversEveryV1NodeType`, negative control). Earns keep — locality.
- `isContainer(t string) bool` in `emit.go` — enumerates leaf node types. Suspected shallow (interface ~= impl: a string→bool predicate). Deletion test: would inline a 9-element type-name slice into `writeNode`'s tail. The named predicate makes the "leaf vs container" mdast distinction discoverable at the call site (`if isContainer(n.Type)` reads as the mdast invariant, not as a manual enumeration). Earns keep — naming clarity.

### Candidates scored

| # | Candidate | Lens hit | Score | Reasoning |
|---|---|---|---|---|
| A | Factor the two `emit.Options{...}` literals in `cli.Run` into a single base + FrontmatterOnly override | locality (slight) | Speculative | Both call sites visible in same function within 10 lines; factoring would extract a 1-field-override helper (shallow). Inline reads as two clear branches of the PRD's "unless --frontmatter-only short-circuits" rule. |
| B | Pull pretty post-process out of `Emit` into a `prettyIndent` helper | none clear | Speculative | Would split a strategy already documented in-place. tdd-log refactor pass #5 explicitly decided against this and named the trigger for a future split (a per-type indented writer for streaming). No such requirement today. |
| C | Hoist `representativeCorpus` and `lossinessCorpus` into a shared testdata corpus | none clear | Speculative | Different consumers test different properties; merging would couple emit-side tests to the v1 type-set coverage requirements. Current split mirrors the boundary between "wire format" (emit) and "wire contract enumeration" (translate). |
| D | Replace `walkTypes` / `collectTypes` duplication with one walker parameterized by a visitor func | depth (slight) | Speculative | Two callers, ~15 lines each. Factoring possible but the visitor closure would not be obviously simpler than the two specialized walkers. Property test's negative-control adapter (`recorderT`) already proves the real seam is the error-reporter interface, not the tree-walker itself. |

No candidate scored **Strong**. All four hit at most a slight lens improvement and would cost either clarity (A, B) or independence between test domains (C) or simplicity (D). Style-only changes are out of scope per role contract.

### Conflicts with ADRs

None. ADR-0001 (encoding/normalization) and ADR-0002 (goldmark extensions) sit upstream of the touched code (read + parse stages); none of the candidates considered would have crossed them.

## Pass 1: no Strong candidates this pass

- Files touched: none.
- Tests after: 63 top-level + 138 subtests passing (unchanged from baseline).
- Reverted? no — no change attempted.

The 10a tdd-log's own "Refactor pass" section (items 1-5) already absorbed the refactor-worthy decisions for this slice. The remaining file set is small (1309 LOC across 4 files, 3 fixture dirs), modules are deep at the right boundaries (`Emit` hides four orthogonal formatting concerns behind one Options struct; `walkTypes` has two real adapters proving its seam), naming aligns with CONTEXT.md verbatim, and the deletion test passes on every suspected shallow helper.

## Final

- Tests: 63 top-level + 138 subtests passing (unchanged from baseline).
- `go vet ./...`: clean (unchanged from baseline).
- LOC delta: 0.
- ADRs added: none.
- CONTEXT.md edits: none — no glossary drift detected.
- Most consequential change: none this pass. Notes-only arch-log; the issue's TDD pass already handled its own architectural hygiene (5 candidates explicitly evaluated and rejected with rationale recorded in the tdd-log). Re-litigating those decisions here without new information would violate "Don't re-litigate ADR / prior-pass decisions casually."

VERDICT: accept
