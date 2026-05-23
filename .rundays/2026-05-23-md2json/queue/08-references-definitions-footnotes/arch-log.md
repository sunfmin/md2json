# Arch log: mini

Started: 2026-05-23T08:57Z
Scope: mini
File set (from S08 tdd-log "Files touched"):
- internal/translate/translate.go (extended: +Identifier/Label/ReferenceType
  fields on Node; `*ast.LinkReferenceDefinition` case + new
  translateLinkReferenceDefinition; `Reference != nil` branch in
  translateLink and translateImage; new translateFootnote /
  translateFootnoteLink; collectFootnoteLabels pre-pass;
  translateChildren FootnoteList-flattening fast-path; helpers
  identifierFromLabel + referenceTypeToMdast)
- internal/translate/position.go (positionTracker gained
  `footnoteLabels map[int]string`)
- internal/translate/translate_test.go (4 new tests + 2 subtests for
  collapsed/shortcut)
- internal/emit/emit.go (5 new writeNode cases: linkReference,
  imageReference, definition, footnoteReference, footnoteDefinition;
  isContainer leaf-set extended with 3 new leaves)
- testdata/fixtures/39..43 (five new wire fixtures)

## Baseline

- Tests: 43 passing, 0 failing across all packages
  (`go test ./... -count=1`).
- `go vet ./...`: clean.
- LOC in scope: translate.go 1161, position.go 93, emit.go 394
  (1648 across the three source files).
- translateNode dispatch case count: 27 case labels (TableHeader and
  TableRow share translateTableRow → 26 distinct branches).
- writeNode per-type switch: 15 cases.

## Candidate inventory + scores

Reading CONTEXT.md, ADR-0001 + ADR-0002, prior arch-logs S05+S06+S07,
the S08 tdd-log, and the four S08 source files (translate.go,
position.go, emit.go, plus the test file); applying CONTEXT.md "mdast
node-set v1" as the naming lens and the four refactor lenses (glossary
alignment, module depth, naming clarity, locality). Re-confirming or
re-scoring the candidates earlier arch-logs already weighed, plus the
S08-specific candidates the prompt flagged.

### C1 — Promote `nullableString` to a shared util package. Score: NOT-STRONG.

`nullableString([]byte) *string` lives in `internal/translate` and now
has THREE callers (S06 had 2):
- `translateLink.Title` (S06)
- `translateImage.Title` (S06)
- `translateLinkReferenceDefinition.Title` (S08)

S06 arch-log conditioned promotion on "the right time is when S08 adds
`definition.Title` / `linkReference.Label`." Now is that time — but the
calculus has shifted:

The promotion target would be a new `internal/util` (or similar)
package. The function is 6 lines including the doc comment. The
PROMOTION cost: a new package directory, an import in translate.go, a
name-disambiguation question (`util.NullableString`?
`mdjson.NullableString`?). The PROMOTION benefit: 0 today — the only
consumer of this seam is `internal/translate`; `internal/emit` does not
call it (emit's nullable-* helpers operate on `*string`/`*int`/`*bool`,
not raw `[]byte`).

Two-adapter rule: 3 in-package callers IS past the "two adapters = real
seam" threshold, BUT they are all in the same package and share the
same import of `goldmark/util`. The seam is already named and
concentrated inside `internal/translate`; promotion to a shared package
adds ceremony for zero new-consumer benefit (the goldmark→mdast `[]byte
→ *string` conversion is fundamentally a translate-stage concern; emit
operates on the already-translated value-tree).

Deletion test: deleting `nullableString` would re-introduce the 4-line
`if len(b)==0 { return nil }; s := string(b); return &s` block at three
callsites — and that re-introduction would happen INSIDE
`internal/translate`. So the helper earns its keep where it lives.
Promotion changes nothing about the locality argument; it just
sprinkles the import statement.

Re-evaluate when a SECOND package (e.g. `internal/emit` for a future
emit-side `[]byte`-based nullable rule, or a new `internal/parse`
helper) becomes a real caller. Today's calculus: deferred-but-not-acted.

### C2 — Group `Identifier`/`Label`/`ReferenceType` into a `Reference *ReferenceData` substruct. Score: NOT-STRONG.

S08 added three new string fields to `Node`. The natural-feeling DRY
move is grouping them:

```go
type ReferenceData struct {
    Identifier    string
    Label         string
    ReferenceType string  // "" for definition + footnote*
}

type Node struct {
    ...
    Reference *ReferenceData
    ...
}
```

This is exactly the pattern S04–S07 deliberately REJECTED for the Node
shape. CONTEXT.md mdast node-set v1 is a tagged-union keyed by `Type`;
the corresponding Go shape is a flat struct so emit's
`switch n.Type` can read each typed field directly with no nested
nil-check (the S04 design doc on `Node`: "single-struct-with-optional-
fields shape over an interface-per-node-type shape… (1) the emit side
stays a single `switch n.Type` in `writeNode` with no reflection;
(2) optional / nullable mdast fields can be expressed via Go pointer
types").

Three concrete arguments against the substruct:

1. The five `Node.Type` values that touch these fields each use a
   DIFFERENT subset:
   - `linkReference` — Identifier + Label + ReferenceType.
   - `imageReference` — Identifier + Label + ReferenceType (+ Alt).
   - `definition` — Identifier + Label (+ URL + Title).
   - `footnoteReference` — Identifier + Label.
   - `footnoteDefinition` — Identifier + Label.
   So a `ReferenceData` struct that "contains the reference triple" is
   only fully populated by 2/5 types; the other 3 carry a nil
   `ReferenceType` field inside the substruct (or use an empty-string
   sentinel, which we explicitly avoid per the
   `writeJSONNullableString` "" vs null rule). The substruct adds
   irrelevant fields to 3/5 cases.

2. Emit's per-case writers in `writeNode` (`case "linkReference"`,
   `case "definition"`, etc.) directly mention the field they
   serialize:

   ```go
   buf.WriteString(`,"identifier":`)
   writeJSONString(buf, n.Identifier)
   ```

   With the substruct this becomes `n.Reference.Identifier` and
   requires a nil-check (or a defensive `if n.Reference != nil` arm
   around each per-case block). That's added ceremony AND defeats the
   "every typed field on Node is directly addressable, no nesting"
   property writeNode relies on.

3. Existing field neighbours on Node are flat too: `URL string` /
   `Title *string` (used by inline `link`, `image`, AND `definition`)
   sit alongside `Identifier` / `Label` without grouping. Grouping
   only the reference triple would create an inconsistent nesting:
   some-typed-fields-nested, others-not. The discriminated-union
   pattern that justifies the flat shape applies to ALL typed fields
   uniformly.

Deletion test: drop the substruct (i.e. inline the fields back onto
Node), and emit's per-case writers shorten by removing the
nil-check + dot-access. The substruct is pure overhead. Reject.

### C3 — `positionTracker.footnoteLabels` side-channel. Score: NOT-STRONG (with a real friction acknowledgment).

This is the candidate the prompt flagged as feeling "snuck into the
position tracker." It IS friction — `positionTracker` was named for a
single responsibility (byte-offset → line/column conversion), and
`footnoteLabels` is parser-state that has nothing to do with positions.
The doc comment on the field is explicit about the trade-off:

> Stashing this on positionTracker (rather than a separate "translate
> state" struct) keeps the existing threading shape — positionTracker
> is already passed through every translate call — and avoids a churn
> in every function signature.

Three structural options for resolving the mismatch, scored:

#### Option A — Rename `positionTracker` to `translateContext` (or `walkState`).

Renames the type to honestly reflect its dual role. Mechanical
find/replace inside `internal/translate` (40 references across
translate.go + position.go, no test-file references). All callers stay
identical shape — only the type name changes.

Cost: 40 line-by-line edits. Risk: zero (rename only).
Benefit: naming clarity matches what the type actually does.

But: the type's PRIMARY responsibility IS still position tracking
(`src`, `lineStarts`, `position`, `point`, `lastIndexLE` — five of
the six members are position machinery). `footnoteLabels` is one
field of side-state added once at S08. Renaming the type because of
one side-state field overcorrects — the next slice adding a similar
"document-level lookup table" map would dilute the name further
(`translateContext` becomes a bag-of-everything). The current
name's misalignment is a 1-line doc-comment problem ("we also stash
footnote labels here"), not a 40-line rename problem.

#### Option B — New `translateState` struct that embeds `*positionTracker` + `footnoteLabels`.

Conceptually clean separation: positionTracker stays minimal,
state-bag is its own type, every translator takes `state` instead of
`pt`. But: same 40-reference rewrite, AND every translator's call
sites change (`pt.position(...)` becomes `state.pt.position(...)`
or requires re-exposing the methods on `translateState`).

Cost: large signature churn. Risk: medium (must propagate through
every function consistently). Benefit: type-level separation of
two concerns into two types.

The S08 doc-comment trade-off ("avoids a churn in every function
signature") was made deliberately. Re-litigating it because the
churn-vs-clarity calculus has not changed is exactly the "refactor
on stylistic preference alone" the constraints forbid.

#### Option C — Leave as-is; the doc-comment carries the load.

The current state: one type with one side-state field, named for its
primary role, with a doc comment explaining the secondary role and
the trade-off. The "feels snuck in" friction is real but the doc
already concentrates it.

Deletion test on the rename / new-struct options: deleting the
proposed change reverts to the current shape (which has a doc comment
naming the trade-off). The current shape is not load-bearing wrong —
it's a documented trade-off. The bar for un-trade is "the next slice's
addition would make this worse." S09 (frontmatter) doesn't add
document-level lookup tables. S10 (position-info pinning) is pure
position machinery and would CONSOLIDATE on positionTracker's primary
role. S11/S12 (CLI, release) don't touch translate.

Re-evaluate if S10 or later adds a SECOND side-state map to
positionTracker. Today's calculus: leave the doc comment to do the
work. Not Strong this pass.

### C4 — Split translate.go into block/inline files. Score: NOT-STRONG (re-confirmed).

S07 set the threshold at ~1300 LOC or "scrolling the dispatch table
requires PageDown more than once." translate.go is at 1161 LOC (S07:
972 → S08: +189). Still under threshold. The dispatch switch itself is
65 lines (lines 268–332) — still within one screen on a standard
editor. The translator-functions-as-page-of-PageDown property the S07
arch-log argued for is intact.

Re-evaluate at ~1300 LOC. S09 (frontmatter lift) and S10
(position-info) likely add ~50-100 LOC each to translate.go, which
puts the file at ~1250-1350 at S10. The right slice to do the split is
S10's arch pass or the final-arch pass before the Acceptance Gate.
Not Strong this pass.

### C5 — `translateNode` dispatch case count (~27 case labels). Score: NOT-STRONG.

S07 set the threshold at ~30 cases for considering a registry/map
dispatch. translateNode has 27 case labels (TableHeader and TableRow
share translateTableRow → 26 distinct branches). Past S07's "re-
evaluate at 25" prompt but still under the act-on threshold.

The S07 arch-log's deletion-test analysis still holds: a map of
`reflect.Type → handler` loses compile-time exhaustiveness, adds a
per-handler typed-cast, and would actually be LONGER once handlers
are individually defined. The switch IS the canonical
goldmark→mdast-type mapping, with the linear `case` order serving as
the package index for "where's the handler for this goldmark type."

Re-evaluate at ~30 cases (S09 likely adds nothing here; S10 adds
nothing here; the only future case-count growth comes from goldmark
extensions we haven't enabled in v1). Not Strong this pass.

### C6 — `writeNode` per-type switch (~15 cases). Score: NOT-STRONG.

S08 added 5 cases (linkReference, imageReference, definition,
footnoteReference, footnoteDefinition). The switch is now at 15 per-
type cases. Same argument as S06 C2 / S07 C7: the switch IS the
canonical mdast-key-order spec; concentrating it in one place is the
locality we want. Splitting per-type would create one-caller helpers
that obscure the canonical key-order contract.

The S08 tdd-log's "Refactor pass" section explicitly considered
compressing the three new identifier/label writers into a shared
helper and rejected it on the same locality grounds. Concur.

### C7 — `translateFootnote` / `translateFootnoteLink` identifier-equals-label convention. Score: NOT-STRONG.

Both functions set `Identifier: label, Label: label` directly (where
`label` is the goldmark `Footnote.Ref` bytes as a string). The doc
comment explains why ("footnote-label-as-identifier convention has no
case-folding distinction in practice, matching remark's emit"). A
helper like `newFootnoteIdentifierLabel(s string) (ident, label string)`
that returns the same string twice would be silly — the convention is
named in the doc comment of both callers, not in a helper.

Naming-clarity wise: the field assignments `Identifier: label, Label:
label` ARE the documentation of the convention at the call site, more
clearly than a helper would be. Reject.

### C8 — `translateLink` / `translateImage` `Reference != nil` branching. Score: NOT-STRONG.

S08 tdd-log already considered hoisting the branch into a shared
dispatch and rejected it: "the two functions emit different child
shapes (link has translated children, image has flat Alt), so the
branch reads more cleanly inline."

Verifying: `translateLink`'s linkReference branch carries Children
(translated mdast); `translateImage`'s imageReference branch carries
Alt (flat string via `flattenAltText`). The two reference-side outputs
are structurally different mdast nodes (linkReference is a container,
imageReference is a leaf — emit's `isContainer` confirms this). A
unifier would need a dispatch over which-output-shape inside the
helper, which is exactly what the per-function inline branch
expresses without the extra layer. Reject.

### C9 — `Identifier`/`Label`/`ReferenceType` field naming vs CONTEXT.md glossary. Score: ALREADY-ALIGNED.

CONTEXT.md mdast node-set v1 enumerates the exact field names:
- `linkReference{identifier, label, referenceType}`
- `imageReference{identifier, label, referenceType}`
- `definition{identifier, label, url, title}`
- `footnoteDefinition{identifier, label}`, `footnoteReference{identifier, label}`

The Node struct fields `Identifier string`, `Label string`,
`ReferenceType string` are the Go-PascalCase form of those exact
names. Emit's per-case writers serialize them as `identifier`,
`label`, `referenceType` (matching the JSON casing). No `_Avoid_`
synonyms or goldmark-internal names appear on the wire or in field
names. Not a candidate — naming is already aligned.

### C10 — `collectFootnoteLabels` lives in translate.go (not position.go). Score: NOT-STRONG (location is correct).

The function operates on goldmark types (`*east.FootnoteList`,
`*east.Footnote`) and populates a positionTracker field. It could
arguably live next to positionTracker in position.go, but:
- It's a translate-stage operation (goldmark-AST traversal), not a
  position-machinery operation.
- Moving it to position.go would force position.go to import goldmark
  `extension/ast`, which it currently doesn't (position.go only
  depends on the local `Point`/`Position`/`positionTracker` types).
- The other "populate positionTracker on init" operation
  (`newPositionTracker`) lives in position.go because it's pure
  position machinery (lineStarts initialization).

The split is principled: position.go owns position machinery,
translate.go owns goldmark-AST traversal (including the pre-pass that
harvests state into positionTracker). Reject.

## Decision

Zero strong candidates this pass. The S08 changes:

1. Re-use existing seams (`nullableString` for the new
   definition.Title; `identifierFromLabel` + `referenceTypeToMdast`
   as named helpers concentrated on first use; `flattenAltText` from
   S06 reused unchanged for imageReference.Alt).
2. Follow established conventions for the discriminated-union Node
   shape (flat fields with per-Type emit cases).
3. Document the one structural friction (positionTracker.footnoteLabels)
   inline, naming the trade-off.
4. The S08 tdd-log's "Refactor pass" section already considered and
   correctly rejected the obvious shared-writer extractions.

Acting on any of the non-strong candidates above would be either
(a) re-litigating a deliberate trade-off the doc comments name
(C3 rename, C2 substruct), (b) premature against an existing
threshold (C1 promotion at 3-in-package callers, C4 split at 1161 LOC,
C5 dispatch at 27 cases), or (c) stylistic preference (C7 / C8 /
C10). Per the constraints, "every change must improve at least one of:
glossary alignment, module depth, naming clarity, removing duplication"
and "stylistic preference alone is not a legitimate reason."

A legitimate accept with no refactor passes — per the methodology:
"If there are no `Strong` candidates, write an arch-log noting 'no
strong candidates this pass' and emit `VERDICT: accept` — that is a
legitimate acceptable outcome."

## Final

- Tests: 43 passing, 0 failing (unchanged from baseline).
- `go vet ./...`: clean.
- LOC delta: 0 (no source files touched this pass).
- Most consequential observation: the `positionTracker.footnoteLabels`
  side-channel is real friction but the doc-comment trade-off (avoid
  signature churn through 40 translate-call sites by stashing
  document-level lookup state on the existing threaded struct) is
  principled. The right time to revisit is S10 if a SECOND side-state
  map joins it on positionTracker — at that point the "this is a
  state bag, not a position tracker" reading becomes irresistible and
  the rename / new-struct cost is amortized across two fields. Today
  it's one field with a doc comment naming the trade-off.

VERDICT: accept
