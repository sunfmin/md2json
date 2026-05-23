// Package cli is the md2json2 entry point: it parses argv, opens input and
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

	"github.com/sunfmin/md2json2/internal/emit"
	"github.com/sunfmin/md2json2/internal/parse"
	"github.com/sunfmin/md2json2/internal/read"
	"github.com/sunfmin/md2json2/internal/translate"
)

// Run is the cli module's single entry point. It takes argv (argv[0] is the
// program name, per Unix convention), an input reader (the injected stdin),
// an output writer (the injected stdout), and an error writer (the injected
// stderr). It returns the process exit code; it never calls os.Exit.
// options collects the parsed v1 flag set. At S01 only the recognition matters
// — nothing reads these values yet. They are kept as fields so later slices
// thread them into read/parse/translate/emit without reshuffling cli.
type options struct {
	output          string // -o / --output <FILE>; "" means stdout
	pretty          bool   // --pretty
	noPosition      bool   // --no-position
	frontmatterOnly bool   // --frontmatter-only
	help            bool   // -h / --help
	version         bool   // -V / --version
	filePath        string // positional FILE; "" or "-" means stdin
	hasPositional   bool   // true once a positional has been seen (incl. "-")
}

// parseArgs walks args (argv minus argv[0]) and returns the populated options
// plus an "ok" flag. The PRD's contract is "known flags recognized, unknown
// flags rejected." S01 does not need to render a polished error message
// (later slices pin exit code 2 + the canonical stderr regex); we only need
// the unknown-flag branch to return a non-ok signal so Run can exit non-zero.
func parseArgs(args []string) (opts options, ok bool) {
	ok = true
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--":
			// End-of-flags sentinel. Everything after is positional.
			i++
			for ; i < len(args); i++ {
				opts.hasPositional = true
				opts.filePath = args[i]
			}
			return opts, ok
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
				return opts, false
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
			opts.hasPositional = true
			opts.filePath = a
			i++
		case len(a) > 0 && a[0] == '-':
			// Unknown flag.
			return opts, false
		default:
			// Positional FILE. S01 accepts only one; if more are given, the
			// last one wins. Multi-file is explicitly out of scope per PRD.
			opts.hasPositional = true
			opts.filePath = a
			i++
		}
	}
	return opts, ok
}

func Run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// argv[0] is the program name (per Unix convention); the rest are the
	// real arguments. Skip argv[0]; treat a missing argv defensively.
	var args []string
	if len(argv) > 1 {
		args = argv[1:]
	}

	opts, ok := parseArgs(args)
	if !ok {
		return 1
	}

	// -h and -V short-circuit successfully. S01 emits placeholder text on
	// stdout; later slices (S11) pin the exact bytes.
	if opts.help {
		_, _ = io.WriteString(stdout, "md2json2: help (placeholder)\n")
		return 0
	}
	if opts.version {
		_, _ = io.WriteString(stdout, "md2json2: version (placeholder)\n")
		return 0
	}

	// Resolve the input source: positional FILE if it is a real path, else
	// stdin (which covers both no-positional and the "-" sentinel). The path
	// token used in any read-stage error line is the file path when reading
	// from disk and the literal "-" when reading from stdin (per the PRD's
	// error-format contract and S02 acceptance criterion #3).
	src := stdin
	pathToken := "-"
	if opts.hasPositional && opts.filePath != "" && opts.filePath != "-" {
		f, err := os.Open(opts.filePath)
		if err != nil {
			return 1
		}
		defer f.Close()
		src = f
		pathToken = opts.filePath
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
		// `md2json2: <path>:<line>:<col>: invalid frontmatter: <msg>` stderr
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
		if err := emit.Emit(stdout, pr.Frontmatter, nil, eopts); err != nil {
			writeDocScopedError(stderr, pathToken, err)
			return 1
		}
		return 0
	}

	// Translate goldmark → mdast-shaped Go value tree, then emit.
	root := translate.Translate(pr.Doc, srcBytes, translate.Options{})
	eopts := emit.Options{NoPosition: opts.noPosition, Pretty: opts.pretty}
	if err := emit.Emit(stdout, pr.Frontmatter, root, eopts); err != nil {
		writeDocScopedError(stderr, pathToken, err)
		return 1
	}
	return 0
}

// writePositionedError renders the canonical stderr line for an error that
// carries a known source line/column, per CONTEXT.md "Error format":
// `^md2json2: ([^:]+):(\d+):(\d+): (.+)$`. line and col are 1-indexed; pass
// `0, 0` only via writeDocScopedError, which handles the document-scoped
// sentinel semantics separately.
//
// This is the single rendering point for the canonical stderr template; all
// typed-error branches in Run that have real line/col info flow through here
// rather than re-rendering the format inline. A future slice that introduces
// another typed error from parse / emit / translate with real line/col info
// should plug an `errors.As` branch into Run that calls this helper.
func writePositionedError(stderr io.Writer, pathToken string, line, col int, msg string) {
	fmt.Fprintf(stderr, "md2json2: %s:%d:%d: %s\n", pathToken, line, col, msg)
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
