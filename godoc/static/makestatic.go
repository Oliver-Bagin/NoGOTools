// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// Command makestatic writes the generated file buffer to "static.go".
// It is intended to be invoked via "go generate" (directive in "gen.go").
package main

import (
	"fmt"
	"os"

	"github.com/tinygo-org/tinygo/x-tools/godoc/static"
)

func main() {
	if err := makestatic(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func makestatic() error {
	buf, err := static.Generate()
	if err != nil {
		return fmt.Errorf("error while generating static.go: %v\n", err)
	}
	err = os.WriteFile("static.go", buf, 0666)
	if err != nil {
		return fmt.Errorf("error while writing static.go: %v\n", err)
	}
	return nil
}
