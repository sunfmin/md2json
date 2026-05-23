// Package parse wraps github.com/yuin/goldmark with the v1 enabled extension
// set (GFM, footnote, YAML frontmatter) and produces a goldmark AST plus a
// frontmatter value lifted out of the document context.
//
// Frontmatter contract (CONTEXT.md "Frontmatter", "Invalid frontmatter
// (policy)"; PRD user stories 21, 22, 28):
//
//   - A document opens with `---` on line 1 → potential YAML frontmatter.
//     We pre-scan the (normalized) bytes once to decide: closed fence,
//     unclosed fence, or no frontmatter at all. The pre-scan is necessary
//     because `go.abhg.dev/goldmark/frontmatter`'s open-without-close
//     behavior is "greedy consume to EOF + try to YAML-parse the body" —
//     which silently lifts body content as frontmatter when the body
//     happens to be valid YAML (e.g. `---\ntitle: x\n` becomes
//     `{"title":"x"}` with empty body). The PRD/CONTEXT rule is the
//     opposite: an unclosed opening fence is NOT frontmatter; the whole
//     document — including the leading `---` line — parses as body and
//     `frontmatter` stays `nil` with exit 0.
//
//   - Closed fence with parseable YAML → lift the value (map or scalar)
//     onto the envelope, strip the block from the body. Handled by
//     `go.abhg.dev/goldmark/frontmatter` with `Formats: [YAML]`.
//
//   - Closed fence with malformed YAML → hard error. The yaml.v3 decoder's
//     `yaml: line N: <msg>` is reshaped into a typed `InvalidFrontmatter`
//     carrying the source line/column so cli can render the canonical
//     `md2json2: <path>:<line>:<col>: invalid frontmatter: <msg>` stderr
//     line and exit 1. yaml.v3 doesn't carry a column, so column falls
//     back to 1 per CONTEXT.md "Error format" (`round unknown column up
//     to 1, never 0`).
//
//   - No frontmatter at all (no `---` at line 1) → goldmark sees the
//     bytes unchanged; `frontmatter` is `nil`.
//
// We construct two goldmark instances rather than one (see `New` and
// `newWithoutFrontmatter`): with the frontmatter extension for the
// closed/no-frontmatter paths, and without it for the unclosed-fence path
// (so the leading `---` line becomes a CommonMark `ThematicBreak` rather
// than the trigger for greedy frontmatter consumption).
package parse

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/frontmatter"
	"gopkg.in/yaml.v3"
)

// New constructs a goldmark.Markdown with the v1 enabled extension set:
// GFM (tables, task lists, strikethrough, autolinks), footnotes, and
// YAML-only frontmatter (TOML is intentionally excluded per the PRD's
// v1 non-goal list). Exposed so a consumer (or test) can inspect the
// configured extension set if needed.
func New() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			&frontmatter.Extender{
				Formats: []frontmatter.Format{frontmatter.YAML},
				// Mode is intentionally zero (default): we decode frontmatter
				// manually via frontmatter.Get on the parser.Context so the
				// final shape on the wire is owned by parse, not by goldmark's
				// document metadata cache.
			},
		),
	)
}

// newWithoutFrontmatter constructs the same goldmark.Markdown as `New` but
// without the frontmatter extender registered. Used for the unclosed-fence
// code path: with the extender disabled, a leading `---` line parses as a
// CommonMark `ThematicBreak` (the body-only interpretation) rather than as
// the opening of a greedy frontmatter consumer.
func newWithoutFrontmatter() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
		),
	)
}

// Result is the typed return of Parse: the goldmark document root plus the
// frontmatter value decoded out of the parser context.
//
// Frontmatter is `any` because YAML scalars (string/number/bool/null) and
// YAML maps both flow through here; emit handles the JSON shape.
type Result struct {
	Doc         *ast.Document
	Frontmatter any
}

// InvalidFrontmatterError is the typed error returned when a document opens
// with a closed `---` fence but the YAML between the fences fails to parse.
// cli converts this into the canonical
// `md2json2: <path>:<line>:<col>: invalid frontmatter: <msg>` stderr line
// and exit 1 (per CONTEXT.md "Invalid frontmatter (policy)" and PRD user
// story 21).
//
// Line is 1-indexed in the source as the user sees it (the line number
// inside the document where the YAML error occurs — NOT inside the YAML
// region alone). Col is 1-indexed but yaml.v3 does not carry column info,
// so we always fall back to 1 per CONTEXT.md "Error format" (`round
// unknown column up to 1, never 0`). Msg is the yaml.v3 message with its
// own `yaml: line N: ` prefix stripped, since that information is already
// embedded in Line.
type InvalidFrontmatterError struct {
	Line int
	Col  int
	Msg  string
}

func (e *InvalidFrontmatterError) Error() string {
	return fmt.Sprintf("invalid frontmatter: %s", e.Msg)
}

// frontmatterState reports what the pre-scan found at the top of the
// (normalized) source. closedFence is the happy path; unclosedFence is the
// "treat as body" rule; noFrontmatter is "source doesn't even start with
// `---`."
type frontmatterState int

const (
	noFrontmatter frontmatterState = iota
	closedFence
	unclosedFence
)

// scanResult records what the pre-scan found. yamlStartLine is the
// 1-indexed source line of the YAML region's first line (always 2 when
// state == closedFence, since the opening fence is on line 1); used by
// mapYAMLError to translate YAML-region-relative line numbers into
// source-relative ones. For the other states it is zero.
type scanResult struct {
	state         frontmatterState
	yamlStartLine int
}

// preScan walks the normalized source once and reports the frontmatter
// state. It does NOT decode YAML; it only finds fence boundaries.
//
// A frontmatter fence is a line consisting of exactly N dashes (N >= 3)
// and nothing else (matching goldmark/frontmatter's `lineDelim`). The
// opening fence must be at line 1; the closing fence is the next line
// after line 1 with the SAME dash count. If no matching closing fence
// is found before EOF, the state is unclosedFence (body-only rule).
func preScan(src []byte) scanResult {
	// Find the end of line 1 (the first '\n', or EOF).
	nl := bytes.IndexByte(src, '\n')
	var firstLine []byte
	if nl < 0 {
		firstLine = src
	} else {
		firstLine = src[:nl]
	}
	openCount := dashFenceCount(firstLine)
	if openCount < 3 {
		return scanResult{state: noFrontmatter}
	}
	if nl < 0 {
		// Edge case: source is exactly the opening fence with no trailing
		// newline. Unclosed by definition.
		return scanResult{state: unclosedFence}
	}
	// Scan subsequent lines for an exact-match closing fence.
	i := nl + 1
	for i < len(src) {
		nlIdx := bytes.IndexByte(src[i:], '\n')
		var line []byte
		var nextStart int
		if nlIdx < 0 {
			line = src[i:]
			nextStart = len(src)
		} else {
			line = src[i : i+nlIdx]
			nextStart = i + nlIdx + 1
		}
		if dashFenceCount(line) == openCount {
			return scanResult{state: closedFence, yamlStartLine: 2}
		}
		i = nextStart
	}
	return scanResult{state: unclosedFence}
}

// dashFenceCount returns N if `line` is exactly N >= 3 dash characters,
// else 0. Mirrors goldmark/frontmatter's `lineDelim` for the YAML format
// (which uses '-' as the delimiter) so our pre-scan agrees with goldmark
// on what counts as a fence line.
func dashFenceCount(line []byte) int {
	if len(line) < 3 {
		return 0
	}
	for _, c := range line {
		if c != '-' {
			return 0
		}
	}
	return len(line)
}

// yamlLineRe extracts the `line N` prefix from yaml.v3's error messages of
// the form `yaml: line N: <message>` (see gopkg.in/yaml.v3 yaml.go failf).
// Used to translate the YAML-region-relative line number into the
// source-relative line number on the canonical stderr line.
var yamlLineRe = regexp.MustCompile(`^yaml: line (\d+): (.*)$`)

// yamlTypeErrorLineRe extracts the `line N` prefix from a yaml.v3
// TypeError's per-entry strings, which have the form
// `line N: <message>` (no `yaml: ` prefix — that lives on the outer error).
// Duplicate-key, type-mismatch, and similar semantic errors land in
// TypeError rather than the parse/scan error path.
var yamlTypeErrorLineRe = regexp.MustCompile(`^line (\d+): (.*)$`)

// Parse runs the configured goldmark parser over the supplied (normalized)
// bytes and returns the root node plus the lifted frontmatter value.
//
// The frontmatter contract (closed-fence happy path, unclosed-fence
// body-only rule, malformed-YAML hard error) is enforced via the pre-scan
// described on the package comment.
func Parse(src []byte) (Result, error) {
	scan := preScan(src)

	if scan.state == unclosedFence {
		// Body-only rule: parse the entire source with the frontmatter
		// extension OFF so the leading `---` parses as a CommonMark
		// ThematicBreak and the rest as normal body content.
		md := newWithoutFrontmatter()
		root := md.Parser().Parse(text.NewReader(src))
		doc, _ := root.(*ast.Document)
		return Result{Doc: doc, Frontmatter: nil}, nil
	}

	// closedFence or noFrontmatter — both use the with-frontmatter parser.
	// In the closedFence case the extender will lift the fenced block and
	// strip it from the body; we then YAML-decode the raw bytes ourselves
	// so the error path can carry source-relative line/col info.
	md := New()
	ctx := parser.NewContext()
	reader := text.NewReader(src)
	root := md.Parser().Parse(reader, parser.WithContext(ctx))
	doc, _ := root.(*ast.Document)

	var fm any
	if data := frontmatter.Get(ctx); data != nil {
		var v any
		if err := data.Decode(&v); err != nil {
			return Result{}, mapYAMLError(err, scan.yamlStartLine)
		}
		fm = v
	}

	return Result{Doc: doc, Frontmatter: fm}, nil
}

// mapYAMLError reshapes a yaml.v3 decode error into our typed
// InvalidFrontmatterError. yaml.v3 has two error shapes:
//
//  1. Parse/scan errors render as `yaml: line N: <message>` on a single
//     line. Extracted via yamlLineRe.
//  2. *yaml.TypeError carries one or more entries of the form
//     `line N: <message>` joined under a `yaml: unmarshal errors:\n` header
//     with one indented entry per line. Duplicate-key, mapping-into-wrong-
//     type, and similar semantic errors land here. The entries' raw line
//     numbers are YAML-region-relative; we extract the first entry's line
//     number and join the rest with `; ` so the canonical stderr regex
//     (one line) still matches.
//
// In both flavors the extracted `line N` is 1-indexed within the YAML
// region; we add (yamlStartLine - 1) to translate to source coordinates.
// yaml.v3 carries no column info, so Col falls back to 1 per CONTEXT.md
// "Error format" (round unknown column up to 1, never 0).
func mapYAMLError(err error, yamlStartLine int) error {
	srcLine := yamlStartLine
	var cleanMsg string

	if te, ok := err.(*yaml.TypeError); ok && len(te.Errors) > 0 {
		// TypeError: flatten the entries into a single-line message and
		// extract the first entry's line number for the canonical position.
		parts := make([]string, 0, len(te.Errors))
		for i, entry := range te.Errors {
			if m := yamlTypeErrorLineRe.FindStringSubmatch(entry); m != nil {
				if i == 0 {
					if n, perr := strconv.Atoi(m[1]); perr == nil {
						srcLine = yamlStartLine + n - 1
					}
				}
				parts = append(parts, m[2])
			} else {
				parts = append(parts, entry)
			}
		}
		cleanMsg = strings.Join(parts, "; ")
	} else {
		msg := err.Error()
		cleanMsg = msg
		if m := yamlLineRe.FindStringSubmatch(msg); m != nil {
			if n, perr := strconv.Atoi(m[1]); perr == nil {
				srcLine = yamlStartLine + n - 1
			}
			cleanMsg = m[2]
		}
		// Defensive: any stray newline in the message would break the
		// canonical stderr regex (one line per error). Flatten.
		cleanMsg = strings.ReplaceAll(cleanMsg, "\n", "; ")
	}

	return &InvalidFrontmatterError{
		Line: srcLine,
		Col:  1,
		Msg:  cleanMsg,
	}
}
