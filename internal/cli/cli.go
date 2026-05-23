// Package cli is the md2json entry point: it parses argv, opens input and
// output sinks, and drives the read → parse → translate → emit pipeline.
// Lower modules return typed errors; cli is the single place that turns those
// into the canonical stderr line + exit code mapping.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sunfmin/md2json/internal/emit"
	"github.com/sunfmin/md2json/internal/parse"
	"github.com/sunfmin/md2json/internal/read"
	"github.com/sunfmin/md2json/internal/translate"
)

// usageText is the canonical -h / --help message. It names every v1 flag
// (CONTEXT.md "v1 flags") and the positional FILE / `-` stdin sentinel
// convention (CONTEXT.md "CLI contract"). Sent to stdout because usage output
// is normal program output for `-h` (exit 0), not diagnostic.
const usageText = `Usage: md2json [FLAGS] [FILE]

Convert a single Markdown document (GFM + YAML frontmatter) to a JSON
envelope on stdout. Reads from FILE if given, otherwise from stdin; the
literal FILE=- is the explicit stdin sentinel.

Flags:
  -o, --output <FILE>    Write the JSON envelope to FILE instead of stdout.
      --pretty           Emit 2-space-indented JSON (default: compact).
      --no-position      Drop the position field from every node.
      --frontmatter-only Emit just the frontmatter value (or null); skip body parse.
  -h, --help             Show this help and exit.
  -V, --version          Show version and exit.

Exit codes:
  0  success
  1  parse / document-scoped error
  2  usage error (bad flag, missing or unreadable FILE)
`

// versionLine renders the canonical -V / --version output by composing the
// build-stamped `Version` package var (see version.go) into the line
// `md2json <Version>\n`. Pulled out of a `const` so the release workflow's
// `-ldflags "-X .../cli.Version=$TAG"` substitution takes effect at link
// time without touching the rendering call site. S13 acceptance criterion #1
// (the released binary's `-V` reflects the published tag).
func versionLine() string { return "md2json " + Version + "\n" }

// preInputPathToken is the `<path>` token used in the canonical stderr line
// for any usage error raised BEFORE an input source has been determined —
// unknown flag, missing flag value, unreadable `FILE` before any bytes are
// read. Per CONTEXT.md "Error format" + PRD user story 20: the literal
// program name `md2json` is the sentinel for "no source was ever in play."
// Distinct from `-` (stdin was the chosen source) and a real file path.
const preInputPathToken = "md2json"

// Run is the cli module's single entry point. It takes argv (argv[0] is the
// program name, per Unix convention), an input reader (the injected stdin),
// an output writer (the injected stdout), and an error writer (the injected
// stderr). It returns the process exit code; it never calls os.Exit.
// options collects the parsed v1 flag set. At S01 only the recognition matters
// — nothing reads these values yet. They are kept as fields so later slices
// thread them into read/parse/translate/emit without reshuffling cli.
//
// Invariant on the positional: `filePath` is the single source of truth. It is
// "" when no positional was seen (stdin source by default) and "-" when the
// explicit stdin sentinel was given; both collapse to "use stdin" at the source-
// resolution branch in Run. Any other value is a real on-disk path.
type options struct {
	output          string // -o / --output <FILE>; "" means stdout
	pretty          bool   // --pretty
	noPosition      bool   // --no-position
	frontmatterOnly bool   // --frontmatter-only
	help            bool   // -h / --help
	version         bool   // -V / --version
	filePath        string // positional FILE; "" or "-" means stdin
}

// parseArgs walks args (argv minus argv[0]) and returns the populated options
// plus, on failure, a usage-error message. The error message becomes the
// `(.+)` tail of the canonical stderr line; the caller renders the
// `md2json: md2json:0:0: <msg>\n` envelope and exits 2 (S12 criterion #4).
// Returning `usageErr=""` signals success.
func parseArgs(args []string) (opts options, usageErr string) {
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--":
			// End-of-flags sentinel. Everything after is positional.
			i++
			for ; i < len(args); i++ {
				opts.filePath = args[i]
			}
			return opts, ""
		case a == "-h" || a == "--help":
			opts.help = true
			i++
		case a == "-V" || a == "--version":
			opts.version = true
			i++
		case a == "--pretty":
			opts.pretty = true
			i++
		case a == "--no-position":
			opts.noPosition = true
			i++
		case a == "--frontmatter-only":
			opts.frontmatterOnly = true
			i++
		case a == "-o" || a == "--output":
			// Value in the next argv element.
			if i+1 >= len(args) {
				return opts, fmt.Sprintf("flag %s requires a value", a)
			}
			opts.output = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--output="):
			opts.output = strings.TrimPrefix(a, "--output=")
			i++
		case strings.HasPrefix(a, "-o="):
			opts.output = strings.TrimPrefix(a, "-o=")
			i++
		case a == "-":
			// Explicit stdin sentinel; equivalent to no positional.
			opts.filePath = a
			i++
		case len(a) > 0 && a[0] == '-':
			// Unknown flag.
			return opts, fmt.Sprintf("unknown flag %s", a)
		default:
			// Positional FILE. S01 accepts only one; if more are given, the
			// last one wins. Multi-file is explicitly out of scope per PRD.
			opts.filePath = a
			i++
		}
	}
	return opts, ""
}

func Run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// argv[0] is the program name (per Unix convention); the rest are the
	// real arguments. Skip argv[0]; treat a missing argv defensively.
	var args []string
	if len(argv) > 1 {
		args = argv[1:]
	}

	opts, usageErr := parseArgs(args)
	if usageErr != "" {
		return writePreInputUsageError(stderr, usageErr)
	}

	// -h / -V short-circuit successfully on stdout (CLI contract: usage and
	// version are normal program output, not diagnostic; exit 0).
	if opts.help {
		_, _ = io.WriteString(stdout, usageText)
		return 0
	}
	if opts.version {
		_, _ = io.WriteString(stdout, versionLine())
		return 0
	}

	// Resolve the input source: positional FILE if it is a real path, else
	// stdin (which covers both no-positional and the "-" sentinel). The path
	// token used in any read-stage error line is the file path when reading
	// from disk and the literal "-" when reading from stdin (per the PRD's
	// error-format contract and S02 acceptance criterion #3).
	src := stdin
	pathToken := "-"
	if opts.filePath != "" && opts.filePath != "-" {
		f, err := os.Open(opts.filePath)
		if err != nil {
			// Pre-input usage error: the file could not be opened, so no
			// bytes have been read.
			return writePreInputUsageError(stderr, err.Error())
		}
		defer f.Close()
		src = f
		pathToken = opts.filePath
	}

	// Resolve the output sink. `-o/--output <FILE>` (opts.output non-empty)
	// writes the envelope to that file (creating or truncating) instead of
	// stdout (S12 criterion #1). A failure to open the output file is treated
	// as a pre-input usage error per the error-format/exit-code mapping —
	// no bytes were ever read from input, so `<path>` stays `md2json` and
	// the exit code is 2.
	out := stdout
	if opts.output != "" {
		f, err := os.Create(opts.output)
		if err != nil {
			return writePreInputUsageError(stderr, err.Error())
		}
		defer f.Close()
		out = f
	}

	// Read + normalize the input. On a typed read-stage error, route it through
	// the canonical stderr line and exit 1 with nothing on stdout.
	srcBytes, err := read.Read(src, pathToken)
	if err != nil {
		var re *read.ReadError
		if errors.As(err, &re) {
			writePositionedError(stderr, re.Path, re.Line, re.Col, re.Msg)
		} else {
			writeDocScopedError(stderr, pathToken, err)
		}
		return 1
	}

	// Parse the normalized bytes. goldmark is configured with GFM, footnote,
	// and YAML-only frontmatter extensions registered (see internal/parse).
	pr, err := parse.Parse(srcBytes)
	if err != nil {
		// The malformed-frontmatter path returns a typed error carrying
		// source-relative line/col so we can render the canonical
		// `md2json: <path>:<line>:<col>: invalid frontmatter: <msg>` stderr
		// line per CONTEXT.md "Invalid frontmatter (policy)" + PRD US 21
		// (S09 criterion #3). Other parse-stage errors are document-scoped
		// and use the `:0:0:` sentinel.
		var ife *parse.InvalidFrontmatterError
		if errors.As(err, &ife) {
			writePositionedError(stderr, pathToken, ife.Line, ife.Col, ife.Error())
		} else {
			writeDocScopedError(stderr, pathToken, err)
		}
		return 1
	}

	// --frontmatter-only short-circuits BEFORE translate per the PRD pipeline
	// rule: "cli calls read, then parse, then (unless --frontmatter-only
	// short-circuits) translate, then emit."
	if opts.frontmatterOnly {
		eopts := emit.Options{NoPosition: opts.noPosition, FrontmatterOnly: true, Pretty: opts.pretty}
		if err := emit.Emit(out, pr.Frontmatter, nil, eopts); err != nil {
			writeDocScopedError(stderr, pathToken, err)
			return 1
		}
		return 0
	}

	// Translate goldmark → mdast-shaped Go value tree, then emit.
	root := translate.Translate(pr.Doc, srcBytes, translate.Options{})
	eopts := emit.Options{NoPosition: opts.noPosition, Pretty: opts.pretty}
	if err := emit.Emit(out, pr.Frontmatter, root, eopts); err != nil {
		writeDocScopedError(stderr, pathToken, err)
		return 1
	}
	return 0
}

// writePositionedError renders the canonical stderr line for an error that
// carries a known source line/column, per CONTEXT.md "Error format":
// `^md2json: ([^:]+):(\d+):(\d+): (.+)$`. line and col are 1-indexed; pass
// `0, 0` only via writeDocScopedError, which handles the document-scoped
// sentinel semantics separately.
//
// This is the single rendering point for the canonical stderr template; all
// typed-error branches in Run that have real line/col info flow through here
// rather than re-rendering the format inline. A future slice that introduces
// another typed error from parse / emit / translate with real line/col info
// should plug an `errors.As` branch into Run that calls this helper.
func writePositionedError(stderr io.Writer, pathToken string, line, col int, msg string) {
	// Column-rounding rule (CONTEXT.md "Error format"; S12 criterion #7):
	// when an error carries a line but no column (col == 0 while line > 0),
	// the column rounds UP to 1 — never `:0:` for a real line, because
	// lines/columns are 1-indexed elsewhere. The `:0:0:` sentinel is
	// reserved for the document-scoped no-position case where BOTH line and
	// column are absent.
	if line > 0 && col < 1 {
		col = 1
	}
	fmt.Fprintf(stderr, "md2json: %s:%d:%d: %s\n", pathToken, line, col, msg)
}

// writeDocScopedError renders the canonical stderr line for a document-scoped
// error (no specific line/column available), using the `:0:0:` sentinel per
// CONTEXT.md "Error format": *"When the error is document-scoped with no
// position at all, use the sentinel `<path>:0:0:` — the same regex still
// matches and `0:0` conventionally means 'no position available.'"*
//
// Delegates to writePositionedError with `line=0, col=0` so the canonical
// template lives in exactly one place. The named wrapper preserves the
// glossary term ("document-scoped") at call sites.
func writeDocScopedError(stderr io.Writer, pathToken string, err error) {
	writePositionedError(stderr, pathToken, 0, 0, err.Error())
}

// writePreInputUsageError renders the canonical stderr line for a pre-input
// usage error — a failure raised BEFORE any input source has been determined
// (unknown flag, missing flag value, unreadable input FILE, uncreatable output
// FILE). Per CONTEXT.md "Error format" + PRD US20 the `<path>` token is the
// literal program name `md2json` (no source ever in play), the position is
// the `:0:0:` sentinel, and the exit code is 2 (usage error, distinct from
// 1 = document-scoped parse error). Returns the exit code so call sites read
// `return writePreInputUsageError(stderr, msg)` and the (path-token, position,
// exit-code) triple lives in exactly one place.
func writePreInputUsageError(stderr io.Writer, msg string) int {
	writePositionedError(stderr, preInputPathToken, 0, 0, msg)
	return 2
}
