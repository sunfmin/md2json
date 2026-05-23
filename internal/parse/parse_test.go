package parse

import (
	"errors"
	"testing"
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
