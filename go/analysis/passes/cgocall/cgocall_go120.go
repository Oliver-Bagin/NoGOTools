// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !go1.21

package cgocall

import "github.com/tinygo-org/tinygo/alt_go/types"

func setGoVersion(tc *types.Config, pkg *types.Package) {
	// no types.Package.GoVersion until Go 1.21
}
