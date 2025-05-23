// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"bytes"
	"github.com/tinygo-org/tinygo/alt_go/build"
	"github.com/tinygo-org/tinygo/alt_go/build/constraint"
	"github.com/tinygo-org/tinygo/alt_go/parser"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"io"
	"path/filepath"
	"strings"

	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/util/bug"
)

type port struct{ GOOS, GOARCH string }

var (
	// preferredPorts holds GOOS/GOARCH combinations for which we dynamically
	// create new Views, by setting GOOS=... and GOARCH=... on top of
	// user-provided configuration when we detect that the default build
	// configuration does not match an open file. Ports are matched in the order
	// defined below, so that when multiple ports match a file we use the port
	// occurring at a lower index in the slice. For that reason, we sort first
	// class ports ahead of secondary ports, and (among first class ports) 64-bit
	// ports ahead of the less common 32-bit ports.
	preferredPorts = []port{
		// First class ports, from https://go.dev/wiki/PortingPolicy.
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"windows", "amd64"},
		{"linux", "arm"},
		{"linux", "386"},
		{"windows", "386"},

		// Secondary ports, from GOROOT/src/internal/platform/zosarch.go.
		// (First class ports are commented out.)
		{"aix", "ppc64"},
		{"dragonfly", "amd64"},
		{"freebsd", "386"},
		{"freebsd", "amd64"},
		{"freebsd", "arm"},
		{"freebsd", "arm64"},
		{"illumos", "amd64"},
		{"linux", "ppc64"},
		{"linux", "ppc64le"},
		{"linux", "mips"},
		{"linux", "mipsle"},
		{"linux", "mips64"},
		{"linux", "mips64le"},
		{"linux", "riscv64"},
		{"linux", "s390x"},
		{"android", "386"},
		{"android", "amd64"},
		{"android", "arm"},
		{"android", "arm64"},
		{"ios", "arm64"},
		{"ios", "amd64"},
		{"js", "wasm"},
		{"netbsd", "386"},
		{"netbsd", "amd64"},
		{"netbsd", "arm"},
		{"netbsd", "arm64"},
		{"openbsd", "386"},
		{"openbsd", "amd64"},
		{"openbsd", "arm"},
		{"openbsd", "arm64"},
		{"openbsd", "mips64"},
		{"plan9", "386"},
		{"plan9", "amd64"},
		{"plan9", "arm"},
		{"solaris", "amd64"},
		{"windows", "arm"},
		{"windows", "arm64"},

		{"aix", "ppc64"},
		{"android", "386"},
		{"android", "amd64"},
		{"android", "arm"},
		{"android", "arm64"},
		// {"darwin", "amd64"},
		// {"darwin", "arm64"},
		{"dragonfly", "amd64"},
		{"freebsd", "386"},
		{"freebsd", "amd64"},
		{"freebsd", "arm"},
		{"freebsd", "arm64"},
		{"freebsd", "riscv64"},
		{"illumos", "amd64"},
		{"ios", "amd64"},
		{"ios", "arm64"},
		{"js", "wasm"},
		// {"linux", "386"},
		// {"linux", "amd64"},
		// {"linux", "arm"},
		// {"linux", "arm64"},
		{"linux", "loong64"},
		{"linux", "mips"},
		{"linux", "mips64"},
		{"linux", "mips64le"},
		{"linux", "mipsle"},
		{"linux", "ppc64"},
		{"linux", "ppc64le"},
		{"linux", "riscv64"},
		{"linux", "s390x"},
		{"linux", "sparc64"},
		{"netbsd", "386"},
		{"netbsd", "amd64"},
		{"netbsd", "arm"},
		{"netbsd", "arm64"},
		{"openbsd", "386"},
		{"openbsd", "amd64"},
		{"openbsd", "arm"},
		{"openbsd", "arm64"},
		{"openbsd", "mips64"},
		{"openbsd", "ppc64"},
		{"openbsd", "riscv64"},
		{"plan9", "386"},
		{"plan9", "amd64"},
		{"plan9", "arm"},
		{"solaris", "amd64"},
		{"wasip1", "wasm"},
		// {"windows", "386"},
		// {"windows", "amd64"},
		{"windows", "arm"},
		{"windows", "arm64"},
	}
)

// matches reports whether the port matches a file with the given absolute path
// and content.
//
// Note that this function accepts content rather than e.g. a file.Handle,
// because we trim content before matching for performance reasons, and
// therefore need to do this outside of matches when considering multiple ports.
func (p port) matches(path string, content []byte) bool {
	ctxt := build.Default // make a copy
	ctxt.UseAllFiles = false
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		bug.Reportf("non-abs file path %q", path)
		return false // fail closed
	}
	dir, name := filepath.Split(path)

	// The only virtualized operation called by MatchFile is OpenFile.
	ctxt.OpenFile = func(p string) (io.ReadCloser, error) {
		if p != path {
			return nil, bug.Errorf("unexpected file %q", p)
		}
		return io.NopCloser(bytes.NewReader(content)), nil
	}

	ctxt.GOOS = p.GOOS
	ctxt.GOARCH = p.GOARCH
	ok, err := ctxt.MatchFile(dir, name)
	return err == nil && ok
}

// trimContentForPortMatch trims the given Go file content to a minimal file
// containing the same build constraints, if any.
//
// This is an unfortunate but necessary optimization, as matching build
// constraints using go/build has significant overhead, and involves parsing
// more than just the build constraint.
//
// TestMatchingPortsConsistency enforces consistency by comparing results
// without trimming content.
func trimContentForPortMatch(content []byte) []byte {
	buildComment := buildComment(content)
	// The package name does not matter, but +build lines
	// require a blank line before the package declaration.
	return []byte(buildComment + "\n\npackage p")
}

// buildComment returns the first matching //go:build comment in the given
// content, or "" if none exists.
func buildComment(content []byte) string {
	var lines []string

	f, err := parser.ParseFile(token.NewFileSet(), "", content, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return ""
	}

	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if constraint.IsGoBuild(c.Text) {
				// A file must have only one //go:build line.
				return c.Text
			}
			if constraint.IsPlusBuild(c.Text) {
				// A file may have several // +build lines.
				lines = append(lines, c.Text)
			}
		}
	}
	return strings.Join(lines, "\n")
}
