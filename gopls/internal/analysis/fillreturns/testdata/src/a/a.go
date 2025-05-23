// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fillreturns

import (
	"errors"
	"github.com/tinygo-org/tinygo/alt_go/ast"
	ast2 "github.com/tinygo-org/tinygo/alt_go/ast"
	"io"
	"net/http"
	. "net/http"
	"net/url"
	"strconv"
)

type T struct{}
type T1 = T
type I interface{}
type I1 = I
type z func(string, http.Handler) error

func x() error {
	return errors.New("foo")
}

// The error messages below changed in 1.18; "return values" covers both forms.

func b() (string, int, error) {
	return "", errors.New("foo") // want "return values"
}

func c() (string, int, error) {
	return 7, errors.New("foo") // want "return values"
}

func d() (string, int, error) {
	return "", 7 // want "return values"
}

func e() (T, error, *bool) {
	return (z(http.ListenAndServe))("", nil) // want "return values"
}

func preserveLeft() (int, int, error) {
	return 1, errors.New("foo") // want "return values"
}

func matchValues() (int, error, string) {
	return errors.New("foo"), 3 // want "return values"
}

func preventDataOverwrite() (int, string) {
	return errors.New("foo") // want "return values"
}

func closure() (string, error) {
	_ = func() (int, error) {
		return // want "return values"
	}
	return // want "return values"
}

func basic() (uint8, uint16, uint32, uint64, int8, int16, int32, int64, float32, float64, complex64, complex128, byte, rune, uint, int, uintptr, string, bool, error) {
	return // want "return values"
}

func complex() (*int, []int, [2]int, map[int]int) {
	return // want "return values"
}

func structsAndInterfaces() (T, url.URL, T1, I, I1, io.Reader, Client, ast2.Stmt) {
	return // want "return values"
}

func m() (int, error) {
	if 1 == 2 {
		return // want "return values"
	} else if 1 == 3 {
		return errors.New("foo") // want "return values"
	} else {
		return 1 // want "return values"
	}
	return // want "return values"
}

func convertibleTypes() (ast2.Expr, int) {
	return &ast2.ArrayType{} // want "return values"
}

func assignableTypes() (map[string]int, int) {
	type X map[string]int
	var x X
	return x // want "return values"
}

func interfaceAndError() (I, int) {
	return errors.New("foo") // want "return values"
}

func funcOneReturn() (string, error) {
	return strconv.Itoa(1) // want "return values"
}

func funcMultipleReturn() (int, error, string) {
	return strconv.Atoi("1")
}

func localFuncMultipleReturn() (string, int, error, string) {
	return b()
}

func multipleUnused() (int, string, string, string) {
	return 3, 4, 5 // want "return values"
}

func gotTooMany() int {
	if true {
		return 0, "" // want "return values"
	} else {
		return 1, 0, nil // want "return values"
	}
	return 0, 5, false // want "return values"
}

func fillVars() (int, string, ast.Node, bool, error) {
	eint := 0
	s := "a"
	var t bool
	if true {
		err := errors.New("fail")
		return // want "return values"
	}
	n := ast.NewIdent("ident")
	int := 3
	var b bool
	return "" // want "return values"
}
