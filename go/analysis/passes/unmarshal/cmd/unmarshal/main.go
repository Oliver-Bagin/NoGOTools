// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The unmarshal command runs the unmarshal analyzer.
package main

import (
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/unmarshal"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/singlechecker"
)

func main() { singlechecker.Main(unmarshal.Analyzer) }
