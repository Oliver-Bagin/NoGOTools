// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssa

// lvalues are the union of addressable expressions and map-index
// expressions.

import (
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"github.com/tinygo-org/tinygo/alt_go/types"

	"github.com/tinygo-org/tinygo/x-tools/internal/typeparams"
)

// An lvalue represents an assignable location that may appear on the
// left-hand side of an assignment.  This is a generalization of a
// pointer to permit updates to elements of maps.
type lvalue interface {
	store(fn *Function, v Value) // stores v into the location
	load(fn *Function) Value     // loads the contents of the location
	address(fn *Function) Value  // address of the location
	typ() types.Type             // returns the type of the location
}

// An address is an lvalue represented by a true pointer.
type address struct {
	addr Value     // must have a pointer core type.
	pos  token.Pos // source position
	expr ast.Expr  // source syntax of the value (not address) [debug mode]
}

func (a *address) load(fn *Function) Value {
	load := emitLoad(fn, a.addr)
	load.pos = a.pos
	return load
}

func (a *address) store(fn *Function, v Value) {
	store := emitStore(fn, a.addr, v, a.pos)
	if a.expr != nil {
		// store.Val is v, converted for assignability.
		emitDebugRef(fn, a.expr, store.Val, false)
	}
}

func (a *address) address(fn *Function) Value {
	if a.expr != nil {
		emitDebugRef(fn, a.expr, a.addr, true)
	}
	return a.addr
}

func (a *address) typ() types.Type {
	return typeparams.MustDeref(a.addr.Type())
}

// An element is an lvalue represented by m[k], the location of an
// element of a map.  These locations are not addressable
// since pointers cannot be formed from them, but they do support
// load() and store().
type element struct {
	m, k Value      // map
	t    types.Type // map element type
	pos  token.Pos  // source position of colon ({k:v}) or lbrack (m[k]=v)
}

func (e *element) load(fn *Function) Value {
	l := &Lookup{
		X:     e.m,
		Index: e.k,
	}
	l.setPos(e.pos)
	l.setType(e.t)
	return fn.emit(l)
}

func (e *element) store(fn *Function, v Value) {
	up := &MapUpdate{
		Map:   e.m,
		Key:   e.k,
		Value: emitConv(fn, v, e.t),
	}
	up.pos = e.pos
	fn.emit(up)
}

func (e *element) address(fn *Function) Value {
	panic("map elements are not addressable")
}

func (e *element) typ() types.Type {
	return e.t
}

// A lazyAddress is an lvalue whose address is the result of an instruction.
// These work like an *address except a new address.address() Value
// is created on each load, store and address call.
// A lazyAddress can be used to control when a side effect (nil pointer
// dereference, index out of bounds) of using a location happens.
type lazyAddress struct {
	addr func(fn *Function) Value // emit to fn the computation of the address
	t    types.Type               // type of the location
	pos  token.Pos                // source position
	expr ast.Expr                 // source syntax of the value (not address) [debug mode]
}

func (l *lazyAddress) load(fn *Function) Value {
	load := emitLoad(fn, l.addr(fn))
	load.pos = l.pos
	return load
}

func (l *lazyAddress) store(fn *Function, v Value) {
	store := emitStore(fn, l.addr(fn), v, l.pos)
	if l.expr != nil {
		// store.Val is v, converted for assignability.
		emitDebugRef(fn, l.expr, store.Val, false)
	}
}

func (l *lazyAddress) address(fn *Function) Value {
	addr := l.addr(fn)
	if l.expr != nil {
		emitDebugRef(fn, l.expr, addr, true)
	}
	return addr
}

func (l *lazyAddress) typ() types.Type { return l.t }

// A blank is a dummy variable whose name is "_".
// It is not reified: loads are illegal and stores are ignored.
type blank struct{}

func (bl blank) load(fn *Function) Value {
	panic("blank.load is illegal")
}

func (bl blank) store(fn *Function, v Value) {
	// no-op
}

func (bl blank) address(fn *Function) Value {
	panic("blank var is not addressable")
}

func (bl blank) typ() types.Type {
	// This should be the type of the blank Ident; the typechecker
	// doesn't provide this yet, but fortunately, we don't need it
	// yet either.
	panic("blank.typ is unimplemented")
}
