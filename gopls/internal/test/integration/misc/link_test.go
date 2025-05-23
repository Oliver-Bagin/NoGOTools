// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package misc

import (
	"strings"
	"testing"

	. "github.com/tinygo-org/tinygo/x-tools/gopls/internal/test/integration"
)

func TestHoverAndDocumentLink(t *testing.T) {
	const program = `
-- go.mod --
module mod.test

go 1.12

require import.test v1.2.3
-- main.go --
package main

import "import.test/pkg"

func main() {
	// Issue 43990: this is not a link that most users can open from an LSP
	// client: mongodb://not.a.link.com
	println(pkg.Hello)
}`

	const proxy = `
-- import.test@v1.2.3/go.mod --
module import.test

go 1.12
-- import.test@v1.2.3/pkg/const.go --
package pkg

const Hello = "Hello"
`
	WithOptions(
		ProxyFiles(proxy),
		WriteGoSum("."),
	).Run(t, program, func(t *testing.T, env *Env) {
		env.OpenFile("main.go")
		env.OpenFile("go.mod")

		modLink := "https://pkg.go.dev/mod/import.test@v1.2.3"
		pkgLink := "https://pkg.go.dev/import.test@v1.2.3/pkg"

		// First, check that we get the expected links via hover and documentLink.
		content, _ := env.Hover(env.RegexpSearch("main.go", "pkg.Hello"))
		if content == nil || !strings.Contains(content.Value, pkgLink) {
			t.Errorf("hover: got %v in main.go, want contains %q", content, pkgLink)
		}
		content, _ = env.Hover(env.RegexpSearch("go.mod", "import.test"))
		if content == nil || !strings.Contains(content.Value, pkgLink) {
			t.Errorf("hover: got %v in go.mod, want contains %q", content, pkgLink)
		}
		links := env.DocumentLink("main.go")
		if len(links) != 1 || *links[0].Target != pkgLink {
			t.Errorf("documentLink: got links %+v for main.go, want one link with target %q", links, pkgLink)
		}
		links = env.DocumentLink("go.mod")
		if len(links) != 1 || *links[0].Target != modLink {
			t.Errorf("documentLink: got links %+v for go.mod, want one link with target %q", links, modLink)
		}

		// Then change the environment to make these links private.
		cfg := env.Editor.Config()
		cfg.Env = map[string]string{"GOPRIVATE": "import.test"}
		env.ChangeConfiguration(cfg)

		// Finally, verify that the links are gone.
		content, _ = env.Hover(env.RegexpSearch("main.go", "pkg.Hello"))
		if content == nil || strings.Contains(content.Value, pkgLink) {
			t.Errorf("hover: got %v in main.go, want non-empty hover without %q", content, pkgLink)
		}
		content, _ = env.Hover(env.RegexpSearch("go.mod", "import.test"))
		if content == nil || strings.Contains(content.Value, modLink) {
			t.Errorf("hover: got %v in go.mod, want contains %q", content, modLink)
		}
		links = env.DocumentLink("main.go")
		if len(links) != 0 {
			t.Errorf("documentLink: got %d document links for main.go, want 0\nlinks: %v", len(links), links)
		}
		links = env.DocumentLink("go.mod")
		if len(links) != 0 {
			t.Errorf("documentLink: got %d document links for go.mod, want 0\nlinks: %v", len(links), links)
		}
	})
}
