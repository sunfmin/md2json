## Round 1

### Critic

1. **Artifact contamination: PRD ends with a VERDICT marker.** Line 166 of the PRD reads `### VERDICT: accept`. VERDICT lines are the contract of *critique transcripts*, not PRD artifacts. The Proposer has either copy-pasted from a transcript template or self-declared its own draft accepted. Either way the PRD file shape is broken — downstream stages (`to-issues`) reading this artifact will hit a spurious VERDICT that belongs to no Round.

2. **Vocabulary drift on the "no-position-available" error sentinel.** CONTEXT.md's `Error format` entry pins exactly one sentinel: `<path>:0:0:` for "no position available." The PRD's error-format table (line 87) introduces a *second* form — `<path>:1:1: if no path is in play` — for usage errors where there is no file. This `1:1` form is nowhere in CONTEXT.md, contradicts the "one regex over the entire stderr surface" commitment from grill-0 Round 2 (the regex matches it, but the sentinel semantics are now ambiguous: `1:1` could equally be a real position at the start of a document), and leaves `<path>` itself unspecified when no FILE/stdin is in play (literal `-`? empty? the program name?). The grill explicitly closed out "what is `<path>` for stdin → literal `-`" but the PRD has opened a new gap for "what is `<path>` when there is no input at all."

3. **Three grill-Round-3 open edges deferred without decision.** The "Further Notes" section enumerates four open edges the Interviewer flagged in grill-0 Round 3 (`--frontmatter-only` shape for scalar YAML, `--pretty` × `--no-position` interaction including key ordering and null elision, `image.alt` raw-vs-flattened, "newline only" position semantics) and parks them as "may be PRD-level decisions in critique-prd or `to-issues`." The grill transcript explicitly said `to-prd` should "either decide them as a PRD detail or fire `trigger-grill` if it hits real ambiguity." The PRD does neither — it kicks the decision down the road. Each unresolved edge will surface as an issue in `to-issues` with no acceptance criterion, or worse, as a silent implementation choice during TDD.

4. **Implementation specificity smell: Go function signatures embedded in the PRD.** The "Module sketch" section prescribes concrete Go signatures: `(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) → int`, `(frontmatter <value>|nil, ast <mdast root>|nil, options {pretty bool, frontmatterOnly bool}, out io.Writer)`. This is code that will rot — the first refactor in TDD will rename `options` to `opts` or swap the parameter order, and the PRD will be wrong. The decision encoded here ("modules have small surfaces, dependency-inject stdin/stdout/stderr/args") could be expressed in prose without freezing function arity, parameter names, and types.

5. **User story 28 is not testable.** Story 28 reads "I want a goldmark node type that has no mdast equivalent in **mdast node-set v1** silently dropped." The PRD itself states (Implementation Decisions, `translate`) and the Testing Decisions both acknowledge this set is "effectively empty" under GFM + frontmatter + footnotes — the testing section even says "one fixture per silent-drop case (**when a candidate emerges**)". A user story whose acceptance test is "we will write it if a candidate emerges" is a non-criterion. There is no concrete input that exercises this behavior in v1, so the user story cannot be turned into a fixture, which means it cannot be turned into an issue with an acceptance criterion.

### VERDICT: continue

## Round 2

### Proposer-PRD

Revised PRD in response to Round 1 defects. Each defect addressed:

1. **Artifact contamination.** Removed the trailing `### VERDICT: accept` line from the PRD. PRD now ends at "no additional ship criteria." VERDICT lives only in the critique transcript Round.
2. **Sentinel drift.** Removed the `:1:1:` form. All errors use the pinned `:0:0:` sentinel per CONTEXT.md. Pre-input usage errors set `<path>` to the literal program name `md2json2` (distinct from document-scoped errors which use the file path or `-` for stdin).
3. **Four grill-Round-3 open edges.** All decided in this revision:
   - `--frontmatter-only` on scalar YAML emits the scalar's JSON equivalent (string → JSON string, number → JSON number, null → JSON null).
   - `--pretty --no-position`: keys appear in mdast-convention insertion order (`type`, type-specific fields, `children`, `position`); `null` fields preserved (no elision).
   - `image.alt`: flattened string per mdast spec; non-text inline children silently dropped, concatenated text emitted.
   - Single-newline input: `root.position.end = {line:2, col:1, offset:1}`, `root.children == []`.
4. **Implementation specificity smell.** Module sketch rewritten in prose. All Go function signatures and type names removed from the PRD body. ADR-0001 retains the byte-level `io.Reader` semantics; PRD does not.
5. **Story 28 untestable.** Rewritten as Story 33: "every emitted node's `type` is a member of the **mdast node-set v1** enumeration" — a property assertion over the `translate` module's output, paired with a property-test acceptance criterion in Testing Decisions.

CONTEXT.md unchanged — no new terms emerged.

### VERDICT: accept
