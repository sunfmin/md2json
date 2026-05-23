package parse

import (
	"errors"
	"testing"

	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/yuin/goldmark/ast"
)

// TestParseClosedFenceMapFrontmatter pins the happy path: a closed YAML
// fence at the top with a map body lifts a map value to Result.Frontmatter.
// Acceptance criterion #1 for S09.
func TestParseClosedFenceMapFrontmatter(t *testing.T) {
	src := []byte("---\ntitle: x\n---\n\nbody\n")
	r, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	m, ok := r.Frontmatter.(map[string]any)
	if !ok {
		t.Fatalf("Frontmatter type: got %T, want map[string]any", r.Frontmatter)
	}
	if m["title"] != "x" {
		t.Errorf("Frontmatter[title]: got %v, want %q", m["title"], "x")
	}
	if r.Doc == nil {
		t.Fatal("Doc should not be nil")
	}
	// Body should have at least one child (a paragraph). The exact shape is
	// owned by translate; here we only assert the body was parsed.
	if r.Doc.ChildCount() == 0 {
		t.Errorf("Doc should have body children; got 0")
	}
}

// TestParseUnclosedFenceTreatsAllAsBody pins the unclosed-fence rule:
// opening `---` on line 1 with no closing fence MUST NOT lift any
// frontmatter, regardless of whether the body coincidentally parses as
// valid YAML. The whole document — including the `---` line — is body.
// Acceptance criterion #2 for S09.
func TestParseUnclosedFenceTreatsAllAsBody(t *testing.T) {
	// `title: x` happens to be valid YAML; pre-S09 the goldmark/frontmatter
	// extension would greedily consume it and silently lift {"title":"x"}.
	// The unclosed-fence rule requires Frontmatter to stay nil and the body
	// to include the leading `---` (as a ThematicBreak in goldmark terms).
	src := []byte("---\ntitle: x\n")
	r, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Frontmatter != nil {
		t.Errorf("Frontmatter: got %v, want nil (unclosed fence is body-only)", r.Frontmatter)
	}
	if r.Doc == nil {
		t.Fatal("Doc should not be nil")
	}
	if r.Doc.ChildCount() == 0 {
		t.Errorf("Doc should have body children (the `---` line is a thematic break); got 0")
	}
}

// TestParseUnclosedFenceWithBodyOnlyYAMLScalar pins the same rule for the
// case where the body would parse as a scalar (not a map). pre-S09 this
// silently lifted `"title x y"` as a string-scalar frontmatter.
func TestParseUnclosedFenceWithBodyOnlyYAMLScalar(t *testing.T) {
	src := []byte("---\ntitle x y\n")
	r, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Frontmatter != nil {
		t.Errorf("Frontmatter: got %v, want nil", r.Frontmatter)
	}
}

// TestParseMalformedYAMLClosedFenceReturnsTypedError pins criterion #3:
// malformed YAML between closed fences MUST return a typed
// *InvalidFrontmatterError carrying source-relative line/col, not a bare
// string error and not the goldmark/yaml `:0:0:` doc-scoped sentinel.
func TestParseMalformedYAMLClosedFenceReturnsTypedError(t *testing.T) {
	src := []byte("---\ntitle: \"unclosed\n---\n")
	_, err := Parse(src)
	if err == nil {
		t.Fatal("Parse: got nil error, want *InvalidFrontmatterError")
	}
	var ife *InvalidFrontmatterError
	if !errors.As(err, &ife) {
		t.Fatalf("Parse error type: got %T (%v), want *InvalidFrontmatterError", err, err)
	}
	// The unbalanced double-quote starts on YAML-region line 1 (source
	// line 2) and yaml.v3 reports the EOF-during-scalar on YAML-region
	// line 2 (source line 3). The pre-scan declares YAML region starting
	// at source line 2, so source line 3 is yaml-region line 2 = 2+2-1=3.
	if ife.Line != 3 {
		t.Errorf("Line: got %d, want 3", ife.Line)
	}
	if ife.Col != 1 {
		t.Errorf("Col: got %d, want 1 (yaml.v3 carries no column; fall back to 1)", ife.Col)
	}
	if ife.Msg == "" {
		t.Errorf("Msg should be non-empty")
	}
	// Msg should NOT carry the `yaml: line N: ` prefix; that information is
	// concentrated into ife.Line.
	if want := "found unexpected end of stream"; ife.Msg != want {
		t.Errorf("Msg: got %q, want %q", ife.Msg, want)
	}
}

// TestParseDuplicateKeyYAMLTypeErrorFlattens pins that the yaml.v3
// TypeError flavor (duplicate-key, type-mismatch, etc.) is flattened into
// a single-line `cleanMsg` so the canonical stderr regex (one line per
// diagnostic) still matches. The duplicate-key entry's YAML-region line
// number translates to source coordinates.
func TestParseDuplicateKeyYAMLTypeErrorFlattens(t *testing.T) {
	src := []byte("---\ntitle: x\ntitle: y\n---\n")
	_, err := Parse(src)
	if err == nil {
		t.Fatal("Parse: got nil error, want *InvalidFrontmatterError")
	}
	var ife *InvalidFrontmatterError
	if !errors.As(err, &ife) {
		t.Fatalf("Parse error type: got %T (%v), want *InvalidFrontmatterError", err, err)
	}
	// "title: y" is at yaml-region line 2 (source line 3).
	if ife.Line != 3 {
		t.Errorf("Line: got %d, want 3", ife.Line)
	}
	// Most important: no embedded newline.
	for _, c := range ife.Msg {
		if c == '\n' {
			t.Errorf("Msg contains '\\n', breaks the one-line canonical stderr regex: %q", ife.Msg)
			break
		}
	}
	// Sanity check on content; we don't pin the exact yaml.v3 wording but
	// the cleaned msg should mention the duplicate key.
	if !contains(ife.Msg, "title") || !contains(ife.Msg, "already defined") {
		t.Errorf("Msg: got %q, want it to mention the duplicate key (\"title\" + \"already defined\")", ife.Msg)
	}
}

// contains is a small helper to avoid pulling in strings just for the
// single Contains call in the test above; keeping the test file's imports
// scoped to the standard testing + errors packages this slice introduced.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestParseNoFrontmatterDoc pins the no-fence-at-all path: a document that
// does not open with `---` parses with Frontmatter == nil and the body
// untouched.
func TestParseNoFrontmatterDoc(t *testing.T) {
	src := []byte("# Hello\nworld\n")
	r, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Frontmatter != nil {
		t.Errorf("Frontmatter: got %v, want nil", r.Frontmatter)
	}
}

// TestParseClosedFenceScalarStringFrontmatter pins the scalar-passthrough
// shape at the parse layer: a YAML scalar between closed fences becomes
// the scalar's Go value (not a singleton map). Emit serializes it at the
// top level under `--frontmatter-only` (S09 criteria #4 and #5).
func TestParseClosedFenceScalarStringFrontmatter(t *testing.T) {
	src := []byte("---\n\"hello\"\n---\n")
	r, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Frontmatter != "hello" {
		t.Errorf("Frontmatter: got %v (%T), want %q (string)", r.Frontmatter, r.Frontmatter, "hello")
	}
}

// TestParseClosedFenceScalarNumberFrontmatter pins the number scalar form.
func TestParseClosedFenceScalarNumberFrontmatter(t *testing.T) {
	src := []byte("---\n42\n---\n")
	r, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// yaml.v3 decodes integer scalars into Go int by default.
	if r.Frontmatter != 42 {
		t.Errorf("Frontmatter: got %v (%T), want 42 (int)", r.Frontmatter, r.Frontmatter)
	}
}

// TestParseClosedFenceScalarNullFrontmatter pins the null scalar form.
// (This is distinct from the empty-doc case: there an explicit `null`
// scalar is between fences, vs there being no frontmatter at all.)
func TestParseClosedFenceScalarNullFrontmatter(t *testing.T) {
	src := []byte("---\nnull\n---\n")
	r, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Frontmatter != nil {
		t.Errorf("Frontmatter: got %v (%T), want nil", r.Frontmatter, r.Frontmatter)
	}
}

// TestParseRegistersMathExtension is the S01 tracer bullet: the math
// extension MUST be registered in `parse.New`'s standard extension set
// (no flag, no opt-in). The observable wire-side proof is that parsing
// `$x$` produces a goldmark node of kind "InlineMath" somewhere in the
// resulting tree — without the extension, the input parses as a plain
// `text` containing the literal `$x$` byte run.
//
// The kind name "InlineMath" is the public goldmark-ast NodeKind
// registered by `github.com/litao91/goldmark-mathjax` (see ADR-0004
// Decision 4 and probe/goldmark-mathjax/block_inline.go:28). Asserting
// on the kind name (a string) rather than importing the library type
// keeps this test resilient to library struct-name churn while still
// pinning the load-bearing contract: the math extension IS wired into
// the standard extension set.
//
// S02 (inline-math happy path) is what surfaces the math node onto the
// JSON wire as `inlineMath{value}`; this S01 test only asserts the
// extension is loaded — the translate-layer mapping is intentionally
// not in scope for S01.
func TestParseRegistersMathExtension(t *testing.T) {
	src := []byte("$x$")
	r, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Doc == nil {
		t.Fatal("Doc should not be nil")
	}
	if !containsNodeKind(r.Doc, "InlineMath") {
		t.Errorf("Parse(%q): no goldmark node of kind %q found in the AST; the math extension is not wired into parse.New (S01 acceptance criterion #2)", src, "InlineMath")
	}
}

// containsNodeKind walks the goldmark AST rooted at n and returns true
// iff any descendant (or n itself) has a `Kind().String()` equal to
// `kind`. Used by TestParseRegistersMathExtension to assert math-node
// presence without importing the goldmark-mathjax library directly
// (the extension's NodeKind names are part of the library's public
// contract per probe/goldmark-mathjax/block_inline.go:28 +
// block_node.go:9).
func containsNodeKind(n ast.Node, kind string) bool {
	if n == nil {
		return false
	}
	if n.Kind().String() == kind {
		return true
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if containsNodeKind(c, kind) {
			return true
		}
	}
	return false
}

// findMathBlock walks the goldmark AST rooted at n and returns the first
// `*mathjax.MathBlock` it finds (depth-first preorder). Returns nil if none.
// Used by TestParseUnclosedAndClosedDisplayMathHaveIdenticalLinesLastStop to
// reach into the library's block node from the parse-layer test scope.
func findMathBlock(n ast.Node) *mathjax.MathBlock {
	if n == nil {
		return nil
	}
	if mb, ok := n.(*mathjax.MathBlock); ok {
		return mb
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if mb := findMathBlock(c); mb != nil {
			return mb
		}
	}
	return nil
}

// TestParseUnclosedAndClosedDisplayMathHaveIdenticalLinesLastStop is S05's
// PRD fixture #14 (library-contract A-vs-B equivalence). It pins the
// load-bearing library-behavior invariant that ADR-0004 Decision 5's
// unclosed-`$$` translate compensation rests on:
//
//	The AST alone cannot distinguish a closed `$$...$$` block from one
//	whose closing fence was never written. Both produce a `*mathjax.MathBlock`
//	whose `Lines().Last().Stop` lands at the byte offset immediately AFTER
//	the body line's terminating LF — identical for input `$$\nx\n` (5 bytes,
//	no closer, EOF after body LF) and `$$\nx\n$$\n` (8 bytes, closed). The
//	closing-fence line is NOT appended to `Lines()` per
//	`probe/goldmark-mathjax/block.go:49-57`, which returns parser.Close
//	BEFORE the body-line append branch at `block.go:60-64`. The closed-vs-
//	unclosed decision the translate post-pass makes therefore MUST inspect
//	source bytes AFTER `Lines().Last().Stop`, not any field on MathBlock
//	itself (MathBlock embeds `ast.BaseBlock` and adds zero fields per
//	`probe/goldmark-mathjax/block_node.go:5-7`).
//
// This is behavioral, not structural — a future library upgrade may add
// fields to MathBlock without breaking this test, as long as the
// Lines().Last().Stop equality on A and B holds. The test will fail
// explicitly (and trigger an ADR-0004 reopen) if a future upgrade either
// switches A to decline-to-match or appends the closing fence to B's
// Lines() (making A's and B's Stop differ).
//
// Cross-ref PRD Testing Decisions §fixture #14 + ADR-0004 Decision 5 +
// CONTEXT.md `Unclosed-display-math fall-through rule`.
func TestParseUnclosedAndClosedDisplayMathHaveIdenticalLinesLastStop(t *testing.T) {
	srcA := []byte("$$\nx\n")    // 5 bytes, unclosed, EOF after body LF
	srcB := []byte("$$\nx\n$$\n") // 8 bytes, closed

	rA, err := Parse(srcA)
	if err != nil {
		t.Fatalf("Parse A: %v", err)
	}
	rB, err := Parse(srcB)
	if err != nil {
		t.Fatalf("Parse B: %v", err)
	}

	mbA := findMathBlock(rA.Doc)
	if mbA == nil {
		t.Fatalf("input A %q: no *mathjax.MathBlock in AST; library declined to match the opening `$$`, which would break ADR-0004 Decision 5's premise (the compensation assumes the library always emits a MathBlock for `$$`-opened blocks regardless of closer)", srcA)
	}
	mbB := findMathBlock(rB.Doc)
	if mbB == nil {
		t.Fatalf("input B %q: no *mathjax.MathBlock in AST; library failed on the closed case, which would break the S03 happy path too", srcB)
	}

	linesA := mbA.Lines()
	if linesA == nil || linesA.Len() == 0 {
		t.Fatalf("A: MathBlock.Lines() is empty; ADR-0004 Decision 5's predicate has no Last().Stop to inspect")
	}
	linesB := mbB.Lines()
	if linesB == nil || linesB.Len() == 0 {
		t.Fatalf("B: MathBlock.Lines() is empty")
	}

	stopA := linesA.At(linesA.Len() - 1).Stop
	stopB := linesB.At(linesB.Len() - 1).Stop

	const wantStop = 5 // orig pos 5 in both A and B (offset past body's terminating LF)
	if stopA != wantStop {
		t.Errorf("A: Lines().Last().Stop = %d, want %d (offset past body line's terminating LF)", stopA, wantStop)
	}
	if stopB != wantStop {
		t.Errorf("B: Lines().Last().Stop = %d, want %d (offset past body line's terminating LF; closing-fence line is NOT appended to Lines() per probe/goldmark-mathjax/block.go:49-57)", stopB, wantStop)
	}
	if stopA != stopB {
		t.Errorf("A and B have different Lines().Last().Stop (A=%d, B=%d) — ADR-0004 Decision 5's premise (closed vs unclosed indistinguishable from AST alone) is broken; reopen ADR-0004", stopA, stopB)
	}
}
