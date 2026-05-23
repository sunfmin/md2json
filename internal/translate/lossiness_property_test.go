package translate_test

// S10a: property test for the silent-drop lossiness policy (US33).
//
// CONTEXT.md "Lossiness policy (goldmark → mdast)" pins the v1 rule:
//
//   Silent drop. Any goldmark construct that does not map to a node in
//   mdast node-set v1 is dropped from the output with no log line, no
//   `html` fallback dump, and no `{"type":"unknown",...}` escape hatch.
//
// This test is the structural guard for that contract: across a hand-curated
// corpus of GFM + frontmatter + footnote inputs covering every node type
// in the v1 set plus a sampling of GFM extension constructs, every emitted
// node's `type` MUST be a member of the closed enumeration `mdastNodeSetV1`
// below. A regression that, say, leaked a goldmark-native type name
// (`autolink`, `taskCheckbox`, `tableHeader`) onto the wire would fail
// here with a precise diagnostic ("offending type X observed in input <Y>").
//
// Lives next to `no_position_property_test.go` in package `translate_test`
// (external) so we can import both `parse` and `emit` without cycles.
// Runs the full pipeline (parse → translate → emit) so the test catches a
// regression in ANY layer that touches the wire `type` string — not just
// the translate switch.

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"

	"github.com/sunfmin/md2json/internal/emit"
	"github.com/sunfmin/md2json/internal/parse"
	"github.com/sunfmin/md2json/internal/translate"
)

// mdastNodeSetV1 is the closed enumeration of mdast node types the v1
// emitter is allowed to produce. The list MUST match CONTEXT.md "mdast
// node-set v1" verbatim — any change to that glossary entry has to be
// mirrored here AND vice versa. A future v1.x that adds a new node type
// updates both places at once, so the property test becomes the
// mechanical enforcement of the wire-contract enumeration.
//
// Block types (top of the list) + inline types (bottom), in the same
// order as CONTEXT.md so a reviewer can diff the two with the eye.
var mdastNodeSetV1 = map[string]bool{
	// Block:
	"root":               true,
	"heading":            true,
	"paragraph":          true,
	"blockquote":         true,
	"list":               true,
	"listItem":           true,
	"code":               true,
	"html":               true,
	"thematicBreak":      true,
	"definition":         true,
	"footnoteDefinition": true,
	"table":              true,
	"tableRow":           true,
	"tableCell":          true,
	// Inline:
	"text":              true,
	"emphasis":          true,
	"strong":            true,
	"inlineCode":        true,
	"link":              true,
	"image":             true,
	"linkReference":     true,
	"imageReference":    true,
	"footnoteReference": true,
	"break":             true,
	"delete":            true,
}

// lossinessCorpus is the hand-curated input list exercising every node type
// in `mdastNodeSetV1` plus a sampling of GFM extension constructs. Each
// fixture's name describes which node-type / extension it exercises so a
// failure pinpoints the input.
//
// Coverage map (one-to-one with the v1 node set):
//
//	root                  every input
//	heading               headings-h1-h6
//	paragraph             plain-paragraph
//	blockquote            blockquote
//	list                  unordered-list, ordered-list
//	listItem              same
//	code                  fenced-code, indented-code
//	html                  html-block, html-inline
//	thematicBreak         thematic-break
//	definition            link-reference-full (+ definition source)
//	footnoteDefinition    footnote-ref-and-def
//	table                 gfm-table
//	tableRow              same
//	tableCell             same
//	text                  every prose input
//	emphasis              emphasis
//	strong                strong
//	inlineCode            inline-code
//	link                  link-with-title, autolink (which → link)
//	image                 image-with-title
//	linkReference         link-reference-full / collapsed / shortcut
//	imageReference        image-reference
//	footnoteReference     footnote-ref-and-def
//	break                 hard-break-two-spaces
//	delete                strikethrough
//
// GFM extras explicitly exercised (the "+ sampling of GFM extension
// constructs" half of S10a acceptance criterion #5):
//
//	GFM table             gfm-table
//	GFM task list         task-list-mixed-checked
//	GFM strikethrough     strikethrough
//	GFM autolink          autolink-angle, autolink-bare
//
// Frontmatter is exercised on every fixture's pipeline (the parse step
// runs the YAML-frontmatter extension); a closed-fence frontmatter
// fixture exercises the YAML lift path explicitly. Frontmatter itself is
// NOT an mdast node (it sits on the envelope), so the type-walk skips it
// implicitly; we do walk the envelope's `ast` subtree.
var lossinessCorpus = []struct {
	name string
	src  string
}{
	{"empty-doc", ""},
	{"headings-h1-h6", "# h1\n## h2\n### h3\n#### h4\n##### h5\n###### h6\n"},
	{"plain-paragraph", "Just a plain line of prose.\n"},
	{"emphasis", "*em*\n"},
	{"strong", "**strong**\n"},
	{"inline-code", "A `code` span.\n"},
	{"unordered-list", "- one\n- two\n"},
	{"ordered-list", "1. one\n2. two\n"},
	{"task-list-mixed-checked", "- [ ] todo\n- [x] done\n"},
	{"blockquote", "> quoted\n"},
	{"thematic-break", "intro\n\n---\n\noutro\n"},
	{"fenced-code", "```go\nfunc x(){}\n```\n"},
	{"indented-code", "    abc\n    def\n"},
	{"html-block", "<div>raw</div>\n"},
	{"html-inline", "para with <span>inline</span> html\n"},
	{"link-with-title", "[label](https://example.com \"title\")\n"},
	{"image-with-title", "![alt](https://example.com/x.png \"title\")\n"},
	{"autolink-angle", "<https://example.com>\n"},
	{"autolink-bare", "see https://example.com here\n"},
	{"hard-break-two-spaces", "line1  \nline2\n"},
	{"hard-break-backslash", "line1\\\nline2\n"},
	{"strikethrough", "~~struck~~\n"},
	{"gfm-table", "| a | b |\n|---|---|\n| 1 | 2 |\n"},
	{"link-reference-full", "[ref][id]\n\n[id]: https://example.com\n"},
	{"link-reference-collapsed", "[id][]\n\n[id]: https://example.com\n"},
	{"link-reference-shortcut", "[id]\n\n[id]: https://example.com\n"},
	{"image-reference-full", "![alt][id]\n\n[id]: https://example.com/x.png\n"},
	{"footnote-ref-and-def", "intro[^a] more\n\n[^a]: footnote body\n"},
	{"closed-fence-frontmatter", "---\ntitle: hi\n---\n\nbody\n"},
	{
		"big-mixed",
		"---\ntitle: kitchen sink\n---\n\n" +
			"# Title\n\nIntro with *em*, **strong**, ~~struck~~, `code`, and a [link](https://example.com).\n\n" +
			"- plain\n- [x] task\n- [ ] todo\n\n" +
			"> quoted with **inner** strong\n\n" +
			"```go\nfunc x(){}\n```\n\n" +
			"    indented\n\n" +
			"<div>raw block</div>\n\n" +
			"---\n\n" +
			"Autolink: <https://example.com>, bare: https://example.org\n\n" +
			"line1  \nline2\n\n" +
			"| h1 | h2 |\n|---|---|\n| a | b |\n\n" +
			"refs: [r1][id], [id], [id][], ![imgref][id]\n\n" +
			"[id]: https://example.com\n\n" +
			"foot[^a]\n\n[^a]: footnote body\n",
	},
}

// TestEveryEmittedTypeIsInMdastNodeSetV1 is the silent-drop property
// test (US33 acceptance / S10a acceptance criteria #5 + #6). For each
// input in `lossinessCorpus`, it:
//
//  1. Runs the full pipeline parse → translate → emit with `--no-position`
//     for byte stability (position-stripping has no bearing on which `type`
//     strings appear).
//  2. Parses the JSON output back into `interface{}`.
//  3. Walks the tree and collects every `type` string observed.
//  4. Asserts every observed `type` is a key of `mdastNodeSetV1`.
//
// On failure the diagnostic identifies the offending `type` string AND
// the input fixture name AND the parent-chain path of the offending node
// in the tree — three pieces of information together name the bug
// uniquely (which node type leaked, in which input, at which tree
// position).
//
// The two-half structure makes the failure mode useful:
//
//   - The negative half (out-of-set type observed) catches a regression
//     that leaks a goldmark-native type name like "autoLink", "taskCheckbox",
//     or "tableHeader" onto the wire.
//   - The corpus's COVERAGE side (TestLossinessCorpusCoversEveryV1NodeType
//     below) catches the orthogonal regression where the corpus loses
//     coverage of a v1 node type — without that guard, this property
//     test could trivially pass on a degenerate corpus that doesn't
//     exercise the interesting paths.
func TestEveryEmittedTypeIsInMdastNodeSetV1(t *testing.T) {
	for _, fx := range lossinessCorpus {
		t.Run(fx.name, func(t *testing.T) {
			out := emitForLossiness(t, []byte(fx.src))

			var parsed any
			if err := json.Unmarshal(out, &parsed); err != nil {
				t.Fatalf("re-parse emitted JSON: %v\n  output: %s", err, out)
			}

			ast := extractAST(parsed)
			if ast == nil {
				// Frontmatter-only or scalar-passthrough cases would skip
				// the envelope; lossinessCorpus's inputs all run in default
				// envelope mode, so this branch is defensive only.
				return
			}

			walkTypes(t, fx.name, "ast", ast)
		})
	}
}

// TestLossinessCorpusCoversEveryV1NodeType is the COVERAGE half of the
// property: across the corpus's union of emitted `type`s, every member
// of `mdastNodeSetV1` MUST appear at least once. Without this guard, the
// property test above could silently pass on a corpus that exercised
// only a strict subset of the v1 set — a regression in the corpus would
// then mask a regression in the wire contract.
//
// On failure the diagnostic lists the missing types so the fixture
// designer knows which node type to add an input for.
func TestLossinessCorpusCoversEveryV1NodeType(t *testing.T) {
	observed := map[string]bool{}
	for _, fx := range lossinessCorpus {
		out := emitForLossiness(t, []byte(fx.src))
		var parsed any
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("re-parse emitted JSON for %s: %v", fx.name, err)
		}
		ast := extractAST(parsed)
		if ast == nil {
			continue
		}
		collectTypes(ast, observed)
	}

	var missing []string
	for typeName := range mdastNodeSetV1 {
		if !observed[typeName] {
			missing = append(missing, typeName)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("lossinessCorpus does not exercise the following mdast node-set v1 types: %v; add an input that produces each one so the silent-drop property test cannot trivially pass on a degenerate corpus", missing)
	}
}

// TestEveryEmittedTypeIsInMdastNodeSetV1DetectsOutOfSetTypes is the
// negative-direction control for the property test: it FEEDS a
// synthetic out-of-set type into the walker and asserts the walker
// reports it. Pinning this gives confidence that
// `TestEveryEmittedTypeIsInMdastNodeSetV1` would actually catch a
// regression — without this control, a bug in the walker
// (e.g. swallowing failures inside a helper) could let the property
// test silently always-pass.
//
// The synthetic data is a small JSON object whose `type` is the literal
// string `goldmarkAutoLink` — a plausible leakage form (a developer
// accidentally writing the goldmark-native type name onto the wire).
// We invoke a test-local probe of walkTypes via a child sub-test that
// expects an error and inverts the t.Errorf to a t.Fatal IF the probe
// did NOT detect the violation. The probe uses a private `testing.T`-
// like recorder so the negative case doesn't fail the outer test.
func TestEveryEmittedTypeIsInMdastNodeSetV1DetectsOutOfSetTypes(t *testing.T) {
	// Synthetic AST: a root containing a node with an invalid type.
	syntheticAST := map[string]any{
		"type": "root",
		"children": []any{
			map[string]any{
				"type":     "goldmarkAutoLink", // NOT in mdast node-set v1
				"children": []any{},
			},
		},
	}
	rec := &recorderT{}
	walkTypes(rec, "synthetic", "ast", syntheticAST)
	if !rec.hadError {
		t.Errorf("walkTypes did not report the out-of-set type %q; the property test would silently pass on a real regression", "goldmarkAutoLink")
	}
}

// recorderT is a minimal testing.TB-like sink used by the
// detects-out-of-set negative control. It records whether t.Errorf or
// t.Fatalf was invoked. walkTypes only ever calls t.Errorf, so
// `hadError` is sufficient.
type recorderT struct {
	hadError bool
}

func (r *recorderT) Errorf(format string, args ...any) { r.hadError = true }

// errorReporter is the subset of *testing.T that walkTypes uses. The
// recorderT type above satisfies this interface, letting the negative
// control reuse the production walkTypes function rather than
// duplicating its logic.
type errorReporter interface {
	Errorf(format string, args ...any)
}

// walkTypes recursively walks a parsed-JSON tree and asserts every
// observed `type` is a member of `mdastNodeSetV1`. `fixtureName` and
// `path` are threaded through for diagnostic precision: a failure
// reports "fixture <name>, path <ast/children[2]/...>, type <X>".
//
// Takes an errorReporter rather than *testing.T directly so the negative
// control above can reuse it with a recorder sink — see
// TestEveryEmittedTypeIsInMdastNodeSetV1DetectsOutOfSetTypes.
func walkTypes(t errorReporter, fixtureName, path string, node any) {
	switch v := node.(type) {
	case map[string]any:
		if typeVal, hasType := v["type"]; hasType {
			typeStr, ok := typeVal.(string)
			if !ok {
				t.Errorf("fixture %s, path %s: `type` field is not a string (got %T = %v)", fixtureName, path, typeVal, typeVal)
				return
			}
			if !mdastNodeSetV1[typeStr] {
				t.Errorf("fixture %s, path %s: emitted `type` %q is NOT a member of mdast node-set v1; either translate leaked a goldmark-native type, or the v1 enumeration needs to be extended", fixtureName, path, typeStr)
			}
		}
		// Recurse into children (the only sub-tree of mdast nodes
		// containing further nodes). Other map keys hold scalars
		// (strings, numbers, nulls) — recurse into them anyway so a
		// hypothetical buggy embedding of a node under a different key
		// would also be caught.
		// Sort keys for deterministic recursion order so paths in
		// diagnostics are stable across Go map iteration orders.
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walkTypes(t, fixtureName, path+"/"+k, v[k])
		}
	case []any:
		for i, c := range v {
			walkTypes(t, fixtureName, path+"["+itoa(i)+"]", c)
		}
	default:
		// Scalars: no node here.
	}
}

// collectTypes walks the tree like walkTypes but accumulates every
// observed `type` into the provided set instead of asserting set
// membership. Used by the coverage half (TestLossinessCorpusCoversEveryV1NodeType).
func collectTypes(node any, out map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		if typeVal, hasType := v["type"]; hasType {
			if typeStr, ok := typeVal.(string); ok {
				out[typeStr] = true
			}
		}
		for _, c := range v {
			collectTypes(c, out)
		}
	case []any:
		for _, c := range v {
			collectTypes(c, out)
		}
	}
}

// extractAST returns the `ast` field of the envelope (when the input
// produced an envelope) or nil otherwise. The frontmatter-only and
// scalar-passthrough output shapes don't carry an envelope, but
// lossinessCorpus inputs all run in default envelope mode so the nil
// branch is defensive only.
func extractAST(parsed any) any {
	envelope, ok := parsed.(map[string]any)
	if !ok {
		return nil
	}
	return envelope["ast"]
}

// emitForLossiness wires through the production pipeline. Equivalent
// shape to `emitEnvelope` in no_position_property_test.go but with the
// translate step explicit (so a regression in the parse → translate
// boundary is included in the property test's scope). Always emits
// with NoPosition=true: position-stripping has no bearing on the
// node-type wire contract, and the smaller compact output is easier
// to inspect in diagnostics.
func emitForLossiness(t *testing.T, src []byte) []byte {
	t.Helper()
	pr, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := translate.Translate(pr.Doc, src, translate.Options{})
	var buf bytes.Buffer
	if err := emit.Emit(&buf, pr.Frontmatter, root, emit.Options{NoPosition: true}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	return buf.Bytes()
}

// itoa is a tiny non-strconv int-to-string helper for index strings in
// the walkTypes path. Translate-side tests have a convention of not
// importing strconv in non-emit code (the convention is documented in
// the S10 tdd-log under "Test 2"). Keeping the helper local is fine —
// it sees only small non-negative ints.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [16]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
