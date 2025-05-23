// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package misc

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/protocol"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/test/compare"
	. "github.com/tinygo-org/tinygo/x-tools/gopls/internal/test/integration"
)

func TestPrepareRenameMainPackage(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- main.go --
package main

import (
	"fmt"
)

func main() {
	fmt.Println(1)
}
`
	const wantErr = "can't rename package \"main\""
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("main.go")
		loc := env.RegexpSearch("main.go", `main`)
		params := &protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.LocationTextDocumentPositionParams(loc),
		}
		_, err := env.Editor.Server.PrepareRename(env.Ctx, params)
		if err == nil {
			t.Errorf("missing can't rename package main error from PrepareRename")
		}

		if err.Error() != wantErr {
			t.Errorf("got %v, want %v", err.Error(), wantErr)
		}
	})
}

// Test case for golang/go#56227
func TestRenameWithUnsafeSlice(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- p.go --
package p

import "unsafe"

type T struct{}

func (T) M() {}

func _() {
	x := [3]int{1, 2, 3}
	ptr := unsafe.Pointer(&x)
	_ = unsafe.Slice((*int)(ptr), 3)
}
`

	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("p.go")
		env.Rename(env.RegexpSearch("p.go", "M"), "N") // must not panic
	})
}

func TestPrepareRenameWithNoPackageDeclaration(t *testing.T) {
	const files = `
go 1.14
-- lib/a.go --
import "fmt"

const A = 1

func bar() {
	fmt.Println("Bar")
}

-- main.go --
package main

import "fmt"

func main() {
	fmt.Println("Hello")
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		err := env.Editor.Rename(env.Ctx, env.RegexpSearch("lib/a.go", "fmt"), "fmt1")
		if got, want := fmt.Sprint(err), "no identifier found"; got != want {
			t.Errorf("Rename: got error %v, want %v", got, want)
		}
	})
}

func TestPrepareRenameFailWithUnknownModule(t *testing.T) {
	const files = `
go 1.14
-- lib/a.go --
package lib

const A = 1

-- main.go --
package main

import (
	"mod.com/lib"
)

func main() {
	println("Hello")
}
`
	const wantErr = "can't rename package: missing module information for package"
	Run(t, files, func(t *testing.T, env *Env) {
		loc := env.RegexpSearch("lib/a.go", "lib")
		params := &protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.LocationTextDocumentPositionParams(loc),
		}
		_, err := env.Editor.Server.PrepareRename(env.Ctx, params)
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("missing cannot rename packages with unknown module from PrepareRename")
		}
	})
}

// This test ensures that each import of a renamed package
// is also renamed if it would otherwise create a conflict.
func TestRenamePackageWithConflicts(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

const A = 1

-- lib/nested/a.go --
package nested

const B = 1

-- lib/x/a.go --
package nested1

const C = 1

-- main.go --
package main

import (
	"mod.com/lib"
	"mod.com/lib/nested"
	nested1 "mod.com/lib/x"
)

func main() {
	println("Hello")
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "lib"), "nested")

		// Check if the new package name exists.
		env.RegexpSearch("nested/a.go", "package nested")
		env.RegexpSearch("main.go", `nested2 "mod.com/nested"`)
		env.RegexpSearch("main.go", "mod.com/nested/nested")
		env.RegexpSearch("main.go", `nested1 "mod.com/nested/x"`)
	})
}

func TestRenamePackageWithAlias(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

const A = 1

-- lib/nested/a.go --
package nested

const B = 1

-- main.go --
package main

import (
	"mod.com/lib"
	lib1 "mod.com/lib/nested"
)

func main() {
	println("Hello")
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "lib"), "nested")

		// Check if the new package name exists.
		env.RegexpSearch("nested/a.go", "package nested")
		env.RegexpSearch("main.go", "mod.com/nested")
		env.RegexpSearch("main.go", `lib1 "mod.com/nested/nested"`)
	})
}

func TestRenamePackageWithDifferentDirectoryPath(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

const A = 1

-- lib/nested/a.go --
package foo

const B = 1

-- main.go --
package main

import (
	"mod.com/lib"
	foo "mod.com/lib/nested"
)

func main() {
	println("Hello")
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "lib"), "nested")

		// Check if the new package name exists.
		env.RegexpSearch("nested/a.go", "package nested")
		env.RegexpSearch("main.go", "mod.com/nested")
		env.RegexpSearch("main.go", `foo "mod.com/nested/nested"`)
	})
}

func TestRenamePackage(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

const A = 1

-- lib/b.go --
package lib

const B = 1

-- lib/nested/a.go --
package nested

const C = 1

-- main.go --
package main

import (
	"mod.com/lib"
	"mod.com/lib/nested"
)

func main() {
	println("Hello")
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "lib"), "lib1")

		// Check if the new package name exists.
		env.RegexpSearch("lib1/a.go", "package lib1")
		env.RegexpSearch("lib1/b.go", "package lib1")
		env.RegexpSearch("main.go", "mod.com/lib1")
		env.RegexpSearch("main.go", "mod.com/lib1/nested")
	})
}

// Test for golang/go#47564.
func TestRenameInTestVariant(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.12
-- stringutil/stringutil.go --
package stringutil

func Identity(s string) string {
	return s
}
-- stringutil/stringutil_test.go --
package stringutil

func TestIdentity(t *testing.T) {
	if got := Identity("foo"); got != "foo" {
		t.Errorf("bad")
	}
}
-- main.go --
package main

import (
	"fmt"

	"mod.com/stringutil"
)

func main() {
	fmt.Println(stringutil.Identity("hello world"))
}
`

	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("main.go")
		env.Rename(env.RegexpSearch("main.go", `stringutil\.(Identity)`), "Identityx")
		env.OpenFile("stringutil/stringutil_test.go")
		text := env.BufferText("stringutil/stringutil_test.go")
		if !strings.Contains(text, "Identityx") {
			t.Errorf("stringutil/stringutil_test.go: missing expected token `Identityx` after rename:\n%s", text)
		}
	})
}

// This is a test that rename operation initiated by the editor function as expected.
func TestRenameFileFromEditor(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.16
-- a/a.go --
package a

const X = 1
-- a/x.go --
package a

const X = 2
-- b/b.go --
package b
`

	Run(t, files, func(t *testing.T, env *Env) {
		// Rename files and verify that diagnostics are affected accordingly.

		// Initially, we should have diagnostics on both X's, for their duplicate declaration.
		env.OnceMet(
			InitialWorkspaceLoad,
			Diagnostics(env.AtRegexp("a/a.go", "X")),
			Diagnostics(env.AtRegexp("a/x.go", "X")),
		)

		// Moving x.go should make the diagnostic go away.
		env.RenameFile("a/x.go", "b/x.go")
		env.AfterChange(
			NoDiagnostics(ForFile("a/a.go")),               // no more duplicate declarations
			Diagnostics(env.AtRegexp("b/b.go", "package")), // as package names mismatch
		)

		// Renaming should also work on open buffers.
		env.OpenFile("b/x.go")

		// Moving x.go back to a/ should cause the diagnostics to reappear.
		env.RenameFile("b/x.go", "a/x.go")
		env.AfterChange(
			Diagnostics(env.AtRegexp("a/a.go", "X")),
			Diagnostics(env.AtRegexp("a/x.go", "X")),
		)

		// Renaming the entire directory should move both the open and closed file.
		env.RenameFile("a", "x")
		env.AfterChange(
			Diagnostics(env.AtRegexp("x/a.go", "X")),
			Diagnostics(env.AtRegexp("x/x.go", "X")),
		)

		// As a sanity check, verify that x/x.go is open.
		if text := env.BufferText("x/x.go"); text == "" {
			t.Fatal("got empty buffer for x/x.go")
		}
	})
}

func TestRenamePackage_Tests(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

const A = 1

-- lib/b.go --
package lib

const B = 1

-- lib/a_test.go --
package lib_test

import (
	"mod.com/lib"
	"fmt
)

const C = 1

-- lib/b_test.go --
package lib

import (
	"fmt
)

const D = 1

-- lib/nested/a.go --
package nested

const D = 1

-- main.go --
package main

import (
	"mod.com/lib"
	"mod.com/lib/nested"
)

func main() {
	println("Hello")
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "lib"), "lib1")

		// Check if the new package name exists.
		env.RegexpSearch("lib1/a.go", "package lib1")
		env.RegexpSearch("lib1/b.go", "package lib1")
		env.RegexpSearch("main.go", "mod.com/lib1")
		env.RegexpSearch("main.go", "mod.com/lib1/nested")

		// Check if the test package is renamed
		env.RegexpSearch("lib1/a_test.go", "package lib1_test")
		env.RegexpSearch("lib1/b_test.go", "package lib1")
	})
}

func TestRenamePackage_NestedModule(t *testing.T) {
	const files = `
-- go.work --
go 1.18
use (
	.
	./foo/bar
	./foo/baz
)

-- go.mod --
module mod.com

go 1.18

require (
    mod.com/foo/bar v0.0.0
)

replace (
	mod.com/foo/bar => ./foo/bar
	mod.com/foo/baz => ./foo/baz
)
-- foo/foo.go --
package foo

import "fmt"

func Bar() {
	fmt.Println("In foo before renamed to foox.")
}

-- foo/bar/go.mod --
module mod.com/foo/bar

-- foo/bar/bar.go --
package bar

const Msg = "Hi from package bar"

-- foo/baz/go.mod --
module mod.com/foo/baz

-- foo/baz/baz.go --
package baz

const Msg = "Hi from package baz"

-- main.go --
package main

import (
	"fmt"
	"mod.com/foo/bar"
	"mod.com/foo/baz"
	"mod.com/foo"
)

func main() {
	foo.Bar()
	fmt.Println(bar.Msg)
	fmt.Println(baz.Msg)
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("foo/foo.go")
		env.Rename(env.RegexpSearch("foo/foo.go", "foo"), "foox")

		env.RegexpSearch("foox/foo.go", "package foox")
		env.OpenFile("foox/bar/bar.go")
		env.OpenFile("foox/bar/go.mod")

		env.RegexpSearch("main.go", "mod.com/foo/bar")
		env.RegexpSearch("main.go", "mod.com/foox")
		env.RegexpSearch("main.go", "foox.Bar()")

		env.RegexpSearch("go.mod", "./foox/bar")
		env.RegexpSearch("go.mod", "./foox/baz")
	})
}

func TestRenamePackage_DuplicateImport(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

const A = 1

-- lib/nested/a.go --
package nested

const B = 1

-- main.go --
package main

import (
	"mod.com/lib"
	lib1 "mod.com/lib"
	lib2 "mod.com/lib/nested"
)

func main() {
	println("Hello")
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "lib"), "nested")

		// Check if the new package name exists.
		env.RegexpSearch("nested/a.go", "package nested")
		env.RegexpSearch("main.go", "mod.com/nested")
		env.RegexpSearch("main.go", `lib1 "mod.com/nested"`)
		env.RegexpSearch("main.go", `lib2 "mod.com/nested/nested"`)
	})
}

func TestRenamePackage_DuplicateBlankImport(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

const A = 1

-- lib/nested/a.go --
package nested

const B = 1

-- main.go --
package main

import (
	"mod.com/lib"
	_ "mod.com/lib"
	lib1 "mod.com/lib/nested"
)

func main() {
	println("Hello")
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "lib"), "nested")

		// Check if the new package name exists.
		env.RegexpSearch("nested/a.go", "package nested")
		env.RegexpSearch("main.go", "mod.com/nested")
		env.RegexpSearch("main.go", `_ "mod.com/nested"`)
		env.RegexpSearch("main.go", `lib1 "mod.com/nested/nested"`)
	})
}

func TestRenamePackage_TestVariant(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.12
-- foo/foo.go --
package foo

const Foo = 42
-- bar/bar.go --
package bar

import "mod.com/foo"

const Bar = foo.Foo
-- bar/bar_test.go --
package bar

import "mod.com/foo"

const Baz = foo.Foo
-- testdata/bar/bar.go --
package bar

import "mod.com/foox"

const Bar = foox.Foo
-- testdata/bar/bar_test.go --
package bar

import "mod.com/foox"

const Baz = foox.Foo
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("foo/foo.go")
		env.Rename(env.RegexpSearch("foo/foo.go", "package (foo)"), "foox")

		checkTestdata(t, env)
	})
}

func TestRenamePackage_IntermediateTestVariant(t *testing.T) {
	// In this test set up, we have the following import edges:
	//   bar_test -> baz -> foo -> bar
	//   bar_test -> foo -> bar
	//   bar_test -> bar
	//
	// As a consequence, bar_x_test.go is in the reverse closure of both
	// `foo [bar.test]` and `baz [bar.test]`. This test confirms that we don't
	// produce duplicate edits in this case.
	const files = `
-- go.mod --
module foo.mod

go 1.12
-- foo/foo.go --
package foo

import "foo.mod/bar"

const Foo = 42

const _ = bar.Bar
-- baz/baz.go --
package baz

import "foo.mod/foo"

const Baz = foo.Foo
-- bar/bar.go --
package bar

var Bar = 123
-- bar/bar_test.go --
package bar

const _ = Bar
-- bar/bar_x_test.go --
package bar_test

import (
	"foo.mod/bar"
	"foo.mod/baz"
	"foo.mod/foo"
)

const _ = bar.Bar + baz.Baz + foo.Foo
-- testdata/foox/foo.go --
package foox

import "foo.mod/bar"

const Foo = 42

const _ = bar.Bar
-- testdata/baz/baz.go --
package baz

import "foo.mod/foox"

const Baz = foox.Foo
-- testdata/bar/bar_x_test.go --
package bar_test

import (
	"foo.mod/bar"
	"foo.mod/baz"
	"foo.mod/foox"
)

const _ = bar.Bar + baz.Baz + foox.Foo
`

	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("foo/foo.go")
		env.Rename(env.RegexpSearch("foo/foo.go", "package (foo)"), "foox")

		checkTestdata(t, env)
	})
}

func TestRenamePackage_Nesting(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

import "mod.com/lib/nested"

const A = 1 + nested.B
-- lib/nested/a.go --
package nested

const B = 1
-- other/other.go --
package other

import (
	"mod.com/lib"
	"mod.com/lib/nested"
)

const C = lib.A + nested.B
-- testdata/libx/a.go --
package libx

import "mod.com/libx/nested"

const A = 1 + nested.B
-- testdata/other/other.go --
package other

import (
	"mod.com/libx"
	"mod.com/libx/nested"
)

const C = libx.A + nested.B
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "package (lib)"), "libx")

		checkTestdata(t, env)
	})
}

func TestRenamePackage_InvalidName(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

import "mod.com/lib/nested"

const A = 1 + nested.B
`

	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/a.go")
		loc := env.RegexpSearch("lib/a.go", "package (lib)")

		for _, badName := range []string{"$$$", "lib_test"} {
			if err := env.Editor.Rename(env.Ctx, loc, badName); err == nil {
				t.Errorf("Rename(lib, libx) succeeded, want non-nil error")
			}
		}
	})
}

func TestRenamePackage_InternalPackage(t *testing.T) {
	const files = `
-- go.mod --
module mod.com

go 1.18
-- lib/a.go --
package lib

import (
	"fmt"
	"mod.com/lib/internal/x"
)

const A = 1

func print() {
	fmt.Println(x.B)
}

-- lib/internal/x/a.go --
package x

const B = 1

-- main.go --
package main

import "mod.com/lib"

func main() {
	lib.print()
}
`
	Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("lib/internal/x/a.go")
		env.Rename(env.RegexpSearch("lib/internal/x/a.go", "x"), "utils")

		// Check if the new package name exists.
		env.RegexpSearch("lib/a.go", "mod.com/lib/internal/utils")
		env.RegexpSearch("lib/a.go", "utils.B")

		// Check if the test package is renamed
		env.RegexpSearch("lib/internal/utils/a.go", "package utils")

		env.OpenFile("lib/a.go")
		env.Rename(env.RegexpSearch("lib/a.go", "lib"), "lib1")

		// Check if the new package name exists.
		env.RegexpSearch("lib1/a.go", "package lib1")
		env.RegexpSearch("lib1/a.go", "mod.com/lib1/internal/utils")
		env.RegexpSearch("main.go", `import "mod.com/lib1"`)
		env.RegexpSearch("main.go", "lib1.print()")
	})
}

// checkTestdata checks that current buffer contents match their corresponding
// expected content in the testdata directory.
func checkTestdata(t *testing.T, env *Env) {
	t.Helper()
	files := env.ListFiles("testdata")
	if len(files) == 0 {
		t.Fatal("no files in testdata directory")
	}
	for _, file := range files {
		suffix := strings.TrimPrefix(file, "testdata/")
		got := env.BufferText(suffix)
		want := env.ReadWorkspaceFile(file)
		if diff := compare.Text(want, got); diff != "" {
			t.Errorf("Rename: unexpected buffer content for %s (-want +got):\n%s", suffix, diff)
		}
	}
}
