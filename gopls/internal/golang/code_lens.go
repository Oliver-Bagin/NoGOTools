// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package golang

import (
	"context"
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"github.com/tinygo-org/tinygo/alt_go/types"
	"regexp"
	"strings"

	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/cache"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/cache/parsego"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/file"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/protocol"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/protocol/command"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/settings"
)

// CodeLensSources returns the supported sources of code lenses for Go files.
func CodeLensSources() map[settings.CodeLensSource]cache.CodeLensSourceFunc {
	return map[settings.CodeLensSource]cache.CodeLensSourceFunc{
		settings.CodeLensGenerate:      goGenerateCodeLens, // commands: Generate
		settings.CodeLensTest:          runTestCodeLens,    // commands: Test
		settings.CodeLensRegenerateCgo: regenerateCgoLens,  // commands: RegenerateCgo
	}
}

var (
	testRe      = regexp.MustCompile(`^Test([^a-z]|$)`) // TestFoo or Test but not Testable
	benchmarkRe = regexp.MustCompile(`^Benchmark([^a-z]|$)`)
)

func runTestCodeLens(ctx context.Context, snapshot *cache.Snapshot, fh file.Handle) ([]protocol.CodeLens, error) {
	var codeLens []protocol.CodeLens

	pkg, pgf, err := NarrowestPackageForFile(ctx, snapshot, fh.URI())
	if err != nil {
		return nil, err
	}
	testFuncs, benchFuncs, err := testsAndBenchmarks(pkg.TypesInfo(), pgf)
	if err != nil {
		return nil, err
	}
	puri := fh.URI()
	for _, fn := range testFuncs {
		cmd := command.NewRunTestsCommand("run test", command.RunTestsArgs{
			URI:   puri,
			Tests: []string{fn.name},
		})
		rng := protocol.Range{Start: fn.rng.Start, End: fn.rng.Start}
		codeLens = append(codeLens, protocol.CodeLens{Range: rng, Command: cmd})
	}

	for _, fn := range benchFuncs {
		cmd := command.NewRunTestsCommand("run benchmark", command.RunTestsArgs{
			URI:        puri,
			Benchmarks: []string{fn.name},
		})
		rng := protocol.Range{Start: fn.rng.Start, End: fn.rng.Start}
		codeLens = append(codeLens, protocol.CodeLens{Range: rng, Command: cmd})
	}

	if len(benchFuncs) > 0 {
		pgf, err := snapshot.ParseGo(ctx, fh, parsego.Full)
		if err != nil {
			return nil, err
		}
		// add a code lens to the top of the file which runs all benchmarks in the file
		rng, err := pgf.PosRange(pgf.File.Package, pgf.File.Package)
		if err != nil {
			return nil, err
		}
		var benches []string
		for _, fn := range benchFuncs {
			benches = append(benches, fn.name)
		}
		cmd := command.NewRunTestsCommand("run file benchmarks", command.RunTestsArgs{
			URI:        puri,
			Benchmarks: benches,
		})
		codeLens = append(codeLens, protocol.CodeLens{Range: rng, Command: cmd})
	}
	return codeLens, nil
}

type testFunc struct {
	name string
	rng  protocol.Range // of *ast.FuncDecl
}

// testsAndBenchmarks returns all Test and Benchmark functions in the
// specified file.
func testsAndBenchmarks(info *types.Info, pgf *parsego.File) (tests, benchmarks []testFunc, _ error) {
	if !strings.HasSuffix(pgf.URI.Path(), "_test.go") {
		return nil, nil, nil // empty
	}

	for _, d := range pgf.File.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}

		rng, err := pgf.NodeRange(fn)
		if err != nil {
			return nil, nil, err
		}

		if matchTestFunc(fn, info, testRe, "T") {
			tests = append(tests, testFunc{fn.Name.Name, rng})
		} else if matchTestFunc(fn, info, benchmarkRe, "B") {
			benchmarks = append(benchmarks, testFunc{fn.Name.Name, rng})
		}
	}
	return
}

func matchTestFunc(fn *ast.FuncDecl, info *types.Info, nameRe *regexp.Regexp, paramID string) bool {
	// Make sure that the function name matches a test function.
	if !nameRe.MatchString(fn.Name.Name) {
		return false
	}
	obj, ok := info.ObjectOf(fn.Name).(*types.Func)
	if !ok {
		return false
	}
	sig := obj.Signature()
	// Test functions should have only one parameter.
	if sig.Params().Len() != 1 {
		return false
	}

	// Check the type of the only parameter
	// (We don't Unalias or use typesinternal.ReceiverNamed
	// in the two checks below because "go test" can't see
	// through aliases when enumerating Test* functions;
	// it's syntactic.)
	paramTyp, ok := sig.Params().At(0).Type().(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := paramTyp.Elem().(*types.Named)
	if !ok {
		return false
	}
	namedObj := named.Obj()
	if namedObj.Pkg().Path() != "testing" {
		return false
	}
	return namedObj.Id() == paramID
}

func goGenerateCodeLens(ctx context.Context, snapshot *cache.Snapshot, fh file.Handle) ([]protocol.CodeLens, error) {
	pgf, err := snapshot.ParseGo(ctx, fh, parsego.Full)
	if err != nil {
		return nil, err
	}
	const ggDirective = "//go:generate"
	for _, c := range pgf.File.Comments {
		for _, l := range c.List {
			if !strings.HasPrefix(l.Text, ggDirective) {
				continue
			}
			rng, err := pgf.PosRange(l.Pos(), l.Pos()+token.Pos(len(ggDirective)))
			if err != nil {
				return nil, err
			}
			dir := fh.URI().Dir()
			nonRecursiveCmd := command.NewGenerateCommand("run go generate", command.GenerateArgs{Dir: dir, Recursive: false})
			recursiveCmd := command.NewGenerateCommand("run go generate ./...", command.GenerateArgs{Dir: dir, Recursive: true})
			return []protocol.CodeLens{
				{Range: rng, Command: recursiveCmd},
				{Range: rng, Command: nonRecursiveCmd},
			}, nil

		}
	}
	return nil, nil
}

func regenerateCgoLens(ctx context.Context, snapshot *cache.Snapshot, fh file.Handle) ([]protocol.CodeLens, error) {
	pgf, err := snapshot.ParseGo(ctx, fh, parsego.Full)
	if err != nil {
		return nil, err
	}
	var c *ast.ImportSpec
	for _, imp := range pgf.File.Imports {
		if imp.Path.Value == `"C"` {
			c = imp
		}
	}
	if c == nil {
		return nil, nil
	}
	rng, err := pgf.NodeRange(c)
	if err != nil {
		return nil, err
	}
	puri := fh.URI()
	cmd := command.NewRegenerateCgoCommand("regenerate cgo definitions", command.URIArg{URI: puri})
	return []protocol.CodeLens{{Range: rng, Command: cmd}}, nil
}
