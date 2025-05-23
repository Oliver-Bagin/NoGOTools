// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// callgraph: a tool for reporting the call graph of a Go program.
// See Usage for details, or run with -help.
package main // import "github.com/tinygo-org/tinygo/x-tools/cmd/callgraph"

// TODO(adonovan):
//
// Features:
// - restrict graph to a single package
// - output
//   - functions reachable from root (use digraph tool?)
//   - unreachable functions (use digraph tool?)
//   - dynamic (runtime) types
//   - indexed output (numbered nodes)
//   - JSON output
//   - additional template fields:
//     callee file/line/col

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"io"
	"os"
	"runtime"
	"text/template"

	"github.com/tinygo-org/tinygo/x-tools/go/callgraph"
	"github.com/tinygo-org/tinygo/x-tools/go/callgraph/cha"
	"github.com/tinygo-org/tinygo/x-tools/go/callgraph/rta"
	"github.com/tinygo-org/tinygo/x-tools/go/callgraph/static"
	"github.com/tinygo-org/tinygo/x-tools/go/callgraph/vta"
	"github.com/tinygo-org/tinygo/x-tools/go/packages"
	"github.com/tinygo-org/tinygo/x-tools/go/ssa"
	"github.com/tinygo-org/tinygo/x-tools/go/ssa/ssautil"
)

// flags
var (
	algoFlag = flag.String("algo", "rta",
		`Call graph construction algorithm (static, cha, rta, vta)`)

	testFlag = flag.Bool("test", false,
		"Loads test code (*_test.go) for imported packages")

	formatFlag = flag.String("format",
		"{{.Caller}}\t--{{.Dynamic}}-{{.Line}}:{{.Column}}-->\t{{.Callee}}",
		"A template expression specifying how to format an edge")

	tagsFlag = flag.String("tags", "", "comma-separated list of extra build tags (see: go help buildconstraint)")
)

const Usage = `callgraph: display the call graph of a Go program.

Usage:

  callgraph [-algo=static|cha|rta|vta] [-test] [-format=...] package...

Flags:

-algo      Specifies the call-graph construction algorithm, one of:

            static      static calls only (unsound)
            cha         Class Hierarchy Analysis
            rta         Rapid Type Analysis
            vta         Variable Type Analysis

           The algorithms are ordered by increasing precision in their
           treatment of dynamic calls (and thus also computational cost).
           RTA requires a whole program (main or test), and
           include only functions reachable from main.

-test      Include the package's tests in the analysis.

-format    Specifies the format in which each call graph edge is displayed.
           One of:

            digraph     output suitable for input to
                        golang.org/x/tools/cmd/digraph.
            graphviz    output in AT&T GraphViz (.dot) format.

           All other values are interpreted using text/template syntax.
           The default value is:

            {{.Caller}}\t--{{.Dynamic}}-{{.Line}}:{{.Column}}-->\t{{.Callee}}

           The structure passed to the template is (effectively):

                   type Edge struct {
                           Caller      *ssa.Function // calling function
                           Callee      *ssa.Function // called function

                           // Call site:
                           Filename    string // containing file
                           Offset      int    // offset within file of '('
                           Line        int    // line number
                           Column      int    // column number of call
                           Dynamic     string // "static" or "dynamic"
                           Description string // e.g. "static method call"
                   }

           Caller and Callee are *ssa.Function values, which print as
           "(*sync/atomic.Mutex).Lock", but other attributes may be
           derived from them. For example:

           - {{.Caller.Pkg.Pkg.Path}} yields the import path of the
             enclosing package; and

           - {{(.Caller.Prog.Fset.Position .Caller.Pos).Filename}}
             yields the name of the file that declares the caller.

           - The 'posn' template function returns the token.Position
             of an ssa.Function, so the previous example can be
             reduced to {{(posn .Caller).Filename}}.

           Consult the documentation for go/token, text/template, and
           golang.org/x/tools/go/ssa for more detail.

Examples:

  Show the call graph of the trivial web server application:

    callgraph -format digraph $GOROOT/src/net/http/triv.go

  Same, but show only the packages of each function:

    callgraph -format '{{.Caller.Pkg.Pkg.Path}} -> {{.Callee.Pkg.Pkg.Path}}' \
      $GOROOT/src/net/http/triv.go | sort | uniq

  Show functions that make dynamic calls into the 'fmt' test package,
  using the Rapid Type Analysis algorithm:

    callgraph -format='{{.Caller}} -{{.Dynamic}}-> {{.Callee}}' -test -algo=rta fmt |
      sed -ne 's/-dynamic-/--/p' |
      sed -ne 's/-->.*fmt_test.*$//p' | sort | uniq

  Show all functions directly called by the callgraph tool's main function:

    callgraph -format=digraph golang.org/x/tools/cmd/callgraph |
      digraph succs golang.org/x/tools/cmd/callgraph.main
`

func init() {
	// If $GOMAXPROCS isn't set, use the full capacity of the machine.
	// For small machines, use at least 4 threads.
	if os.Getenv("GOMAXPROCS") == "" {
		n := runtime.NumCPU()
		if n < 4 {
			n = 4
		}
		runtime.GOMAXPROCS(n)
	}
}

func main() {
	flag.Parse()
	if err := doCallgraph("", "", *algoFlag, *formatFlag, *testFlag, flag.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "callgraph: %s\n", err)
		os.Exit(1)
	}
}

var stdout io.Writer = os.Stdout

func doCallgraph(dir, gopath, algo, format string, tests bool, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, Usage)
		return nil
	}

	cfg := &packages.Config{
		Mode:       packages.LoadAllSyntax,
		BuildFlags: []string{"-tags=" + *tagsFlag},
		Tests:      tests,
		Dir:        dir,
	}
	if gopath != "" {
		cfg.Env = append(os.Environ(), "GOPATH="+gopath) // to enable testing
	}
	initial, err := packages.Load(cfg, args...)
	if err != nil {
		return err
	}
	if packages.PrintErrors(initial) > 0 {
		return fmt.Errorf("packages contain errors")
	}

	// Create and build SSA-form program representation.
	mode := ssa.InstantiateGenerics // instantiate generics by default for soundness
	prog, pkgs := ssautil.AllPackages(initial, mode)
	prog.Build()

	// -- call graph construction ------------------------------------------

	var cg *callgraph.Graph

	switch algo {
	case "static":
		cg = static.CallGraph(prog)

	case "cha":
		cg = cha.CallGraph(prog)

	case "pta":
		return fmt.Errorf("pointer analysis is no longer supported (see Go issue #59676)")

	case "rta":
		mains, err := mainPackages(pkgs)
		if err != nil {
			return err
		}
		var roots []*ssa.Function
		for _, main := range mains {
			roots = append(roots, main.Func("init"), main.Func("main"))
		}
		rtares := rta.Analyze(roots, true)
		cg = rtares.CallGraph

		// NB: RTA gives us Reachable and RuntimeTypes too.

	case "vta":
		cg = vta.CallGraph(ssautil.AllFunctions(prog), nil)

	default:
		return fmt.Errorf("unknown algorithm: %s", algo)
	}

	cg.DeleteSyntheticNodes()

	// -- output------------------------------------------------------------

	var before, after string

	// Pre-canned formats.
	switch format {
	case "digraph":
		format = `{{printf "%q %q" .Caller .Callee}}`

	case "graphviz":
		before = "digraph callgraph {\n"
		after = "}\n"
		format = `  {{printf "%q" .Caller}} -> {{printf "%q" .Callee}}`
	}

	funcMap := template.FuncMap{
		"posn": func(f *ssa.Function) token.Position {
			return f.Prog.Fset.Position(f.Pos())
		},
	}
	tmpl, err := template.New("-format").Funcs(funcMap).Parse(format)
	if err != nil {
		return fmt.Errorf("invalid -format template: %v", err)
	}

	// Allocate these once, outside the traversal.
	var buf bytes.Buffer
	data := Edge{fset: prog.Fset}

	fmt.Fprint(stdout, before)
	if err := callgraph.GraphVisitEdges(cg, func(edge *callgraph.Edge) error {
		data.position.Offset = -1
		data.edge = edge
		data.Caller = edge.Caller.Func
		data.Callee = edge.Callee.Func

		buf.Reset()
		if err := tmpl.Execute(&buf, &data); err != nil {
			return err
		}
		stdout.Write(buf.Bytes())
		if len := buf.Len(); len == 0 || buf.Bytes()[len-1] != '\n' {
			fmt.Fprintln(stdout)
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Fprint(stdout, after)
	return nil
}

// mainPackages returns the main packages to analyze.
// Each resulting package is named "main" and has a main function.
func mainPackages(pkgs []*ssa.Package) ([]*ssa.Package, error) {
	var mains []*ssa.Package
	for _, p := range pkgs {
		if p != nil && p.Pkg.Name() == "main" && p.Func("main") != nil {
			mains = append(mains, p)
		}
	}
	if len(mains) == 0 {
		return nil, fmt.Errorf("no main packages")
	}
	return mains, nil
}

type Edge struct {
	Caller *ssa.Function
	Callee *ssa.Function

	edge     *callgraph.Edge
	fset     *token.FileSet
	position token.Position // initialized lazily
}

func (e *Edge) pos() *token.Position {
	if e.position.Offset == -1 {
		e.position = e.fset.Position(e.edge.Pos()) // called lazily
	}
	return &e.position
}

func (e *Edge) Filename() string { return e.pos().Filename }
func (e *Edge) Column() int      { return e.pos().Column }
func (e *Edge) Line() int        { return e.pos().Line }
func (e *Edge) Offset() int      { return e.pos().Offset }

func (e *Edge) Dynamic() string {
	if e.edge.Site != nil && e.edge.Site.Common().StaticCallee() == nil {
		return "dynamic"
	}
	return "static"
}

func (e *Edge) Description() string { return e.edge.Description() }
