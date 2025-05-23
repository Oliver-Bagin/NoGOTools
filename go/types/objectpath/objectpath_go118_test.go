// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objectpath_test

import (
	"github.com/tinygo-org/tinygo/alt_go/types"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/types/objectpath"
)

// TODO(adonovan): merge this back into objectpath_test.go.
func TestGenericPaths(t *testing.T) {
	const src = `
-- go.mod --
module x.io
go 1.18

-- b/b.go --
package b

const C int = 1

type T[TP0 any, TP1 interface{ M0(); M1() }] struct{}

func (T[RP0, RP1]) M() {}

type N int

func (N) M0()
func (N) M1()

type A = T[int, N]

func F[FP0 any, FP1 interface{ M() }](FP0, FP1) {}
`

	pkgmap := loadPackages(t, src, "./b")

	paths := []pathTest{
		// Good paths
		{"b", "T", "type b.T[TP0 any, TP1 interface{M0(); M1()}] struct{}", ""},
		{"b", "T.O", "type b.T[TP0 any, TP1 interface{M0(); M1()}] struct{}", ""},
		{"b", "T.M0", "func (b.T[RP0, RP1]).M()", ""},
		{"b", "T.M0.r1O", "type parameter RP1 interface{M0(); M1()}", ""},
		{"b", "T.M0.r1CM1", "func (interface).M1()", ""},
		{"b", "T.T0O", "type parameter TP0 any", ""},
		{"b", "T.T1O", "type parameter TP1 interface{M0(); M1()}", ""},
		{"b", "T.T1CM0", "func (interface).M0()", ""},
		{"b", "F.T0O", "type parameter FP0 any", ""},
		{"b", "F.T1CM0", "func (interface).M()", ""},
		// Obj of an instance is the generic declaration.
		{"b", "A.O", "type b.T[TP0 any, TP1 interface{M0(); M1()}] struct{}", ""},
		{"b", "A.M0", "func (b.T[int, b.N]).M()", ""},

		// Bad paths
		{"b", "N.C", "", "invalid path: ends with 'C', want [AFMO]"},
		{"b", "N.CO", "", "cannot apply 'C' to b.N (got *types.Named, want type parameter)"},
		{"b", "N.T", "", `invalid path: bad numeric operand "" for code 'T'`},
		{"b", "N.T0", "", "tuple index 0 out of range [0-0)"},
		{"b", "T.T2O", "", "tuple index 2 out of range [0-2)"},
		{"b", "T.T1M0", "", "cannot apply 'M' to TP1 (got *types.TypeParam, want interface or named)"},
		{"b", "C.T0", "", "cannot apply 'T' to int (got *types.Basic, want named or signature)"},
	}
	for _, test := range paths {
		if err := testPath(pkgmap, test); err != nil {
			t.Error(err)
		}
	}

	// bad objects
	for _, test := range []struct {
		obj     types.Object
		wantErr string
	}{
		{types.Universe.Lookup("any"), "predeclared type any = interface{} has no path"},
		{types.Universe.Lookup("comparable"), "predeclared type comparable interface{comparable} has no path"},
	} {
		path, err := objectpath.For(test.obj)
		if err == nil {
			t.Errorf("Object(%s) = %q, want error", test.obj, path)
			continue
		}
		if err.Error() != test.wantErr {
			t.Errorf("Object(%s) error was %q, want %q", test.obj, err, test.wantErr)
			continue
		}
	}
}

func TestGenericPaths_Issue51717(t *testing.T) {
	const src = `
-- go.mod --
module x.io
go 1.18

-- p/p.go --
package p

type S struct{}

func (_ S) M() {
	// The go vet stackoverflow crash disappears when the following line is removed
	panic("")
}

func F[WL interface{ N(item W) WL }, W any]() {
}

func main() {}
`
	pkgmap := loadPackages(t, src, "./p")

	paths := []pathTest{
		{"p", "F.T0CM0.RA0", "var  WL", ""},
		{"p", "F.T0CM0.RA0.CM0", "func (interface).N(item W) WL", ""},

		// Finding S.M0 reproduced the infinite recursion reported in #51717,
		// because F is searched before S.
		{"p", "S.M0", "func (p.S).M()", ""},
	}
	for _, test := range paths {
		if err := testPath(pkgmap, test); err != nil {
			t.Error(err)
		}
	}
}
