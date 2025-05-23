// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package completion

import (
	"github.com/tinygo-org/tinygo/alt_go/ast"

	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/golang/completion/snippet"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/util/safetoken"
)

// structFieldSnippet calculates the snippet for struct literal field names.
func (c *completer) structFieldSnippet(cand candidate, detail string, snip *snippet.Builder) {
	if !wantStructFieldCompletions(c.enclosingCompositeLiteral) {
		return
	}

	// If we are in a deep completion then we can't be completing a field
	// name (e.g. "Foo{f<>}" completing to "Foo{f.Bar}" should not generate
	// a snippet).
	if len(cand.path) > 0 {
		return
	}

	clInfo := c.enclosingCompositeLiteral

	// If we are already in a key-value expression, we don't want a snippet.
	if clInfo.kv != nil {
		return
	}

	// A plain snippet turns "Foo{Ba<>" into "Foo{Bar: <>".
	snip.WriteText(": ")
	snip.WritePlaceholder(func(b *snippet.Builder) {
		// A placeholder snippet turns "Foo{Ba<>" into "Foo{Bar: <*int*>".
		if c.opts.placeholders {
			b.WriteText(detail)
		}
	})

	fset := c.pkg.FileSet()

	// If the cursor position is on a different line from the literal's opening brace,
	// we are in a multiline literal. Ignore line directives.
	if safetoken.StartPosition(fset, c.pos).Line != safetoken.StartPosition(fset, clInfo.cl.Lbrace).Line {
		snip.WriteText(",")
	}
}

// functionCallSnippet calculates the snippet for function calls.
//
// Callers should omit the suffix of type parameters that are
// constrained by the argument types, to avoid offering completions
// that contain instantiations that are redundant because of type
// inference, such as f[int](1) for func f[T any](x T).
func (c *completer) functionCallSnippet(name string, tparams, params []string, snip *snippet.Builder) {
	if !c.opts.completeFunctionCalls {
		snip.WriteText(name)
		return
	}

	// If there is no suffix then we need to reuse existing call parens
	// "()" if present. If there is an identifier suffix then we always
	// need to include "()" since we don't overwrite the suffix.
	if c.surrounding != nil && c.surrounding.Suffix() == "" && len(c.path) > 1 {
		// If we are the left side (i.e. "Fun") part of a call expression,
		// we don't want a snippet since there are already parens present.
		switch n := c.path[1].(type) {
		case *ast.CallExpr:
			// The Lparen != Rparen check detects fudged CallExprs we
			// inserted when fixing the AST. In this case, we do still need
			// to insert the calling "()" parens.
			if n.Fun == c.path[0] && n.Lparen != n.Rparen {
				return
			}
		case *ast.SelectorExpr:
			if len(c.path) > 2 {
				if call, ok := c.path[2].(*ast.CallExpr); ok && call.Fun == c.path[1] && call.Lparen != call.Rparen {
					return
				}
			}
		}
	}

	snip.WriteText(name)

	if len(tparams) > 0 {
		snip.WriteText("[")
		if c.opts.placeholders {
			for i, tp := range tparams {
				if i > 0 {
					snip.WriteText(", ")
				}
				snip.WritePlaceholder(func(b *snippet.Builder) {
					b.WriteText(tp)
				})
			}
		} else {
			snip.WritePlaceholder(nil)
		}
		snip.WriteText("]")
	}

	snip.WriteText("(")

	if c.opts.placeholders {
		// A placeholder snippet turns "someFun<>" into "someFunc(<*i int*>, *s string*)".
		for i, p := range params {
			if i > 0 {
				snip.WriteText(", ")
			}
			snip.WritePlaceholder(func(b *snippet.Builder) {
				b.WriteText(p)
			})
		}
	} else {
		// A plain snippet turns "someFun<>" into "someFunc(<>)".
		if len(params) > 0 {
			snip.WritePlaceholder(nil)
		}
	}

	snip.WriteText(")")
}
