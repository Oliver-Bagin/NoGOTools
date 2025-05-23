// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package golang

import (
	"fmt"
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"strings"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis"
	"github.com/tinygo-org/tinygo/x-tools/go/ast/astutil"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/cache"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/cache/parsego"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/util/safetoken"
	"github.com/tinygo-org/tinygo/x-tools/internal/astutil/cursor"
)

// invertIfCondition is a singleFileFixFunc that inverts an if/else statement
func invertIfCondition(pkg *cache.Package, pgf *parsego.File, start, end token.Pos) (*token.FileSet, *analysis.SuggestedFix, error) {
	var (
		fset = pkg.FileSet()
		src  = pgf.Src
	)

	ifStatement, _, err := canInvertIfCondition(pgf.Cursor, start, end)
	if err != nil {
		return nil, nil, err
	}

	var replaceElse analysis.TextEdit

	endsWithReturn, err := endsWithReturn(ifStatement.Else)
	if err != nil {
		return nil, nil, err
	}

	if endsWithReturn {
		// Replace the whole else part with an empty line and an unindented
		// version of the original if body
		sourcePos := safetoken.StartPosition(fset, ifStatement.Pos())

		indent := sourcePos.Column - 1
		if indent < 0 {
			indent = 0
		}

		standaloneBodyText := ifBodyToStandaloneCode(fset, ifStatement.Body, src)
		replaceElse = analysis.TextEdit{
			Pos:     ifStatement.Body.Rbrace + 1, // 1 == len("}")
			End:     ifStatement.End(),
			NewText: []byte("\n\n" + strings.Repeat("\t", indent) + standaloneBodyText),
		}
	} else {
		// Replace the else body text with the if body text
		bodyStart := safetoken.StartPosition(fset, ifStatement.Body.Lbrace)
		bodyEnd := safetoken.EndPosition(fset, ifStatement.Body.Rbrace+1) // 1 == len("}")
		bodyText := src[bodyStart.Offset:bodyEnd.Offset]
		replaceElse = analysis.TextEdit{
			Pos:     ifStatement.Else.Pos(),
			End:     ifStatement.Else.End(),
			NewText: bodyText,
		}
	}

	// Replace the if text with the else text
	elsePosInSource := safetoken.StartPosition(fset, ifStatement.Else.Pos())
	elseEndInSource := safetoken.EndPosition(fset, ifStatement.Else.End())
	elseText := src[elsePosInSource.Offset:elseEndInSource.Offset]
	replaceBodyWithElse := analysis.TextEdit{
		Pos:     ifStatement.Body.Pos(),
		End:     ifStatement.Body.End(),
		NewText: elseText,
	}

	// Replace the if condition with its inverse
	inverseCondition, err := invertCondition(fset, ifStatement.Cond, src)
	if err != nil {
		return nil, nil, err
	}
	replaceConditionWithInverse := analysis.TextEdit{
		Pos:     ifStatement.Cond.Pos(),
		End:     ifStatement.Cond.End(),
		NewText: inverseCondition,
	}

	// Return a SuggestedFix with just that TextEdit in there
	return fset, &analysis.SuggestedFix{
		TextEdits: []analysis.TextEdit{
			replaceConditionWithInverse,
			replaceBodyWithElse,
			replaceElse,
		},
	}, nil
}

func endsWithReturn(elseBranch ast.Stmt) (bool, error) {
	elseBlock, isBlockStatement := elseBranch.(*ast.BlockStmt)
	if !isBlockStatement {
		return false, fmt.Errorf("unable to figure out whether this ends with return: %T", elseBranch)
	}

	if len(elseBlock.List) == 0 {
		// Empty blocks don't end in returns
		return false, nil
	}

	lastStatement := elseBlock.List[len(elseBlock.List)-1]

	_, lastStatementIsReturn := lastStatement.(*ast.ReturnStmt)
	return lastStatementIsReturn, nil
}

// Turn { fmt.Println("Hello") } into just fmt.Println("Hello"), with one less
// level of indentation.
//
// The first line of the result will not be indented, but all of the following
// lines will.
func ifBodyToStandaloneCode(fset *token.FileSet, ifBody *ast.BlockStmt, src []byte) string {
	// Get the whole body (without the surrounding braces) as a string
	bodyStart := safetoken.StartPosition(fset, ifBody.Lbrace+1) // 1 == len("}")
	bodyEnd := safetoken.EndPosition(fset, ifBody.Rbrace)
	bodyWithoutBraces := string(src[bodyStart.Offset:bodyEnd.Offset])
	bodyWithoutBraces = strings.TrimSpace(bodyWithoutBraces)

	// Unindent
	bodyWithoutBraces = strings.ReplaceAll(bodyWithoutBraces, "\n\t", "\n")

	return bodyWithoutBraces
}

func invertCondition(fset *token.FileSet, cond ast.Expr, src []byte) ([]byte, error) {
	condStart := safetoken.StartPosition(fset, cond.Pos())
	condEnd := safetoken.EndPosition(fset, cond.End())
	oldText := string(src[condStart.Offset:condEnd.Offset])

	switch expr := cond.(type) {
	case *ast.Ident, *ast.ParenExpr, *ast.CallExpr, *ast.StarExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.SelectorExpr:
		newText := "!" + oldText
		if oldText == "true" {
			newText = "false"
		} else if oldText == "false" {
			newText = "true"
		}

		return []byte(newText), nil

	case *ast.UnaryExpr:
		if expr.Op != token.NOT {
			// This should never happen
			return dumbInvert(fset, cond, src), nil
		}

		inverse := expr.X
		if p, isParen := inverse.(*ast.ParenExpr); isParen {
			// We got !(x), remove the parentheses with the ! so we get just "x"
			inverse = p.X

			start := safetoken.StartPosition(fset, inverse.Pos())
			end := safetoken.EndPosition(fset, inverse.End())
			if start.Line != end.Line {
				// The expression is multi-line, so we can't remove the parentheses
				inverse = expr.X
			}
		}

		start := safetoken.StartPosition(fset, inverse.Pos())
		end := safetoken.EndPosition(fset, inverse.End())
		textWithoutNot := src[start.Offset:end.Offset]

		return textWithoutNot, nil

	case *ast.BinaryExpr:
		// These inversions are unsound for floating point NaN, but that's ok.
		negations := map[token.Token]string{
			token.EQL: "!=",
			token.LSS: ">=",
			token.GTR: "<=",
			token.NEQ: "==",
			token.LEQ: ">",
			token.GEQ: "<",
		}

		negation, negationFound := negations[expr.Op]
		if !negationFound {
			return invertAndOr(fset, expr, src)
		}

		xPosInSource := safetoken.StartPosition(fset, expr.X.Pos())
		opPosInSource := safetoken.StartPosition(fset, expr.OpPos)
		yPosInSource := safetoken.StartPosition(fset, expr.Y.Pos())

		textBeforeOp := string(src[xPosInSource.Offset:opPosInSource.Offset])

		oldOpWithTrailingWhitespace := string(src[opPosInSource.Offset:yPosInSource.Offset])
		newOpWithTrailingWhitespace := negation + oldOpWithTrailingWhitespace[len(expr.Op.String()):]

		textAfterOp := string(src[yPosInSource.Offset:condEnd.Offset])

		return []byte(textBeforeOp + newOpWithTrailingWhitespace + textAfterOp), nil
	}

	return dumbInvert(fset, cond, src), nil
}

// dumbInvert is a fallback, inverting cond into !(cond).
func dumbInvert(fset *token.FileSet, expr ast.Expr, src []byte) []byte {
	start := safetoken.StartPosition(fset, expr.Pos())
	end := safetoken.EndPosition(fset, expr.End())
	text := string(src[start.Offset:end.Offset])
	return []byte("!(" + text + ")")
}

func invertAndOr(fset *token.FileSet, expr *ast.BinaryExpr, src []byte) ([]byte, error) {
	if expr.Op != token.LAND && expr.Op != token.LOR {
		// Neither AND nor OR, don't know how to invert this
		return dumbInvert(fset, expr, src), nil
	}

	oppositeOp := "&&"
	if expr.Op == token.LAND {
		oppositeOp = "||"
	}

	xEndInSource := safetoken.EndPosition(fset, expr.X.End())
	opPosInSource := safetoken.StartPosition(fset, expr.OpPos)
	whitespaceAfterBefore := src[xEndInSource.Offset:opPosInSource.Offset]

	invertedBefore, err := invertCondition(fset, expr.X, src)
	if err != nil {
		return nil, err
	}

	invertedAfter, err := invertCondition(fset, expr.Y, src)
	if err != nil {
		return nil, err
	}

	yPosInSource := safetoken.StartPosition(fset, expr.Y.Pos())

	oldOpWithTrailingWhitespace := string(src[opPosInSource.Offset:yPosInSource.Offset])
	newOpWithTrailingWhitespace := oppositeOp + oldOpWithTrailingWhitespace[len(expr.Op.String()):]

	return []byte(string(invertedBefore) + string(whitespaceAfterBefore) + newOpWithTrailingWhitespace + string(invertedAfter)), nil
}

// canInvertIfCondition reports whether we can do invert-if-condition on the
// code in the given range.
func canInvertIfCondition(curFile cursor.Cursor, start, end token.Pos) (*ast.IfStmt, bool, error) {
	file := curFile.Node().(*ast.File)
	// TODO(adonovan): simplify, using Cursor.
	path, _ := astutil.PathEnclosingInterval(file, start, end)
	for _, node := range path {
		stmt, isIfStatement := node.(*ast.IfStmt)
		if !isIfStatement {
			continue
		}

		if stmt.Else == nil {
			// Can't invert conditions without else clauses
			return nil, false, fmt.Errorf("else clause required")
		}

		if _, hasElseIf := stmt.Else.(*ast.IfStmt); hasElseIf {
			// Can't invert conditions with else-if clauses, unclear what that
			// would look like
			return nil, false, fmt.Errorf("else-if not supported")
		}

		return stmt, true, nil
	}

	return nil, false, fmt.Errorf("not an if statement")
}
