// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.11

package gccgoimporter

import "github.com/tinygo-org/tinygo/alt_go/types"

func newInterface(methods []*types.Func, embeddeds []types.Type) *types.Interface {
	return types.NewInterfaceType(methods, embeddeds)
}
