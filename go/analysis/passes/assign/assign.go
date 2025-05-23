// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package assign

// TODO(adonovan): check also for assignments to struct fields inside
// methods that are on T instead of *T.

import (
	_ "embed"
	"fmt"
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"github.com/tinygo-org/tinygo/alt_go/types"
	"reflect"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/inspect"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/internal/analysisutil"
	"github.com/tinygo-org/tinygo/x-tools/go/ast/inspector"
	"github.com/tinygo-org/tinygo/x-tools/internal/analysisinternal"
)

//go:embed doc.go
var doc string

var Analyzer = &analysis.Analyzer{
	Name:     "assign",
	Doc:      analysisutil.MustExtractDoc(doc, "assign"),
	URL:      "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/assign",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
	}
	inspect.Preorder(nodeFilter, func(n ast.Node) {
		stmt := n.(*ast.AssignStmt)
		if stmt.Tok != token.ASSIGN {
			return // ignore :=
		}
		if len(stmt.Lhs) != len(stmt.Rhs) {
			// If LHS and RHS have different cardinality, they can't be the same.
			return
		}
		for i, lhs := range stmt.Lhs {
			rhs := stmt.Rhs[i]
			if analysisutil.HasSideEffects(pass.TypesInfo, lhs) ||
				analysisutil.HasSideEffects(pass.TypesInfo, rhs) ||
				isMapIndex(pass.TypesInfo, lhs) {
				continue // expressions may not be equal
			}
			if reflect.TypeOf(lhs) != reflect.TypeOf(rhs) {
				continue // short-circuit the heavy-weight gofmt check
			}
			le := analysisinternal.Format(pass.Fset, lhs)
			re := analysisinternal.Format(pass.Fset, rhs)
			if le == re {
				pass.Report(analysis.Diagnostic{
					Pos: stmt.Pos(), Message: fmt.Sprintf("self-assignment of %s to %s", re, le),
					SuggestedFixes: []analysis.SuggestedFix{{
						Message: "Remove self-assignment",
						TextEdits: []analysis.TextEdit{{
							Pos: stmt.Pos(),
							End: stmt.End(),
						}}},
					},
				})
			}
		}
	})

	return nil, nil
}

// isMapIndex returns true if e is a map index expression.
func isMapIndex(info *types.Info, e ast.Expr) bool {
	if idx, ok := ast.Unparen(e).(*ast.IndexExpr); ok {
		if typ := info.Types[idx.X].Type; typ != nil {
			_, ok := typ.Underlying().(*types.Map)
			return ok
		}
	}
	return false
}
