# Arch log: mini

Started: 2026-05-23
Scope: mini
File set (from S09 tdd-log "Files touched"):
- internal/parse/parse.go (rewritten: +preScan, +dashFenceCount,
  +newWithoutFrontmatter, +InvalidFrontmatterError, +mapYAMLError,
  +scanResult / frontmatterState; Parse branches on pre-scan)
- internal/parse/parse_test.go (NEW file; 9 unit tests)
- internal/cli/cli.go (+`errors.As(err, &ife)` branch on
  `*parse.InvalidFrontmatterError` in Run, between read and translate)
- testdata/fixtures/{44..51} (8 new wire fixtures)

## Baseline

- Tests: 52 passing, 0 failing across all packages
  (`go test ./... -count=1`).
- `go vet ./...`: clean.
- LOC in S09 file set: parse.go 323, parse_test.go 207, cli.go 212
  (742 across the three Go source files).

## Candidate inventory + scores

Reading CONTEXT.md ("Invalid frontmatter (policy)", "Error format",
"Frontmatter"), ADR-0001 + ADR-0002, prior arch-logs (S03 centralized
`writeDocScopedError`; S06/S07/S08 extracted helpers), the S09 tdd-log,
and the three S09 source files. Applying the four refactor lenses
flagged by the prompt.

### L1 — Module depth: parse's branching path. Score: NOT-STRONG.

The prompt asks: would callers benefit from a
`parse.Result{Frontmatter, Doc, FrontmatterError}` shape instead of two
paths?

Apply the deletion test on the proposed shape change: cli currently
does `errors.As(err, &ife)`. Switching to `pr.FrontmatterError` would
replace that with `if pr.FrontmatterError != nil`. The Go-idiomatic
return shape is `(Result, error)`; making the typed error a field
breaks Go convention for marginal locality gain. Worse: a "partial
success" Result (Doc populated, FrontmatterError non-nil) would
conflate "frontmatter failed but body parsed" with "everything
succeeded," forcing every caller to defensively check the field. The
current shape makes the failure unmissable.

The internal `if scan.state == unclosedFence { ... return ... }` early
return is a textbook short-circuit, not a branch arm in the public
interface. The public interface is one function with `(Result, error)`
return — already deep (5 PRD-level rules behind one entry point: the
closed-fence happy path, the unclosed-fence body-only rule, the
malformed-YAML hard error, the no-frontmatter pass-through, and the
scalar/map shape uniformity).

Reject. The current branch logic is correct; the friction the prompt
gestures at would amplify with the proposed shape, not shrink.

### L2 — Naming clarity: `InvalidFrontmatterError`. Score: ALREADY-ALIGNED.

CONTEXT.md "Invalid frontmatter (policy)" names the failure class
exactly:

> Hard error. When the document opens with a closed `---` fence but
> the YAML between the fences does not parse (tab indentation,
> unbalanced quotes, duplicate keys, etc.), write `md2json:
> <path>:<line>:<col>: invalid frontmatter: <yaml error>` to stderr,
> exit `1`, nothing on stdout.

The Go type name `InvalidFrontmatterError` is the exact PascalCase
form of "Invalid frontmatter" + the conventional `Error` suffix for
typed Go errors. `e.Error()` returns `"invalid frontmatter: <msg>"` —
the exact `<msg>` portion of the canonical stderr template (cli adds
the path/line/col prefix per the helper concentration in L3 below).
Not a candidate.

### L3 — Locality: cli's per-typed-error dispatch. Score: STRONG. Acted.

This is the candidate worth acting on this pass. S03 created
`writeDocScopedError` in cli with this stated intent in its doc
comment:

> This is the single rendering point for the `md2json: <path>:0:0:
> <msg>` template; all non-typed (or not-yet-typed) error paths in Run
> flow through here. **A future slice that introduces a typed error
> from parse / emit with real line/col info should plug an `errors.As`
> branch into this helper rather than re-rendering the template
> inline.**

S09 introduced exactly that typed error — `parse.InvalidFrontmatterError`
— but instead of plugging it into the helper-family, the cli wiring
re-rendered the template inline:

```go
fmt.Fprintf(stderr, "md2json: %s:%d:%d: %s\n", pathToken, ife.Line,
    ife.Col, ife.Error())
```

And the existing `*read.ReadError` branch (from S02) ALSO renders the
template inline via `re.Error()` (which has a self-built
`<path>:<line>:<col>: <msg>` shape). So the canonical CONTEXT.md
"Error format" template `^md2json: ([^:]+):(\d+):(\d+): (.+)$` was
being rendered at THREE places: `writeDocScopedError` plus two inline
`fmt.Fprintf`s, only one of which delegated to the helper.

This is a real locality regression that S03's doc-comment foretold.

Lens hits:

- **Locality**: concentrates the canonical stderr template (CONTEXT.md
  "Error format") to ONE place per S03's stated design intent.
- **Module depth**: cli's three error-rendering paths now route through
  one function (`writePositionedError`) instead of three different
  renderers. Smaller surface for the same behavior; future template
  changes (if e.g. CONTEXT.md "Error format" ever revisits "should
  `:0:0:` be `: :` instead?" — a v1 non-question, but the point is
  there is exactly one place to look) live in one place.
- **Glossary alignment**: CONTEXT.md "Error format" treats positioned
  and document-scoped (`:0:0:`) renderings as instances of the SAME
  regex; the new shape (one renderer, one wrapper) matches that
  unification.

## Pass 1: extract `writePositionedError`; route both typed-error branches and the doc-scoped wrapper through it.

- Files touched: internal/cli/cli.go.
- Change:
  - Added `writePositionedError(stderr, pathToken, line, col, msg)`
    rendering the canonical stderr template once. Doc comment cites
    CONTEXT.md "Error format" verbatim including the regex.
  - Rewrote `writeDocScopedError` as a 1-line wrapper that delegates
    with `line=0, col=0`. Kept the wrapper so call sites preserve the
    "document-scoped" glossary term where it matters (no specific
    line/col info → `:0:0:` sentinel per CONTEXT.md "Error format").
  - In `Run`: the `*read.ReadError` branch now calls
    `writePositionedError(stderr, re.Path, re.Line, re.Col, re.Msg)`
    instead of `fmt.Fprintf(stderr, "md2json: %s\n", re.Error())`.
    `re.Path` and the cli's `pathToken` are identical by construction
    (cli passes `pathToken` to `read.Read` which stores it directly
    on `ReadError.Path`); using `re.Path` keeps the helper's "the
    error owns its position" reading clean.
  - In `Run`: the `*parse.InvalidFrontmatterError` branch now calls
    `writePositionedError(stderr, pathToken, ife.Line, ife.Col,
    ife.Error())` — `ife.Error()` returns `"invalid frontmatter:
    <msg>"`, the `<msg>` portion of the canonical template.
- Bytes preserved: all three renderers produced the same wire format
  before this change (verified by hand against the regex). After the
  change, the rendering function is shared but the output bytes are
  identical. Spot-checked tests:
  - `TestFixtures/46-malformed-yaml-hard-error`: PASS (the
    `:3:1: invalid frontmatter: ...` line).
  - `TestFixtures/51-frontmatter-only-malformed-yaml`: PASS.
  - `TestLeadingInvalidUTF8StdinHardErrors`: PASS (the `:1:1: invalid
    utf-8 byte at offset 0` line, exercising the `ReadError` →
    `writePositionedError` path).
  - `TestMidDocumentInvalidUTF8StdinHardErrors`: PASS.
- Tests after: 52 passing, 0 failing (`go test ./... -count=1`).
- `go vet ./...`: clean.
- Reverted? no.

## L4 — Module depth: `preScan` + `dashFenceCount` location. Score: NOT-STRONG.

Could they live in a `frontmatter` sub-module (e.g.
`internal/parse/frontmatter/`)? The two helpers are 50 LOC together,
used at EXACTLY one site (`Parse`), and share knowledge with the rest
of `parse` (the `yamlStartLine = 2` constant, the `mapYAMLError`
line-translation math, the goldmark-extension toggle).

Apply the deletion test on the proposed promotion: a new
`internal/parse/frontmatter` package would have one consumer
(`parse.Parse`) and would force the `frontmatterState` /
`scanResult` types to either become exported (cross-package surface
where there is no second adapter), or stay unexported and the two
helpers return raw tuples to `parse.Parse` (more ceremony). The
two-adapter rule fails: one in-package caller, no other potential
consumer in v1.

S10 (position-info pinning) and S11 (CLI flags) don't touch
frontmatter at all. S12 (release) is build/CI only. No future v1
slice opens a second consumer. Reject.

The current shape — package-private helpers inside `parse/parse.go`,
co-located with `mapYAMLError` and the goldmark instance factories
that share their knowledge — is correct locality.

## L5 — speculative future: per-error-type formatter registry. Score: SPECULATIVE (deferred).

The prompt asks: "Could a per-error-type formatter table emerge if
S10+ adds more typed errors?" Possible, but with 2 typed-error
branches (`ReadError`, `InvalidFrontmatterError`) and each one being
3 lines of `errors.As` + `writePositionedError(...)`, a registry would
be a 2-entry map of `reflect.Type → func` — premature ceremony.

The "1 adapter = hypothetical seam, 2 = real seam" rule is met at 2
typed errors, BUT the registry shape adds reflection cost and
indirection where the current shape (two parallel `if errors.As`
arms calling one shared helper) reads as a flat dispatch already. If
S10/S11 adds a third or fourth typed error (e.g. an
`emit.MaxDepthError`, or a `translate.UnsupportedNodeError`), then
the registry shape earns its keep.

Defer. Re-evaluate at 3+ typed errors. The Pass 1 refactor already
established the seam (one canonical renderer); the registry would be
the next iteration on top of it.

## L6 — `parse.Parse`'s "with vs without frontmatter" instance pair. Score: NOT-STRONG (re-confirmed).

The S09 tdd-log "Refactor pass" section already considered this:

> Did NOT refactor the two-goldmark-instance pattern. Considered
> factoring `New` + `newWithoutFrontmatter` into a single `func
> newMarkdown(withFrontmatter bool) goldmark.Markdown` to remove the
> structural duplication. Rejected: the two factory names communicate
> the intent (`New` = the public extension set; `newWithoutFrontmatter`
> = the unclosed-fence branch's helper) more clearly than a boolean
> parameter.

Concur. The two factories differ by exactly one `goldmark.WithExtensions`
argument; a boolean-parameterized factory would hide that the
unclosed-fence path is a deliberate-and-named exception, not a knob.
Naming clarity > DRY for a 6-line factory pair.

## Final

- Tests: 52 passing, 0 failing (unchanged from baseline; `go test
  ./... -count=1`).
- `go vet ./...`: clean.
- LOC delta: cli.go 212 → 225 (+13). The +13 is the new
  `writePositionedError` function with its doc comment; the inline
  rendering at two call sites collapsed (-3 LOC) but the new helper's
  doc comment is +16 LOC net. parse.go and parse_test.go untouched.
- Most consequential change: cli now renders the canonical CONTEXT.md
  "Error format" template (`^md2json: ([^:]+):(\d+):(\d+): (.+)$`)
  in exactly ONE function (`writePositionedError`), with
  `writeDocScopedError` as a named wrapper for the `:0:0:`
  document-scoped sentinel arm. Closes S03's foretold-but-not-acted-
  upon design intent ("a future slice that introduces a typed error
  ... should plug an `errors.As` branch into this helper rather than
  re-rendering the template inline"). Three error-renderers became
  one + one named wrapper; both typed-error branches in `Run` now
  call the shared helper.

VERDICT: accept
