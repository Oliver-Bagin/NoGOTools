// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package defers

import (
	_ "embed"
	"github.com/tinygo-org/tinygo/alt_go/ast"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/inspect"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/internal/analysisutil"
	"github.com/tinygo-org/tinygo/x-tools/go/ast/inspector"
	"github.com/tinygo-org/tinygo/x-tools/go/types/typeutil"
	"github.com/tinygo-org/tinygo/x-tools/internal/analysisinternal"
)

//go:embed doc.go
var doc string

// Analyzer is the defers analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "defers",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	URL:      "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/defers",
	Doc:      analysisutil.MustExtractDoc(doc, "defers"),
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	if !analysisinternal.Imports(pass.Pkg, "time") {
		return nil, nil
	}

	checkDeferCall := func(node ast.Node) bool {
		switch v := node.(type) {
		case *ast.CallExpr:
			if analysisinternal.IsFunctionNamed(typeutil.Callee(pass.TypesInfo, v), "time", "Since") {
				pass.Reportf(v.Pos(), "call to time.Since is not deferred")
			}
		case *ast.FuncLit:
			return false // prune
		}
		return true
	}

	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.DeferStmt)(nil),
	}

	inspect.Preorder(nodeFilter, func(n ast.Node) {
		d := n.(*ast.DeferStmt)
		ast.Inspect(d.Call, checkDeferCall)
	})

	return nil, nil
}
