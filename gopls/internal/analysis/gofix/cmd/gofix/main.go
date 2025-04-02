// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The inline command applies the inliner to the specified packages of
// Go source code. Run with:
//
//	$ go run ./internal/analysis/gofix/main.go -fix packages...
package main

import (
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/singlechecker"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/analysis/gofix"
)

func main() { singlechecker.Main(gofix.Analyzer) }
