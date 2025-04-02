// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package atomic

import (
	_ "embed"
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/token"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/inspect"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/internal/analysisutil"
	"github.com/tinygo-org/tinygo/x-tools/go/ast/inspector"
	"github.com/tinygo-org/tinygo/x-tools/go/types/typeutil"
	"github.com/tinygo-org/tinygo/x-tools/internal/analysisinternal"
)

//go:embed doc.go
var doc string

var Analyzer = &analysis.Analyzer{
	Name:             "atomic",
	Doc:              analysisutil.MustExtractDoc(doc, "atomic"),
	URL:              "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/atomic",
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	RunDespiteErrors: true,
	Run:              run,
}

func run(pass *analysis.Pass) (any, error) {
	if !analysisinternal.Imports(pass.Pkg, "sync/atomic") {
		return nil, nil // doesn't directly import sync/atomic
	}

	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
	}
	inspect.Preorder(nodeFilter, func(node ast.Node) {
		n := node.(*ast.AssignStmt)
		if len(n.Lhs) != len(n.Rhs) {
			return
		}
		if len(n.Lhs) == 1 && n.Tok == token.DEFINE {
			return
		}

		for i, right := range n.Rhs {
			call, ok := right.(*ast.CallExpr)
			if !ok {
				continue
			}
			obj := typeutil.Callee(pass.TypesInfo, call)
			if analysisinternal.IsFunctionNamed(obj, "sync/atomic", "AddInt32", "AddInt64", "AddUint32", "AddUint64", "AddUintptr") {
				checkAtomicAddAssignment(pass, n.Lhs[i], call)
			}
		}
	})
	return nil, nil
}

// checkAtomicAddAssignment walks the atomic.Add* method calls checking
// for assigning the return value to the same variable being used in the
// operation
func checkAtomicAddAssignment(pass *analysis.Pass, left ast.Expr, call *ast.CallExpr) {
	if len(call.Args) != 2 {
		return
	}
	arg := call.Args[0]
	broken := false

	gofmt := func(e ast.Expr) string { return analysisinternal.Format(pass.Fset, e) }

	if uarg, ok := arg.(*ast.UnaryExpr); ok && uarg.Op == token.AND {
		broken = gofmt(left) == gofmt(uarg.X)
	} else if star, ok := left.(*ast.StarExpr); ok {
		broken = gofmt(star.X) == gofmt(arg)
	}

	if broken {
		pass.ReportRangef(left, "direct assignment to atomic value")
	}
}
