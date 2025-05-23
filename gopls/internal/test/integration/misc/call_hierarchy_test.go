// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package misc

import (
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/protocol"
	. "github.com/tinygo-org/tinygo/x-tools/gopls/internal/test/integration"
)

// Test for golang/go#49125
func TestCallHierarchy_Issue49125(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.12
-- p.go --
package pkg
`
	// TODO(rfindley): this could probably just be a marker test.
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("p.go")
		loc := env.RegexpSearch("p.go", "pkg")

		var params protocol.CallHierarchyPrepareParams
		params.TextDocument.URI = loc.URI
		params.Position = loc.Range.Start

		// Check that this doesn't panic.
		env.Editor.Server.PrepareCallHierarchy(env.Ctx, &params)
	})
}
