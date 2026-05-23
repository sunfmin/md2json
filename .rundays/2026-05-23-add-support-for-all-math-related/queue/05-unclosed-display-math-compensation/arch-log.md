# Arch log: 05-unclosed-display-math-compensation

Started: 2026-05-24T00:00:00Z
Scope: mini
File set (from S05 tdd-log):
- internal/parse/parse_test.go
- internal/translate/translate.go
- internal/translate/translate_test.go
- testdata/fixtures/73-unclosed-display-math-demotes-to-paragraph-nopos/
- testdata/fixtures/74-unclosed-inline-math-rides-through-as-text-nopos/
- testdata/fixtures/75-unclosed-display-math-demotes-to-paragraph-default/

## Baseline
- Tests: 6 packages PASS (md2json, cli, emit, parse, read, translate). 0 failing.
- Baseline command: `go test ./...` from product root.

## Candidate review

### Candidate A: factor S04 `currencyPostPass` + S05 `displayMathClosed`/`demoteUnclosedDisplayMath` behind a shared "library-compensation pass" abstraction.

Verdict: **Speculative**. Deletion test: imagining a shared seam, what
would it look like? `currencyPostPass` is a recursive doc walker that
MUTATES the goldmark AST in place (replaces `*mathjax.InlineMath` with
`*ast.Text`); the S05 compensation is a per-node dispatch arm inside
`translateMath` that EMITS an mdast `*Node` directly. Different layer
(pre-walk AST mutate vs. translate-time emit), different trigger
(global walk vs. dispatch arm), different output type. ADR-0004
Consequences section already frames them as two distinct compensations
("the second library-behavior-specific compensation in `translate`").
A shared abstraction today would either be a marker interface with one
method (no leverage) or force one mechanism into the other's shape
(loses locality). One adapter, not two — hypothetical seam, not real.
Re-evaluate when a third library-compensation lands.

### Candidate B: hoist three ASCII byte-class helpers into a separate file.

Verdict: **Worth exploring → downgraded to a co-location pass (see
Pass 1)**. `isASCIIWhitespace`, `isASCIIDigit`, `isAllASCIIWhitespace`
are all called only from within `translate.go` (currency post-pass +
displayMathClosed). Moving to a new file would add an import boundary
without callers outside translate, violating "only refactor when at
least one of glossary/depth/naming/locality improves". But the helpers
were physically apart in the file (two at top, one mid-file), which is
a locality miss — same intent, scattered placement. The Pass 1 edit
co-locates the third one with its siblings and adds a one-paragraph
group header explaining the cohesion.

### Candidate C: `translateMath` doc paragraph is stale post-S05.

Verdict: **Strong**. The trailing paragraph in `translateMath`'s doc
comment described how `blockOffsets` would naturally handle the
unclosed-fence case — written before S05, when there was no
demote branch. Post-S05, the unclosed case never reaches that
`blockOffsets` call; it routes through `demoteUnclosedDisplayMath`.
Reader of the comment would expect the closed-case body of
`translateMath` to handle both shapes; the in-body inline comment
duplicating ADR-0004 Decision 5 then takes two screens of reading to
reconcile. Pass 2 rewrites the doc to describe the actual two-branch
shape and drops the duplicated inline comment.

### Candidate D: `demoteUnclosedDisplayMath` builds an mdast paragraph by hand — is there a helper?

Verdict: **No candidate**. The prompt framing was wrong — the function
builds a translate-side mdast `*Node`, not a goldmark `*ast.Paragraph`.
Every `translate*` function in the file does the same direct-`&Node{}`
construction; there is no factory helper because the construction is
type-uniform (small struct literal). Introducing one would shadow the
existing pattern.

### Candidate E: naming review.

Verdict: **No drift**. `displayMathClosed` / `demoteUnclosedDisplayMath`
both align with CONTEXT.md "Unclosed-display-math fall-through rule"
("closing fence", "demote to prose"). `isAllASCIIWhitespace` matches
the existing `isASCIIWhitespace` cousin. No load-bearing CONTEXT.md
term is missing or rendered as an `_Avoid_` synonym.

## Pass 1: co-locate `isAllASCIIWhitespace` with its byte-class siblings

- Files touched: internal/translate/translate.go
- What: moved the `isAllASCIIWhitespace` definition from line ~633
  (mid-file, sitting between `displayMathClosed` and
  `demoteUnclosedDisplayMath`) up next to `isASCIIWhitespace` and
  `isASCIIDigit` at the top of the file. Added a one-paragraph group
  header naming the three as the "ASCII byte-class predicates used by
  the math compensation passes". No behavior change; pure relocation +
  documentation.
- Why (lens): locality. A reader scanning for "what byte-class helpers
  exist?" now sees them as a single block, not three call-sites apart.
  Also pre-positions any future "fourth byte-class predicate" hit
  toward the same group.
- Tests after: 6 packages PASS. No regressions.
- Reverted? no.

## Pass 2: rewrite stale `translateMath` doc paragraph; drop duplicated inline comment

- Files touched: internal/translate/translate.go
- What: replaced the post-position-paragraph in `translateMath`'s doc
  (the one describing how `blockOffsets` handles the unclosed case)
  with a 6-line "closed-vs-unclosed branch" paragraph that names the
  actual S05 branch (`displayMathClosed` → `demoteUnclosedDisplayMath`).
  Removed the inline comment block immediately before the branch,
  which duplicated ADR-0004 Decision 5's prose (still pinned in the
  ADR; not load-bearing inside the function).
- Why (lens): naming clarity at the comment level. The previous text
  described a code path that no longer exists ("the same `blockOffsets`
  call naturally ends at the body's last LF since there is no closer
  to include"). A new reader would chase that into the closed-case
  emit and find no unclosed handling there at all. The new text
  matches the actual two-branch shape and the in-body code (`if
  !displayMathClosed(...) { return demoteUnclosedDisplayMath(...) }`)
  is now self-evident from the function header.
- Tests after: 6 packages PASS. No regressions.
- Reverted? no.

## Final

- Tests: 6 packages PASS (unchanged from baseline).
- LOC delta: roughly -20 (removed duplicated S05-branch inline comment
  and the stale doc paragraph; added a short group header and a tight
  6-line branch description).
- Most consequential change: Pass 2 — the `translateMath` doc no longer
  describes a code path that doesn't exist. The closed-vs-unclosed
  routing is now visible at the function-header level, mirroring how
  `translateLink`/`translateImage` already document their
  inline-vs-reference branch.

VERDICT: accept
