// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.9

package main

import "github.com/tinygo-org/tinygo/alt_go/types"

func isAlias(obj *types.TypeName) bool {
	return obj.IsAlias()
}
