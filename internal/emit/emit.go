// Package emit JSON-encodes the md2json output envelope. The wire shape is
// `{"frontmatter": <object|scalar|null>, "ast": <mdast root node>}` in default
// mode; under `--frontmatter-only` the envelope is bypassed and just the
// frontmatter value (or `null`) is emitted at the top level.
//
// Key ordering per CONTEXT.md "JSON envelope" and the PRD's pretty-print
// compose rule:
//   1. `type` first
//   2. type-specific fields in their declared order (e.g. `depth` on heading,
//      `value` on text/code, `ordered`/`start`/`spread` on list) — not exercised
//      at S03 since only `root` is emitted
//   3. `children`
//   4. `position` if present (dropped uniformly under --no-position)
//
// Default mode is compact (single-line, no trailing newline). `--pretty` is
// owned by a later slice; this module only ships the compact path at S03.
//
// `--frontmatter-only` follows the "scalar passthrough" rule (user story 28,
// PRD): when the frontmatter is a YAML scalar, emit the JSON equivalent at
// top level, not wrapped in an object. For the empty-doc baseline (S03's
// concern) the scalar is `nil` and the output is the literal `null`.
package emit

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	"github.com/sunfmin/md2json/internal/translate"
)

// Options drives the emit stage. At S03 we need NoPosition (drop the
// `position` key on every emitted node) and FrontmatterOnly (skip the
// envelope and emit just the frontmatter value). S10a adds Pretty: when
// true, the same compact byte stream is re-formatted with `json.Indent`
// using 2-space indentation, per CONTEXT.md "JSON envelope" rule
// (*"Compact (single-line) by default; `--pretty` switches to 2-space-
// indented form"*). Composing pretty as a post-processing step over the
// existing compact writer guarantees that pretty and compact paths walk
// the value tree IDENTICALLY — pretty is just compact + whitespace, so
// the key ordering established by writeNode's per-type switch (S05–S09)
// holds in both modes by construction. Pretty mode keeps the never-elide
// null-field invariant because the underlying compact stream already
// preserves explicit nulls (S05/S06's `*bool`/`*string` pointer fields).
type Options struct {
	NoPosition      bool
	FrontmatterOnly bool
	Pretty          bool
}

// Emit writes the envelope to w. `frontmatter` is the lifted frontmatter
// value (any JSON-able Go value, including nil → JSON `null`); `root` is the
// mdast root node (or nil if FrontmatterOnly short-circuited translate).
//
// On --frontmatter-only, only the frontmatter value is emitted at the top
// level (scalar passthrough rule). On default, the envelope is written compact
// with mdast-convention key ordering on every emitted node.
//
// No trailing newline is written (matches the v1 ship criterion's byte-exact
// "no trailing newline" stdout assertion).
func Emit(w io.Writer, frontmatter any, root *translate.Node, opts Options) error {
	var buf bytes.Buffer
	if opts.FrontmatterOnly {
		if err := writeJSONValue(&buf, frontmatter); err != nil {
			return err
		}
	} else {
		buf.WriteString(`{"frontmatter":`)
		if err := writeJSONValue(&buf, frontmatter); err != nil {
			return err
		}
		buf.WriteString(`,"ast":`)
		writeNode(&buf, root, opts)
		buf.WriteByte('}')
	}
	if opts.Pretty {
		// Pretty is a pure whitespace re-format of the compact stream:
		// `json.Indent` walks the compact bytes and re-emits them with
		// the requested indentation (2 spaces), preserving the byte order
		// of every token (including the key ordering writeNode just
		// established) and every explicit `null` (json.Indent does not
		// elide values). Implementing pretty this way means there is
		// exactly ONE walker — writeNode — and the indent/compact choice
		// is a downstream byte-level concern, so the compact-vs-pretty
		// byte-stable-up-to-whitespace property is true by construction.
		var indented bytes.Buffer
		if err := json.Indent(&indented, buf.Bytes(), "", "  "); err != nil {
			return err
		}
		_, err := w.Write(indented.Bytes())
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// writeJSONValue serializes an arbitrary Go value (the frontmatter side of
// the envelope) using encoding/json's compact form. Used by both the
// envelope path and the FrontmatterOnly short-circuit so scalar passthrough
// is uniform.
func writeJSONValue(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	// json.Encoder always appends a trailing newline; strip it after writing.
	startLen := buf.Len()
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Drop the trailing '\n' that encoding/json appends; the v1 contract is
	// "no trailing newline anywhere in the JSON document."
	if buf.Len() > startLen && buf.Bytes()[buf.Len()-1] == '\n' {
		buf.Truncate(buf.Len() - 1)
	}
	return nil
}

// writeNode emits a single mdast node with the v1 key ordering:
//   1. `type` first
//   2. type-specific fields in their canonical order (`depth` on heading,
//      `value` on text/inlineCode/code/html, etc.)
//   3. `children` (for container nodes; absent on value-only leaf nodes
//      like `text` since mdast leaves don't carry an empty children array)
//   4. `position` (dropped uniformly under --no-position)
//
// The container-vs-leaf decision is driven by the node Type: `text` is a
// leaf (no children key); everything else translate emits at S04 is a
// container (root, paragraph, heading, emphasis, strong).
func writeNode(buf *bytes.Buffer, n *translate.Node, opts Options) {
	if n == nil {
		buf.WriteString("null")
		return
	}
	buf.WriteByte('{')
	buf.WriteString(`"type":`)
	writeJSONString(buf, n.Type)

	// Type-specific fields, in canonical mdast key order. The switch on
	// n.Type is the single decision point; adding a new value-bearing or
	// attribute-bearing node type in a later slice means adding a case here.
	switch n.Type {
	case "heading":
		buf.WriteString(`,"depth":`)
		buf.WriteString(strconv.Itoa(n.Depth))
	case "text":
		if n.ValuePresent {
			buf.WriteString(`,"value":`)
			writeJSONString(buf, n.Value)
		}
	case "list":
		// mdast key order for list: ordered, start, spread (before children).
		buf.WriteString(`,"ordered":`)
		writeJSONBool(buf, n.Ordered)
		buf.WriteString(`,"start":`)
		writeJSONNullableInt(buf, n.Start)
		buf.WriteString(`,"spread":`)
		writeJSONBool(buf, n.Spread)
	case "listItem":
		// mdast key order for listItem: spread, checked (before children).
		// `checked: null` is NEVER elided per CONTEXT.md mdast node-set v1
		// (and the S05 issue's acceptance criterion #7) — that rule is
		// concentrated in writeJSONNullableBool.
		buf.WriteString(`,"spread":`)
		writeJSONBool(buf, n.Spread)
		buf.WriteString(`,"checked":`)
		writeJSONNullableBool(buf, n.Checked)
	case "code":
		// mdast key order for code: lang, meta, value (before position).
		// Both lang and meta are nullable per CONTEXT.md mdast node-set v1
		// ("`lang` and `meta` are `null` for indented"); the rule that
		// they are NEVER elided (always present as either a string or
		// JSON null) is concentrated in writeJSONNullableString.
		buf.WriteString(`,"lang":`)
		writeJSONNullableString(buf, n.Lang)
		buf.WriteString(`,"meta":`)
		writeJSONNullableString(buf, n.Meta)
		buf.WriteString(`,"value":`)
		writeJSONString(buf, n.Value)
	case "inlineCode":
		// inlineCode is a leaf carrying only `value`. mdast key order: value.
		buf.WriteString(`,"value":`)
		writeJSONString(buf, n.Value)
	case "inlineMath":
		// inlineMath is a leaf carrying only `value` (v1.x math Run; ADR-0004
		// Decision 4; CONTEXT.md `inlineMath node` entry). mdast key order:
		// value. The wire shape is `{type, value, position}` — no `meta`, no
		// `data`, no `children`.
		buf.WriteString(`,"value":`)
		writeJSONString(buf, n.Value)
	case "html":
		// html is a leaf carrying only `value`. Block and inline raw HTML
		// both serialize through this case — mdast does not distinguish.
		buf.WriteString(`,"value":`)
		writeJSONString(buf, n.Value)
	case "link":
		// mdast key order for link: url, title (before children, position).
		// `title` is nullable per CONTEXT.md mdast node-set v1.
		buf.WriteString(`,"url":`)
		writeJSONString(buf, n.URL)
		buf.WriteString(`,"title":`)
		writeJSONNullableString(buf, n.Title)
	case "image":
		// mdast key order for image: url, title, alt. image is a leaf
		// (`alt` is a flat string, not a children tree, per CONTEXT.md
		// mdast node-set v1 `image{url, title, alt}`). `title` is
		// nullable; `alt` is a non-nullable plain string.
		buf.WriteString(`,"url":`)
		writeJSONString(buf, n.URL)
		buf.WriteString(`,"title":`)
		writeJSONNullableString(buf, n.Title)
		buf.WriteString(`,"alt":`)
		writeJSONString(buf, n.Alt)
	case "table":
		// mdast key order for table: align (before children, position).
		// `align` is a per-column array of `"left"|"right"|"center"|null`
		// per CONTEXT.md mdast node-set v1. Bracketed even when empty —
		// the no-column-alignments degenerate case writes `"align":[]`.
		buf.WriteString(`,"align":`)
		writeJSONNullableStringSlice(buf, n.Align)
	case "linkReference":
		// mdast key order: identifier, label, referenceType (before children).
		// All three are plain strings (no nullable forms — CommonMark
		// guarantees identifier and label are non-empty when a linkReference
		// resolves, and referenceType is always one of the three canonical
		// strings).
		buf.WriteString(`,"identifier":`)
		writeJSONString(buf, n.Identifier)
		buf.WriteString(`,"label":`)
		writeJSONString(buf, n.Label)
		buf.WriteString(`,"referenceType":`)
		writeJSONString(buf, n.ReferenceType)
	case "imageReference":
		// mdast key order: identifier, label, referenceType, alt (leaf —
		// no children). Mirrors `linkReference` for the reference triple,
		// then carries the flat `alt` string like the inline `image` does.
		buf.WriteString(`,"identifier":`)
		writeJSONString(buf, n.Identifier)
		buf.WriteString(`,"label":`)
		writeJSONString(buf, n.Label)
		buf.WriteString(`,"referenceType":`)
		writeJSONString(buf, n.ReferenceType)
		buf.WriteString(`,"alt":`)
		writeJSONString(buf, n.Alt)
	case "definition":
		// mdast key order: identifier, label, url, title (before position;
		// definition is a leaf — no children). Title is nullable.
		buf.WriteString(`,"identifier":`)
		writeJSONString(buf, n.Identifier)
		buf.WriteString(`,"label":`)
		writeJSONString(buf, n.Label)
		buf.WriteString(`,"url":`)
		writeJSONString(buf, n.URL)
		buf.WriteString(`,"title":`)
		writeJSONNullableString(buf, n.Title)
	case "footnoteReference":
		// mdast key order: identifier, label (inline leaf — no children).
		buf.WriteString(`,"identifier":`)
		writeJSONString(buf, n.Identifier)
		buf.WriteString(`,"label":`)
		writeJSONString(buf, n.Label)
	case "footnoteDefinition":
		// mdast key order: identifier, label (before children, position).
		// Block container — the body is translated as children.
		buf.WriteString(`,"identifier":`)
		writeJSONString(buf, n.Identifier)
		buf.WriteString(`,"label":`)
		writeJSONString(buf, n.Label)
	}

	// children: emitted on container nodes only. mdast leaves (text,
	// inlineCode, thematicBreak, break, etc.) do not carry an empty children
	// array on the wire.
	if isContainer(n.Type) {
		buf.WriteString(`,"children":[`)
		for i, c := range n.Children {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeNode(buf, c, opts)
		}
		buf.WriteByte(']')
	}

	// position is dropped uniformly under --no-position; otherwise it is the
	// trailing field per mdast convention.
	if !opts.NoPosition && n.Position != nil {
		buf.WriteString(`,"position":`)
		writePosition(buf, n.Position)
	}
	buf.WriteByte('}')
}

// isContainer reports whether a node type carries a `children` array on the
// wire. mdast leaves (text, inlineCode, code, html, image, thematicBreak,
// break, ...) do NOT carry an empty children array on the wire; every other
// emitted type does. Note that `link` IS a container (it carries inline
// children like a `text` value), but `image` is a leaf because `alt` is a
// flat string per the mdast `image.alt: string` constraint, not a child
// tree.
func isContainer(t string) bool {
	switch t {
	case "text", "inlineCode", "inlineMath", "code", "html", "image", "thematicBreak", "break",
		"imageReference", "definition", "footnoteReference":
		// imageReference is a leaf (alt is a flat string, same as image);
		// definition has no children (it carries url/title scalar fields);
		// footnoteReference is an inline leaf carrying only identifier/label.
		return false
	default:
		return true
	}
}

// writePosition serializes a Position with stable key order:
// {"start":{"line":...,"column":...,"offset":...},"end":{...}}.
func writePosition(buf *bytes.Buffer, p *translate.Position) {
	buf.WriteString(`{"start":`)
	writePoint(buf, p.Start)
	buf.WriteString(`,"end":`)
	writePoint(buf, p.End)
	buf.WriteByte('}')
}

func writePoint(buf *bytes.Buffer, p translate.Point) {
	buf.WriteString(`{"line":`)
	buf.WriteString(strconv.Itoa(p.Line))
	buf.WriteString(`,"column":`)
	buf.WriteString(strconv.Itoa(p.Column))
	buf.WriteString(`,"offset":`)
	buf.WriteString(strconv.Itoa(p.Offset))
	buf.WriteByte('}')
}

// writeJSONBool writes the literal `true` or `false` for a Go bool. Used by
// the per-type field writers in writeNode where the canonical mdast key has
// already been written by the caller; this helper writes only the value.
func writeJSONBool(buf *bytes.Buffer, v bool) {
	if v {
		buf.WriteString("true")
	} else {
		buf.WriteString("false")
	}
}

// writeJSONNullableInt writes `null` when p is nil, else the decimal form
// of *p. Concentrates CONTEXT.md's "pointer types for nullable mdast
// fields" convention for the `*int` flavor (used for `list.start` at S05;
// likely future use as more nullable-int fields land).
func writeJSONNullableInt(buf *bytes.Buffer, p *int) {
	if p == nil {
		buf.WriteString("null")
		return
	}
	buf.WriteString(strconv.Itoa(*p))
}

// writeJSONNullableBool writes `null` when p is nil, `true` or `false`
// when set. Concentrates CONTEXT.md's "pointer types for nullable mdast
// fields" convention for the `*bool` flavor (used for `listItem.checked`
// at S05). The key rule: nil renders as `null`, NEVER as `false` and
// NEVER elided — that's the S05 acceptance criterion #7 contract.
func writeJSONNullableBool(buf *bytes.Buffer, p *bool) {
	if p == nil {
		buf.WriteString("null")
		return
	}
	writeJSONBool(buf, *p)
}

// writeJSONNullableString writes `null` when p is nil, else the JSON-quoted
// string form of *p. Concentrates CONTEXT.md's "pointer types for nullable
// mdast fields" convention for the `*string` flavor — used for `code.lang`,
// `code.meta`, `link.title`, `image.title` (and likely `definition.title`,
// `linkReference.label`, etc. in S08). The key rule: nil renders as `null`,
// NEVER as `""` (those are distinct on the wire — `null` means "no
// language/title/meta provided", `""` would mean "empty language/title/meta
// provided"; the indented-code-block case is `null` for both lang and meta).
func writeJSONNullableString(buf *bytes.Buffer, p *string) {
	if p == nil {
		buf.WriteString("null")
		return
	}
	writeJSONString(buf, *p)
}

// writeJSONNullableStringSlice writes a JSON array whose elements are
// either a quoted string or the literal `null`. Each nil slot in the input
// slice serializes as `null`; each non-nil slot serializes as the
// JSON-quoted form of `*p`. Concentrates CONTEXT.md's "pointer-to-T for
// nullable per-element" convention for `table.align` (and any future
// per-element nullable string slice). Empty / nil input slice serializes
// as `[]` (an empty array), not `null` — the no-elements case is distinct
// from "field absent" or "all elements null".
func writeJSONNullableStringSlice(buf *bytes.Buffer, slice []*string) {
	buf.WriteByte('[')
	for i, p := range slice {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeJSONNullableString(buf, p)
	}
	buf.WriteByte(']')
}

// writeJSONString emits a JSON-quoted string. Uses encoding/json's encoder
// with `SetEscapeHTML(false)` because the wire contract is "raw bytes flow
// through to value-bearing nodes" (CONTEXT.md "Text/Code value
// preservation"): `<`, `>`, and `&` must appear literally inside `value`
// strings (e.g. `html.value: "<div>raw</div>"`), not unicode-escaped as
// `<>`. This matches `writeJSONValue`'s frontmatter path, which
// also disables HTML escaping for the same reason.
func writeJSONString(buf *bytes.Buffer, s string) {
	var sb bytes.Buffer
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		// Encoding a Go string cannot realistically fail; defensively fall
		// back to a manual quote.
		buf.WriteByte('"')
		buf.WriteString(s)
		buf.WriteByte('"')
		return
	}
	// json.Encoder always appends a trailing newline; strip it.
	out := sb.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	buf.Write(out)
}
