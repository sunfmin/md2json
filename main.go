// md2json2 reads a single Markdown document from stdin or a positional FILE
// and writes a JSON envelope of {"frontmatter": ..., "ast": ...} to stdout.
//
// This top-level main() is intentionally a thin shell: it injects the process
// globals (os.Args, os.Stdin/Stdout/Stderr) into cli.Run, which is the testable
// entry point. cli.Run never calls os.Exit; only this main() does.
package main

import (
	"os"

	"github.com/sunfmin/md2json2/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
