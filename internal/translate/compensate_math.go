package translate

// compensate_math.go houses the two library-behavior-specific math
// compensations ADR-0004 pins (Decision 3 currency-rule demote-only post-pass
// and Decision 5 unclosed-`$$` src-byte predicate), plus the cohesive group
// of ASCII byte-class helpers their predicates share.
//
// Why a separate file (same package as translate.go):
//
//   - ADR-0004's "Negative" bullets explicitly frame these as a pair ("two
//     library-behavior-specific compensations in `translate`"). The
//     file-split gives that concept a name on disk.
//   - The two compensations sit ~400 lines apart in translate.go's history
//     (currency post-pass near the top, unclosed-`$$` predicate adjacent to
//     `translateMath`). Concept-locality is the win: a maintainer searching
//     "library compensation" lands in one file, not two regions.
//   - Same Go package — no public-API change, no new interface, no new seam
//     beyond the file boundary. The two entry points (`currencyPostPass`,
//     `displayMathClosed` + `demoteUnclosedDisplayMath`) are called from
//     `Translate` and `translateMath` respectively in translate.go.
//
// What stayed in translate.go:
//
//   - `inlineMathDelimitedSpan` — has two callers, one of which is the
//     happy-path `translateInlineMath`. The shared seam between
//     compensation and happy-path stays with the happy-path mappers.
//   - `lineStartOffset` — shared between `demoteUnclosedDisplayMath` (here)
//     and `blockOffsets` (heading/table/etc. position math in
//     translate.go). The shared utility stays with its mixed-caller set.

import (
	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/yuin/goldmark/ast"
	textm "github.com/yuin/goldmark/text"
)

// ASCII byte-class predicates used by the math compensation passes.
// Cohesive group: `isASCIIWhitespace` + `isASCIIDigit` are the byte-level
// primitives the remark-math currency-rule predicates inspect (S04
// `currencyPostPass`); `isAllASCIIWhitespace` is the slice-level lift used
// by the unclosed-`$$` predicate to recognize blank lines and the
// closing-fence whitespace tail (S05 `displayMathClosed`). Co-located so
// "every byte-class helper" is one block to scan, not three call-sites
// apart.

// isASCIIWhitespace reports whether b is one of the ASCII whitespace bytes
// CommonMark treats as whitespace for the purposes of the remark-math
// currency rule predicates. Space, tab, LF, CR, FF, VT — the standard
// CommonMark whitespace set.
func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}

// isASCIIDigit reports whether b is one of `0`..`9`.
func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// isAllASCIIWhitespace reports whether every byte in b is one of the
// CommonMark ASCII whitespace set. Empty slice returns true (trivially
// all-whitespace). Named (and not inlined) so the closing-fence predicate
// reads as English at the call site.
func isAllASCIIWhitespace(b []byte) bool {
	for _, c := range b {
		if !isASCIIWhitespace(c) {
			return false
		}
	}
	return true
}

// currencyPostPass walks the goldmark document and, for every
// `*mathjax.InlineMath` it finds, re-applies the three remark-math currency
// predicates against the original source bytes (CONTEXT.md `remark-math
// currency rule`; PRD §Implementation Decisions sub-point 2; ADR-0004
// Decision 3). On any predicate FAILURE the post-pass replaces the
// `*mathjax.InlineMath` node with an `*ast.Text` whose `Segment` covers the
// full original `$...$` range (opening `$`, interior bytes, closing `$` all
// included). The library's inline parser at
// `probe/goldmark-mathjax/inline.go:24-52` matches purely by `$`-run-length
// equality and does NOT check the predicates; translate enforces them one
// layer up.
//
// The three predicates (verbatim CONTEXT.md `remark-math currency rule`):
//   - (i)   opener-followed-by-non-whitespace: src[opener_pos+1] non-whitespace.
//   - (ii)  closer-preceded-by-non-whitespace: src[closer_pos-1] non-whitespace.
//   - (iii) closer-not-followed-by-digit: src[closer_pos+1] is EOF or non-digit.
//
// Opener / closer position derivation: the library stores the interior of
// the `$...$` match as one or more child `*ast.Text` segments on the
// InlineMath node. The `$`-run delimiters are NOT inside the children's
// segments — they were consumed by the inline parser before any child was
// appended. So we recover the delimiter positions by walking left from the
// first child's `Segment.Start` across the leading `$` run and right from
// the last child's `Segment.Stop` across the trailing `$` run. This is the
// same recovery `translateInlineMath` uses to compute the position span;
// both call sites share `inlineMathDelimitedSpan` (defined in translate.go
// alongside its happy-path caller) as the named seam.
// For the (asymmetric) trim-halfspace case (`inline.go:62-82`) where one
// side trimmed a space, the walk-leftward still lands on the opening `$`
// run because the bytes immediately before the (post-trim) first child
// are " $...". The opener_pos+1 byte is then the trimmed-away space, which
// (correctly) fails predicate (i) — exactly the divergence shape pinned by
// PRD fixture #4b.
//
// Demote-only — per ADR-0004 Decision 3, the post-pass NEVER re-promotes
// and NEVER re-scans the demoted range for an inner valid `$...$`. This is
// the load-bearing semantic difference vs. pure remark-math (which would
// recursive-rescan). Convergence and divergence traces are pinned by PRD
// fixtures #4a / #4b respectively. The library's `$$...$$` block math
// matches map to `*mathjax.MathBlock` and are NOT touched by this pass —
// the predicates apply to inline math only (CONTEXT.md: "Display `$$...$$`
// has no such guard").
//
// Coalescing of adjacent text siblings after demote is handled by the
// existing offset-contiguity check at `translateChildren`
// (translate.go); no new coalescing code is introduced here.
func currencyPostPass(n ast.Node, src []byte) {
	// Walk children, collecting *mathjax.InlineMath nodes to potentially
	// demote. We can't mutate-during-iterate (replacing a child mid-walk
	// shifts the sibling pointers); collect first, then act.
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		// Recurse into every node — inline math can live inside any inline
		// container (emphasis, strong, link, table cell, blockquote, list
		// item, footnote, etc.). The recursion is bounded by the AST's
		// tree shape; no cycles.
		currencyPostPass(c, src)
	}
	// Second pass: examine direct children and demote predicate-failing
	// InlineMath. Two-pass ordering guarantees we don't visit a node that
	// was replaced mid-iteration.
	var toDemote []*mathjax.InlineMath
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		im, ok := c.(*mathjax.InlineMath)
		if !ok {
			continue
		}
		if !currencyPredicatesPass(im, src) {
			toDemote = append(toDemote, im)
		}
	}
	for _, im := range toDemote {
		startOff, endOff := inlineMathDelimitedSpan(im, src)
		repl := ast.NewTextSegment(textm.NewSegment(startOff, endOff))
		n.ReplaceChild(n, im, repl)
	}
}

// currencyPredicatesPass returns true iff all three remark-math currency
// predicates pass for the given InlineMath node against the source bytes.
// A return value of false means the node should be demoted to text.
func currencyPredicatesPass(im *mathjax.InlineMath, src []byte) bool {
	startOff, endOff := inlineMathDelimitedSpan(im, src)
	// `endOff` is the offset PAST the closing `$` run, so the closer's last
	// `$` byte is at endOff-1. The "byte immediately after the closer" is
	// at endOff (or EOF).
	openerPos := startOff
	closerPos := endOff - 1
	// (i) opener followed by non-whitespace.
	if openerPos+1 >= len(src) || isASCIIWhitespace(src[openerPos+1]) {
		return false
	}
	// (ii) closer preceded by non-whitespace.
	if closerPos-1 < 0 || isASCIIWhitespace(src[closerPos-1]) {
		return false
	}
	// (iii) closer NOT followed by digit (EOF passes).
	if closerPos+1 < len(src) && isASCIIDigit(src[closerPos+1]) {
		return false
	}
	return true
}

// displayMathClosed returns true iff the source bytes after the body
// (`src[Lines().Last().Stop:]`) contain a closing-fence line — two-or-more
// `$` characters followed by a whitespace-only tail (LF or EOF). Returns
// false (unclosed) when the tail is empty (EOF) or contains nothing but
// blank / non-fence lines before EOF.
//
// Scan rules (per ADR-0004 Decision 5 + `probe/goldmark-mathjax/block.go:49-57`):
//   - Walk forward from `Lines().Last().Stop`.
//   - Skip lines that are empty (LF only) or whitespace-only — but a
//     pure-blank line inside the body never reaches here because the
//     library appends those to Lines() (see PRD §Out of Scope on the
//     no-internal-blank-line scope restriction). So in practice the
//     first non-LF byte after the body either starts the closing fence
//     or is EOF.
//   - A closing fence line is a run of `$` of length >= 2 followed by
//     `util.IsBlank` (whitespace-only) bytes until the next LF or EOF.
//   - Anything else → unclosed.
//
// Deterministic single forward scan over a handful of trailing bytes;
// not heuristic. Cross-ref CONTEXT.md `Unclosed-display-math fall-through
// rule`'s "If `litao91/goldmark-mathjax` emits a partial `ast.Math` or
// hard-errors on unclosed `$$` rather than declining to match, that is
// a TDD-blocking finding for the `translate` layer to compensate
// (demote to prose), not a rule reopen."
func displayMathClosed(lines *textm.Segments, src []byte) bool {
	if lines == nil || lines.Len() == 0 {
		// Degenerate (library never produces this for an actually-opened
		// `$$` block) — treat as closed so we don't fire compensation on
		// a malformed AST shape we can't reason about.
		return true
	}
	tailStart := lines.At(lines.Len() - 1).Stop
	i := tailStart
	for i < len(src) {
		// Locate the end of the current line (next LF or EOF).
		lineEnd := i
		for lineEnd < len(src) && src[lineEnd] != '\n' {
			lineEnd++
		}
		line := src[i:lineEnd]
		// Skip blank / whitespace-only lines (per the library's IsBlank
		// definition applied uniformly here — ASCII whitespace).
		if isAllASCIIWhitespace(line) {
			// Advance past the LF (if any).
			if lineEnd < len(src) {
				i = lineEnd + 1
				continue
			}
			// EOF after blank tail → unclosed.
			return false
		}
		// Non-blank line. Closing fence? Skip any leading ASCII spaces/tabs
		// (the closing `$$` may be indented when the MathBlock is nested
		// inside a listItem / blockquote — the library's block parser
		// dedents the line stream before its `util.IndentWidth(line, 0) < 4`
		// check fires at `probe/goldmark-mathjax/block.go:49`, but the
		// src-tail bytes we inspect here are pre-dedent), then look for a
		// `$`-run of length >= 2 followed by a whitespace-only tail. S06
		// list-item fixture `- $$\n  x\n  $$\n` exercises this: the
		// closer line in source is `  $$\n` — two-space-indented `$$`.
		j := 0
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		k := j
		for k < len(line) && line[k] == '$' {
			k++
		}
		if k-j >= 2 && isAllASCIIWhitespace(line[k:]) {
			return true
		}
		// Not a closing fence — unclosed.
		return false
	}
	// EOF reached with no non-blank line seen → unclosed.
	return false
}

// demoteUnclosedDisplayMath emits the unclosed-`$$` compensation: a
// `paragraph` whose `text` children mirror goldmark's standard prose-
// paragraph segmentation (one `*ast.Text` per source line, segments
// stop BEFORE the LF, no embedded LF in any text value).
//
// Source range:
//   - First child (opening `$$` line): from `lineStartOffset(src, Lines().At(0).Start-1)`
//     to `Lines().At(0).Start - 1` (excludes the LF). For our canonical input
//     `$$\n\frac{a}{b}\n` (PRD fixture #5), the body's first segment starts
//     at offset 3, so the opening line spans [0, 2) → text{value:"$$"}.
//   - Body lines: each Lines() segment, with the trailing LF stripped.
//     For the canonical input, Lines() has one segment [3, 14) covering
//     `\frac{a}{b}\n`; we trim the LF and emit text{value:"\\frac{a}{b}"}
//     spanning [3, 13).
//
// Scope restriction: ADR-0004 Decision 5 + PRD §Out of Scope explicitly
// declare unclosed-`$$` blocks containing an internal blank line OUT OF
// SCOPE for v1 — goldmark's prose parsing would split those into multiple
// paragraphs, but this singular-paragraph compensation cannot represent
// that shape faithfully. We emit whatever the per-line walk produces and
// don't pin a fixture for that case (see tdd-log).
//
// Paragraph position spans from the opening-line start to the body's last
// segment Stop (matching PRD fixture #5's source range; the trailing LF
// after the body is excluded since it would normally be consumed by the
// closing-fence line that doesn't exist here).
func demoteUnclosedDisplayMath(m *mathjax.MathBlock, src []byte, pt *positionTracker) *Node {
	lines := m.Lines()
	firstBodyStart := lines.At(0).Start
	// Opening-`$$` line: [openingStart, openingEnd) excluding the LF.
	openingStart := lineStartOffset(src, firstBodyStart-1)
	openingEnd := firstBodyStart - 1
	if openingEnd < openingStart {
		// Defensive: if the source has no LF between the opening fence
		// and the body (shouldn't happen — the library's Open requires
		// `$$\n` per `block.go:25-43`), collapse to a zero-width opener
		// segment.
		openingEnd = openingStart
	}
	children := make([]*Node, 0, lines.Len()+1)
	children = append(children, &Node{
		Type:         "text",
		Value:        string(src[openingStart:openingEnd]),
		ValuePresent: true,
		Position:     pt.position(openingStart, openingEnd),
	})
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		// Strip the trailing LF (if any) from this body line so the
		// emitted text value has no embedded LF — mirrors goldmark's
		// standard prose-paragraph text segmentation (one `*ast.Text` per
		// source line, segments stop BEFORE the LF, per PRD §Notes
		// "Soft-line-break handling note").
		stop := seg.Stop
		if stop > seg.Start && stop <= len(src) && src[stop-1] == '\n' {
			stop--
		}
		children = append(children, &Node{
			Type:         "text",
			Value:        string(src[seg.Start:stop]),
			ValuePresent: true,
			Position:     pt.position(seg.Start, stop),
		})
	}
	lastBodyStop := lines.At(lines.Len() - 1).Stop
	// Match the per-line "stop before LF" convention for the paragraph's
	// end offset too — so the wire position spans content bytes only,
	// excluding the trailing LF that closes the body. Consistent with
	// `paragraphOffsets`'s body-segment-stop usage.
	endOff := lastBodyStop
	if endOff > openingStart && endOff <= len(src) && src[endOff-1] == '\n' {
		endOff--
	}
	return &Node{
		Type:     "paragraph",
		Children: children,
		Position: pt.position(openingStart, endOff),
	}
}
