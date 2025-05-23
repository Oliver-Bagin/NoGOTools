// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package checker_test

import (
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/internal/checker"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/inspect"
	"github.com/tinygo-org/tinygo/x-tools/go/ast/inspector"
	"github.com/tinygo-org/tinygo/x-tools/internal/testenv"
)

// TestStartFixes make sure modifying the first character
// of the file takes effect.
func TestStartFixes(t *testing.T) {
	testenv.NeedsGoPackages(t)
	testenv.RedirectStderr(t) // associated checker.Run output with this test

	files := map[string]string{
		"comment/doc.go": `/* Package comment */
package comment
`}

	want := `// Package comment
package comment
`

	testdata, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(testdata, "src/comment/doc.go")
	checker.Fix = true
	checker.Run([]string{"file=" + path}, []*analysis.Analyzer{commentAnalyzer})
	checker.Fix = false

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got := string(contents)
	if got != want {
		t.Errorf("contents of rewritten file\ngot: %s\nwant: %s", got, want)
	}

	defer cleanup()
}

var commentAnalyzer = &analysis.Analyzer{
	Name:     "comment",
	Doc:      "comment",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      commentRun,
}

func commentRun(pass *analysis.Pass) (any, error) {
	const (
		from = "/* Package comment */"
		to   = "// Package comment"
	)
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	inspect.Preorder(nil, func(n ast.Node) {
		if n, ok := n.(*ast.Comment); ok && n.Text == from {
			pass.Report(analysis.Diagnostic{
				Pos: n.Pos(),
				End: n.End(),
				SuggestedFixes: []analysis.SuggestedFix{{
					TextEdits: []analysis.TextEdit{{
						Pos:     n.Pos(),
						End:     n.End(),
						NewText: []byte(to),
					}},
				}},
			})
		}
	})

	return nil, nil
}
