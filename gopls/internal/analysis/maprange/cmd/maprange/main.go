// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The maprange command applies the golang.org/x/tools/gopls/internal/analysis/maprange
// analysis to the specified packages of Go source code.
package main

import (
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/singlechecker"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/analysis/maprange"
)

func main() { singlechecker.Main(maprange.Analyzer) }
