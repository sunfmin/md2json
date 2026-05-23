# Arch log: 12-release-pipeline-and-go-install

Started: 2026-05-23
Scope: mini
File set (from tdd-log.md):
- `internal/cli/version.go`
- `integration_test.go`
- `release_workflow_test.go`
- `.github/workflows/release.yml`
- (`docs/adr/0003-release-pipeline.md` — doc, already pinned)

## Baseline
- `go test ./...`: all 6 packages pass (root, internal/cli, internal/emit, internal/parse, internal/read, internal/translate)
- `go vet ./...`: clean
- Total LOC in scope (rough): `version.go` 19, `integration_test.go` 473, `release_workflow_test.go` 448, `release.yml` 173

## Candidate scan

Applied deletion test + glossary lens to the file set.

- **`version.go`**: 19 lines, one `var Version = "dev"`. Deletion would force inlining into `cli.go`. Earned: the ldflags `-X .../cli.Version=...` symbol path needs an addressable package var, and isolating it in its own file makes the build-stamp surface a single grep. **Pass — no change.** Score: not a candidate.

- **`release.yml`**: matrix uses `ext` and `smoke` flags per entry; comments are heavy but informative. No friction. Score: not a candidate.

- **`release_workflow_test.go`** helper-extraction shape: each step-walking test does its own `for _, raw := range steps { step["run"].(string) }`. Could extract `runText(step) string`. Saves ~3 lines per call site over 3 sites; tests are already fairly readable. Score: **Worth exploring** — defer (not Strong; deletion test marginal).

- **`integration_test.go`** harness shape (`fixture`, `loadFixture`, `runFixture`, `compareFixture`, `assertFixture`): already deep and well-named; not in scope for behavior-changing rework. Score: not a candidate.

- **v1 ship-criterion envelope literal duplication**: the string `{"frontmatter":null,"ast":{"type":"root","children":[]}}` is load-bearing in **CONTEXT.md "v1 ship criterion"** and appears unannotated as a raw literal in two Go test files in `package main_test`:
  - `integration_test.go:441` — `const wantEnvelope` inside `TestGoInstallProducesBinaryPassingShipCriterion`
  - `release_workflow_test.go:432` — bare literal inside `TestReleaseWorkflowSmokeTestRunsShipCriterion` substring scan

  Both reference the same load-bearing glossary concept ("v1 ship criterion"). Two call sites of the same named concept means the seam is **real**, not hypothetical. Promoting to a package-level named constant improves:
  - **glossary alignment** — name the constant after the CONTEXT.md term
  - **locality** — the canonical envelope lives once; the third call site (workflow YAML) is intrinsically out-of-language but is now anchored to a Go name a reader can grep for

  (Note: a third literal in `integration_test.go:177-180` is **intentionally different by one byte** — it is the negative-control input to `TestHarnessDetectsSingleByteStdoutDiff`. Not the same load-bearing concept; left untouched.)

  Score: **Strong**.

## Pass 1: name the v1 ship-criterion envelope at file scope

- Files touched:
  - `integration_test.go` (promote local `const wantEnvelope` to file-scope `const v1ShipCriterionEnvelope` with a docstring tying it to CONTEXT.md)
  - `release_workflow_test.go` (replace bare literal in `TestReleaseWorkflowSmokeTestRunsShipCriterion` with the named constant)
- Lens: **glossary alignment** + **locality**.
- Tests after: `go test ./...` all pass; `go vet ./...` clean. Unchanged from baseline.
- Reverted? no.

## Final
- Tests: all packages green (unchanged from baseline).
- `go vet ./...`: clean (unchanged).
- LOC delta: +5 (docstring on the new constant; literal references shortened).
- Most consequential change: the v1 ship-criterion envelope — a CONTEXT.md load-bearing term — is now a single named identifier (`v1ShipCriterionEnvelope`) in `package main_test`. The two Go test files that pin acceptance criterion #5 (smoke step) and criterion #1 (`go install` produces a working binary) share one source of truth that a reader can grep from the glossary entry. The third instance (the `diff` in `.github/workflows/release.yml`) remains a raw literal because the workflow is not Go, but the canonical name in Go makes the cross-reference obvious.

## Notes for future passes (not acted on this pass)

- `runText(step any) string` helper in `release_workflow_test.go` would deduplicate three step-walking call sites. Marginal — defer to a final-arch pass when the file set is broader.

## ADRs

No new ADR. Pass 1 is a local naming improvement, not an architectural decision worth pinning beyond ADR-0003.

VERDICT: accept
