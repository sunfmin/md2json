package translate

import "bytes"

// positionTracker converts byte offsets in the normalized source into
// (line, column) Points lazily. CONTEXT.md "Position info" pins the rule:
// line and column are 1-indexed; column counts UTF-8 code points within the
// (normalized) line; offset is a byte offset into the normalized
// (post-BOM-strip, LF-only) document.
//
// Implementation: precompute a sorted slice of byte offsets where each line
// begins (offset 0 is line 1's start by definition; each `\n` byte marks the
// next line's start at the byte AFTER it). Then `position(start, end)` does a
// binary search to find the line and walks the line to count columns. This
// keeps the hot path linear in the line length, not the document length.
type positionTracker struct {
	src        []byte
	lineStarts []int // lineStarts[i] = byte offset of the start of line (i+1)
	// footnoteLabels maps goldmark's 1-based `Footnote.Index` to the original
	// `Footnote.Ref` bytes the source markdown used as the footnote label
	// (e.g. "a" for `[^a]`). Translate populates this in a single document-
	// level pre-pass before walking children, so the inline-side
	// `*east.FootnoteLink` (which only carries Index, not the source label
	// bytes) can recover the mdast `footnoteReference.identifier`/`label`
	// from its Index. Empty map when the document has no footnotes.
	//
	// Stashing this on positionTracker (rather than a separate "translate
	// state" struct) keeps the existing threading shape — positionTracker
	// is already passed through every translate call — and avoids a churn
	// in every function signature.
	footnoteLabels map[int]string

	// inlineSearchCursor is the byte offset from which `findInline` starts
	// looking for its next match. Some goldmark inline nodes (AutoLink,
	// FootnoteLink) do NOT expose their source segment on the public API —
	// AutoLink's inner `value *Text` is unexported, and FootnoteLink carries
	// only a numeric `Index`. To recover their source positions, translate
	// locates their literal source syntax (URL bytes / `[^label]` bytes) via
	// `bytes.Index(src[cursor:], needle)` and advances `inlineSearchCursor`
	// past each match. The cursor is per-Translate state (re-initialized to
	// 0 in every `Translate` call) and follows the AST walk order
	// (depth-first, in-source order) so successive matches monotonically
	// advance through the source. This is the same property goldmark itself
	// relies on for inline parsing: inline nodes are emitted in source order
	// within their parent block.
	inlineSearchCursor int
}

func newPositionTracker(src []byte) *positionTracker {
	starts := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &positionTracker{src: src, lineStarts: starts, footnoteLabels: map[int]string{}}
}

// findInline locates the next occurrence of `needle` in `src` starting at
// `inlineSearchCursor`, advances the cursor to one past the match end, and
// returns the match's [start, end) byte range. When the needle is not
// found at or after the cursor (degenerate case — would only happen if
// the caller passed bytes goldmark never emitted), returns (cursor,
// cursor) without advancing, so the caller gets a zero-width position at
// the cursor location rather than `(0, 0)` placeholder bytes.
//
// Empty needle is treated as a no-op: returns (cursor, cursor) without
// advancing. This is defensive — callers should never pass empty needles
// (an autolink has at least one URL byte; a footnote label is at least
// one character per goldmark's parser) but the guard makes the helper
// robust against a future refactor.
func (pt *positionTracker) findInline(needle []byte) (start, end int) {
	if len(needle) == 0 {
		return pt.inlineSearchCursor, pt.inlineSearchCursor
	}
	idx := bytes.Index(pt.src[pt.inlineSearchCursor:], needle)
	if idx < 0 {
		return pt.inlineSearchCursor, pt.inlineSearchCursor
	}
	start = pt.inlineSearchCursor + idx
	end = start + len(needle)
	pt.inlineSearchCursor = end
	return start, end
}

// position returns a Position spanning byte offsets [startOff, endOff). Both
// endpoints are translated to (line, column, offset) Points.
func (pt *positionTracker) position(startOff, endOff int) *Position {
	return &Position{
		Start: pt.point(startOff),
		End:   pt.point(endOff),
	}
}

// point returns the (line, column, offset) Point for a byte offset into the
// source. `column` counts UTF-8 code points (NOT bytes) since the start of
// the line. Offsets past the end of source clamp to the last position.
func (pt *positionTracker) point(offset int) Point {
	if offset < 0 {
		offset = 0
	}
	if offset > len(pt.src) {
		offset = len(pt.src)
	}
	// Find the line: largest i such that lineStarts[i] <= offset.
	line := lastIndexLE(pt.lineStarts, offset)
	lineStart := pt.lineStarts[line]
	col := 1
	for i := lineStart; i < offset; i++ {
		b := pt.src[i]
		// Skip UTF-8 continuation bytes; they don't advance the column.
		if b&0xC0 == 0x80 {
			continue
		}
		col++
	}
	return Point{
		Line:   line + 1, // 1-indexed
		Column: col,
		Offset: offset,
	}
}

// lastIndexLE returns the largest index `i` with `xs[i] <= target`. Assumes
// xs is sorted ascending and non-empty.
func lastIndexLE(xs []int, target int) int {
	lo, hi := 0, len(xs)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if xs[mid] <= target {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}
