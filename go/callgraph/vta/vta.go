// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package vta computes the call graph of a Go program using the Variable
// Type Analysis (VTA) algorithm originally described in "Practical Virtual
// Method Call Resolution for Java," Vijay Sundaresan, Laurie Hendren,
// Chrislain Razafimahefa, Raja Vallée-Rai, Patrick Lam, Etienne Gagnon, and
// Charles Godin.
//
// Note: this package is in experimental phase and its interface is
// subject to change.
// TODO(zpavlinovic): reiterate on documentation.
//
// The VTA algorithm overapproximates the set of types (and function literals)
// a variable can take during runtime by building a global type propagation
// graph and propagating types (and function literals) through the graph.
//
// A type propagation is a directed, labeled graph. A node can represent
// one of the following:
//   - A field of a struct type.
//   - A local (SSA) variable of a method/function.
//   - All pointers to a non-interface type.
//   - The return value of a method.
//   - All elements in an array.
//   - All elements in a slice.
//   - All elements in a map.
//   - All elements in a channel.
//   - A global variable.
//
// In addition, the implementation used in this package introduces
// a few Go specific kinds of nodes:
//   - (De)references of nested pointers to interfaces are modeled
//     as a unique nestedPtrInterface node in the type propagation graph.
//   - Each function literal is represented as a function node whose
//     internal value is the (SSA) representation of the function. This
//     is done to precisely infer flow of higher-order functions.
//
// Edges in the graph represent flow of types (and function literals) through
// the program. That is, the model 1) typing constraints that are induced by
// assignment statements or function and method calls and 2) higher-order flow
// of functions in the program.
//
// The labeling function maps each node to a set of types and functions that
// can intuitively reach the program construct the node represents. Initially,
// every node is assigned a type corresponding to the program construct it
// represents. Function nodes are also assigned the function they represent.
// The labeling function then propagates types and function through the graph.
//
// The result of VTA is a type propagation graph in which each node is labeled
// with a conservative overapproximation of the set of types (and functions)
// it may have. This information is then used to construct the call graph.
// For each unresolved call site, vta uses the set of types and functions
// reaching the node representing the call site to create a set of callees.
package vta

// TODO(zpavlinovic): update VTA for how it handles generic function bodies and instantiation wrappers.

import (
	"github.com/tinygo-org/tinygo/alt_go/types"

	"github.com/tinygo-org/tinygo/x-tools/go/callgraph"
	"github.com/tinygo-org/tinygo/x-tools/go/ssa"
)

// CallGraph uses the VTA algorithm to compute call graph for all functions
// f:true in funcs. VTA refines the results of initial call graph and uses it
// to establish interprocedural type flow. If initial is nil, VTA uses a more
// efficient approach to construct a CHA call graph.
//
// The resulting graph does not have a root node.
//
// CallGraph does not make any assumptions on initial types global variables
// and function/method inputs can have. CallGraph is then sound, modulo use of
// reflection and unsafe, if the initial call graph is sound.
func CallGraph(funcs map[*ssa.Function]bool, initial *callgraph.Graph) *callgraph.Graph {
	callees := makeCalleesFunc(funcs, initial)
	vtaG, canon := typePropGraph(funcs, callees)
	types := propagate(vtaG, canon)

	c := &constructor{types: types, callees: callees, cache: make(methodCache)}
	return c.construct(funcs)
}

// constructor type linearly traverses the input program
// and constructs a callgraph based on the results of the
// VTA type propagation phase.
type constructor struct {
	types   propTypeMap
	cache   methodCache
	callees calleesFunc
}

func (c *constructor) construct(funcs map[*ssa.Function]bool) *callgraph.Graph {
	cg := &callgraph.Graph{Nodes: make(map[*ssa.Function]*callgraph.Node)}
	for f, in := range funcs {
		if in {
			c.constrct(cg, f)
		}
	}
	return cg
}

func (c *constructor) constrct(g *callgraph.Graph, f *ssa.Function) {
	caller := g.CreateNode(f)
	for _, call := range calls(f) {
		for _, c := range c.resolves(call) {
			callgraph.AddEdge(caller, call, g.CreateNode(c))
		}
	}
}

// resolves computes the set of functions to which VTA resolves `c`. The resolved
// functions are intersected with functions to which `c.initial` resolves `c`.
func (c *constructor) resolves(call ssa.CallInstruction) []*ssa.Function {
	cc := call.Common()
	if cc.StaticCallee() != nil {
		return []*ssa.Function{cc.StaticCallee()}
	}

	// Skip builtins as they are not *ssa.Function.
	if _, ok := cc.Value.(*ssa.Builtin); ok {
		return nil
	}

	// Cover the case of dynamic higher-order and interface calls.
	var res []*ssa.Function
	resolved := resolve(call, c.types, c.cache)
	for f := range siteCallees(call, c.callees) {
		if _, ok := resolved[f]; ok {
			res = append(res, f)
		}
	}
	return res
}

// resolve returns a set of functions `c` resolves to based on the
// type propagation results in `types`.
func resolve(c ssa.CallInstruction, types propTypeMap, cache methodCache) map[*ssa.Function]empty {
	fns := make(map[*ssa.Function]empty)
	n := local{val: c.Common().Value}
	for p := range types.propTypes(n) {
		for _, f := range propFunc(p, c, cache) {
			fns[f] = empty{}
		}
	}
	return fns
}

// propFunc returns the functions modeled with the propagation type `p`
// assigned to call site `c`. If no such function exists, nil is returned.
func propFunc(p propType, c ssa.CallInstruction, cache methodCache) []*ssa.Function {
	if p.f != nil {
		return []*ssa.Function{p.f}
	}

	if c.Common().Method == nil {
		return nil
	}

	return cache.methods(p.typ, c.Common().Method.Name(), c.Parent().Prog)
}

// methodCache serves as a type -> method name -> methods
// cache when computing methods of a type using the
// ssa.Program.MethodSets and ssa.Program.MethodValue
// APIs. The cache is used to speed up querying of
// methods of a type as the mentioned APIs are expensive.
type methodCache map[types.Type]map[string][]*ssa.Function

// methods returns methods of a type `t` named `name`. First consults
// `mc` and otherwise queries `prog` for the method. If no such method
// exists, nil is returned.
func (mc methodCache) methods(t types.Type, name string, prog *ssa.Program) []*ssa.Function {
	if ms, ok := mc[t]; ok {
		return ms[name]
	}

	ms := make(map[string][]*ssa.Function)
	mset := prog.MethodSets.MethodSet(t)
	for i, n := 0, mset.Len(); i < n; i++ {
		// f can be nil when t is an interface or some
		// other type without any runtime methods.
		if f := prog.MethodValue(mset.At(i)); f != nil {
			ms[f.Name()] = append(ms[f.Name()], f)
		}
	}
	mc[t] = ms
	return ms[name]
}
