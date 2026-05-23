// Package translate walks a goldmark AST and produces a Go value tree of
// mdast-shaped nodes, ready for JSON encoding by the emit module.
//
// The wire contract is mdast (see CONTEXT.md "mdast node-set v1" and
// "AST (output) / mdast"); goldmark is treated as a parser library, not as a
// schema. This module is the only place that knows the goldmark→mdast mapping.
//
// Supported nodes are the full closed enumeration `mdast node-set v1` from
// CONTEXT.md: root, paragraph, heading, text, emphasis, strong, delete,
// inlineCode, code, blockquote, list, listItem, thematicBreak, link, image,
// linkReference, imageReference, definition, html, table, tableRow,
// tableCell, footnoteDefinition, footnoteReference, break, plus the v1.x
// math Run additions inlineMath and math. Dispatch lives in `translateNode`.
//
// Output type: a Go value tree (pointers to `Node` structs), NOT goldmark's
// native AST. After translate, the rest of the pipeline never sees goldmark
// types.
//
// Math compensations (ADR-0004 Decisions 3 and 5) live in the sibling file
// `compensate_math.go`; see this file's package-internal comment block above
// `inlineMathDelimitedSpan` for the entry-point map.
package translate

import (
	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	textm "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// The two ADR-0004 library-behavior-specific math compensations live in a
// sibling file `compensate_math.go` (same package). Entry points called
// from this file:
//
//   - `currencyPostPass(doc, src)` — invoked by `Translate` before walking
//     children; mutates the goldmark AST in place to enforce CONTEXT.md
//     `remark-math currency rule` (ADR-0004 Decision 3).
//   - `displayMathClosed(lines, src)` + `demoteUnclosedDisplayMath(m, src, pt)`
//     — invoked by `translateMath` to route the unclosed-`$$` case to the
//     prose-paragraph compensation (ADR-0004 Decision 5).
//
// `inlineMathDelimitedSpan` stays in this file because it has two callers:
// the happy-path `translateInlineMath` (defined here) and the currency
// post-pass (defined in `compensate_math.go`). The shared seam between
// compensation and happy-path stays adjacent to the happy-path mapper.

// inlineMathDelimitedSpan returns the byte-offset range [startOff, endOff)
// in src that the InlineMath's `$...$` match covers, INCLUDING the opening
// and closing `$` delimiter runs (hence "delimited" — the span encloses the
// delimiters, it is not the interior-only span). Derived by walking leftward
// across the leading `$` run from the first child's segment start, and
// rightward across the trailing `$` run from the last child's segment stop.
//
// Two callers: `currencyPredicatesPass` needs the delimiter-byte positions
// to apply the three remark-math currency-rule predicates (which inspect
// `src[opener+1]`, `src[closer-1]`, `src[closer+1]`); `translateInlineMath`
// needs the same range as the `position` field on the emitted mdast
// `inlineMath` node (CONTEXT.md "Position info": a node's position spans
// the source bytes that produced it, delimiters included). The two-adapter
// rule justifies the seam.
//
// Returns (0, 0) for a degenerate InlineMath with no Text children — the
// library never produces this (the inline parser only appends RawText
// segments after a successful match), but the defensive return keeps the
// caller's predicate logic well-typed.
func inlineMathDelimitedSpan(im *mathjax.InlineMath, src []byte) (start, end int) {
	first := im.FirstChild()
	last := im.LastChild()
	if first == nil || last == nil {
		return 0, 0
	}
	ft, ok := first.(*ast.Text)
	if !ok {
		return 0, 0
	}
	lt, ok := last.(*ast.Text)
	if !ok {
		return 0, 0
	}
	start = ft.Segment.Start
	for start > 0 && src[start-1] == '$' {
		start--
	}
	end = lt.Segment.Stop
	for end < len(src) && src[end] == '$' {
		end++
	}
	return start, end
}

// Point is one end of a source-range position (line/column/offset). 1-indexed
// for line and column; offset is a byte offset into the normalized
// (post-BOM-strip, LF-only) document.
type Point struct {
	Line   int
	Column int
	Offset int
}

// Position is the source-range span attached to every emitted node by default.
// `--no-position` is applied at the emit stage, not here — translate always
// attaches the position so the structure is uniform; emit decides whether to
// serialize it.
type Position struct {
	Start Point
	End   Point
}

// Node is the mdast node value-tree shape.
//
// Type-specific fields:
//   - Depth (heading) — non-zero for heading nodes (1..6); zero means "no
//     depth field on the wire". Emit uses the node's Type, not the zero
//     check, to decide whether to write the field.
//   - Value (text, inlineCode, code, html) — the literal string content of
//     value-bearing nodes. ValuePresent distinguishes "no value field" from
//     `value: ""` because Go's zero string is the same as an explicit empty
//     value.
//   - Ordered, Start, Spread (list) — `ordered` is bool, `start` is a
//     pointer-to-int so the unordered case can serialize as `null` (mdast
//     convention: unordered lists carry `start: null`, not "start omitted").
//   - Spread (listItem) — bool, mdast convention.
//   - Checked (listItem) — pointer-to-bool so non-task items can serialize
//     as `checked: null` (acceptance criterion S05#7: never elided). S07
//     hoists the task-checkbox state onto this field; in S05 every emitted
//     listItem carries `null` here.
//   - Lang, Meta (code) — pointer-to-string so the indented-code case
//     (`lang: null, meta: null` per CONTEXT.md mdast node-set v1) can be
//     distinguished from a fenced code block with an empty info string
//     (which would still serialize the fields as non-null strings).
//   - URL (link, image) — plain string (never null per mdast); Title
//     (link, image) — pointer-to-string so `title: null` (no title text)
//     serializes distinct from `title: ""` (empty title). Alt (image) —
//     plain string (`image.alt: string` per mdast; goldmark's non-text
//     inline children are flattened into the alt string by translate, so
//     the alt itself is always a flat string).
//
// Position is a pointer so the emit stage can express "no position" by
// nil-ing it (used internally; the public --no-position flag is handled by
// the emit module via a separate option, not by mutating this field).
//
// The mdast spec is a tagged-union keyed by `Type`. We chose a
// single-struct-with-optional-fields shape over an interface-per-node-type
// shape for two reasons: (1) the emit side stays a single `switch n.Type`
// in `writeNode` with no reflection; (2) optional / nullable mdast fields
// can be expressed via Go pointer types (`*int`, `*bool`, `*string`) so
// the JSON `null` distinction from "field absent" or "field equals zero"
// is preserved on the wire. The existing `ValuePresent` flag (S04) is the
// boolean-flag flavor of the same pattern for `Value` (the `value: ""`
// vs "no value field" distinction); S05 extends the pattern to
// `Start *int` and `Checked *bool`; S06 extends to `Lang *string`,
// `Meta *string`, `Title *string`.
type Node struct {
	Type         string
	Depth        int
	Value        string
	ValuePresent bool
	Ordered      bool
	Start        *int
	Spread       bool
	Checked      *bool
	Lang         *string
	Meta         *string
	URL          string
	Title        *string
	Alt          string
	// Align (table) — per-column alignment, one slot per column. Each slot
	// is `*string` so the JSON serialization can express the
	// CONTEXT.md mdast node-set v1 contract for `table.align`
	// ("`left`/`right`/`center`/`null`"): nil → JSON `null`, non-nil →
	// the corresponding alignment string. S07 extends S05/S06's
	// "pointer-to-T for nullable mdast fields" convention from the scalar
	// case (`Start *int`, `Checked *bool`, `Lang *string`) to the
	// per-element case on a slice. `[]string` with an empty-string
	// sentinel was rejected because `""` is the empty-string distinct
	// value on the wire per the writeJSONNullableString rule, not the
	// no-alignment sentinel.
	Align []*string
	// Identifier, Label, ReferenceType (S08) — reference-style link/image,
	// definition, footnote reference, footnote definition. Per CONTEXT.md
	// mdast node-set v1 `linkReference{identifier, label, referenceType}`
	// and `definition{identifier, label, url, title}`. Identifier is the
	// normalized (case-folded, whitespace-collapsed) reference key; Label
	// preserves the raw label text as written. ReferenceType is the empty
	// string for `definition` and footnote-* nodes (they do not carry the
	// field on the wire); `linkReference` / `imageReference` carry
	// `"full"`, `"collapsed"`, or `"shortcut"` per CommonMark §6.6.
	Identifier    string
	Label         string
	ReferenceType string
	Children      []*Node
	Position      *Position
}

// Options controls per-translate behavior. No per-translate knobs are
// exposed today; the struct is kept so future slices can add per-node
// options without reshuffling the public API.
type Options struct{}

// Translate walks the goldmark document root and returns the mdast root node
// as a Go value tree. `src` is the normalized source bytes (needed for value
// extraction on inline nodes and for position math).
//
// Footnote pre-pass: before walking, we scan the document for the
// `*east.FootnoteList` container goldmark's footnote AST transformer
// appends to the document root and harvest its Index → Ref-label map. The
// inline `*east.FootnoteLink` only carries `Index`; the source label
// (`"a"` for `[^a]`) lives on the corresponding `*east.Footnote` in the
// FootnoteList. The map lets translateFootnoteLink emit
// `footnoteReference{identifier, label}` with the source label rather than
// the 1-based numeric index. The pre-pass is O(definitions) and a no-op
// when the document has no footnotes.
func Translate(doc *ast.Document, src []byte, opts Options) *Node {
	pt := newPositionTracker(src)
	collectFootnoteLabels(doc, pt)
	// Currency-rule demote-only post-pass (ADR-0004 Decision 3, PRD
	// §Implementation Decisions sub-point 2). Definition lives in the
	// sibling file `compensate_math.go`. Mutates the goldmark AST in place
	// by replacing predicate-failing `*mathjax.InlineMath` nodes with
	// `*ast.Text` covering the full original `$...$` range. Adjacent
	// `*ast.Text` siblings are then coalesced by `translateChildren`'s
	// existing offset-contiguity check (see the `if len(out) > 0 && n.Type
	// == "text"` branch in translateChildren) — no new coalesce code
	// introduced. Demote-only: never re-promotes, never re-scans the
	// demoted range for an inner valid match.
	currencyPostPass(doc, src)
	root := &Node{
		Type:     "root",
		Children: translateChildren(doc, src, pt),
	}
	root.Position = rootPosition(pt)
	return root
}

// collectFootnoteLabels walks the document's direct children for any
// `*east.FootnoteList` and records each child `*east.Footnote`'s
// `Index → string(Ref)` mapping on the positionTracker. The footnote AST
// transformer always appends the list to the document root (see
// goldmark `extension/footnote.go::footnoteASTTransformer.Transform`), so
// a single-level scan suffices.
func collectFootnoteLabels(doc *ast.Document, pt *positionTracker) {
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		list, ok := c.(*east.FootnoteList)
		if !ok {
			continue
		}
		for fn := list.FirstChild(); fn != nil; fn = fn.NextSibling() {
			f, ok := fn.(*east.Footnote)
			if !ok {
				continue
			}
			pt.footnoteLabels[f.Index] = string(f.Ref)
		}
	}
}

// translateChildren walks the children of a goldmark node and returns the
// mdast Node slice. Two cross-cutting rules apply:
//
//  1. Consecutive sibling `*ast.Text` nodes whose segments are contiguous are
//     coalesced into a single mdast `text` node (mdast spec: one `text` node
//     per uninterrupted text run); goldmark internally splits text runs at
//     segment boundaries that have no semantic meaning on the wire.
//  2. When a goldmark `*ast.Text` has its HardLineBreak flag set (trailing
//     two-space OR `\`-escaped newline per CommonMark §6.7), translate inserts
//     a synthetic mdast `break` node IMMEDIATELY AFTER the `text` node it
//     translated to. The `break` is not a goldmark sibling — it is derived
//     from the HardLineBreak flag on the source Text node. Position of the
//     synthetic break spans from the end of the preceding text segment to the
//     start of the following text segment (the "two-space + newline" or
//     "`\` + newline" run that goldmark consumed).
func translateChildren(parent ast.Node, src []byte, pt *positionTracker) []*Node {
	out := []*Node{}
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		// FootnoteList flattening: goldmark wraps every footnote definition
		// in a `*east.FootnoteList` container appended to the document
		// root. mdast has no analog — `footnoteDefinition`s are top-level
		// siblings (see CONTEXT.md mdast node-set v1). Splice each child
		// `*east.Footnote` (translated to a `footnoteDefinition`) directly
		// into the parent's children list. The FootnoteList wrapper itself
		// is silent-dropped.
		if list, ok := c.(*east.FootnoteList); ok {
			for fn := list.FirstChild(); fn != nil; fn = fn.NextSibling() {
				if def := translateFootnote(fn, src, pt); def != nil {
					out = append(out, def)
				}
			}
			continue
		}
		n := translateNode(c, src, pt)
		if n == nil {
			// Silent drop per CONTEXT.md "Lossiness policy" — goldmark
			// constructs not in the v1 node set produce no wire output.
			continue
		}
		// Coalesce contiguous `text` siblings into one. Suppressed when the
		// preceding goldmark text had a HardLineBreak, since a synthetic
		// `break` will be appended between them (the offsets in that case are
		// non-contiguous anyway — goldmark drops the two-space / backslash —
		// so the contiguity check below would already fail, but pinning the
		// rule here keeps the intent explicit).
		if len(out) > 0 && n.Type == "text" && out[len(out)-1].Type == "text" {
			prev := out[len(out)-1]
			if prev.Position != nil && n.Position != nil && prev.Position.End.Offset == n.Position.Start.Offset {
				prev.Value += n.Value
				prev.Position.End = n.Position.End
				continue
			}
		}
		out = append(out, n)
		// Hard-line-break post-step: if the goldmark source was a Text with
		// HardLineBreak set, append a synthetic `break` node spanning the
		// gap to the next sibling's start (the consumed two-spaces-and-LF
		// or backslash-and-LF). If there's no next sibling (paragraph ends
		// on the hard break — degenerate), span from text end to text end+1.
		if t, ok := c.(*ast.Text); ok && t.HardLineBreak() {
			breakStart := t.Segment.Stop
			breakEnd := breakStart + 1
			if next := c.NextSibling(); next != nil {
				if nt, ok := next.(*ast.Text); ok {
					breakEnd = nt.Segment.Start
				}
			}
			out = append(out, &Node{
				Type:     "break",
				Position: pt.position(breakStart, breakEnd),
			})
		}
	}
	return out
}

// translateNode dispatches on the goldmark node kind. Returns nil for goldmark
// types that have no v1 mdast mapping (silent drop per CONTEXT.md "Lossiness
// policy").
//
// The recognized set is the switch arms below — that switch is the canonical
// dispatch table, not a duplicated list in this comment. Two emission rules
// do not have a switch arm:
//   - The synthetic `break` node for hard line breaks is emitted by
//     `translateChildren` based on the preceding Text node's HardLineBreak()
//     flag (goldmark stores the hard-break signal on the Text node, not as a
//     sibling).
//   - Emphasis level 1 → mdast `emphasis`; Emphasis level 2 → mdast `strong`.
//     The level discriminator lives inside `translateEmphasis`.
func translateNode(n ast.Node, src []byte, pt *positionTracker) *Node {
	switch v := n.(type) {
	case *ast.Heading:
		return translateHeading(v, src, pt)
	case *ast.Paragraph:
		return translateParagraph(v, src, pt)
	case *ast.Text:
		return translateText(v, src, pt)
	case *ast.Emphasis:
		return translateEmphasis(v, src, pt)
	case *ast.List:
		return translateList(v, src, pt)
	case *ast.ListItem:
		return translateListItem(v, src, pt)
	case *ast.TextBlock:
		// goldmark wraps a tight-list-item's inline content in a TextBlock
		// (a paragraph-like container that does not imply surrounding blank
		// lines). mdast has no TextBlock type; the inline content is wrapped
		// in a plain `paragraph` instead. S05's acceptance criterion #1
		// pins this shape: `- a` produces a listItem whose child is a
		// `paragraph` containing the text — not a listItem whose child is
		// the text directly. translateTextBlock returns a `paragraph` node.
		return translateTextBlock(v, src, pt)
	case *ast.Blockquote:
		return translateBlockquote(v, src, pt)
	case *ast.ThematicBreak:
		return translateThematicBreak(v, src, pt)
	case *ast.FencedCodeBlock:
		return translateFencedCodeBlock(v, src, pt)
	case *ast.CodeBlock:
		return translateCodeBlock(v, src, pt)
	case *ast.HTMLBlock:
		return translateHTMLBlock(v, src, pt)
	case *ast.RawHTML:
		return translateRawHTML(v, src, pt)
	case *ast.CodeSpan:
		return translateCodeSpan(v, src, pt)
	case *ast.Link:
		return translateLink(v, src, pt)
	case *ast.Image:
		return translateImage(v, src, pt)
	case *ast.AutoLink:
		return translateAutoLink(v, src, pt)
	case *ast.LinkReferenceDefinition:
		return translateLinkReferenceDefinition(v, src, pt)
	case *east.Table:
		return translateTable(v, src, pt)
	case *east.TableHeader:
		// A header row is a `tableRow` in mdast — same node type as a data
		// row; the header-vs-data distinction is positional (first child of
		// the table), not type-encoded. So TableHeader translates to the
		// same shape as TableRow.
		return translateTableRow(v, src, pt)
	case *east.TableRow:
		return translateTableRow(v, src, pt)
	case *east.TableCell:
		return translateTableCell(v, src, pt)
	case *east.Strikethrough:
		return translateStrikethrough(v, src, pt)
	case *east.FootnoteLink:
		return translateFootnoteLink(v, pt)
	case *mathjax.InlineMath:
		return translateInlineMath(v, src, pt)
	case *mathjax.MathBlock:
		return translateMath(v, src, pt)
	default:
		return nil
	}
}

// translateMath maps `*mathjax.MathBlock` → mdast
// `math{value, meta: null, position}` per ADR-0004 Decision 4 (1:1 name
// alignment between goldmark-side `*mathjax.MathBlock` and mdast `math`).
//
// `value` is the literal interior bytes between the `$$` fences, with each
// content line's trailing `\n` preserved including the final line's `\n`,
// per CONTEXT.md "Text/Code value preservation" (`code.value` analogy) and
// the `math node` entry. The library's block parser records one
// `text.Segment` per body line in `Lines()`; `Lines().Value(src)`
// concatenates them. Per `probe/goldmark-mathjax/block.go:45-65`, the
// closing-fence branch returns parser.Close BEFORE appending the closing
// fence line to `Lines()`, so the closing `$$` is NOT in `value` —
// mirrors the `code.value` exclusion of the closing fence.
//
// `meta` is always `null` in v1.x (CONTEXT.md `math node` entry: "for
// `$$...$$` it is always `null`"). The field exists in the mdast schema
// as forward-compat for a deferred fenced-math Run (` ```math ... ``` `);
// translate emits a non-pointer-but-flag pattern via leaving Node.Meta as
// nil, which emit's `writeJSONNullableString` serializes as JSON `null`.
//
// Position spans the body's source range — `Lines().At(0).Start`
// through `Lines().Last().Stop`. Mirrors `translateFencedCodeBlock`'s
// use of `blockOffsets`: the body lines drive the span, and the
// opening / closing fence lines lie outside it (CONTEXT.md "Position
// info" treats fenced-block fences as out-of-span on the same precedent;
// translate's fenced-code helper has shipped with this shape since S06).
//
// Closed-vs-unclosed branch (S05, ADR-0004 Decision 5): the library
// emits a MathBlock regardless of whether the closing `$$` was actually
// written, so `translateMath` first asks `displayMathClosed` and routes
// the unclosed case to `demoteUnclosedDisplayMath` (paragraph emit).
// Only the closed case reaches the `math`-node emit below; its position
// math is described by the paragraph above.
func translateMath(m *mathjax.MathBlock, src []byte, pt *positionTracker) *Node {
	lines := m.Lines()
	if !displayMathClosed(lines, src) {
		return demoteUnclosedDisplayMath(m, src, pt)
	}
	value := string(lines.Value(src))
	startOff, endOff := blockOffsets(lines, src)
	return &Node{
		Type:         "math",
		Value:        value,
		ValuePresent: true,
		Position:     pt.position(startOff, endOff),
	}
}

// translateInlineMath maps `*mathjax.InlineMath` → mdast
// `inlineMath{value, position}` per ADR-0004 Decision 4 (1:1 name alignment
// between goldmark-side `*mathjax.InlineMath` and mdast `inlineMath`).
//
// `value` is the literal interior bytes between the `$` delimiters — the
// concatenation of the InlineMath's child `*ast.Text` segments, taken
// verbatim per CONTEXT.md "Text/Code value preservation" and the
// `inlineMath node` entry ("byte-for-byte, delimiters stripped"). The
// library's inline parser stores the interior bytes as a single
// `*ast.RawTextSegment` child whose segment covers the interior of the
// `$...$` range (post any trim-halfspace, see `inline.go:62-82` — for the
// happy-path inputs covered by S02 the trim does not fire because no
// fixture's interior begins AND ends with a space).
//
// Position spans the full source bytes including the opener and closer
// `$` delimiters: we walk leftward from the first child's `Segment.Start`
// across the run of `$` opener bytes, and rightward from the last child's
// `Segment.Stop` across the run of `$` closer bytes. The library does NOT
// expose the opener/closer width on the AST node itself, but the source
// bytes immediately bracketing the children's span are `$` runs by
// construction (per `inline.go:24-52`: the inline parser only fires on
// `$`-runs, so the bytes adjacent to the interior segment MUST be `$`s
// of equal length).
func translateInlineMath(im *mathjax.InlineMath, src []byte, pt *positionTracker) *Node {
	var value string
	for c := im.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		seg := t.Segment
		value += string(src[seg.Start:seg.Stop])
	}
	startOff, endOff := inlineMathDelimitedSpan(im, src)
	return &Node{
		Type:         "inlineMath",
		Value:        value,
		ValuePresent: true,
		Position:     pt.position(startOff, endOff),
	}
}

// translateHeading maps `*ast.Heading{Level}` → `heading{depth, children}`.
// The position spans the source line including the `#` markers: start anchors
// at the beginning of the line containing `lines[0]`; end anchors at
// `lines[last].Stop`.
func translateHeading(h *ast.Heading, src []byte, pt *positionTracker) *Node {
	startOff, endOff := blockOffsets(h.Lines(), src)
	return &Node{
		Type:     "heading",
		Depth:    h.Level,
		Children: translateChildren(h, src, pt),
		Position: pt.position(startOff, endOff),
	}
}

// translateParagraph maps `*ast.Paragraph` → `paragraph{children}`. Position
// spans `lines[0].Start` through `lines[last].Stop`; for paragraphs there is
// no marker to expand past, so the start IS the first content byte.
func translateParagraph(p *ast.Paragraph, src []byte, pt *positionTracker) *Node {
	startOff, endOff := paragraphOffsets(p.Lines())
	return &Node{
		Type:     "paragraph",
		Children: translateChildren(p, src, pt),
		Position: pt.position(startOff, endOff),
	}
}

// translateText maps `*ast.Text` → `text{value}`. The `value` is the literal
// segment bytes as goldmark exposes them (CONTEXT.md "Text/Code value
// preservation": no trimming, no re-escaping).
func translateText(t *ast.Text, src []byte, pt *positionTracker) *Node {
	seg := t.Segment
	return &Node{
		Type:         "text",
		Value:        string(src[seg.Start:seg.Stop]),
		ValuePresent: true,
		Position:     pt.position(seg.Start, seg.Stop),
	}
}

// translateEmphasis maps `*ast.Emphasis{Level: 1}` → `emphasis`,
// `*ast.Emphasis{Level: 2}` → `strong`. mdast's spec: `strong` is a distinct
// node type, not an "emphasis with depth 2".
//
// Position spans the source bytes that produced the emphasis, including the
// `*`/`_` (or `**`/`__`) delimiters on both sides. We derive the span from
// the union of the child segments and extend by `Level` bytes on each side
// to include the delimiters (goldmark uses ASCII single-byte delimiters).
func translateEmphasis(e *ast.Emphasis, src []byte, pt *positionTracker) *Node {
	t := "emphasis"
	if e.Level == 2 {
		t = "strong"
	}
	children := translateChildren(e, src, pt)
	startOff, endOff := spanWithDelimiter(children, e.Level, len(src))
	return &Node{
		Type:     t,
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// translateList maps `*ast.List` → `list{ordered, start, spread, children}`.
//
//   - `ordered` reflects goldmark's `IsOrdered()` (marker == '.' or ')').
//   - `start` is `nil` for unordered lists and `*<n>` for ordered lists, where
//     `<n>` is goldmark's `List.Start` (the explicit start number). The mdast
//     spec serializes this as JSON `null` for unordered, so the pointer type
//     is the load-bearing distinction.
//   - `spread` is the inverse of goldmark's `IsTight` — tight (no blank lines
//     between items) → `spread: false`, loose → `spread: true`. (mdast names
//     the loose case `spread: true`; goldmark names the inverse `IsTight`.)
//
// Position spans the union of the child listItem positions (goldmark does not
// store a per-list segment of its own).
func translateList(l *ast.List, src []byte, pt *positionTracker) *Node {
	children := translateChildren(l, src, pt)
	startOff, endOff := childrenSpan(children)
	var start *int
	ordered := l.IsOrdered()
	if ordered {
		s := l.Start
		start = &s
	}
	return &Node{
		Type:     "list",
		Ordered:  ordered,
		Start:    start,
		Spread:   !l.IsTight,
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// translateListItem maps `*ast.ListItem` → `listItem{spread, checked, children}`.
// `spread` is inherited as the per-item flavor of list-spread; mdast
// distinguishes them but in practice they agree for the v1 fixture set
// (all tight). We leave Spread false and rely on a later slice for the
// loose-list shape.
//
// Task-checkbox hoist (S07): GFM's task-list extension inserts a
// `*east.TaskCheckBox` as the first inline child of the listItem's
// `*ast.TextBlock` (the wrapper for inline content inside a tight list
// item; see goldmark `extension/tasklist.go`). When that child exists,
// translate HOISTS its `IsChecked` boolean onto `listItem.checked` and
// drops the TaskCheckBox from the wire — translateNode already returns
// nil for `*east.TaskCheckBox` (via the default arm: silent drop per
// CONTEXT.md "Lossiness policy"), so the only work here is the hoist.
// Non-task list items continue to carry `Checked: nil`, which emit
// serializes as `"checked":null` (never elided per CONTEXT.md mdast
// node-set v1 `listItem{checked}` and the S05 acceptance contract).
func translateListItem(li *ast.ListItem, src []byte, pt *positionTracker) *Node {
	checked := extractTaskCheckboxChecked(li)
	children := translateChildren(li, src, pt)
	startOff, endOff := childrenSpan(children)
	return &Node{
		Type:     "listItem",
		Spread:   false,
		Checked:  checked, // nil for non-task items; *true/*false for task items
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// extractTaskCheckboxChecked walks the listItem's first-child container
// (goldmark's `*ast.TextBlock` for tight items, `*ast.Paragraph` for loose
// items — both have inline children) and returns the IsChecked bool of the
// FIRST `*east.TaskCheckBox` it finds among the direct inline children of
// that container, wrapped as `*bool`. Returns nil when no TaskCheckBox is
// present (i.e. this is a plain non-task list item) — the caller then
// emits `Checked: nil` which serializes as JSON `null` per the never-elided
// rule.
//
// Only the FIRST direct inline child is examined: the tasklist parser
// (`extension/tasklist.go`) only inserts a TaskCheckBox there, and only
// when the parent container has no other children yet. Recursing or
// scanning further would be wrong (it would catch a stray `[x]`-shaped
// inline elsewhere in the item body, which is not a task checkbox).
func extractTaskCheckboxChecked(li *ast.ListItem) *bool {
	container := li.FirstChild()
	if container == nil {
		return nil
	}
	first := container.FirstChild()
	if first == nil {
		return nil
	}
	cb, ok := first.(*east.TaskCheckBox)
	if !ok {
		return nil
	}
	v := cb.IsChecked
	return &v
}

// translateTextBlock maps goldmark's `*ast.TextBlock` to mdast `paragraph`.
// goldmark uses TextBlock as the inline-content container inside a tight
// listItem (where a Paragraph would imply surrounding blank lines); mdast has
// no such distinction. The two pour through the same downstream shape.
func translateTextBlock(tb *ast.TextBlock, src []byte, pt *positionTracker) *Node {
	startOff, endOff := paragraphOffsets(tb.Lines())
	return &Node{
		Type:     "paragraph",
		Children: translateChildren(tb, src, pt),
		Position: pt.position(startOff, endOff),
	}
}

// translateBlockquote maps `*ast.Blockquote` → `blockquote{children}`.
// goldmark does not expose a per-blockquote segment; the position is derived
// from the union of children spans. The `>` marker is therefore NOT included
// in the position — mdast convention varies here, and the existing fixtures
// at S05 all use --no-position so the question is moot. Revisit in S10 (the
// position-info pinning slice) if a default-mode fixture needs to assert the
// marker-inclusion contract.
func translateBlockquote(bq *ast.Blockquote, src []byte, pt *positionTracker) *Node {
	children := translateChildren(bq, src, pt)
	startOff, endOff := childrenSpan(children)
	return &Node{
		Type:     "blockquote",
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// translateThematicBreak maps `*ast.ThematicBreak` → `thematicBreak` (a leaf).
// goldmark does not populate `Lines()` for ThematicBreak (it's a marker-only
// block), but `BaseBlock.Pos()` returns the byte offset of the first dash of
// the `---` / `***` / `___` marker. S10 derives the end offset by scanning
// forward from that start to the next `\n` (or end of source), so the
// position spans the entire marker line.
func translateThematicBreak(tb *ast.ThematicBreak, src []byte, pt *positionTracker) *Node {
	start := tb.Pos()
	end := start
	for end < len(src) && src[end] != '\n' {
		end++
	}
	return &Node{
		Type:     "thematicBreak",
		Position: pt.position(start, end),
	}
}

// translateFencedCodeBlock maps `*ast.FencedCodeBlock` → `code{lang, meta, value}`.
//
// The info string convention (CommonMark §4.5): the first word (delimited by a
// space) of the info text is the language; the remainder (with one consumed
// separating space) is the meta string. Empty info string → `lang: nil, meta:
// nil` per CONTEXT.md "mdast node-set v1" (a fenced block with no info string
// has both fields null, same as an indented block; the *distinction* between
// fenced-no-info and indented is not on the wire — both serialize identically).
//
// `value` is `Lines().Value(src)` — the concatenation of the per-line segments
// goldmark records, which per CommonMark and CONTEXT.md "Text/Code value
// preservation" preserves every content line's trailing `\n` including the
// final line's `\n` and excludes the closing fence.
func translateFencedCodeBlock(f *ast.FencedCodeBlock, src []byte, pt *positionTracker) *Node {
	startOff, endOff := blockOffsets(f.Lines(), src)
	var lang *string
	var meta *string
	if f.Info != nil {
		info := string(f.Info.Segment.Value(src))
		// Split at the first space: prefix is lang, remainder (after the
		// single separating space) is meta. CommonMark info-string rule.
		spaceIdx := -1
		for i := 0; i < len(info); i++ {
			if info[i] == ' ' {
				spaceIdx = i
				break
			}
		}
		if spaceIdx == -1 {
			if info != "" {
				l := info
				lang = &l
			}
		} else {
			l := info[:spaceIdx]
			if l != "" {
				lang = &l
			}
			m := info[spaceIdx+1:]
			if m != "" {
				meta = &m
			}
		}
	}
	value := string(f.Lines().Value(src))
	return &Node{
		Type:         "code",
		Lang:         lang,
		Meta:         meta,
		Value:        value,
		ValuePresent: true,
		Position:     pt.position(startOff, endOff),
	}
}

// translateCodeBlock maps `*ast.CodeBlock` (the indented-code-block kind) →
// `code{lang: nil, meta: nil, value}`. Per CONTEXT.md "mdast node-set v1" and
// "Text/Code value preservation": indented code carries both lang and meta as
// `null` (never elided); `value` is the dedented content of the lines, which
// `Lines().Value(src)` already produces (goldmark stores per-line segments
// that start at the column-5 byte of each indented line).
func translateCodeBlock(c *ast.CodeBlock, src []byte, pt *positionTracker) *Node {
	startOff, endOff := blockOffsets(c.Lines(), src)
	return &Node{
		Type:         "code",
		Lang:         nil,
		Meta:         nil,
		Value:        string(c.Lines().Value(src)),
		ValuePresent: true,
		Position:     pt.position(startOff, endOff),
	}
}

// translateHTMLBlock maps `*ast.HTMLBlock` → `html{value}`. mdast does not
// distinguish block from inline raw HTML — both flow through the same `html`
// node type per CONTEXT.md "mdast node-set v1". `value` is the literal lines
// of the block, including the closure line if present (`<div>...</div>` on
// one line has no closure; multi-line forms with a separate `</div>` have one).
// Per CONTEXT.md "Text/Code value preservation": no entity expansion, no
// tag-name lowercasing, no whitespace normalization — bytes flow through.
func translateHTMLBlock(h *ast.HTMLBlock, src []byte, pt *positionTracker) *Node {
	startOff, endOff := blockOffsets(h.Lines(), src)
	value := string(h.Lines().Value(src))
	if h.HasClosure() {
		value += string(h.ClosureLine.Value(src))
		// Extend the position to include the closure line.
		if h.ClosureLine.Stop > endOff {
			endOff = h.ClosureLine.Stop
		}
	}
	return &Node{
		Type:         "html",
		Value:        value,
		ValuePresent: true,
		Position:     pt.position(startOff, endOff),
	}
}

// translateRawHTML maps `*ast.RawHTML` (inline raw HTML) → `html{value}`. Same
// mdast type as block HTML — the position context (block vs paragraph child)
// is what distinguishes them in the tree, not a separate node type. Goldmark
// stores the raw text across one or more Segments; concatenate them to get
// the wire value.
func translateRawHTML(r *ast.RawHTML, src []byte, pt *positionTracker) *Node {
	var value string
	startOff, endOff := 0, 0
	if r.Segments != nil && r.Segments.Len() > 0 {
		startOff = r.Segments.At(0).Start
		endOff = r.Segments.At(r.Segments.Len() - 1).Stop
		value = string(r.Segments.Value(src))
	}
	return &Node{
		Type:         "html",
		Value:        value,
		ValuePresent: true,
		Position:     pt.position(startOff, endOff),
	}
}

// translateCodeSpan maps `*ast.CodeSpan` → `inlineCode{value}`. Goldmark's
// CodeSpan contains `*ast.Text` children whose segments are the literal
// content between the backticks (CommonMark's one-space-trim rule already
// applied by goldmark per CONTEXT.md "Text/Code value preservation"). The
// translate side concatenates the child segments verbatim.
func translateCodeSpan(c *ast.CodeSpan, src []byte, pt *positionTracker) *Node {
	var value string
	for child := c.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			seg := t.Segment
			value += string(src[seg.Start:seg.Stop])
		}
	}
	startOff, endOff := textChildrenSpan(c)
	return &Node{
		Type:         "inlineCode",
		Value:        value,
		ValuePresent: true,
		Position:     pt.position(startOff, endOff),
	}
}

// translateLink maps `*ast.Link` → `link{url, title, children}` for inline
// links, OR `linkReference{identifier, label, referenceType, children}` for
// reference-style links. The discriminator is goldmark's `Link.Reference`
// field: nil → inline, non-nil → reference-style (CONTEXT.md mdast node-set
// v1 "reference-style links/images are preserved, not flattened to inline
// `link`/`image`"). `url` is always a string (mdast `link.url: string`);
// `title` is `*string` so the no-title case serializes as `title: null`
// (mdast convention).
func translateLink(l *ast.Link, src []byte, pt *positionTracker) *Node {
	children := translateChildren(l, src, pt)
	startOff, endOff := childrenSpan(children)
	if l.Reference != nil {
		return &Node{
			Type:          "linkReference",
			Identifier:    identifierFromLabel(l.Reference.Value),
			Label:         string(l.Reference.Value),
			ReferenceType: referenceTypeToMdast(l.Reference.Type),
			Children:      children,
			Position:      pt.position(startOff, endOff),
		}
	}
	return &Node{
		Type:     "link",
		URL:      string(l.Destination),
		Title:    nullableString(l.Title),
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// translateLinkReferenceDefinition maps `*ast.LinkReferenceDefinition` →
// `definition{identifier, label, url, title}`. goldmark retains the
// definition as a sibling block via `linkReferenceParagraphTransformer`
// (see `parser/link_ref.go`); translate emits one mdast `definition` per
// `LinkReferenceDefinition`. The identifier is the normalized label
// (lower-cased, whitespace-collapsed per CommonMark §4.7); the label
// preserves the raw label text.
func translateLinkReferenceDefinition(d *ast.LinkReferenceDefinition, src []byte, pt *positionTracker) *Node {
	startOff, endOff := blockOffsets(d.Lines(), src)
	return &Node{
		Type:       "definition",
		Identifier: identifierFromLabel(d.Label),
		Label:      string(d.Label),
		URL:        string(d.Destination),
		Title:      nullableString(d.Title),
		Position:   pt.position(startOff, endOff),
	}
}

// identifierFromLabel normalizes a raw reference label into its mdast
// `identifier` form: lower-cased, leading/trailing whitespace trimmed,
// inner whitespace collapsed to a single space. CommonMark §4.7 calls
// this "matching link references"; goldmark's `util.ToLinkReference`
// applies exactly that normalization, so we delegate.
func identifierFromLabel(label []byte) string {
	return util.ToLinkReference(label)
}

// referenceTypeToMdast converts goldmark's `ReferenceLinkType` enum to the
// mdast wire string (`"full"`, `"collapsed"`, `"shortcut"`). Anything
// unrecognized maps to `"full"` — the parser only produces the three
// canonical values, so the fallback is defensive only.
func referenceTypeToMdast(t ast.ReferenceLinkType) string {
	switch t {
	case ast.ReferenceLinkFull:
		return "full"
	case ast.ReferenceLinkCollapsed:
		return "collapsed"
	case ast.ReferenceLinkShortcut:
		return "shortcut"
	default:
		return "full"
	}
}

// translateImage maps `*ast.Image` → `image{url, title, alt}` for inline
// images, OR `imageReference{identifier, label, referenceType, alt}` for
// reference-style images. The discriminator is goldmark's `Image.Reference`
// field: nil → inline, non-nil → reference-style — same rule
// `translateLink` applies to its `*ast.Link` (both share `baseLink`).
// Per CONTEXT.md mdast node-set v1, `image.alt` is a flat string (no nested
// children). When the source markdown has inline structure inside the alt
// (e.g. `![an *emph* alt](url)`), `flattenAltText` walks the inline
// children and concatenates their textual content, silently dropping
// non-text inline structure. The same `alt` flattening rule applies to
// `imageReference` — its `alt` is also a flat string per mdast convention.
func translateImage(i *ast.Image, src []byte, pt *positionTracker) *Node {
	startOff, endOff := textChildrenSpan(i)
	if i.Reference != nil {
		return &Node{
			Type:          "imageReference",
			Identifier:    identifierFromLabel(i.Reference.Value),
			Label:         string(i.Reference.Value),
			ReferenceType: referenceTypeToMdast(i.Reference.Type),
			Alt:           flattenAltText(i, src),
			Position:      pt.position(startOff, endOff),
		}
	}
	return &Node{
		Type:     "image",
		URL:      string(i.Destination),
		Title:    nullableString(i.Title),
		Alt:      flattenAltText(i, src),
		Position: pt.position(startOff, endOff),
	}
}

// translateTable maps `*east.Table` → `table{align, children}`. The mdast
// contract pins alignment as a per-column property on `table` (not per-cell),
// with each slot being `"left" | "right" | "center" | null` (CONTEXT.md
// mdast node-set v1 `table{align}`). goldmark stores the same data on the
// `*east.Table.Alignments` slice as `east.Alignment` enum values — translate
// converts the per-column enum (`AlignLeft`/`AlignRight`/`AlignCenter`/
// `AlignNone`) into the `[]*string` shape: nil slot for `AlignNone`,
// `*string` for the three named alignments. The string values match
// goldmark's `Alignment.String()` for the three non-`None` cases
// ("left"/"right"/"center"), but we DON'T forward the literal "none" string
// for `AlignNone` — mdast's convention is JSON `null` there, not the string
// `"none"`.
//
// Position spans the union of children spans; goldmark's Table does not
// expose its own segment.
func translateTable(t *east.Table, src []byte, pt *positionTracker) *Node {
	align := alignmentsToMdast(t.Alignments)
	children := translateChildren(t, src, pt)
	startOff, endOff := childrenSpan(children)
	return &Node{
		Type:     "table",
		Align:    align,
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// translateTableRow maps `*east.TableRow` (or `*east.TableHeader`, which has
// the same shape) → `tableRow{children}`. mdast does not distinguish header
// from data rows by node type; the first row of a `table` is the header by
// convention. The TableRow's children are the `*east.TableCell`s.
func translateTableRow(row ast.Node, src []byte, pt *positionTracker) *Node {
	children := translateChildren(row, src, pt)
	startOff, endOff := childrenSpan(children)
	return &Node{
		Type:     "tableRow",
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// translateTableCell maps `*east.TableCell` → `tableCell{children}`.
//
// Per CONTEXT.md mdast node-set v1 `table{align}` rule: `tableCell` carries
// NO `align` field on the wire — alignment is a per-column property of
// `table`, not per-cell. goldmark's `*east.TableCell.Alignment` is therefore
// intentionally dropped here (the same information is already captured on
// the parent `table.align[colIndex]`).
//
// The cell's inline content lives in `Lines()` at the goldmark layer; the
// block→inline pass populates the cell's child list with the usual inline
// node kinds (`*ast.Text`, `*ast.Emphasis`, etc.). `translateChildren` walks
// those as for any other inline container.
func translateTableCell(cell *east.TableCell, src []byte, pt *positionTracker) *Node {
	children := translateChildren(cell, src, pt)
	startOff, endOff := childrenSpan(children)
	return &Node{
		Type:     "tableCell",
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// translateAutoLink maps `*ast.AutoLink` → mdast `link{url, title:null,
// children:[text(value:URL)]}`. The goldmark `AutoLink` type does NOT
// appear on the wire per CONTEXT.md mdast node-set v1 autolink rule:
// "autolinks collapse to mdast `link{url, title:null}`… goldmark's
// distinct `AutoLink` type is an implementation detail not exposed on the
// wire". The same translation handles both shapes goldmark produces for
// autolinks:
//   - `<https://…>` — CommonMark's angle-bracket autolink, parsed by
//     goldmark's core autoLinkParser. `URL(src)` returns the literal URL.
//     Goldmark's inner `*Text` segment covers the URL but NOT the
//     surrounding `<` / `>`. Translate extends the position by 1 byte on
//     each side when those delimiters are present in the source.
//   - bare `https://…` — GFM linkify, parsed by extension/linkify. The
//     inner `*Text` segment covers the literal URL bytes; no surrounding
//     delimiters in the source.
//
// Position: goldmark's `AutoLink` is a `BaseInline` whose inner
// `value *Text` (carrying the URL segment) is unexported. We can't pull
// the segment directly. Pragmatic recovery: use `Label(src)` to get the
// literal URL bytes (which match the inner segment, not the protocol-
// prefixed URL), then locate that substring in the source via the
// translate-level cursor on `positionTracker` (`inlineSearchCursor`)
// which is advanced after each successful match. The cursor is
// per-Translate state, not document-wide, so it cannot drift across
// invocations. For the angle-bracket flavor we check the byte immediately
// before the match for `<` and extend the span by 1 on each side if
// present (mdast convention: position spans the full source syntax,
// including delimiters).
//
// The synthetic child `text` node carries the URL string as its `value`
// (mdast convention: the autolink's display text IS the URL itself,
// stored as a `text` child of the `link`); its position matches the
// inner URL segment (without the angle-bracket delimiters), mirroring
// how a regular `[text](url)` link's inner `text` child carries its own
// inline-text segment, NOT the surrounding bracket+url syntax.
func translateAutoLink(a *ast.AutoLink, src []byte, pt *positionTracker) *Node {
	url := string(a.URL(src))
	// Label(src) returns the inner URL bytes WITHOUT the linkify protocol
	// prefix — i.e. the literal source bytes goldmark stored in the inner
	// `*Text` segment. That's exactly what we need to locate the segment
	// in the source.
	label := string(a.Label(src))
	innerStart, innerEnd := pt.findInline([]byte(label))
	// Angle-bracket flavor: the source has `<` immediately before and `>`
	// immediately after the inner URL segment. Extend the outer position
	// by 1 byte on each side so it covers the full `<URL>` syntax. We
	// gate on the actual source bytes rather than `a.AutoLinkType` because
	// the type field encodes URL-vs-email, not angle-bracket-vs-linkify.
	outerStart, outerEnd := innerStart, innerEnd
	if innerStart > 0 && src[innerStart-1] == '<' && innerEnd < len(src) && src[innerEnd] == '>' {
		outerStart--
		outerEnd++
	}
	child := &Node{
		Type:         "text",
		Value:        url,
		ValuePresent: true,
		Position:     pt.position(innerStart, innerEnd),
	}
	return &Node{
		Type:     "link",
		URL:      url,
		Title:    nil, // mdast convention: autolinks carry `title: null`
		Children: []*Node{child},
		Position: pt.position(outerStart, outerEnd),
	}
}

// translateStrikethrough maps `*east.Strikethrough` → `delete{children}`.
// The mdast type name `delete` matches CONTEXT.md mdast node-set v1
// "delete (GFM strikethrough)" — it is NOT named `strikethrough` on the
// wire (mdast follows the HTML element name `<del>`).
//
// Position spans the `~~`/`~~` delimiters on both sides. Strikethrough's
// delimiter is two characters per side (`~~`), so we extend the children-
// span by 2 bytes on each side, mirroring `translateEmphasis`'s
// delimiter-extension trick. goldmark's `BaseInline` does not carry a
// segment for the Strikethrough container itself, so the children-span +
// delimiter-extension approach is how we recover the source span.
func translateStrikethrough(s *east.Strikethrough, src []byte, pt *positionTracker) *Node {
	children := translateChildren(s, src, pt)
	const delim = 2 // length of `~~`
	startOff, endOff := spanWithDelimiter(children, delim, len(src))
	return &Node{
		Type:     "delete",
		Children: children,
		Position: pt.position(startOff, endOff),
	}
}

// translateFootnote maps a `*east.Footnote` (a single definition inside the
// FootnoteList container) → `footnoteDefinition{identifier, label, children}`.
// The identifier and label are both the source `Ref` bytes (e.g. "a"); they
// are stored identically per the mdast convention for footnotes (no
// case-folding distinction in the wild — the footnote label is the
// identifier, character-for-character). The children are translated as a
// normal block container; any presentation-only `*east.FootnoteBacklink`
// goldmark's transformer appended is silent-dropped by translateNode's
// default arm.
func translateFootnote(n ast.Node, src []byte, pt *positionTracker) *Node {
	fn, ok := n.(*east.Footnote)
	if !ok {
		return nil
	}
	label := string(fn.Ref)
	children := translateChildren(fn, src, pt)
	startOff, endOff := childrenSpan(children)
	return &Node{
		Type:       "footnoteDefinition",
		Identifier: label,
		Label:      label,
		Children:   children,
		Position:   pt.position(startOff, endOff),
	}
}

// translateFootnoteLink maps `*east.FootnoteLink` → `footnoteReference{
// identifier, label}`. The goldmark inline node only carries the 1-based
// `Index`; the source label (e.g. "a") lives on the corresponding
// `*east.Footnote` in the FootnoteList, harvested into
// `pt.footnoteLabels` by Translate's pre-pass. If the index is missing
// from the map (degenerate case — would only happen if the parser emitted
// a FootnoteLink without a matching definition, which goldmark's parser
// does not), the reference silent-drops by returning nil.
//
// Position: `*east.FootnoteLink` is a `BaseInline` carrying ONLY an
// integer `Index` — no segment, no source bytes. To recover its source
// position we locate the literal `[^label]` syntax in the source via
// `pt.findInline`, which advances a translate-level cursor in source
// order. The same per-Translate cursor mechanism powers
// `translateAutoLink`'s position recovery; see `position.go`'s
// `inlineSearchCursor` comment for the in-source-order ordering
// guarantee.
func translateFootnoteLink(f *east.FootnoteLink, pt *positionTracker) *Node {
	label, ok := pt.footnoteLabels[f.Index]
	if !ok {
		return nil
	}
	needle := []byte("[^" + label + "]")
	start, end := pt.findInline(needle)
	return &Node{
		Type:       "footnoteReference",
		Identifier: label,
		Label:      label,
		Position:   pt.position(start, end),
	}
}

// alignmentsToMdast converts goldmark's `[]east.Alignment` (the per-column
// alignment enum) into mdast's `[]*string` shape:
//   - AlignLeft   → *"left"
//   - AlignRight  → *"right"
//   - AlignCenter → *"center"
//   - AlignNone   → nil  (mdast convention: JSON null, NOT the string "none")
//
// Returns a non-nil empty slice when goldmark records zero alignments —
// matches the rest of translate's "empty slice means empty, not nil"
// convention so emit's bracketing helper writes `"align":[]` not
// `"align":null` (the no-column-alignments case is degenerate; v1 fixtures
// all have at least one column).
func alignmentsToMdast(aligns []east.Alignment) []*string {
	out := make([]*string, 0, len(aligns))
	for _, a := range aligns {
		switch a {
		case east.AlignLeft:
			s := "left"
			out = append(out, &s)
		case east.AlignRight:
			s := "right"
			out = append(out, &s)
		case east.AlignCenter:
			s := "center"
			out = append(out, &s)
		default:
			// east.AlignNone (or any unrecognized future value) → nil slot.
			out = append(out, nil)
		}
	}
	return out
}

// nullableString converts a goldmark `[]byte` field (typical of `Title`,
// `Destination`, etc.) into a `*string` suitable for an mdast nullable
// field. Empty (zero-length) bytes → nil (serializes as JSON `null`);
// non-empty → `*string`. This concentrates the "empty `[]byte` is the
// no-value sentinel" goldmark convention into one named place for the
// translate layer; S06 currently uses it for `link.Title` / `image.Title`,
// S08 will reuse it for `definition.Title` / `linkReference.Label`.
func nullableString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}

// flattenAltText walks an inline goldmark subtree and concatenates the
// textual content of every `*ast.Text` descendant it finds. Non-text inline
// structure (emphasis, strong, inlineCode, link, etc.) is traversed but its
// container marker is dropped — only the leaf text content survives. This
// is the mdast `image.alt: string` contract: the alt is a flat string, not
// a child-tree.
//
// Delegates the recursion to `walkTextSegments` (the named seam for "walk
// every *ast.Text descendant of an inline subtree, recursing into inline
// containers"); this body is the visitor that decides what to accumulate
// at each leaf. `textChildrenSpan` is the sibling adapter that accumulates
// a min/max byte-offset span instead of a string.
func flattenAltText(n ast.Node, src []byte) string {
	var out string
	walkTextSegments(n, func(seg textm.Segment) {
		out += string(seg.Value(src))
	})
	return out
}

// textChildrenSpan returns the min/max byte-offset range across the
// `*ast.Text` DESCENDANTS of a goldmark parent — direct Text children
// AND inner Text children of inline containers like `*ast.Emphasis`,
// `*ast.Link`, `*east.Strikethrough`. The recursion is necessary
// because goldmark wraps inline content in container nodes (an image's
// alt `![*emph*](url)` puts the only text inside an Emphasis with no
// direct Text sibling); a non-recursive walk would return (0, 0) for
// such cases, collapsing the image / code-span position to the
// placeholder span. S06 arch-log C8 flagged this as a deferred bug;
// S10 fixes by recursing.
//
// Returns (0, 0) when the parent has no `*ast.Text` descendant at all —
// matches the (0, 0)-on-empty convention used by `childrenSpan`,
// `blockOffsets`, and `paragraphOffsets`.
//
// Sibling to `childrenSpan` below: that one walks already-translated mdast
// children; this one walks goldmark inline descendants. The pair names the
// two "position span from descendants" flavors translate needs.
func textChildrenSpan(parent ast.Node) (start, end int) {
	start, end = -1, -1
	walkTextSegments(parent, func(seg textm.Segment) {
		if start == -1 || seg.Start < start {
			start = seg.Start
		}
		if end == -1 || seg.Stop > end {
			end = seg.Stop
		}
	})
	if start == -1 {
		return 0, 0
	}
	return start, end
}

// walkTextSegments invokes fn for every `*ast.Text` descendant of
// `parent`. Recurses into inline containers; does NOT cross into block
// children (the function is only ever called from inline-position helpers
// — `translateCodeSpan` and `translateImage` — whose goldmark subtree is
// inline-only by construction).
func walkTextSegments(parent ast.Node, fn func(seg textm.Segment)) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			fn(t.Segment)
			continue
		}
		// Recurse into inline containers (emphasis, strong, delete,
		// codespan, link, etc.) so their inner text segments contribute
		// to the span. The recursion is bounded by the inline-subtree
		// shape — there are no cycles in a goldmark inline subtree.
		walkTextSegments(c, fn)
	}
}

// spanWithDelimiter is the "paired-delimiter inline" position rule: take
// the union span of an inline container's already-translated mdast children
// and extend it by `delim` bytes on each side to include the open/close
// delimiter runs that goldmark consumed but doesn't expose as children.
// The result is clamped to `[0, srcLen]` so a child-less or zero-positioned
// subtree cannot produce negative or out-of-source offsets.
//
// Concentrates the "expand children-span by N, clamped" arithmetic used by
// inline nodes whose goldmark form does NOT carry a segment for the
// container itself (only for its children). Today's two callers:
//   - `translateEmphasis` — delim is `e.Level` (1 for `*foo*`, 2 for `**foo**`).
//   - `translateStrikethrough` — delim is 2 (`~~foo~~`).
//
// Both `*ast.Emphasis` and `*east.Strikethrough` are `BaseInline`s with no
// segment accessor of their own; this helper is the named seam where their
// position math lives.
func spanWithDelimiter(children []*Node, delim, srcLen int) (start, end int) {
	start, end = childrenSpan(children)
	start -= delim
	end += delim
	if start < 0 {
		start = 0
	}
	if end > srcLen {
		end = srcLen
	}
	return start, end
}

// childrenSpan returns the minimum start offset and maximum end offset across
// the children's positions.
func childrenSpan(children []*Node) (start, end int) {
	if len(children) == 0 {
		return 0, 0
	}
	start, end = -1, -1
	for _, c := range children {
		if c.Position == nil {
			continue
		}
		if start == -1 || c.Position.Start.Offset < start {
			start = c.Position.Start.Offset
		}
		if end == -1 || c.Position.End.Offset > end {
			end = c.Position.End.Offset
		}
	}
	if start == -1 {
		return 0, 0
	}
	return start, end
}

// blockOffsets returns the byte-offset range a heading-style block spans in
// source. The start is anchored at the beginning of the line containing the
// first content segment (so the `#` markers of an ATX heading are included);
// the end is the stop offset of the last content segment.
func blockOffsets(lines *textm.Segments, src []byte) (start, end int) {
	if lines == nil || lines.Len() == 0 {
		return 0, 0
	}
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)
	start = lineStartOffset(src, first.Start)
	end = last.Stop
	return start, end
}

// paragraphOffsets returns the byte-offset range a paragraph spans, which is
// `lines[0].Start` through `lines[last].Stop`. Paragraphs have no implicit
// marker so the start is the first content byte.
func paragraphOffsets(lines *textm.Segments) (start, end int) {
	if lines == nil || lines.Len() == 0 {
		return 0, 0
	}
	return lines.At(0).Start, lines.At(lines.Len() - 1).Stop
}

// lineStartOffset returns the byte offset of the start of the line containing
// `offset`. Walks back to the byte after the previous `\n`, or 0 if none.
func lineStartOffset(src []byte, offset int) int {
	if offset <= 0 || offset > len(src) {
		return 0
	}
	i := offset
	for i > 0 && src[i-1] != '\n' {
		i--
	}
	return i
}

// rootPosition computes the source-range position for the root node by
// translating the source-byte endpoints (0, len(src)) through the
// positionTracker. This keeps the "line is 1-indexed, column counts UTF-8
// code points" rule (CONTEXT.md "Position info") in exactly one place —
// positionTracker.point — rather than duplicating the walk here.
//
// Empty document baseline: pt.point(0) on an empty source returns
// {1,1,0}, so start == end == {1,1,0} (zero-width position).
// Single-newline boundary case (user story 27): pt.point(1) on "\n" returns
// {2,1,1}, so end becomes {line:2, column:1, offset:1}.
func rootPosition(pt *positionTracker) *Position {
	return pt.position(0, len(pt.src))
}
