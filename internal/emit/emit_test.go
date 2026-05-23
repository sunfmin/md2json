package emit_test

// S10a tests for the emit module's --pretty contract. The unit tests here
// pin three load-bearing properties of the formatting layer that are
// awkward to express as fixtures (which compare byte-for-byte, so they
// pin one specific input → output mapping but not a CROSS-input property):
//
//  1. Compact and pretty outputs are byte-stable up to whitespace — i.e.
//     stripping every space / newline / tab from the pretty output yields
//     exactly the compact output. This pins the "pretty is just whitespace
//     re-format of the compact key-ordered stream" contract from S10a's
//     acceptance criterion #3.
//  2. Explicit `null` fields (`listItem.checked`, `code.lang`, `code.meta`,
//     `link.title` and friends) are preserved verbatim in BOTH modes — the
//     never-elide rule for nullable mdast fields from CONTEXT.md
//     "mdast node-set v1" applies regardless of `--pretty` (S10a criterion #4).
//  3. `--pretty` composes cleanly with `--no-position`: the position key is
//     absent from every node in pretty mode, exactly as in compact mode,
//     without disturbing the surrounding indentation (S10a criterion #2).
//
// Lives in the external `emit_test` package so we can import the parse /
// translate modules without creating an import cycle through emit's own
// translate dependency. The unit tests exercise emit through the same
// public entry point cli.Run uses (`emit.Emit`), so they cover the actual
// production code path.

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sunfmin/md2json/internal/emit"
	"github.com/sunfmin/md2json/internal/parse"
	"github.com/sunfmin/md2json/internal/translate"
)

// representativeCorpus is a small but type-diverse input that exercises
// nearly every mdast node type currently emitted. Used as a single shared
// fixture for all three property tests so a regression in any one node
// type's pretty-vs-compact handling surfaces in all three at once.
//
// The corpus deliberately includes:
//   - indented code (null `lang`/`meta` — pins the null-preservation case
//     called out explicitly in S10a criterion #1 + #4)
//   - a plain non-task list item (null `checked` — same)
//   - a link without a title (null `title` — same)
//   - a fenced code block with explicit lang (non-null `lang` for the
//     positive case)
//   - strong + emphasis + paragraph + heading nesting so the indentation
//     reaches multiple levels and exercises the json.Indent walker
const representativeCorpus = "# Title\n\n" +
	"Body with **bold** and *em*.\n\n" +
	"- plain item\n\n" +
	"```go\nfunc x(){}\n```\n\n" +
	"    indented\n\n" +
	"[no-title-link](https://example.com)\n"

// emitOnce wires through the production `emit.Emit` entry point for a
// single Options shape and returns the bytes. The helper hides the boilerplate
// of building the buffer + asserting no error so the test bodies stay focused
// on the property they're pinning.
func emitOnce(t *testing.T, src []byte, opts emit.Options) []byte {
	t.Helper()
	pr, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := translate.Translate(pr.Doc, src, translate.Options{})
	var buf bytes.Buffer
	if err := emit.Emit(&buf, pr.Frontmatter, root, opts); err != nil {
		t.Fatalf("emit: %v", err)
	}
	return buf.Bytes()
}

// TestPrettyAndCompactAreByteStableUpToWhitespace pins S10a acceptance
// criterion #3: stripping every space / newline / tab from `--pretty`
// output yields exactly the compact output. This is the "one walker
// emits the same byte stream, pretty is just whitespace re-format"
// contract. The property is true by construction in the current
// implementation (pretty mode is `json.Indent` post-processing over
// the compact byte stream), but the test pins it so a future refactor
// that introduced a separate pretty walker would have to keep the two
// walkers byte-stable.
//
// Whitespace-stripping is the right comparison op for this property
// because `json.Indent`'s only contract is "preserve every JSON token
// in order, insert/remove whitespace between them" — so stripping
// whitespace from valid pretty JSON MUST yield the original compact
// JSON.
func TestPrettyAndCompactAreByteStableUpToWhitespace(t *testing.T) {
	src := []byte(representativeCorpus)

	compact := emitOnce(t, src, emit.Options{NoPosition: true})
	pretty := emitOnce(t, src, emit.Options{NoPosition: true, Pretty: true})

	prettyStripped := stripJSONWhitespace(pretty)
	if !bytes.Equal(compact, prettyStripped) {
		t.Errorf("compact and whitespace-stripped pretty differ\n  compact:         %q\n  pretty-stripped: %q", compact, prettyStripped)
	}

	// Sanity: pretty MUST actually have inserted whitespace (otherwise the
	// property is trivially true and tells us nothing). Pretty output for a
	// non-trivial corpus must be strictly longer than compact.
	if len(pretty) <= len(compact) {
		t.Errorf("pretty output (%d bytes) should be strictly longer than compact (%d bytes); did --pretty actually indent?", len(pretty), len(compact))
	}
}

// TestNullFieldsPreservedInBothModes pins S10a acceptance criterion #4:
// explicit `null` fields are preserved verbatim in BOTH compact and
// pretty modes. The corpus's `- plain item` → `listItem.checked: null`,
// `    indented` → `code.lang: null, meta: null`, and
// `[no-title-link](https://example.com)` → `link.title: null` together
// cover every never-elide rule called out in S10a acceptance #4.
//
// The test asserts each `<key>: null` (or `<key>":null` in compact form)
// substring appears the expected number of times in each mode. This is
// a substring check rather than a full structural compare because the
// per-property focus is on the never-elide rule, not on the full byte
// stream (which is pinned separately by the fixture suite).
func TestNullFieldsPreservedInBothModes(t *testing.T) {
	src := []byte(representativeCorpus)

	compact := emitOnce(t, src, emit.Options{NoPosition: true})
	pretty := emitOnce(t, src, emit.Options{NoPosition: true, Pretty: true})

	// The corpus contains:
	//   - one non-task listItem → "checked":null
	//   - one indented code block → "lang":null AND "meta":null
	//     (the fenced code block has lang:"go", meta:null — so meta:null
	//     appears TWICE total: once for indented, once for fenced)
	//   - one link without a title → "title":null
	// In compact form the substring is `"<key>":null`. In pretty form
	// json.Indent inserts a single space after the colon: `"<key>": null`.
	// We verify both forms separately to pin the never-elide rule against
	// either whitespace policy.
	checks := []struct {
		name           string
		compactSubstr  string
		prettySubstr   string
		wantOccurences int
	}{
		{"checked-null-non-task-listItem", `"checked":null`, `"checked": null`, 1},
		{"lang-null-indented-code", `"lang":null`, `"lang": null`, 1},
		{"meta-null-fenced-and-indented-code", `"meta":null`, `"meta": null`, 2},
		{"title-null-link-no-title", `"title":null`, `"title": null`, 1},
	}

	for _, c := range checks {
		t.Run(c.name+"-compact", func(t *testing.T) {
			got := bytes.Count(compact, []byte(c.compactSubstr))
			if got != c.wantOccurences {
				t.Errorf("compact output: substring %q occurred %d times, want %d\n  output: %s", c.compactSubstr, got, c.wantOccurences, compact)
			}
		})
		t.Run(c.name+"-pretty", func(t *testing.T) {
			got := bytes.Count(pretty, []byte(c.prettySubstr))
			if got != c.wantOccurences {
				t.Errorf("pretty output: substring %q occurred %d times, want %d\n  output: %s", c.prettySubstr, got, c.wantOccurences, pretty)
			}
		})
	}
}

// TestPrettyComposesWithNoPosition pins S10a acceptance criterion #2:
// `--pretty --no-position` strips the position key uniformly AND
// produces clean indentation (no leftover whitespace, no leftover
// trailing commas where position used to be). The property is exercised
// indirectly by fixture 57-pretty-title-and-bold (which uses both flags)
// but this unit test makes the cross-flag composition explicit: it
// asserts that pretty + no-position output contains ZERO `"position"`
// substrings AND is still valid pretty JSON (whitespace-stripping it
// yields valid compact JSON that round-trips through json.Indent
// byte-identically to the original pretty output).
func TestPrettyComposesWithNoPosition(t *testing.T) {
	src := []byte(representativeCorpus)

	pretty := emitOnce(t, src, emit.Options{NoPosition: true, Pretty: true})
	if n := bytes.Count(pretty, []byte(`"position"`)); n != 0 {
		t.Errorf("--pretty --no-position output contains %d `\"position\"` substrings (want 0)\n  output: %s", n, pretty)
	}

	// Round-trip: strip-whitespace → re-indent → must equal the original pretty
	// output. This verifies the pretty output is well-formed JSON with no
	// accidental trailing-comma artifacts that would have been left behind if
	// the no-position gate had been implemented incorrectly (e.g. by writing
	// the position-comma-and-key, then deleting only the position value).
	compactFromPretty := stripJSONWhitespace(pretty)
	var roundTrip bytes.Buffer
	if err := json.Indent(&roundTrip, compactFromPretty, "", "  "); err != nil {
		t.Fatalf("json.Indent on stripped pretty: %v", err)
	}
	if !bytes.Equal(roundTrip.Bytes(), pretty) {
		t.Errorf("round-trip strip → indent does not match original pretty output\n  pretty:    %q\n  roundTrip: %q", pretty, roundTrip.Bytes())
	}
}

// stripJSONWhitespace removes structural whitespace from a JSON byte stream
// without touching whitespace INSIDE string values. Walks the bytes and
// skips space / tab / LF / CR ONLY when not inside a `"..."` literal (with
// `\"` escape handling). The result is the equivalent compact JSON.
func stripJSONWhitespace(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b))
	inString := false
	escaped := false
	for _, c := range b {
		if inString {
			out.WriteByte(c)
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		// Not in a string: skip structural whitespace.
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		out.WriteByte(c)
		if c == '"' {
			inString = true
		}
	}
	return out.Bytes()
}
