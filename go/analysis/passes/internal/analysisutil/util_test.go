// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package analysisutil_test

import (
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/parser"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"github.com/tinygo-org/tinygo/alt_go/types"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/internal/analysisutil"
)

func TestHasSideEffects(t *testing.T) {
	src := `package p

type T int

type G[P any] int

func _() {
	var x int
	_ = T(x)
	_ = G[int](x)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var conf types.Config
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	_, err = conf.Check("", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if got := analysisutil.HasSideEffects(info, call); got != false {
			t.Errorf("HasSideEffects(%s) = true, want false", types.ExprString(call))
		}
		return true
	})
}
