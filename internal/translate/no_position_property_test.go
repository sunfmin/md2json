package translate_test

import (
	"bytes"
	"testing"

	"github.com/sunfmin/md2json/internal/emit"
	"github.com/sunfmin/md2json/internal/parse"
	"github.com/sunfmin/md2json/internal/translate"
)

// TestEmitNoPositionStripsPositionKeyFromEveryNode pins acceptance
// criterion #4 (S10): `--no-position` must strip the `position` key
// from EVERY emitted node uniformly. The translate side always
// attaches Position so the structure is uniform; the emit side decides
// whether to serialize it. This test runs a complex document
// (every-node-type-touched corpus) through translate + emit twice —
// once with NoPosition=false, once with NoPosition=true — and asserts:
//
//  1. The no-position output contains ZERO `"position":` occurrences.
//  2. The default-mode output contains EXACTLY one `"position":` per
//     emitted Node.
//
// The second assertion is the more interesting half: it cross-checks
// the emit side's per-type `if !opts.NoPosition && n.Position != nil`
// gate against the translate side's "always attach Position" contract.
// A regression that either skipped Position attachment on some node
// type, OR forgot to write `,"position":` for a new node type in
// writeNode's switch, would break the count.
//
// Lives in `package translate_test` (external test package) so we can
// import the emit module without creating an import cycle — emit
// already imports translate.
func TestEmitNoPositionStripsPositionKeyFromEveryNode(t *testing.T) {
	src := []byte("# H1\n\n" +
		"para *em* **strong**\n\n" +
		"- item one\n- [x] task\n\n" +
		"```go\nfunc x(){}\n```\n\n" +
		"    indented\n\n" +
		"<div>raw</div>\n\n" +
		"[label](https://example.com)\n\n" +
		"![alt](https://example.com/x.png)\n\n" +
		"> quoted\n\n" +
		"---\n\n" +
		"line1  \nline2\n\n" +
		"| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
		"~~struck~~\n\n" +
		"<https://example.com>\n\n" +
		"[ref][id]\n\n" +
		"[id]: https://example.com\n\n" +
		"foo[^a]\n\n" +
		"[^a]: footnote\n")

	r, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := translate.Translate(r.Doc, src, translate.Options{})

	nodeCount := countNodes(root)
	if nodeCount < 20 {
		t.Fatalf("corpus too small: only %d nodes; the test needs many node types to be meaningful", nodeCount)
	}

	defaultOut := emitEnvelope(t, root, false)
	noPosOut := emitEnvelope(t, root, true)

	defaultPositions := bytes.Count(defaultOut, []byte(`"position":`))
	noPosPositions := bytes.Count(noPosOut, []byte(`"position":`))

	if noPosPositions != 0 {
		t.Errorf("--no-position output contains %d `\"position\":` occurrences (want 0; --no-position must strip the position key uniformly from every node)\n  output: %s", noPosPositions, noPosOut)
	}
	if defaultPositions != nodeCount {
		t.Errorf("default-mode output contains %d `\"position\":` occurrences but the translated AST has %d nodes (want one position key per node — translate-side `Position` attachment and emit-side write must agree)", defaultPositions, nodeCount)
	}
}

// countNodes counts the total Node objects in a translate subtree
// (including the root). The mirror of writeNode's "emit one node per
// translated Node" invariant — used to verify default-mode emits one
// `"position":` per node.
func countNodes(n *translate.Node) int {
	if n == nil {
		return 0
	}
	c := 1
	for _, child := range n.Children {
		c += countNodes(child)
	}
	return c
}

// emitEnvelope wires through the same emit.Emit entry point the CLI
// uses, with NoPosition gated on the parameter so this test exercises
// the production code path. Returns just the bytes — no envelope vs
// frontmatter-only branching needed; the envelope path always wraps
// the AST.
func emitEnvelope(t *testing.T, root *translate.Node, noPos bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := emit.Emit(&buf, nil, root, emit.Options{NoPosition: noPos}); err != nil {
		t.Fatalf("emit (noPos=%v): %v", noPos, err)
	}
	return buf.Bytes()
}
