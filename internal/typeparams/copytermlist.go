// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// copytermlist.go copies the term list algorithm from GOROOT/src/go/types.

package main

import (
	"bytes"
	"fmt"
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/format"
	"github.com/tinygo-org/tinygo/alt_go/parser"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/tinygo-org/tinygo/x-tools/go/ast/astutil"
)

func main() {
	if err := doCopy(); err != nil {
		fmt.Fprintf(os.Stderr, "error copying from go/types: %v", err)
		os.Exit(1)
	}
}

func doCopy() error {
	dir := filepath.Join(runtime.GOROOT(), "src", "go", "types")
	for _, name := range []string{"typeterm.go", "termlist.go"} {
		path := filepath.Join(dir, name)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		file.Name.Name = "typeparams"
		file.Doc = &ast.CommentGroup{List: []*ast.Comment{{Text: "DO NOT MODIFY"}}}
		var needImport bool
		selectorType := reflect.TypeOf((*ast.SelectorExpr)(nil))
		astutil.Apply(file, func(c *astutil.Cursor) bool {
			if id, _ := c.Node().(*ast.Ident); id != nil {
				// Check if this ident should be qualified with types. For simplicity,
				// assume the copied files do not themselves contain any exported
				// symbols.

				// As a simple heuristic, just verify that the ident may be replaced by
				// a selector.
				if !token.IsExported(id.Name) {
					return false
				}
				v := reflect.TypeOf(c.Parent()).Elem() // ast nodes are all pointers
				field, ok := v.FieldByName(c.Name())
				if !ok {
					panic("missing field")
				}
				t := field.Type
				if c.Index() > 0 { // => t is a slice
					t = t.Elem()
				}
				if !selectorType.AssignableTo(t) {
					return false
				}
				needImport = true
				c.Replace(&ast.SelectorExpr{
					X:   &ast.Ident{NamePos: id.NamePos, Name: "types"},
					Sel: &ast.Ident{NamePos: id.NamePos, Name: id.Name, Obj: id.Obj},
				})
			}
			return true
		}, nil)
		if needImport {
			astutil.AddImport(fset, file, "github.com/tinygo-org/tinygo/alt_go/types")
		}

		var b bytes.Buffer
		if err := format.Node(&b, fset, file); err != nil {
			return err
		}

		// Hack in the 'generated' byline.
		content := b.String()
		header := "// Code generated by copytermlist.go DO NOT EDIT.\n\npackage typeparams"
		content = strings.Replace(content, "package typeparams", header, 1)

		if err := os.WriteFile(name, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}
