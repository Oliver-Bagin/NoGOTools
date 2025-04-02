// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noresultvalues

import (
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"strings"

	_ "embed"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/inspect"
	"github.com/tinygo-org/tinygo/x-tools/go/ast/inspector"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/util/moreiters"
	"github.com/tinygo-org/tinygo/x-tools/internal/analysisinternal"
	"github.com/tinygo-org/tinygo/x-tools/internal/astutil/cursor"
	"github.com/tinygo-org/tinygo/x-tools/internal/typesinternal"
)

//go:embed doc.go
var doc string

var Analyzer = &analysis.Analyzer{
	Name:             "noresultvalues",
	Doc:              analysisinternal.MustExtractDoc(doc, "noresultvalues"),
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	Run:              run,
	RunDespiteErrors: true,
	URL:              "https://pkg.go.dev/golang.org/x/tools/gopls/internal/analysis/noresultvalues",
}

func run(pass *analysis.Pass) (any, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	for _, typErr := range pass.TypeErrors {
		if !fixesError(typErr.Msg) {
			continue // irrelevant error
		}
		_, start, end, ok := typesinternal.ErrorCodeStartEnd(typErr)
		if !ok {
			continue // can't get position info
		}
		curErr, ok := cursor.Root(inspect).FindPos(start, end)
		if !ok {
			continue // can't find errant node
		}
		// Find first enclosing return statement, if any.
		if curRet, ok := moreiters.First(curErr.Enclosing((*ast.ReturnStmt)(nil))); ok {
			ret := curRet.Node()
			pass.Report(analysis.Diagnostic{
				Pos:     start,
				End:     end,
				Message: typErr.Msg,
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: "Delete return values",
					TextEdits: []analysis.TextEdit{{
						Pos: ret.Pos() + token.Pos(len("return")),
						End: ret.End(),
					}},
				}},
			})
		}
	}
	return nil, nil
}

func fixesError(msg string) bool {
	return msg == "no result values expected" ||
		strings.HasPrefix(msg, "too many return values") && strings.Contains(msg, "want ()")
}
