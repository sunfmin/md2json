package main_test

// Acceptance criterion #5 (S03): bytes flow read → parse → translate → emit;
// no module reaches into process globals (os.Args, os.Stdin/Stdout/Stderr,
// os.Exit) — these are injected at the top level only.
//
// This is a structural test: it walks every .go file under the module root
// (excluding the top-level main.go, which is the ONE allowed entry point for
// process globals per the PRD's "process globals are injected at the top
// level" rule) and asserts none of them references the forbidden identifiers.
//
// If a future refactor accidentally introduces an `os.Exit(1)` deep inside
// a module — or has the `read` module reach into `os.Stdin` directly — this
// test fails before the structural invariant rots.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoProcessGlobalsOutsideMain(t *testing.T) {
	forbidden := []string{
		"os.Args",
		"os.Stdin",
		"os.Stdout",
		"os.Stderr",
		"os.Exit",
	}

	root := "."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip vendor and any hidden dirs (e.g. .rundays); also skip
			// testdata so fixture inputs don't get scanned.
			base := filepath.Base(path)
			if base == "vendor" || base == "testdata" || (len(base) > 1 && base[0] == '.') {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// main.go at the module root is the ONE allowed file.
		if path == "main.go" || path == "./main.go" {
			return nil
		}
		// Skip test files: tests legitimately call os.ReadFile, os.MkdirTemp,
		// etc.; the contract is about production code reaching into process
		// globals as injected-IO substitutes, not about test plumbing.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the file's Go syntax and inspect SelectorExprs of the form
		// `os.<ident>` — comments and string literals are ignored by the
		// parser, so doc-strings mentioning os.Exit don't trigger false
		// positives.
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		forbiddenSet := map[string]bool{}
		for _, n := range forbidden {
			// forbidden entries are "os.<name>"; key the set by <name>.
			parts := strings.SplitN(n, ".", 2)
			if len(parts) == 2 {
				forbiddenSet[parts[1]] = true
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name != "os" {
				return true
			}
			if forbiddenSet[sel.Sel.Name] {
				t.Errorf("forbidden process-global reference os.%s in %s; only main.go may touch process globals (PRD: \"process globals are injected at the top level\")", sel.Sel.Name, path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
