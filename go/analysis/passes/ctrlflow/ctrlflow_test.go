// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ctrlflow_test

import (
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/ctrlflow"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, ctrlflow.Analyzer, "a", "typeparams")

	// Perform a minimal smoke test on
	// the result (CFG) computed by ctrlflow.
	for _, result := range results {
		cfgs := result.Result.(*ctrlflow.CFGs)

		for _, decl := range result.Action.Package.Syntax[0].Decls {
			if decl, ok := decl.(*ast.FuncDecl); ok && decl.Body != nil {
				if cfgs.FuncDecl(decl) == nil {
					t.Errorf("%s: no CFG for func %s",
						result.Action.Package.Fset.Position(decl.Pos()), decl.Name.Name)
				}
			}
		}
	}
}
