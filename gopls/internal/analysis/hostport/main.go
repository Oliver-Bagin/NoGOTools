// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

import (
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/singlechecker"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/analysis/hostport"
)

func main() { singlechecker.Main(hostport.Analyzer) }
