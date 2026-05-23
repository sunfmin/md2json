package translate

import (
	"reflect"
	"testing"

	"github.com/sunfmin/md2json/internal/parse"
	"github.com/yuin/goldmark/ast"
)

// S03 Test 3 (criterion #4): translate's output for an empty document is a Go
// value tree of *Node — NOT a goldmark node. The type assertion below would
// fail to compile if the function ever returned a goldmark type, and the
// reflection check on the goldmark "node" interface confirms the returned
// value doesn't accidentally satisfy goldmark's ast.Node.
func TestTranslateEmptyDocProducesGoValueTreeNotGoldmarkNode(t *testing.T) {
	doc := ast.NewDocument()
	root := Translate(doc, []byte{}, Options{})

	// Compile-time check: Translate's return type is *Node, not ast.Node.
	if root == nil {
		t.Fatalf("Translate returned nil for empty document")
	}

	// Run-time check: the returned value does NOT implement goldmark's
	// ast.Node interface. *translate.Node is a plain Go struct; if a future
	// refactor accidentally returned a goldmark wrapper, this would fail.
	var asGoldmark ast.Node
	if reflect.TypeOf(root).Implements(reflect.TypeOf(&asGoldmark).Elem()) {
		t.Errorf("translate.Node should not implement goldmark/ast.Node; type=%T", root)
	}

	// Shape check: root has type "root", empty children, and a zero-width
	// position at {1,1,0}-{1,1,0} (the empty-doc baseline).
	if root.Type != "root" {
		t.Errorf("Type: got %q, want %q", root.Type, "root")
	}
	if root.Children == nil {
		t.Errorf("Children should be non-nil (empty slice), got nil")
	}
	if len(root.Children) != 0 {
		t.Errorf("Children length: got %d, want 0", len(root.Children))
	}
	if root.Position == nil {
		t.Fatalf("Position should be non-nil for empty doc baseline")
	}
	wantPos := &Position{
		Start: Point{Line: 1, Column: 1, Offset: 0},
		End:   Point{Line: 1, Column: 1, Offset: 0},
	}
	if !reflect.DeepEqual(root.Position, wantPos) {
		t.Errorf("Position mismatch\n  got:  %+v\n  want: %+v", root.Position, wantPos)
	}
}

// S03 Test 3b (criterion #4, single-newline boundary per user story 27): a
// document of exactly one `\n` byte yields root with empty children and
// end position {line:2,column:1,offset:1}. This is not exercised by the
// CLI fixture suite at S03 but the rootPosition math has to be right for
// later slices, so we pin it here as a unit test on translate.
func TestTranslateSingleNewlineRootPosition(t *testing.T) {
	doc := ast.NewDocument()
	root := Translate(doc, []byte("\n"), Options{})

	if root.Type != "root" {
		t.Errorf("Type: got %q, want %q", root.Type, "root")
	}
	if len(root.Children) != 0 {
		t.Errorf("Children length: got %d, want 0 (a blank line produces no block content)", len(root.Children))
	}
	wantPos := &Position{
		Start: Point{Line: 1, Column: 1, Offset: 0},
		End:   Point{Line: 2, Column: 1, Offset: 1},
	}
	if !reflect.DeepEqual(root.Position, wantPos) {
		t.Errorf("Position mismatch\n  got:  %+v\n  want: %+v", root.Position, wantPos)
	}
}

// S04 Test: a single-text-run paragraph (`Hello world.`) coalesces goldmark's
// two adjacent `*ast.Text` segments into one mdast `text` node with the merged
// value and the union position. The mdast spec is "one text node per
// uninterrupted run"; goldmark's internal segment split is an implementation
// detail that translate hides.
func TestTranslateCoalescesContiguousTextSiblings(t *testing.T) {
	src := []byte("Hello world.\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})

	if len(root.Children) != 1 {
		t.Fatalf("root.Children length: got %d, want 1", len(root.Children))
	}
	para := root.Children[0]
	if para.Type != "paragraph" {
		t.Fatalf("root.Children[0].Type: got %q, want %q", para.Type, "paragraph")
	}
	if len(para.Children) != 1 {
		t.Fatalf("paragraph.Children length: got %d (want 1 — adjacent text segments must coalesce); children: %+v", len(para.Children), para.Children)
	}
	text := para.Children[0]
	if text.Type != "text" {
		t.Errorf("text.Type: got %q, want %q", text.Type, "text")
	}
	if text.Value != "Hello world." {
		t.Errorf("text.Value: got %q, want %q", text.Value, "Hello world.")
	}
	if !text.ValuePresent {
		t.Errorf("text.ValuePresent should be true for any *ast.Text mapping")
	}
}

// S04 Test: `**hello**` translates to mdast `strong`, NOT
// `{type:"emphasis", depth:2}` and NOT `{type:"emphasis"}` with a level
// field. The level→type mapping is the load-bearing rule.
func TestTranslateEmphasisLevelTwoEmitsStrong(t *testing.T) {
	src := []byte("**hello**\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})

	if len(root.Children) != 1 {
		t.Fatalf("root.Children length: got %d, want 1", len(root.Children))
	}
	para := root.Children[0]
	if para.Type != "paragraph" {
		t.Fatalf("paragraph type: got %q, want %q", para.Type, "paragraph")
	}
	if len(para.Children) != 1 {
		t.Fatalf("paragraph children: got %d, want 1", len(para.Children))
	}
	strong := para.Children[0]
	if strong.Type != "strong" {
		t.Errorf("inline type: got %q, want %q (level-2 emphasis must map to strong, not emphasis-with-depth)", strong.Type, "strong")
	}
	if strong.Depth != 0 {
		t.Errorf("strong.Depth: got %d, want 0 (strong has no depth field in mdast)", strong.Depth)
	}
}

// S05 Test (acceptance criterion #7): every non-task listItem carries
// `Checked: nil` in the Go value tree, which the emit module serializes as
// `"checked":null` on the wire — never elided. This anchors the rule at the
// translate-package boundary so a future refactor that accidentally turned
// `Checked` into a non-pointer bool (and lost the "null vs false"
// distinction) would fail here, NOT only at the wire fixture level. The
// fixture suite covers the wire side; this covers the translate side.
func TestTranslateListItemNonTaskCarriesNilChecked(t *testing.T) {
	src := []byte("- a\n- b\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})

	if len(root.Children) != 1 {
		t.Fatalf("root.Children length: got %d, want 1", len(root.Children))
	}
	list := root.Children[0]
	if list.Type != "list" {
		t.Fatalf("root.Children[0].Type: got %q, want %q", list.Type, "list")
	}
	if len(list.Children) != 2 {
		t.Fatalf("list.Children length: got %d, want 2 (- a, - b)", len(list.Children))
	}
	for i, li := range list.Children {
		if li.Type != "listItem" {
			t.Errorf("list.Children[%d].Type: got %q, want %q", i, li.Type, "listItem")
		}
		if li.Checked != nil {
			t.Errorf("list.Children[%d].Checked: got %v, want nil (non-task listItem must carry nil; the emit module then serializes nil as `checked: null`, never elided)", i, *li.Checked)
		}
	}
}

// S05 Test: unordered list yields `Start: nil` (which the emit module
// serializes as `"start":null`). Pointer-vs-zero distinction is the
// load-bearing rule — a plain `int` field with zero would either elide
// (wrong: mdast wants `start: null`) or serialize as `"start":0` (also
// wrong). This unit test anchors the rule at the translate boundary.
func TestTranslateUnorderedListHasNilStart(t *testing.T) {
	src := []byte("- a\n- b\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	list := root.Children[0]
	if list.Ordered != false {
		t.Errorf("list.Ordered: got %v, want false", list.Ordered)
	}
	if list.Start != nil {
		t.Errorf("list.Start: got *%d, want nil (unordered lists carry start=nil on the Go side and `start: null` on the wire)", *list.Start)
	}
}

// S05 Test: ordered list with explicit start `3.` yields `Start: *int(3)`.
// Complements TestTranslateUnorderedListHasNilStart: pins the ordered side
// of the pointer-nullable convention.
func TestTranslateOrderedListStartThreeHasPointerStart(t *testing.T) {
	src := []byte("3. a\n4. b\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	list := root.Children[0]
	if !list.Ordered {
		t.Errorf("list.Ordered: got false, want true")
	}
	if list.Start == nil {
		t.Fatalf("list.Start: got nil, want *3")
	}
	if *list.Start != 3 {
		t.Errorf("*list.Start: got %d, want 3", *list.Start)
	}
}

// S05 Test (acceptance criteria #5, #6): hard-line-break post-step. A
// paragraph containing `line1<two-spaces>\nline2` produces three children:
// text("line1"), break, text("line2"). Per CONTEXT.md "Text/Code value
// preservation": no trailing whitespace survives in the preceding text's
// value — the two spaces are entirely consumed by the break boundary.
// Same shape for the backslash flavor.
func TestTranslateHardLineBreakInsertsBreakBetweenTexts(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
	}{
		{"two-space", []byte("line1  \nline2\n")},
		{"backslash", []byte("line1\\\nline2\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := parse.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			root := Translate(r.Doc, tc.src, Options{})
			if len(root.Children) != 1 {
				t.Fatalf("root.Children: got %d, want 1", len(root.Children))
			}
			para := root.Children[0]
			if para.Type != "paragraph" {
				t.Fatalf("para.Type: got %q, want %q", para.Type, "paragraph")
			}
			if len(para.Children) != 3 {
				t.Fatalf("para.Children: got %d, want 3 (text, break, text); children: %+v", len(para.Children), para.Children)
			}
			if para.Children[0].Type != "text" || para.Children[0].Value != "line1" {
				t.Errorf("para.Children[0]: got type=%q value=%q, want type=%q value=%q (no trailing whitespace must survive in the preceding text's value)", para.Children[0].Type, para.Children[0].Value, "text", "line1")
			}
			if para.Children[1].Type != "break" {
				t.Errorf("para.Children[1].Type: got %q, want %q", para.Children[1].Type, "break")
			}
			if para.Children[2].Type != "text" || para.Children[2].Value != "line2" {
				t.Errorf("para.Children[2]: got type=%q value=%q, want type=%q value=%q (no leading whitespace must survive in the following text's value)", para.Children[2].Type, para.Children[2].Value, "text", "line2")
			}
		})
	}
}

// S06 Test (acceptance criterion #3, load-bearing): an indented code block
// produces a `code` node whose `Lang` and `Meta` fields are BOTH nil — not
// empty-string pointers, not unset, but explicitly nil. The emit module
// serializes nil as `lang: null` / `meta: null`; an empty-string pointer
// would serialize as `lang: ""` / `meta: ""`, which is a different wire
// shape and violates CONTEXT.md mdast node-set v1 (`lang` and `meta` are
// `null` for indented). This anchors the contract at the Go layer.
func TestTranslateIndentedCodeBlockHasNilLangAndMeta(t *testing.T) {
	src := []byte("    abc\n    def\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	if len(root.Children) != 1 {
		t.Fatalf("root.Children length: got %d, want 1", len(root.Children))
	}
	code := root.Children[0]
	if code.Type != "code" {
		t.Fatalf("code.Type: got %q, want %q", code.Type, "code")
	}
	if code.Lang != nil {
		t.Errorf("code.Lang: got *%q, want nil (indented code carries lang=nil; emit serializes nil as `lang: null`)", *code.Lang)
	}
	if code.Meta != nil {
		t.Errorf("code.Meta: got *%q, want nil (indented code carries meta=nil; emit serializes nil as `meta: null`)", *code.Meta)
	}
	if code.Value != "abc\ndef\n" {
		t.Errorf("code.Value: got %q, want %q (every content line's trailing newline preserved per CONTEXT.md Text/Code value preservation)", code.Value, "abc\ndef\n")
	}
}

// S06 Test (acceptance criterion #1, fenced code value preservation): a
// fenced code block ` ```go\nfunc x(){}\n``` ` preserves the trailing `\n`
// of the final content line in `value`; the closing fence is NOT part of
// `value`. CONTEXT.md "Text/Code value preservation" calls this out as the
// canonical example. Anchored at the Go layer so a future refactor that
// accidentally strips the trailing newline (e.g. by calling strings.TrimRight)
// would surface here.
func TestTranslateFencedCodeBlockPreservesTrailingNewline(t *testing.T) {
	src := []byte("```go\nfunc x(){}\n```\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	if len(root.Children) != 1 {
		t.Fatalf("root.Children length: got %d, want 1", len(root.Children))
	}
	code := root.Children[0]
	if code.Type != "code" {
		t.Fatalf("code.Type: got %q, want %q", code.Type, "code")
	}
	if code.Lang == nil || *code.Lang != "go" {
		t.Errorf("code.Lang: got %v, want *\"go\" (info string's first word)", code.Lang)
	}
	if code.Meta != nil {
		t.Errorf("code.Meta: got *%q, want nil (no meta portion in info string)", *code.Meta)
	}
	if code.Value != "func x(){}\n" {
		t.Errorf("code.Value: got %q, want %q (trailing newline preserved; closing fence not in value)", code.Value, "func x(){}\n")
	}
}

// S06 Test (acceptance criterion #7): `image.alt` is a flat string with
// non-text inline structure silently dropped per the mdast `image.alt:
// string` constraint. The source `![an *emph* alt](url)` has an emphasis
// node inside the alt; translate must flatten it so `alt == "an emph alt"`,
// NOT `"an *emph* alt"` (raw source) and NOT something like an alt-with-
// children shape. This anchors the load-bearing flattening rule at the Go
// layer; the fixture pins the wire side.
func TestTranslateImageAltFlattensNonTextInlineStructure(t *testing.T) {
	src := []byte("![an *emph* alt](https://example.com/x.png)\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	if len(root.Children) != 1 {
		t.Fatalf("root.Children length: got %d, want 1", len(root.Children))
	}
	para := root.Children[0]
	if para.Type != "paragraph" {
		t.Fatalf("para.Type: got %q, want %q", para.Type, "paragraph")
	}
	if len(para.Children) != 1 {
		t.Fatalf("para.Children length: got %d, want 1", len(para.Children))
	}
	img := para.Children[0]
	if img.Type != "image" {
		t.Fatalf("img.Type: got %q, want %q", img.Type, "image")
	}
	if img.Alt != "an emph alt" {
		t.Errorf("img.Alt: got %q, want %q (non-text inline children must be flattened to their textual content per mdast image.alt: string)", img.Alt, "an emph alt")
	}
	if len(img.Children) != 0 {
		t.Errorf("img.Children: got %d, want 0 (image is a leaf — alt is a flat string, not a child tree)", len(img.Children))
	}
}

// S07 Test (acceptance criterion #1, load-bearing): a 3-column GFM table
// with `:--`, `---`, `--:` alignments produces a `table` node whose `Align`
// is a slice of three `*string` slots with values `*"left"`, `nil`,
// `*"right"` — the middle slot is explicitly nil (not the literal string
// `"none"`, NOT an empty-string pointer). The emit module serializes the
// nil slot as JSON `null`; an `AlignNone → *"none"` mapping would
// serialize as `"none"` on the wire, which the fixture catches, but the
// Go-side anchor is here so a translate-internal refactor that drops
// `alignmentsToMdast`'s `default: nil` arm would fail at unit-test level.
func TestTranslateTableAlignNoneMapsToNilSlot(t *testing.T) {
	src := []byte("| a | b | c |\n|:--|---|--:|\n| 1 | 2 | 3 |\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	if len(root.Children) != 1 {
		t.Fatalf("root.Children: got %d, want 1", len(root.Children))
	}
	table := root.Children[0]
	if table.Type != "table" {
		t.Fatalf("table.Type: got %q, want %q", table.Type, "table")
	}
	if len(table.Align) != 3 {
		t.Fatalf("table.Align: got %d slots, want 3", len(table.Align))
	}
	if table.Align[0] == nil || *table.Align[0] != "left" {
		t.Errorf("table.Align[0]: got %v, want *\"left\"", table.Align[0])
	}
	if table.Align[1] != nil {
		t.Errorf("table.Align[1]: got *%q, want nil (AlignNone must map to a nil slot; mdast convention is JSON null, NOT the string \"none\")", *table.Align[1])
	}
	if table.Align[2] == nil || *table.Align[2] != "right" {
		t.Errorf("table.Align[2]: got %v, want *\"right\"", table.Align[2])
	}
}

// S07 Test (acceptance criterion #1): `tableCell` carries no per-cell
// alignment field in the translate Go-tree. mdast deviates from goldmark
// here — goldmark's `*east.TableCell.Alignment` carries per-cell info,
// but mdast's `tableCell` has no `align` field; alignment is purely a
// `table.align[colIndex]` property. The `Node` struct has no per-cell
// alignment field at all (only the table-level `Align []*string`), so
// this assertion is structural: walk into a table's cells and verify
// they carry no `Align` (its slice should remain nil because cells never
// set it). A future refactor that mistakenly added a per-cell alignment
// field (or reused `Align` on cells) would fail here.
func TestTranslateTableCellHasNoAlignField(t *testing.T) {
	src := []byte("| a | b | c |\n|:--|---|--:|\n| 1 | 2 | 3 |\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	table := root.Children[0]
	if len(table.Children) == 0 {
		t.Fatalf("table.Children: got 0 rows, want >= 1")
	}
	for ri, row := range table.Children {
		if row.Type != "tableRow" {
			t.Errorf("row[%d].Type: got %q, want %q", ri, row.Type, "tableRow")
		}
		for ci, cell := range row.Children {
			if cell.Type != "tableCell" {
				t.Errorf("row[%d].cell[%d].Type: got %q, want %q", ri, ci, cell.Type, "tableCell")
			}
			if cell.Align != nil {
				t.Errorf("row[%d].cell[%d].Align: got non-nil %v, want nil (mdast tableCell carries no align field; alignment is per-column on table)", ri, ci, cell.Align)
			}
		}
	}
}

// S07 Test (acceptance criterion #2, load-bearing): the GFM task-checkbox
// node is HOISTED onto `listItem.Checked` and DROPPED from the child tree.
// A `- [x] done` line yields a listItem with `Checked: *true` and a
// paragraph child whose only inline child is `text(value:"done")` — no
// checkbox-shaped child node, and no `[x]` literal text scrubbing
// artifact in the body. The "no checkbox child" half of the rule is the
// load-bearing one: a future refactor that translated TaskCheckBox to
// some new node type (instead of silent-drop after hoist) would surface
// here.
func TestTranslateTaskCheckboxHoistedAndDropped(t *testing.T) {
	src := []byte("- [x] done\n- [ ] todo\n- plain\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	if len(root.Children) != 1 {
		t.Fatalf("root.Children: got %d, want 1", len(root.Children))
	}
	list := root.Children[0]
	if list.Type != "list" {
		t.Fatalf("list.Type: got %q, want %q", list.Type, "list")
	}
	if len(list.Children) != 3 {
		t.Fatalf("list.Children: got %d, want 3", len(list.Children))
	}
	// Per-item expected Checked: *true, *false, nil.
	wantCheckedStrs := []string{"*true", "*false", "nil"}
	for i, li := range list.Children {
		if li.Type != "listItem" {
			t.Errorf("listItem[%d].Type: got %q, want %q", i, li.Type, "listItem")
		}
		switch i {
		case 0:
			if li.Checked == nil || *li.Checked != true {
				t.Errorf("listItem[%d].Checked: got %v, want %s", i, li.Checked, wantCheckedStrs[i])
			}
		case 1:
			if li.Checked == nil || *li.Checked != false {
				t.Errorf("listItem[%d].Checked: got %v, want %s", i, li.Checked, wantCheckedStrs[i])
			}
		case 2:
			if li.Checked != nil {
				t.Errorf("listItem[%d].Checked: got *%v, want nil (third item is a plain list item, NOT a task)", i, *li.Checked)
			}
		}
		// The listItem's only child should be a paragraph containing one
		// text node — the task-checkbox-shape child should NOT survive
		// translation. We assert by walking the entire subtree of the
		// listItem and confirming no Node has Type == "taskCheckBox" /
		// "taskCheckbox" / "checkbox" (any of these would indicate a
		// future refactor that introduced a wire node type for the
		// dropped checkbox).
		assertNoCheckboxNodeAnywhere(t, li, i)
	}
}

// assertNoCheckboxNodeAnywhere walks a Node subtree and fails the test if
// any descendant has a Type that looks checkbox-like. Used by the task-
// hoist test to pin the "checkbox is silent-dropped after hoist" half of
// the rule independently of the "Checked is set correctly" half.
func assertNoCheckboxNodeAnywhere(t *testing.T, n *Node, listItemIdx int) {
	t.Helper()
	if n == nil {
		return
	}
	switch n.Type {
	case "taskCheckBox", "taskCheckbox", "checkbox":
		t.Errorf("listItem[%d] subtree contains a checkbox-shaped descendant node (Type=%q); task-checkbox must be silent-dropped after hoisting IsChecked to listItem.Checked", listItemIdx, n.Type)
	}
	for _, c := range n.Children {
		assertNoCheckboxNodeAnywhere(t, c, listItemIdx)
	}
}

// S07 Test (acceptance criterion #3): a strikethrough span produces an
// mdast `delete` node — NOT a node named `strikethrough`. The wire fixture
// (37) pins the same rule at the wire level; this anchors it at the
// translate boundary so a refactor that renamed the type to
// `"strikethrough"` would fail here independently. The `delete` name
// matches CONTEXT.md mdast node-set v1 ("delete (GFM strikethrough)")
// and remark's convention (mdast follows HTML's `<del>`).
func TestTranslateStrikethroughTypeIsDeleteNotStrikethrough(t *testing.T) {
	src := []byte("~~struck~~\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	if len(root.Children) != 1 {
		t.Fatalf("root.Children: got %d, want 1", len(root.Children))
	}
	para := root.Children[0]
	if para.Type != "paragraph" {
		t.Fatalf("para.Type: got %q, want %q", para.Type, "paragraph")
	}
	if len(para.Children) != 1 {
		t.Fatalf("para.Children: got %d, want 1", len(para.Children))
	}
	del := para.Children[0]
	if del.Type != "delete" {
		t.Errorf("strikethrough node.Type: got %q, want %q (CONTEXT.md mdast node-set v1 names this `delete`, NOT `strikethrough` — mdast follows HTML's <del> element name)", del.Type, "delete")
	}
}

// S07 Test (acceptance criteria #4 and #5, load-bearing): both autolink
// shapes — angle-bracket `<https://…>` and bare-URL linkify
// `https://…` — translate to mdast `link` with the URL as a `text`
// child's value (NOT as a structural property of the link). This anchors
// the "autolinks collapse to mdast `link`" rule at the translate
// boundary: any future refactor that emitted goldmark's distinct
// `AutoLink` type name on the wire (or that lost the synthetic text
// child) would fail here.
func TestTranslateAutoLinkCollapsesToLinkWithTextChild(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
	}{
		{"angle-bracket", []byte("<https://example.com>\n")},
		{"bare-url-linkify", []byte("https://example.com\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := parse.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			root := Translate(r.Doc, tc.src, Options{})
			if len(root.Children) != 1 {
				t.Fatalf("root.Children: got %d, want 1", len(root.Children))
			}
			para := root.Children[0]
			if para.Type != "paragraph" {
				t.Fatalf("para.Type: got %q, want %q", para.Type, "paragraph")
			}
			if len(para.Children) != 1 {
				t.Fatalf("para.Children: got %d, want 1", len(para.Children))
			}
			link := para.Children[0]
			if link.Type != "link" {
				t.Errorf("autolink node.Type: got %q, want %q (autolinks collapse to mdast `link`; goldmark's `AutoLink` type must not appear on the wire per CONTEXT.md mdast node-set v1)", link.Type, "link")
			}
			if link.URL != "https://example.com" {
				t.Errorf("link.URL: got %q, want %q", link.URL, "https://example.com")
			}
			if link.Title != nil {
				t.Errorf("link.Title: got *%q, want nil (autolinks carry `title: null` per CONTEXT.md mdast node-set v1 autolink rule)", *link.Title)
			}
			if len(link.Children) != 1 {
				t.Fatalf("link.Children: got %d, want 1 (the URL must appear as a synthetic text child)", len(link.Children))
			}
			txt := link.Children[0]
			if txt.Type != "text" || txt.Value != "https://example.com" || !txt.ValuePresent {
				t.Errorf("link's text child: got type=%q value=%q valuePresent=%v, want type=%q value=%q valuePresent=true", txt.Type, txt.Value, txt.ValuePresent, "text", "https://example.com")
			}
		})
	}
}

// S08 Test (acceptance criterion #1, load-bearing): a reference-style link with
// a full label `[text][id]` plus a trailing `[id]: url "title"` definition
// produces TWO sibling top-level nodes — a `paragraph` whose only child is a
// `linkReference` (identifier "id", label "id", referenceType "full", with one
// `text` child "text") and a `definition` (identifier "id", label "id", url,
// title). No inline `link` is emitted: goldmark's `Link.Reference != nil`
// signal must steer us into the linkReference arm, NOT the inline-link arm.
func TestTranslateLinkReferenceFullEmitsLinkReferenceAndDefinition(t *testing.T) {
	src := []byte("[text][id]\n\n[id]: https://example.com \"t\"\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})

	if len(root.Children) != 2 {
		t.Fatalf("root.Children length: got %d, want 2 (paragraph holding linkReference + sibling definition)", len(root.Children))
	}
	para := root.Children[0]
	if para.Type != "paragraph" {
		t.Fatalf("root.Children[0].Type: got %q, want %q", para.Type, "paragraph")
	}
	if len(para.Children) != 1 {
		t.Fatalf("paragraph.Children length: got %d, want 1", len(para.Children))
	}
	lr := para.Children[0]
	if lr.Type != "linkReference" {
		t.Errorf("inline node.Type: got %q, want %q (reference-style link must NOT flatten to inline `link`)", lr.Type, "linkReference")
	}
	if lr.Identifier != "id" {
		t.Errorf("linkReference.Identifier: got %q, want %q", lr.Identifier, "id")
	}
	if lr.Label != "id" {
		t.Errorf("linkReference.Label: got %q, want %q", lr.Label, "id")
	}
	if lr.ReferenceType != "full" {
		t.Errorf("linkReference.ReferenceType: got %q, want %q", lr.ReferenceType, "full")
	}
	if len(lr.Children) != 1 {
		t.Fatalf("linkReference.Children: got %d, want 1", len(lr.Children))
	}
	if lr.Children[0].Type != "text" || lr.Children[0].Value != "text" {
		t.Errorf("linkReference text child: got type=%q value=%q, want type=%q value=%q", lr.Children[0].Type, lr.Children[0].Value, "text", "text")
	}

	def := root.Children[1]
	if def.Type != "definition" {
		t.Fatalf("root.Children[1].Type: got %q, want %q", def.Type, "definition")
	}
	if def.Identifier != "id" {
		t.Errorf("definition.Identifier: got %q, want %q", def.Identifier, "id")
	}
	if def.Label != "id" {
		t.Errorf("definition.Label: got %q, want %q", def.Label, "id")
	}
	if def.URL != "https://example.com" {
		t.Errorf("definition.URL: got %q, want %q", def.URL, "https://example.com")
	}
	if def.Title == nil || *def.Title != "t" {
		t.Errorf("definition.Title: got %v, want *\"t\"", def.Title)
	}
}

// S08 Test (acceptance criteria #2 and #3, load-bearing): the three
// reference-style flavors map to the three mdast `referenceType` strings.
// `[text][]` (collapsed) and `[text]` (shortcut) both source their
// identifier/label from the visible link text, not from a separate label.
// This anchors the goldmark `ReferenceLinkCollapsed`/`ReferenceLinkShortcut`
// → mdast `"collapsed"`/`"shortcut"` mapping at the translate boundary, so a
// future refactor that collapsed the three branches of `referenceTypeToMdast`
// would fail here independently of the fixture suite.
func TestTranslateLinkReferenceCollapsedAndShortcutMapToCorrectReferenceType(t *testing.T) {
	cases := []struct {
		name              string
		src               []byte
		wantReferenceType string
	}{
		{"collapsed", []byte("[text][]\n\n[text]: https://example.com\n"), "collapsed"},
		{"shortcut", []byte("[text]\n\n[text]: https://example.com\n"), "shortcut"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := parse.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			root := Translate(r.Doc, tc.src, Options{})
			if len(root.Children) != 2 {
				t.Fatalf("root.Children length: got %d, want 2 (paragraph + definition)", len(root.Children))
			}
			para := root.Children[0]
			if para.Type != "paragraph" {
				t.Fatalf("root.Children[0].Type: got %q, want %q", para.Type, "paragraph")
			}
			if len(para.Children) != 1 {
				t.Fatalf("paragraph.Children: got %d, want 1", len(para.Children))
			}
			lr := para.Children[0]
			if lr.Type != "linkReference" {
				t.Errorf("inline.Type: got %q, want %q", lr.Type, "linkReference")
			}
			if lr.ReferenceType != tc.wantReferenceType {
				t.Errorf("referenceType: got %q, want %q", lr.ReferenceType, tc.wantReferenceType)
			}
			if lr.Identifier != "text" {
				t.Errorf("identifier: got %q, want %q (collapsed/shortcut take identifier from the link text)", lr.Identifier, "text")
			}
			if lr.Label != "text" {
				t.Errorf("label: got %q, want %q", lr.Label, "text")
			}
		})
	}
}

// S08 Test (acceptance criterion #4, load-bearing): `![alt][id]` plus its
// definition produces an `imageReference` paired with a `definition`. The
// imageReference is a leaf (no children — alt is a flat string, mirroring
// the S06 inline-image rule). Anchored at the translate boundary so a
// future refactor that accidentally retained inline structure under the
// imageReference (or that emitted a flat `image` instead) would fail here.
func TestTranslateImageReferenceFullEmitsImageReferenceAndDefinition(t *testing.T) {
	src := []byte("![alt][id]\n\n[id]: https://example.com/x.png \"pic\"\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	if len(root.Children) != 2 {
		t.Fatalf("root.Children: got %d, want 2", len(root.Children))
	}
	para := root.Children[0]
	if para.Type != "paragraph" {
		t.Fatalf("root.Children[0].Type: got %q, want %q", para.Type, "paragraph")
	}
	if len(para.Children) != 1 {
		t.Fatalf("paragraph.Children: got %d, want 1", len(para.Children))
	}
	ir := para.Children[0]
	if ir.Type != "imageReference" {
		t.Errorf("inline.Type: got %q, want %q (reference-style image must NOT flatten to inline `image`)", ir.Type, "imageReference")
	}
	if ir.Identifier != "id" {
		t.Errorf("imageReference.Identifier: got %q, want %q", ir.Identifier, "id")
	}
	if ir.Label != "id" {
		t.Errorf("imageReference.Label: got %q, want %q", ir.Label, "id")
	}
	if ir.ReferenceType != "full" {
		t.Errorf("imageReference.ReferenceType: got %q, want %q", ir.ReferenceType, "full")
	}
	if ir.Alt != "alt" {
		t.Errorf("imageReference.Alt: got %q, want %q (flat string per CONTEXT.md mdast node-set v1 image.alt)", ir.Alt, "alt")
	}
	if len(ir.Children) != 0 {
		t.Errorf("imageReference.Children: got %d, want 0 (imageReference is a leaf — alt is flat, same as inline image)", len(ir.Children))
	}

	def := root.Children[1]
	if def.Type != "definition" {
		t.Fatalf("root.Children[1].Type: got %q, want %q", def.Type, "definition")
	}
	if def.URL != "https://example.com/x.png" {
		t.Errorf("definition.URL: got %q, want %q", def.URL, "https://example.com/x.png")
	}
	if def.Title == nil || *def.Title != "pic" {
		t.Errorf("definition.Title: got %v, want *\"pic\"", def.Title)
	}
}

// S08 Test (acceptance criterion #5, load-bearing): `text[^a]\n\n[^a]: footnote
// body` produces a `footnoteReference{identifier:"a", label:"a"}` inline in
// the paragraph and a sibling top-level `footnoteDefinition{identifier:"a",
// label:"a"}` whose children carry the body (a `paragraph` with a `text`).
// Two load-bearing wrinkles:
//
//  1. The mdast `identifier`/`label` for footnotes is the SOURCE label (`"a"`),
//     NOT goldmark's 1-based `Index`. The identifier round-trips the
//     reference→definition lookup on the wire.
//
//  2. goldmark wraps every footnote definition in a `*extension/ast.FootnoteList`
//     container that has NO mdast analog. Translate must FLATTEN the
//     FootnoteList — its child `Footnote`s become top-level
//     `footnoteDefinition` siblings of the document root. Backlinks
//     (`*extension/ast.FootnoteBacklink`) injected by goldmark's transformer
//     are presentation-only; silent-drop per CONTEXT.md "Lossiness policy".
//
// This test anchors both rules at the translate boundary.
func TestTranslateFootnoteReferenceAndDefinition(t *testing.T) {
	src := []byte("text[^a]\n\n[^a]: footnote body\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})

	if len(root.Children) != 2 {
		t.Fatalf("root.Children length: got %d, want 2 (paragraph holding footnoteReference + sibling footnoteDefinition — no FootnoteList wrapper on the mdast side); children: %+v", len(root.Children), root.Children)
	}
	para := root.Children[0]
	if para.Type != "paragraph" {
		t.Fatalf("root.Children[0].Type: got %q, want %q", para.Type, "paragraph")
	}
	if len(para.Children) != 2 {
		t.Fatalf("paragraph.Children length: got %d, want 2 (text \"text\" + footnoteReference); children: %+v", len(para.Children), para.Children)
	}
	if para.Children[0].Type != "text" || para.Children[0].Value != "text" {
		t.Errorf("paragraph.Children[0]: got type=%q value=%q, want type=%q value=%q", para.Children[0].Type, para.Children[0].Value, "text", "text")
	}
	fnref := para.Children[1]
	if fnref.Type != "footnoteReference" {
		t.Errorf("inline node.Type: got %q, want %q (FootnoteLink must translate to mdast footnoteReference, NOT an inline link or an indexed reference)", fnref.Type, "footnoteReference")
	}
	if fnref.Identifier != "a" {
		t.Errorf("footnoteReference.Identifier: got %q, want %q (identifier is the SOURCE label, NOT goldmark's 1-based Index)", fnref.Identifier, "a")
	}
	if fnref.Label != "a" {
		t.Errorf("footnoteReference.Label: got %q, want %q", fnref.Label, "a")
	}
	if len(fnref.Children) != 0 {
		t.Errorf("footnoteReference.Children: got %d, want 0 (footnoteReference is an inline leaf)", len(fnref.Children))
	}

	fndef := root.Children[1]
	if fndef.Type != "footnoteDefinition" {
		t.Errorf("root.Children[1].Type: got %q, want %q (FootnoteList must be flattened — Footnote blocks become top-level footnoteDefinition siblings of root)", fndef.Type, "footnoteDefinition")
	}
	if fndef.Identifier != "a" {
		t.Errorf("footnoteDefinition.Identifier: got %q, want %q", fndef.Identifier, "a")
	}
	if fndef.Label != "a" {
		t.Errorf("footnoteDefinition.Label: got %q, want %q", fndef.Label, "a")
	}
	if len(fndef.Children) != 1 {
		t.Fatalf("footnoteDefinition.Children: got %d, want 1 (a paragraph holding the body); children: %+v", len(fndef.Children), fndef.Children)
	}
	body := fndef.Children[0]
	if body.Type != "paragraph" {
		t.Fatalf("footnoteDefinition.Children[0].Type: got %q, want %q", body.Type, "paragraph")
	}
	if len(body.Children) != 1 {
		t.Fatalf("footnoteDefinition body paragraph children: got %d, want 1; children: %+v", len(body.Children), body.Children)
	}
	if body.Children[0].Type != "text" || body.Children[0].Value != "footnote body" {
		t.Errorf("footnoteDefinition body text: got type=%q value=%q, want type=%q value=%q (presentation-only FootnoteBacklink must be silent-dropped per CONTEXT.md \"Lossiness policy\")", body.Children[0].Type, body.Children[0].Value, "text", "footnote body")
	}
}

// S10 Test (acceptance criterion #3, load-bearing property): EVERY emitted
// node carries a Position whose Start.Offset and End.Offset are NOT both
// zero — i.e. no `(0,0)` placeholders survive translate. The root node and
// inline leaves anchored to source byte 0 are exceptions (a single-line
// document's root starts at offset 0; a paragraph whose first byte is the
// very first source byte also starts at 0), so the rule is encoded as
// "not both endpoints == 0" rather than "Start.Offset != 0". This catches
// the historic placeholders (thematicBreak / autolink / footnoteReference
// used `pt.position(0, 0)`) without false-flagging legit zero-anchored
// nodes.
//
// The corpus exercises every node type in the v1 mdast node set with
// CONTENT past byte 0 so the (0,0) signature is unambiguous for the
// placeholder-bearing types. Each fixture's first line is plain text or
// frontmatter, putting the interesting node well past offset 0.
func TestTranslateEveryEmittedNodeHasNonPlaceholderPosition(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
	}{
		{"thematicBreak", []byte("intro\n\n---\n\noutro\n")},
		{"autolink-angle", []byte("intro\n\n<https://example.com>\n")},
		{"autolink-bare", []byte("intro\n\nhttps://example.com\n")},
		{"footnote-ref-and-def", []byte("intro\n\ntext[^a]\n\n[^a]: footnote body\n")},
		{"image-with-emph-alt", []byte("intro\n\n![an *emph* alt](https://example.com/x.png)\n")},
		{"image-alt-entirely-inside-emph", []byte("intro\n\n![*emph*](https://example.com/x.png)\n")},
		{"linkReference", []byte("intro\n\n[text][id]\n\n[id]: https://example.com\n")},
		{"imageReference", []byte("intro\n\n![alt][id]\n\n[id]: https://example.com/x.png\n")},
		{"table", []byte("intro\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")},
		{"task-list", []byte("intro\n\n- [x] done\n- [ ] todo\n")},
		{"strikethrough", []byte("intro\n\n~~struck~~\n")},
		{"hard-break", []byte("intro\n\nline1  \nline2\n")},
		{"blockquote", []byte("intro\n\n> quoted\n")},
		{"code-fenced", []byte("intro\n\n```go\nfunc x(){}\n```\n")},
		{"code-indented", []byte("intro\n\n    abc\n    def\n")},
		{"inline-code", []byte("intro\n\nuse `foo` here\n")},
		{"html-block", []byte("intro\n\n<div>raw</div>\n")},
		{"html-inline", []byte("intro\n\ntext with <span>raw</span> inline\n")},
		{"link-inline", []byte("intro\n\n[label](https://example.com \"t\")\n")},
		{"image-inline", []byte("intro\n\n![alt](https://example.com/x.png)\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := parse.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			root := Translate(r.Doc, tc.src, Options{})
			walkAndAssertPosition(t, root, "root")
		})
	}
}

// walkAndAssertPosition fails the test for any descendant Node whose
// Position is nil or whose Start.Offset and End.Offset are BOTH zero
// (the placeholder signature). The `path` argument is a slash-separated
// trail of node Types so the failure message points at the offending node
// without ambiguity in a deeply nested tree.
func walkAndAssertPosition(t *testing.T, n *Node, path string) {
	t.Helper()
	if n == nil {
		return
	}
	if n.Position == nil {
		t.Errorf("%s (type=%q): Position is nil (every emitted node must carry a position by default)", path, n.Type)
	} else if n.Position.Start.Offset == 0 && n.Position.End.Offset == 0 && path != "root" {
		t.Errorf("%s (type=%q): Position is the (0,0)-placeholder (start.offset=0, end.offset=0); every non-root node must carry a source-accurate position", path, n.Type)
	}
	for i, c := range n.Children {
		walkAndAssertPosition(t, c, path+"/"+n.Type+"["+itoa(i)+"]")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// S10 Test (acceptance criterion #3, accuracy half): autolink and
// footnoteReference positions must be SOURCE-ACCURATE, not just non-zero.
// The property test in TestTranslateEveryEmittedNodeHasNonPlaceholderPosition
// only catches the `(0,0)` placeholder; it would happily accept any other
// position. This test pins the actual source byte ranges so a future
// refactor that broke `findInline`'s monotonic-cursor invariant (e.g. by
// re-walking out of source order) would surface here.
func TestTranslateAutoLinkAndFootnotePositionsAreSourceAccurate(t *testing.T) {
	// Source: "<https://x.com> <https://y.com>\n"
	// Two angle-bracket autolinks on the same line. The cursor must
	// advance past the first match before locating the second.
	t.Run("angle-bracket-twice", func(t *testing.T) {
		src := []byte("<https://x.com> <https://y.com>\n")
		r, err := parse.Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		root := Translate(r.Doc, src, Options{})
		para := root.Children[0]
		// Children: link("https://x.com"), text(" "), link("https://y.com")
		if len(para.Children) < 3 {
			t.Fatalf("para.Children: got %d, want >= 3", len(para.Children))
		}
		linkA := para.Children[0]
		// Want: spans `<https://x.com>` — offsets [0, 15).
		if linkA.Position.Start.Offset != 0 || linkA.Position.End.Offset != 15 {
			t.Errorf("first link Position: got [%d, %d), want [0, 15) (covers `<https://x.com>` including angle brackets)", linkA.Position.Start.Offset, linkA.Position.End.Offset)
		}
		// Find the second link (skipping any intervening text node).
		var linkB *Node
		for _, c := range para.Children[1:] {
			if c.Type == "link" {
				linkB = c
				break
			}
		}
		if linkB == nil {
			t.Fatalf("second link not found among para.Children: %+v", para.Children)
		}
		// Want: spans `<https://y.com>` — offsets [16, 31).
		if linkB.Position.Start.Offset != 16 || linkB.Position.End.Offset != 31 {
			t.Errorf("second link Position: got [%d, %d), want [16, 31) (cursor must advance past first match before locating second)", linkB.Position.Start.Offset, linkB.Position.End.Offset)
		}
	})

	// FootnoteLink position: `[^a]` at known offset.
	t.Run("footnote-reference", func(t *testing.T) {
		src := []byte("intro text[^a] more\n\n[^a]: body\n")
		// `[^a]` starts at offset 10 (after "intro text"), spans 4 bytes (`[^a]`), so [10, 14).
		r, err := parse.Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		root := Translate(r.Doc, src, Options{})
		para := root.Children[0]
		var fnref *Node
		for _, c := range para.Children {
			if c.Type == "footnoteReference" {
				fnref = c
				break
			}
		}
		if fnref == nil {
			t.Fatalf("footnoteReference not found in paragraph children: %+v", para.Children)
		}
		if fnref.Position.Start.Offset != 10 || fnref.Position.End.Offset != 14 {
			t.Errorf("footnoteReference Position: got [%d, %d), want [10, 14) (covers `[^a]` in source)", fnref.Position.Start.Offset, fnref.Position.End.Offset)
		}
	})

	// Bare-URL linkify: position covers just the URL bytes (no surrounding
	// delimiters in the source).
	t.Run("bare-url-linkify", func(t *testing.T) {
		src := []byte("see https://example.com here\n")
		// "https://example.com" starts at offset 4, spans 19 bytes, so [4, 23).
		r, err := parse.Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		root := Translate(r.Doc, src, Options{})
		para := root.Children[0]
		var link *Node
		for _, c := range para.Children {
			if c.Type == "link" {
				link = c
				break
			}
		}
		if link == nil {
			t.Fatalf("link not found in paragraph children: %+v", para.Children)
		}
		if link.Position.Start.Offset != 4 || link.Position.End.Offset != 23 {
			t.Errorf("bare-URL link Position: got [%d, %d), want [4, 23) (no angle-bracket extension for the linkify flavor)", link.Position.Start.Offset, link.Position.End.Offset)
		}
	})
}

// S10 Test (acceptance criterion #1): the empty-document baseline still
// holds end-to-end. Pins the zero-width root position contract that S03
// established at the translate-unit layer; the new fixture `52-single-
// newline-default` pins the one-newline shape. This test re-asserts the
// empty case at the translate-unit layer so a regression in S10's
// changes (the new thematicBreak handler, the findInline cursor, etc.)
// can't silently break the empty-doc baseline without surfacing here too.
func TestTranslateEmptyDocStillHasZeroWidthRootPosition(t *testing.T) {
	src := []byte{}
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	if root.Position == nil {
		t.Fatalf("root.Position is nil for empty doc")
	}
	want := &Position{
		Start: Point{Line: 1, Column: 1, Offset: 0},
		End:   Point{Line: 1, Column: 1, Offset: 0},
	}
	if !reflect.DeepEqual(root.Position, want) {
		t.Errorf("root.Position for empty doc:\n  got:  %+v\n  want: %+v", root.Position, want)
	}
}

// S10 Test (S06 arch-log C8 deferred bug): `translateImage`'s position
// walks `textChildrenSpan` which only sees DIRECT `*ast.Text` children
// and silently skips inline containers like `*ast.Emphasis`. The bug is
// only observable when the inline-container child is at the BOUNDARY of
// the inline sequence (first or last child) — when the emphasis is in
// the middle (`![an *emph* alt](url)`) the surrounding Text segments
// already give a correct min/max because the start anchors on the
// leading Text and the end anchors on the trailing Text. We exercise
// the boundary case where the WHOLE alt is inside an emphasis container
// (`![*emph*](url)`): goldmark emits no direct Text child of the Image
// at all; the only descendant text lives inside an Emphasis. Pre-S10
// `textChildrenSpan` returns (0,0) here because it finds no direct
// Text child; the image position therefore collapses to a zero-width
// (0,0) placeholder — exactly the failure mode the property test in
// Test 2 should catch, but doesn't because the OTHER `![an *emph* alt]`
// fixture has surrounding direct Text children that mask the bug.
func TestTranslateImagePositionWhenAltIsEntirelyInsideInlineContainer(t *testing.T) {
	src := []byte("![*emph*](https://example.com/x.png)\n")
	// Inner emphasis text "emph" segment: [3, 7). The image's text-content
	// span must cover the emphasis content, so the image position spans
	// the inner Text segment's range [3, 7) at minimum.
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})
	para := root.Children[0]
	img := para.Children[0]
	if img.Type != "image" {
		t.Fatalf("img.Type: got %q, want %q", img.Type, "image")
	}
	if img.Position == nil {
		t.Fatalf("img.Position: nil")
	}
	// Pre-S10: (0,0) — the property test would NOT have caught this
	// because we only assert "not both zero", and that fixture wasn't in
	// the corpus. Post-S10: the span recurses into Emphasis and finds the
	// inner Text segment at [3, 7).
	const wantStart, wantEnd = 3, 7
	if img.Position.Start.Offset != wantStart || img.Position.End.Offset != wantEnd {
		t.Errorf("image position: got [%d, %d), want [%d, %d) (textChildrenSpan must recurse into inline containers; alt entirely inside an Emphasis must contribute its inner Text segment)", img.Position.Start.Offset, img.Position.End.Offset, wantStart, wantEnd)
	}
}

// S04 Test: a heading's position spans the line including the `#` markers.
// goldmark's per-line segment starts at the first content byte (offset 2 for
// `# Hello`); the translate stage snaps the start back to column 1 of the
// heading line.
func TestTranslateHeadingPositionIncludesAtxMarkers(t *testing.T) {
	src := []byte("# Hello\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})

	if len(root.Children) != 1 {
		t.Fatalf("root.Children length: got %d, want 1", len(root.Children))
	}
	h := root.Children[0]
	if h.Type != "heading" {
		t.Fatalf("Type: got %q, want %q", h.Type, "heading")
	}
	if h.Depth != 1 {
		t.Errorf("Depth: got %d, want 1", h.Depth)
	}
	if h.Position == nil {
		t.Fatalf("Position should be non-nil")
	}
	wantStart := Point{Line: 1, Column: 1, Offset: 0}
	if h.Position.Start != wantStart {
		t.Errorf("Position.Start: got %+v, want %+v (must include the `# ` marker, not start at the text content)", h.Position.Start, wantStart)
	}
}

// S03 Go-layer anchor (issue 03 acceptance bullets #1 + #3): display math
// `$$\n\frac{a}{b}\n$$\n` produces a single mdast `math{value, meta:nil}`
// child whose `value` is the literal body bytes with the trailing `\n`
// preserved and whose `meta` is `nil` (emit serializes nil `*string` as
// JSON `null`). Anchors translate's `*mathjax.MathBlock` mapping at the
// Go layer; the per-fixture byte-exact compare pins the wire side.
//
// Closing-fence-NOT-in-value invariant (cross-ref CONTEXT.md
// "Text/Code value preservation" `code.value` analogy): the body
// segment `Lines().Value(src)` covers only the interior body lines
// because `probe/goldmark-mathjax/block.go:49-57` returns parser.Close
// BEFORE appending the closing fence line; a future library upgrade
// that changes this branch ordering would fail here with a precise
// diagnostic.
func TestTranslateDisplayMathHappyPath(t *testing.T) {
	src := []byte("$$\n\\frac{a}{b}\n$$\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})

	if len(root.Children) != 1 {
		t.Fatalf("root.Children length: got %d, want 1", len(root.Children))
	}
	m := root.Children[0]
	if m.Type != "math" {
		t.Fatalf("Type: got %q, want %q", m.Type, "math")
	}
	if m.Value != "\\frac{a}{b}\n" {
		t.Errorf("Value: got %q, want %q (trailing newline preserved; closing fence NOT in value)", m.Value, "\\frac{a}{b}\n")
	}
	if m.Meta != nil {
		t.Errorf("Meta: got *%q, want nil (CONTEXT.md `math node` entry: `meta` is always nil for `$$...$$` in v1.x; emit serializes nil *string as JSON null)", *m.Meta)
	}
	if len(m.Children) != 0 {
		t.Errorf("Children: got %d, want 0 (math is a leaf — value/meta scalars only)", len(m.Children))
	}
}

// S03 value-preservation anchor (issue 03 acceptance bullet #3): mhchem
// source inside `$$...$$` rides through `value` byte-for-byte. md2json is
// transport-only — no validation, no expansion, no normalization. Anchors
// CONTEXT.md "Text/Code value preservation" + "Dollar-sign math
// (transport-only)" at the Go layer for the display-math path.
func TestTranslateDisplayMathPreservesMhchemValue(t *testing.T) {
	src := []byte("$$\n\\ce{H2O}\n$$\n")
	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := Translate(r.Doc, src, Options{})

	if len(root.Children) != 1 {
		t.Fatalf("root.Children: got %d, want 1", len(root.Children))
	}
	m := root.Children[0]
	if m.Type != "math" {
		t.Fatalf("Type: got %q, want %q", m.Type, "math")
	}
	if m.Value != "\\ce{H2O}\n" {
		t.Errorf("Value: got %q, want %q (mhchem source rides through byte-for-byte; transport-only posture)", m.Value, "\\ce{H2O}\n")
	}
}
