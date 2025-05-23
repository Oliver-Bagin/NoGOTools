// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssa

// This package defines a high-level intermediate representation for
// Go programs using static single-assignment (SSA) form.

import (
	"fmt"
	"github.com/tinygo-org/tinygo/alt_go/ast"
	"github.com/tinygo-org/tinygo/alt_go/constant"
	"github.com/tinygo-org/tinygo/alt_go/token"
	"github.com/tinygo-org/tinygo/alt_go/types"
	"sync"

	"github.com/tinygo-org/tinygo/x-tools/go/types/typeutil"
	"github.com/tinygo-org/tinygo/x-tools/internal/typeparams"
)

// A Program is a partial or complete Go program converted to SSA form.
type Program struct {
	Fset       *token.FileSet              // position information for the files of this Program
	imported   map[string]*Package         // all importable Packages, keyed by import path
	packages   map[*types.Package]*Package // all created Packages
	mode       BuilderMode                 // set of mode bits for SSA construction
	MethodSets typeutil.MethodSetCache     // cache of type-checker's method-sets

	canon *canonizer     // type canonicalization map
	ctxt  *types.Context // cache for type checking instantiations

	methodsMu  sync.Mutex
	methodSets typeutil.Map // maps type to its concrete *methodSet

	// memoization of whether a type refers to type parameters
	hasParamsMu sync.Mutex
	hasParams   typeparams.Free

	// set of concrete types used as MakeInterface operands
	makeInterfaceTypesMu sync.Mutex
	makeInterfaceTypes   map[types.Type]unit // (may contain redundant identical types)

	// objectMethods is a memoization of objectMethod
	// to avoid creation of duplicate methods from type information.
	objectMethodsMu sync.Mutex
	objectMethods   map[*types.Func]*Function
}

// A Package is a single analyzed Go package containing Members for
// all package-level functions, variables, constants and types it
// declares.  These may be accessed directly via Members, or via the
// type-specific accessor methods Func, Type, Var and Const.
//
// Members also contains entries for "init" (the synthetic package
// initializer) and "init#%d", the nth declared init function,
// and unspecified other things too.
type Package struct {
	Prog    *Program                // the owning program
	Pkg     *types.Package          // the corresponding go/types.Package
	Members map[string]Member       // all package members keyed by name (incl. init and init#%d)
	objects map[types.Object]Member // mapping of package objects to members (incl. methods). Contains *NamedConst, *Global, *Function (values but not types)
	init    *Function               // Func("init"); the package's init function
	debug   bool                    // include full debug info in this package
	syntax  bool                    // package was loaded from syntax

	// The following fields are set transiently, then cleared
	// after building.
	buildOnce   sync.Once           // ensures package building occurs once
	ninit       int32               // number of init functions
	info        *types.Info         // package type information
	files       []*ast.File         // package ASTs
	created     []*Function         // members created as a result of building this package (includes declared functions, wrappers)
	initVersion map[ast.Expr]string // goversion to use for each global var init expr
}

// A Member is a member of a Go package, implemented by *NamedConst,
// *Global, *Function, or *Type; they are created by package-level
// const, var, func and type declarations respectively.
type Member interface {
	Name() string                    // declared name of the package member
	String() string                  // package-qualified name of the package member
	RelString(*types.Package) string // like String, but relative refs are unqualified
	Object() types.Object            // typechecker's object for this member, if any
	Pos() token.Pos                  // position of member's declaration, if known
	Type() types.Type                // type of the package member
	Token() token.Token              // token.{VAR,FUNC,CONST,TYPE}
	Package() *Package               // the containing package
}

// A Type is a Member of a Package representing a package-level named type.
type Type struct {
	object *types.TypeName
	pkg    *Package
}

// A NamedConst is a Member of a Package representing a package-level
// named constant.
//
// Pos() returns the position of the declaring ast.ValueSpec.Names[*]
// identifier.
//
// NB: a NamedConst is not a Value; it contains a constant Value, which
// it augments with the name and position of its 'const' declaration.
type NamedConst struct {
	object *types.Const
	Value  *Const
	pkg    *Package
}

// A Value is an SSA value that can be referenced by an instruction.
type Value interface {
	// Name returns the name of this value, and determines how
	// this Value appears when used as an operand of an
	// Instruction.
	//
	// This is the same as the source name for Parameters,
	// Builtins, Functions, FreeVars, Globals.
	// For constants, it is a representation of the constant's value
	// and type.  For all other Values this is the name of the
	// virtual register defined by the instruction.
	//
	// The name of an SSA Value is not semantically significant,
	// and may not even be unique within a function.
	Name() string

	// If this value is an Instruction, String returns its
	// disassembled form; otherwise it returns unspecified
	// human-readable information about the Value, such as its
	// kind, name and type.
	String() string

	// Type returns the type of this value.  Many instructions
	// (e.g. IndexAddr) change their behaviour depending on the
	// types of their operands.
	Type() types.Type

	// Parent returns the function to which this Value belongs.
	// It returns nil for named Functions, Builtin, Const and Global.
	Parent() *Function

	// Referrers returns the list of instructions that have this
	// value as one of their operands; it may contain duplicates
	// if an instruction has a repeated operand.
	//
	// Referrers actually returns a pointer through which the
	// caller may perform mutations to the object's state.
	//
	// Referrers is currently only defined if Parent()!=nil,
	// i.e. for the function-local values FreeVar, Parameter,
	// Functions (iff anonymous) and all value-defining instructions.
	// It returns nil for named Functions, Builtin, Const and Global.
	//
	// Instruction.Operands contains the inverse of this relation.
	Referrers() *[]Instruction

	// Pos returns the location of the AST token most closely
	// associated with the operation that gave rise to this value,
	// or token.NoPos if it was not explicit in the source.
	//
	// For each ast.Node type, a particular token is designated as
	// the closest location for the expression, e.g. the Lparen
	// for an *ast.CallExpr.  This permits a compact but
	// approximate mapping from Values to source positions for use
	// in diagnostic messages, for example.
	//
	// (Do not use this position to determine which Value
	// corresponds to an ast.Expr; use Function.ValueForExpr
	// instead.  NB: it requires that the function was built with
	// debug information.)
	Pos() token.Pos
}

// An Instruction is an SSA instruction that computes a new Value or
// has some effect.
//
// An Instruction that defines a value (e.g. BinOp) also implements
// the Value interface; an Instruction that only has an effect (e.g. Store)
// does not.
type Instruction interface {
	// String returns the disassembled form of this value.
	//
	// Examples of Instructions that are Values:
	//       "x + y"     (BinOp)
	//       "len([])"   (Call)
	// Note that the name of the Value is not printed.
	//
	// Examples of Instructions that are not Values:
	//       "return x"  (Return)
	//       "*y = x"    (Store)
	//
	// (The separation Value.Name() from Value.String() is useful
	// for some analyses which distinguish the operation from the
	// value it defines, e.g., 'y = local int' is both an allocation
	// of memory 'local int' and a definition of a pointer y.)
	String() string

	// Parent returns the function to which this instruction
	// belongs.
	Parent() *Function

	// Block returns the basic block to which this instruction
	// belongs.
	Block() *BasicBlock

	// setBlock sets the basic block to which this instruction belongs.
	setBlock(*BasicBlock)

	// Operands returns the operands of this instruction: the
	// set of Values it references.
	//
	// Specifically, it appends their addresses to rands, a
	// user-provided slice, and returns the resulting slice,
	// permitting avoidance of memory allocation.
	//
	// The operands are appended in undefined order, but the order
	// is consistent for a given Instruction; the addresses are
	// always non-nil but may point to a nil Value.  Clients may
	// store through the pointers, e.g. to effect a value
	// renaming.
	//
	// Value.Referrers is a subset of the inverse of this
	// relation.  (Referrers are not tracked for all types of
	// Values.)
	Operands(rands []*Value) []*Value

	// Pos returns the location of the AST token most closely
	// associated with the operation that gave rise to this
	// instruction, or token.NoPos if it was not explicit in the
	// source.
	//
	// For each ast.Node type, a particular token is designated as
	// the closest location for the expression, e.g. the Go token
	// for an *ast.GoStmt.  This permits a compact but approximate
	// mapping from Instructions to source positions for use in
	// diagnostic messages, for example.
	//
	// (Do not use this position to determine which Instruction
	// corresponds to an ast.Expr; see the notes for Value.Pos.
	// This position may be used to determine which non-Value
	// Instruction corresponds to some ast.Stmts, but not all: If
	// and Jump instructions have no Pos(), for example.)
	Pos() token.Pos
}

// A Node is a node in the SSA value graph.  Every concrete type that
// implements Node is also either a Value, an Instruction, or both.
//
// Node contains the methods common to Value and Instruction, plus the
// Operands and Referrers methods generalized to return nil for
// non-Instructions and non-Values, respectively.
//
// Node is provided to simplify SSA graph algorithms.  Clients should
// use the more specific and informative Value or Instruction
// interfaces where appropriate.
type Node interface {
	// Common methods:
	String() string
	Pos() token.Pos
	Parent() *Function

	// Partial methods:
	Operands(rands []*Value) []*Value // nil for non-Instructions
	Referrers() *[]Instruction        // nil for non-Values
}

// Function represents the parameters, results, and code of a function
// or method.
//
// If Blocks is nil, this indicates an external function for which no
// Go source code is available.  In this case, FreeVars, Locals, and
// Params are nil too.  Clients performing whole-program analysis must
// handle external functions specially.
//
// Blocks contains the function's control-flow graph (CFG).
// Blocks[0] is the function entry point; block order is not otherwise
// semantically significant, though it may affect the readability of
// the disassembly.
// To iterate over the blocks in dominance order, use DomPreorder().
//
// Recover is an optional second entry point to which control resumes
// after a recovered panic.  The Recover block may contain only a return
// statement, preceded by a load of the function's named return
// parameters, if any.
//
// A nested function (Parent()!=nil) that refers to one or more
// lexically enclosing local variables ("free variables") has FreeVars.
// Such functions cannot be called directly but require a
// value created by MakeClosure which, via its Bindings, supplies
// values for these parameters.
//
// If the function is a method (Signature.Recv() != nil) then the first
// element of Params is the receiver parameter.
//
// A Go package may declare many functions called "init".
// For each one, Object().Name() returns "init" but Name() returns
// "init#1", etc, in declaration order.
//
// Pos() returns the declaring ast.FuncLit.Type.Func or the position
// of the ast.FuncDecl.Name, if the function was explicit in the
// source. Synthetic wrappers, for which Synthetic != "", may share
// the same position as the function they wrap.
// Syntax.Pos() always returns the position of the declaring "func" token.
//
// When the operand of a range statement is an iterator function,
// the loop body is transformed into a synthetic anonymous function
// that is passed as the yield argument in a call to the iterator.
// In that case, Function.Pos is the position of the "range" token,
// and Function.Syntax is the ast.RangeStmt.
//
// Synthetic functions, for which Synthetic != "", are functions
// that do not appear in the source AST. These include:
//   - method wrappers,
//   - thunks,
//   - bound functions,
//   - empty functions built from loaded type information,
//   - yield functions created from range-over-func loops,
//   - package init functions, and
//   - instantiations of generic functions.
//
// Synthetic wrapper functions may share the same position
// as the function they wrap.
//
// Type() returns the function's Signature.
//
// A generic function is a function or method that has uninstantiated type
// parameters (TypeParams() != nil). Consider a hypothetical generic
// method, (*Map[K,V]).Get. It may be instantiated with all
// non-parameterized types as (*Map[string,int]).Get or with
// parameterized types as (*Map[string,U]).Get, where U is a type parameter.
// In both instantiations, Origin() refers to the instantiated generic
// method, (*Map[K,V]).Get, TypeParams() refers to the parameters [K,V] of
// the generic method. TypeArgs() refers to [string,U] or [string,int],
// respectively, and is nil in the generic method.
type Function struct {
	name      string
	object    *types.Func // symbol for declared function (nil for FuncLit or synthetic init)
	method    *selection  // info about provenance of synthetic methods; thunk => non-nil
	Signature *types.Signature
	pos       token.Pos

	// source information
	Synthetic string      // provenance of synthetic function; "" for true source functions
	syntax    ast.Node    // *ast.Func{Decl,Lit}, if from syntax (incl. generic instances) or (*ast.RangeStmt if a yield function)
	info      *types.Info // type annotations (if syntax != nil)
	goversion string      // Go version of syntax (NB: init is special)

	parent *Function // enclosing function if anon; nil if global
	Pkg    *Package  // enclosing package; nil for shared funcs (wrappers and error.Error)
	Prog   *Program  // enclosing program

	buildshared *task // wait for a shared function to be done building (may be nil if <=1 builder ever needs to wait)

	// These fields are populated only when the function body is built:

	Params    []*Parameter  // function parameters; for methods, includes receiver
	FreeVars  []*FreeVar    // free variables whose values must be supplied by closure
	Locals    []*Alloc      // frame-allocated variables of this function
	Blocks    []*BasicBlock // basic blocks of the function; nil => external
	Recover   *BasicBlock   // optional; control transfers here after recovered panic
	AnonFuncs []*Function   // anonymous functions (from FuncLit,RangeStmt) directly beneath this one
	referrers []Instruction // referring instructions (iff Parent() != nil)
	anonIdx   int32         // position of a nested function in parent's AnonFuncs. fn.Parent()!=nil => fn.Parent().AnonFunc[fn.anonIdx] == fn.

	typeparams     *types.TypeParamList // type parameters of this function. typeparams.Len() > 0 => generic or instance of generic function
	typeargs       []types.Type         // type arguments that instantiated typeparams. len(typeargs) > 0 => instance of generic function
	topLevelOrigin *Function            // the origin function if this is an instance of a source function. nil if Parent()!=nil.
	generic        *generic             // instances of this function, if generic

	// The following fields are cleared after building.
	build        buildFunc                // algorithm to build function body (nil => built)
	currentBlock *BasicBlock              // where to emit code
	vars         map[*types.Var]Value     // addresses of local variables
	results      []*Alloc                 // result allocations of the current function
	returnVars   []*types.Var             // variables for a return statement. Either results or for range-over-func a parent's results
	targets      *targets                 // linked stack of branch targets
	lblocks      map[*types.Label]*lblock // labelled blocks
	subst        *subster                 // type parameter substitutions (if non-nil)
	jump         *types.Var               // synthetic variable for the yield state (non-nil => range-over-func)
	deferstack   *types.Var               // synthetic variable holding enclosing ssa:deferstack()
	source       *Function                // nearest enclosing source function
	exits        []*exit                  // exits of the function that need to be resolved
	uniq         int64                    // source of unique ints within the source tree while building
}

// BasicBlock represents an SSA basic block.
//
// The final element of Instrs is always an explicit transfer of
// control (If, Jump, Return, or Panic).
//
// A block may contain no Instructions only if it is unreachable,
// i.e., Preds is nil.  Empty blocks are typically pruned.
//
// BasicBlocks and their Preds/Succs relation form a (possibly cyclic)
// graph independent of the SSA Value graph: the control-flow graph or
// CFG.  It is illegal for multiple edges to exist between the same
// pair of blocks.
//
// Each BasicBlock is also a node in the dominator tree of the CFG.
// The tree may be navigated using Idom()/Dominees() and queried using
// Dominates().
//
// The order of Preds and Succs is significant (to Phi and If
// instructions, respectively).
type BasicBlock struct {
	Index        int            // index of this block within Parent().Blocks
	Comment      string         // optional label; no semantic significance
	parent       *Function      // parent function
	Instrs       []Instruction  // instructions in order
	Preds, Succs []*BasicBlock  // predecessors and successors
	succs2       [2]*BasicBlock // initial space for Succs
	dom          domInfo        // dominator tree info
	gaps         int            // number of nil Instrs (transient)
	rundefers    int            // number of rundefers (transient)
}

// Pure values ----------------------------------------

// A FreeVar represents a free variable of the function to which it
// belongs.
//
// FreeVars are used to implement anonymous functions, whose free
// variables are lexically captured in a closure formed by
// MakeClosure.  The value of such a free var is an Alloc or another
// FreeVar and is considered a potentially escaping heap address, with
// pointer type.
//
// FreeVars are also used to implement bound method closures.  Such a
// free var represents the receiver value and may be of any type that
// has concrete methods.
//
// Pos() returns the position of the value that was captured, which
// belongs to an enclosing function.
type FreeVar struct {
	name      string
	typ       types.Type
	pos       token.Pos
	parent    *Function
	referrers []Instruction

	// Transiently needed during building.
	outer Value // the Value captured from the enclosing context.
}

// A Parameter represents an input parameter of a function.
type Parameter struct {
	name      string
	object    *types.Var // non-nil
	typ       types.Type
	parent    *Function
	referrers []Instruction
}

// A Const represents a value known at build time.
//
// Consts include true constants of boolean, numeric, and string types, as
// defined by the Go spec; these are represented by a non-nil Value field.
//
// Consts also include the "zero" value of any type, of which the nil values
// of various pointer-like types are a special case; these are represented
// by a nil Value field.
//
// Pos() returns token.NoPos.
//
// Example printed forms:
//
//		42:int
//		"hello":untyped string
//		3+4i:MyComplex
//		nil:*int
//		nil:[]string
//		[3]int{}:[3]int
//		struct{x string}{}:struct{x string}
//	    0:interface{int|int64}
//	    nil:interface{bool|int} // no go/constant representation
type Const struct {
	typ   types.Type
	Value constant.Value
}

// A Global is a named Value holding the address of a package-level
// variable.
//
// Pos() returns the position of the ast.ValueSpec.Names[*]
// identifier.
type Global struct {
	name   string
	object types.Object // a *types.Var; may be nil for synthetics e.g. init$guard
	typ    types.Type
	pos    token.Pos

	Pkg *Package
}

// A Builtin represents a specific use of a built-in function, e.g. len.
//
// Builtins are immutable values.  Builtins do not have addresses.
// Builtins can only appear in CallCommon.Value.
//
// Name() indicates the function: one of the built-in functions from the
// Go spec (excluding "make" and "new") or one of these ssa-defined
// intrinsics:
//
//	// wrapnilchk returns ptr if non-nil, panics otherwise.
//	// (For use in indirection wrappers.)
//	func ssa:wrapnilchk(ptr *T, recvType, methodName string) *T
//
// Object() returns a *types.Builtin for built-ins defined by the spec,
// nil for others.
//
// Type() returns a *types.Signature representing the effective
// signature of the built-in for this call.
type Builtin struct {
	name string
	sig  *types.Signature
}

// Value-defining instructions  ----------------------------------------

// The Alloc instruction reserves space for a variable of the given type,
// zero-initializes it, and yields its address.
//
// Alloc values are always addresses, and have pointer types, so the
// type of the allocated variable is actually
// Type().Underlying().(*types.Pointer).Elem().
//
// If Heap is false, Alloc zero-initializes the same local variable in
// the call frame and returns its address; in this case the Alloc must
// be present in Function.Locals. We call this a "local" alloc.
//
// If Heap is true, Alloc allocates a new zero-initialized variable
// each time the instruction is executed. We call this a "new" alloc.
//
// When Alloc is applied to a channel, map or slice type, it returns
// the address of an uninitialized (nil) reference of that kind; store
// the result of MakeSlice, MakeMap or MakeChan in that location to
// instantiate these types.
//
// Pos() returns the ast.CompositeLit.Lbrace for a composite literal,
// or the ast.CallExpr.Rparen for a call to new() or for a call that
// allocates a varargs slice.
//
// Example printed form:
//
//	t0 = local int
//	t1 = new int
type Alloc struct {
	register
	Comment string
	Heap    bool
	index   int // dense numbering; for lifting
}

// The Phi instruction represents an SSA φ-node, which combines values
// that differ across incoming control-flow edges and yields a new
// value.  Within a block, all φ-nodes must appear before all non-φ
// nodes.
//
// Pos() returns the position of the && or || for short-circuit
// control-flow joins, or that of the *Alloc for φ-nodes inserted
// during SSA renaming.
//
// Example printed form:
//
//	t2 = phi [0: t0, 1: t1]
type Phi struct {
	register
	Comment string  // a hint as to its purpose
	Edges   []Value // Edges[i] is value for Block().Preds[i]
}

// The Call instruction represents a function or method call.
//
// The Call instruction yields the function result if there is exactly
// one.  Otherwise it returns a tuple, the components of which are
// accessed via Extract.
//
// See CallCommon for generic function call documentation.
//
// Pos() returns the ast.CallExpr.Lparen, if explicit in the source.
//
// Example printed form:
//
//	t2 = println(t0, t1)
//	t4 = t3()
//	t7 = invoke t5.Println(...t6)
type Call struct {
	register
	Call CallCommon
}

// The BinOp instruction yields the result of binary operation X Op Y.
//
// Pos() returns the ast.BinaryExpr.OpPos, if explicit in the source.
//
// Example printed form:
//
//	t1 = t0 + 1:int
type BinOp struct {
	register
	// One of:
	// ADD SUB MUL QUO REM          + - * / %
	// AND OR XOR SHL SHR AND_NOT   & | ^ << >> &^
	// EQL NEQ LSS LEQ GTR GEQ      == != < <= < >=
	Op   token.Token
	X, Y Value
}

// The UnOp instruction yields the result of Op X.
// ARROW is channel receive.
// MUL is pointer indirection (load).
// XOR is bitwise complement.
// SUB is negation.
// NOT is logical negation.
//
// If CommaOk and Op=ARROW, the result is a 2-tuple of the value above
// and a boolean indicating the success of the receive.  The
// components of the tuple are accessed using Extract.
//
// Pos() returns the ast.UnaryExpr.OpPos, if explicit in the source.
// For receive operations (ARROW) implicit in ranging over a channel,
// Pos() returns the ast.RangeStmt.For.
// For implicit memory loads (STAR), Pos() returns the position of the
// most closely associated source-level construct; the details are not
// specified.
//
// Example printed form:
//
//	t0 = *x
//	t2 = <-t1,ok
type UnOp struct {
	register
	Op      token.Token // One of: NOT SUB ARROW MUL XOR ! - <- * ^
	X       Value
	CommaOk bool
}

// The ChangeType instruction applies to X a value-preserving type
// change to Type().
//
// Type changes are permitted:
//   - between a named type and its underlying type.
//   - between two named types of the same underlying type.
//   - between (possibly named) pointers to identical base types.
//   - from a bidirectional channel to a read- or write-channel,
//     optionally adding/removing a name.
//   - between a type (t) and an instance of the type (tσ), i.e.
//     Type() == σ(X.Type()) (or X.Type()== σ(Type())) where
//     σ is the type substitution of Parent().TypeParams by
//     Parent().TypeArgs.
//
// This operation cannot fail dynamically.
//
// Type changes may to be to or from a type parameter (or both). All
// types in the type set of X.Type() have a value-preserving type
// change to all types in the type set of Type().
//
// Pos() returns the ast.CallExpr.Lparen, if the instruction arose
// from an explicit conversion in the source.
//
// Example printed form:
//
//	t1 = changetype *int <- IntPtr (t0)
type ChangeType struct {
	register
	X Value
}

// The Convert instruction yields the conversion of value X to type
// Type().  One or both of those types is basic (but possibly named).
//
// A conversion may change the value and representation of its operand.
// Conversions are permitted:
//   - between real numeric types.
//   - between complex numeric types.
//   - between string and []byte or []rune.
//   - between pointers and unsafe.Pointer.
//   - between unsafe.Pointer and uintptr.
//   - from (Unicode) integer to (UTF-8) string.
//
// A conversion may imply a type name change also.
//
// Conversions may to be to or from a type parameter. All types in
// the type set of X.Type() can be converted to all types in the type
// set of Type().
//
// This operation cannot fail dynamically.
//
// Conversions of untyped string/number/bool constants to a specific
// representation are eliminated during SSA construction.
//
// Pos() returns the ast.CallExpr.Lparen, if the instruction arose
// from an explicit conversion in the source.
//
// Example printed form:
//
//	t1 = convert []byte <- string (t0)
type Convert struct {
	register
	X Value
}

// The MultiConvert instruction yields the conversion of value X to type
// Type(). Either X.Type() or Type() must be a type parameter. Each
// type in the type set of X.Type() can be converted to each type in the
// type set of Type().
//
// See the documentation for Convert, ChangeType, and SliceToArrayPointer
// for the conversions that are permitted. Additionally conversions of
// slices to arrays are permitted.
//
// This operation can fail dynamically (see SliceToArrayPointer).
//
// Pos() returns the ast.CallExpr.Lparen, if the instruction arose
// from an explicit conversion in the source.
//
// Example printed form:
//
//	t1 = multiconvert D <- S (t0) [*[2]rune <- []rune | string <- []rune]
type MultiConvert struct {
	register
	X        Value
	from, to types.Type
}

// ChangeInterface constructs a value of one interface type from a
// value of another interface type known to be assignable to it.
// This operation cannot fail.
//
// Pos() returns the ast.CallExpr.Lparen if the instruction arose from
// an explicit T(e) conversion; the ast.TypeAssertExpr.Lparen if the
// instruction arose from an explicit e.(T) operation; or token.NoPos
// otherwise.
//
// Example printed form:
//
//	t1 = change interface interface{} <- I (t0)
type ChangeInterface struct {
	register
	X Value
}

// The SliceToArrayPointer instruction yields the conversion of slice X to
// array pointer.
//
// Pos() returns the ast.CallExpr.Lparen, if the instruction arose
// from an explicit conversion in the source.
//
// Conversion may to be to or from a type parameter. All types in
// the type set of X.Type() must be a slice types that can be converted to
// all types in the type set of Type() which must all be pointer to array
// types.
//
// This operation can fail dynamically if the length of the slice is less
// than the length of the array.
//
// Example printed form:
//
//	t1 = slice to array pointer *[4]byte <- []byte (t0)
type SliceToArrayPointer struct {
	register
	X Value
}

// MakeInterface constructs an instance of an interface type from a
// value of a concrete type.
//
// Use Program.MethodSets.MethodSet(X.Type()) to find the method-set
// of X, and Program.MethodValue(m) to find the implementation of a method.
//
// To construct the zero value of an interface type T, use:
//
//	NewConst(constant.MakeNil(), T, pos)
//
// Pos() returns the ast.CallExpr.Lparen, if the instruction arose
// from an explicit conversion in the source.
//
// Example printed form:
//
//	t1 = make interface{} <- int (42:int)
//	t2 = make Stringer <- t0
type MakeInterface struct {
	register
	X Value
}

// The MakeClosure instruction yields a closure value whose code is
// Fn and whose free variables' values are supplied by Bindings.
//
// Type() returns a (possibly named) *types.Signature.
//
// Pos() returns the ast.FuncLit.Type.Func for a function literal
// closure or the ast.SelectorExpr.Sel for a bound method closure.
//
// Example printed form:
//
//	t0 = make closure anon@1.2 [x y z]
//	t1 = make closure bound$(main.I).add [i]
type MakeClosure struct {
	register
	Fn       Value   // always a *Function
	Bindings []Value // values for each free variable in Fn.FreeVars
}

// The MakeMap instruction creates a new hash-table-based map object
// and yields a value of kind map.
//
// Type() returns a (possibly named) *types.Map.
//
// Pos() returns the ast.CallExpr.Lparen, if created by make(map), or
// the ast.CompositeLit.Lbrack if created by a literal.
//
// Example printed form:
//
//	t1 = make map[string]int t0
//	t1 = make StringIntMap t0
type MakeMap struct {
	register
	Reserve Value // initial space reservation; nil => default
}

// The MakeChan instruction creates a new channel object and yields a
// value of kind chan.
//
// Type() returns a (possibly named) *types.Chan.
//
// Pos() returns the ast.CallExpr.Lparen for the make(chan) that
// created it.
//
// Example printed form:
//
//	t0 = make chan int 0
//	t0 = make IntChan 0
type MakeChan struct {
	register
	Size Value // int; size of buffer; zero => synchronous.
}

// The MakeSlice instruction yields a slice of length Len backed by a
// newly allocated array of length Cap.
//
// Both Len and Cap must be non-nil Values of integer type.
//
// (Alloc(types.Array) followed by Slice will not suffice because
// Alloc can only create arrays of constant length.)
//
// Type() returns a (possibly named) *types.Slice.
//
// Pos() returns the ast.CallExpr.Lparen for the make([]T) that
// created it.
//
// Example printed form:
//
//	t1 = make []string 1:int t0
//	t1 = make StringSlice 1:int t0
type MakeSlice struct {
	register
	Len Value
	Cap Value
}

// The Slice instruction yields a slice of an existing string, slice
// or *array X between optional integer bounds Low and High.
//
// Dynamically, this instruction panics if X evaluates to a nil *array
// pointer.
//
// Type() returns string if the type of X was string, otherwise a
// *types.Slice with the same element type as X.
//
// Pos() returns the ast.SliceExpr.Lbrack if created by a x[:] slice
// operation, the ast.CompositeLit.Lbrace if created by a literal, or
// NoPos if not explicit in the source (e.g. a variadic argument slice).
//
// Example printed form:
//
//	t1 = slice t0[1:]
type Slice struct {
	register
	X              Value // slice, string, or *array
	Low, High, Max Value // each may be nil
}

// The FieldAddr instruction yields the address of Field of *struct X.
//
// The field is identified by its index within the field list of the
// struct type of X.
//
// Dynamically, this instruction panics if X evaluates to a nil
// pointer.
//
// Type() returns a (possibly named) *types.Pointer.
//
// Pos() returns the position of the ast.SelectorExpr.Sel for the
// field, if explicit in the source. For implicit selections, returns
// the position of the inducing explicit selection. If produced for a
// struct literal S{f: e}, it returns the position of the colon; for
// S{e} it returns the start of expression e.
//
// Example printed form:
//
//	t1 = &t0.name [#1]
type FieldAddr struct {
	register
	X     Value // *struct
	Field int   // index into CoreType(CoreType(X.Type()).(*types.Pointer).Elem()).(*types.Struct).Fields
}

// The Field instruction yields the Field of struct X.
//
// The field is identified by its index within the field list of the
// struct type of X; by using numeric indices we avoid ambiguity of
// package-local identifiers and permit compact representations.
//
// Pos() returns the position of the ast.SelectorExpr.Sel for the
// field, if explicit in the source. For implicit selections, returns
// the position of the inducing explicit selection.

// Example printed form:
//
//	t1 = t0.name [#1]
type Field struct {
	register
	X     Value // struct
	Field int   // index into CoreType(X.Type()).(*types.Struct).Fields
}

// The IndexAddr instruction yields the address of the element at
// index Index of collection X.  Index is an integer expression.
//
// The elements of maps and strings are not addressable; use Lookup (map),
// Index (string), or MapUpdate instead.
//
// Dynamically, this instruction panics if X evaluates to a nil *array
// pointer.
//
// Type() returns a (possibly named) *types.Pointer.
//
// Pos() returns the ast.IndexExpr.Lbrack for the index operation, if
// explicit in the source.
//
// Example printed form:
//
//	t2 = &t0[t1]
type IndexAddr struct {
	register
	X     Value // *array, slice or type parameter with types array, *array, or slice.
	Index Value // numeric index
}

// The Index instruction yields element Index of collection X, an array,
// string or type parameter containing an array, a string, a pointer to an,
// array or a slice.
//
// Pos() returns the ast.IndexExpr.Lbrack for the index operation, if
// explicit in the source.
//
// Example printed form:
//
//	t2 = t0[t1]
type Index struct {
	register
	X     Value // array, string or type parameter with types array, *array, slice, or string.
	Index Value // integer index
}

// The Lookup instruction yields element Index of collection map X.
// Index is the appropriate key type.
//
// If CommaOk, the result is a 2-tuple of the value above and a
// boolean indicating the result of a map membership test for the key.
// The components of the tuple are accessed using Extract.
//
// Pos() returns the ast.IndexExpr.Lbrack, if explicit in the source.
//
// Example printed form:
//
//	t2 = t0[t1]
//	t5 = t3[t4],ok
type Lookup struct {
	register
	X       Value // map
	Index   Value // key-typed index
	CommaOk bool  // return a value,ok pair
}

// SelectState is a helper for Select.
// It represents one goal state and its corresponding communication.
type SelectState struct {
	Dir       types.ChanDir // direction of case (SendOnly or RecvOnly)
	Chan      Value         // channel to use (for send or receive)
	Send      Value         // value to send (for send)
	Pos       token.Pos     // position of token.ARROW
	DebugNode ast.Node      // ast.SendStmt or ast.UnaryExpr(<-) [debug mode]
}

// The Select instruction tests whether (or blocks until) one
// of the specified sent or received states is entered.
//
// Let n be the number of States for which Dir==RECV and T_i (0<=i<n)
// be the element type of each such state's Chan.
// Select returns an n+2-tuple
//
//	(index int, recvOk bool, r_0 T_0, ... r_n-1 T_n-1)
//
// The tuple's components, described below, must be accessed via the
// Extract instruction.
//
// If Blocking, select waits until exactly one state holds, i.e. a
// channel becomes ready for the designated operation of sending or
// receiving; select chooses one among the ready states
// pseudorandomly, performs the send or receive operation, and sets
// 'index' to the index of the chosen channel.
//
// If !Blocking, select doesn't block if no states hold; instead it
// returns immediately with index equal to -1.
//
// If the chosen channel was used for a receive, the r_i component is
// set to the received value, where i is the index of that state among
// all n receive states; otherwise r_i has the zero value of type T_i.
// Note that the receive index i is not the same as the state
// index index.
//
// The second component of the triple, recvOk, is a boolean whose value
// is true iff the selected operation was a receive and the receive
// successfully yielded a value.
//
// Pos() returns the ast.SelectStmt.Select.
//
// Example printed form:
//
//	t3 = select nonblocking [<-t0, t1<-t2]
//	t4 = select blocking []
type Select struct {
	register
	States   []*SelectState
	Blocking bool
}

// The Range instruction yields an iterator over the domain and range
// of X, which must be a string or map.
//
// Elements are accessed via Next.
//
// Type() returns an opaque and degenerate "rangeIter" type.
//
// Pos() returns the ast.RangeStmt.For.
//
// Example printed form:
//
//	t0 = range "hello":string
type Range struct {
	register
	X Value // string or map
}

// The Next instruction reads and advances the (map or string)
// iterator Iter and returns a 3-tuple value (ok, k, v).  If the
// iterator is not exhausted, ok is true and k and v are the next
// elements of the domain and range, respectively.  Otherwise ok is
// false and k and v are undefined.
//
// Components of the tuple are accessed using Extract.
//
// The IsString field distinguishes iterators over strings from those
// over maps, as the Type() alone is insufficient: consider
// map[int]rune.
//
// Type() returns a *types.Tuple for the triple (ok, k, v).
// The types of k and/or v may be types.Invalid.
//
// Example printed form:
//
//	t1 = next t0
type Next struct {
	register
	Iter     Value
	IsString bool // true => string iterator; false => map iterator.
}

// The TypeAssert instruction tests whether interface value X has type
// AssertedType.
//
// If !CommaOk, on success it returns v, the result of the conversion
// (defined below); on failure it panics.
//
// If CommaOk: on success it returns a pair (v, true) where v is the
// result of the conversion; on failure it returns (z, false) where z
// is AssertedType's zero value.  The components of the pair must be
// accessed using the Extract instruction.
//
// If Underlying: tests whether interface value X has the underlying
// type AssertedType.
//
// If AssertedType is a concrete type, TypeAssert checks whether the
// dynamic type in interface X is equal to it, and if so, the result
// of the conversion is a copy of the value in the interface.
//
// If AssertedType is an interface, TypeAssert checks whether the
// dynamic type of the interface is assignable to it, and if so, the
// result of the conversion is a copy of the interface value X.
// If AssertedType is a superinterface of X.Type(), the operation will
// fail iff the operand is nil.  (Contrast with ChangeInterface, which
// performs no nil-check.)
//
// Type() reflects the actual type of the result, possibly a
// 2-types.Tuple; AssertedType is the asserted type.
//
// Depending on the TypeAssert's purpose, Pos may return:
//   - the ast.CallExpr.Lparen of an explicit T(e) conversion;
//   - the ast.TypeAssertExpr.Lparen of an explicit e.(T) operation;
//   - the ast.CaseClause.Case of a case of a type-switch statement;
//   - the Ident(m).NamePos of an interface method value i.m
//     (for which TypeAssert may be used to effect the nil check).
//
// Example printed form:
//
//	t1 = typeassert t0.(int)
//	t3 = typeassert,ok t2.(T)
type TypeAssert struct {
	register
	X            Value
	AssertedType types.Type
	CommaOk      bool
}

// The Extract instruction yields component Index of Tuple.
//
// This is used to access the results of instructions with multiple
// return values, such as Call, TypeAssert, Next, UnOp(ARROW) and
// IndexExpr(Map).
//
// Example printed form:
//
//	t1 = extract t0 #1
type Extract struct {
	register
	Tuple Value
	Index int
}

// Instructions executed for effect.  They do not yield a value. --------------------

// The Jump instruction transfers control to the sole successor of its
// owning block.
//
// A Jump must be the last instruction of its containing BasicBlock.
//
// Pos() returns NoPos.
//
// Example printed form:
//
//	jump done
type Jump struct {
	anInstruction
}

// The If instruction transfers control to one of the two successors
// of its owning block, depending on the boolean Cond: the first if
// true, the second if false.
//
// An If instruction must be the last instruction of its containing
// BasicBlock.
//
// Pos() returns NoPos.
//
// Example printed form:
//
//	if t0 goto done else body
type If struct {
	anInstruction
	Cond Value
}

// The Return instruction returns values and control back to the calling
// function.
//
// len(Results) is always equal to the number of results in the
// function's signature.
//
// If len(Results) > 1, Return returns a tuple value with the specified
// components which the caller must access using Extract instructions.
//
// There is no instruction to return a ready-made tuple like those
// returned by a "value,ok"-mode TypeAssert, Lookup or UnOp(ARROW) or
// a tail-call to a function with multiple result parameters.
//
// Return must be the last instruction of its containing BasicBlock.
// Such a block has no successors.
//
// Pos() returns the ast.ReturnStmt.Return, if explicit in the source.
//
// Example printed form:
//
//	return
//	return nil:I, 2:int
type Return struct {
	anInstruction
	Results []Value
	pos     token.Pos
}

// The RunDefers instruction pops and invokes the entire stack of
// procedure calls pushed by Defer instructions in this function.
//
// It is legal to encounter multiple 'rundefers' instructions in a
// single control-flow path through a function; this is useful in
// the combined init() function, for example.
//
// Pos() returns NoPos.
//
// Example printed form:
//
//	rundefers
type RunDefers struct {
	anInstruction
}

// The Panic instruction initiates a panic with value X.
//
// A Panic instruction must be the last instruction of its containing
// BasicBlock, which must have no successors.
//
// NB: 'go panic(x)' and 'defer panic(x)' do not use this instruction;
// they are treated as calls to a built-in function.
//
// Pos() returns the ast.CallExpr.Lparen if this panic was explicit
// in the source.
//
// Example printed form:
//
//	panic t0
type Panic struct {
	anInstruction
	X   Value // an interface{}
	pos token.Pos
}

// The Go instruction creates a new goroutine and calls the specified
// function within it.
//
// See CallCommon for generic function call documentation.
//
// Pos() returns the ast.GoStmt.Go.
//
// Example printed form:
//
//	go println(t0, t1)
//	go t3()
//	go invoke t5.Println(...t6)
type Go struct {
	anInstruction
	Call CallCommon
	pos  token.Pos
}

// The Defer instruction pushes the specified call onto a stack of
// functions to be called by a RunDefers instruction or by a panic.
//
// If DeferStack != nil, it indicates the defer list that the defer is
// added to. Defer list values come from the Builtin function
// ssa:deferstack. Calls to ssa:deferstack() produces the defer stack
// of the current function frame. DeferStack allows for deferring into an
// alternative function stack than the current function.
//
// See CallCommon for generic function call documentation.
//
// Pos() returns the ast.DeferStmt.Defer.
//
// Example printed form:
//
//	defer println(t0, t1)
//	defer t3()
//	defer invoke t5.Println(...t6)
type Defer struct {
	anInstruction
	Call       CallCommon
	DeferStack Value // stack of deferred functions (from ssa:deferstack() intrinsic) onto which this function is pushed
	pos        token.Pos
}

// The Send instruction sends X on channel Chan.
//
// Pos() returns the ast.SendStmt.Arrow, if explicit in the source.
//
// Example printed form:
//
//	send t0 <- t1
type Send struct {
	anInstruction
	Chan, X Value
	pos     token.Pos
}

// The Store instruction stores Val at address Addr.
// Stores can be of arbitrary types.
//
// Pos() returns the position of the source-level construct most closely
// associated with the memory store operation.
// Since implicit memory stores are numerous and varied and depend upon
// implementation choices, the details are not specified.
//
// Example printed form:
//
//	*x = y
type Store struct {
	anInstruction
	Addr Value
	Val  Value
	pos  token.Pos
}

// The MapUpdate instruction updates the association of Map[Key] to
// Value.
//
// Pos() returns the ast.KeyValueExpr.Colon or ast.IndexExpr.Lbrack,
// if explicit in the source.
//
// Example printed form:
//
//	t0[t1] = t2
type MapUpdate struct {
	anInstruction
	Map   Value
	Key   Value
	Value Value
	pos   token.Pos
}

// A DebugRef instruction maps a source-level expression Expr to the
// SSA value X that represents the value (!IsAddr) or address (IsAddr)
// of that expression.
//
// DebugRef is a pseudo-instruction: it has no dynamic effect.
//
// Pos() returns Expr.Pos(), the start position of the source-level
// expression.  This is not the same as the "designated" token as
// documented at Value.Pos(). e.g. CallExpr.Pos() does not return the
// position of the ("designated") Lparen token.
//
// If Expr is an *ast.Ident denoting a var or func, Object() returns
// the object; though this information can be obtained from the type
// checker, including it here greatly facilitates debugging.
// For non-Ident expressions, Object() returns nil.
//
// DebugRefs are generated only for functions built with debugging
// enabled; see Package.SetDebugMode() and the GlobalDebug builder
// mode flag.
//
// DebugRefs are not emitted for ast.Idents referring to constants or
// predeclared identifiers, since they are trivial and numerous.
// Nor are they emitted for ast.ParenExprs.
//
// (By representing these as instructions, rather than out-of-band,
// consistency is maintained during transformation passes by the
// ordinary SSA renaming machinery.)
//
// Example printed form:
//
//	; *ast.CallExpr @ 102:9 is t5
//	; var x float64 @ 109:72 is x
//	; address of *ast.CompositeLit @ 216:10 is t0
type DebugRef struct {
	// TODO(generics): Reconsider what DebugRefs are for generics.
	anInstruction
	Expr   ast.Expr     // the referring expression (never *ast.ParenExpr)
	object types.Object // the identity of the source var/func
	IsAddr bool         // Expr is addressable and X is the address it denotes
	X      Value        // the value or address of Expr
}

// Embeddable mix-ins and helpers for common parts of other structs. -----------

// register is a mix-in embedded by all SSA values that are also
// instructions, i.e. virtual registers, and provides a uniform
// implementation of most of the Value interface: Value.Name() is a
// numbered register (e.g. "t0"); the other methods are field accessors.
//
// Temporary names are automatically assigned to each register on
// completion of building a function in SSA form.
//
// Clients must not assume that the 'id' value (and the Name() derived
// from it) is unique within a function.  As always in this API,
// semantics are determined only by identity; names exist only to
// facilitate debugging.
type register struct {
	anInstruction
	num       int        // "name" of virtual register, e.g. "t0".  Not guaranteed unique.
	typ       types.Type // type of virtual register
	pos       token.Pos  // position of source expression, or NoPos
	referrers []Instruction
}

// anInstruction is a mix-in embedded by all Instructions.
// It provides the implementations of the Block and setBlock methods.
type anInstruction struct {
	block *BasicBlock // the basic block of this instruction
}

// CallCommon is contained by Go, Defer and Call to hold the
// common parts of a function or method call.
//
// Each CallCommon exists in one of two modes, function call and
// interface method invocation, or "call" and "invoke" for short.
//
// 1. "call" mode: when Method is nil (!IsInvoke), a CallCommon
// represents an ordinary function call of the value in Value,
// which may be a *Builtin, a *Function or any other value of kind
// 'func'.
//
// Value may be one of:
//
//	(a) a *Function, indicating a statically dispatched call
//	    to a package-level function, an anonymous function, or
//	    a method of a named type.
//	(b) a *MakeClosure, indicating an immediately applied
//	    function literal with free variables.
//	(c) a *Builtin, indicating a statically dispatched call
//	    to a built-in function.
//	(d) any other value, indicating a dynamically dispatched
//	    function call.
//
// StaticCallee returns the identity of the callee in cases
// (a) and (b), nil otherwise.
//
// Args contains the arguments to the call.  If Value is a method,
// Args[0] contains the receiver parameter.
//
// Example printed form:
//
//	t2 = println(t0, t1)
//	go t3()
//	defer t5(...t6)
//
// 2. "invoke" mode: when Method is non-nil (IsInvoke), a CallCommon
// represents a dynamically dispatched call to an interface method.
// In this mode, Value is the interface value and Method is the
// interface's abstract method. The interface value may be a type
// parameter. Note: an interface method may be shared by multiple
// interfaces due to embedding; Value.Type() provides the specific
// interface used for this call.
//
// Value is implicitly supplied to the concrete method implementation
// as the receiver parameter; in other words, Args[0] holds not the
// receiver but the first true argument.
//
// Example printed form:
//
//	t1 = invoke t0.String()
//	go invoke t3.Run(t2)
//	defer invoke t4.Handle(...t5)
//
// For all calls to variadic functions (Signature().Variadic()),
// the last element of Args is a slice.
type CallCommon struct {
	Value  Value       // receiver (invoke mode) or func value (call mode)
	Method *types.Func // interface method (invoke mode)
	Args   []Value     // actual parameters (in static method call, includes receiver)
	pos    token.Pos   // position of CallExpr.Lparen, iff explicit in source
}

// IsInvoke returns true if this call has "invoke" (not "call") mode.
func (c *CallCommon) IsInvoke() bool {
	return c.Method != nil
}

func (c *CallCommon) Pos() token.Pos { return c.pos }

// Signature returns the signature of the called function.
//
// For an "invoke"-mode call, the signature of the interface method is
// returned.
//
// In either "call" or "invoke" mode, if the callee is a method, its
// receiver is represented by sig.Recv, not sig.Params().At(0).
func (c *CallCommon) Signature() *types.Signature {
	if c.Method != nil {
		return c.Method.Type().(*types.Signature)
	}
	return typeparams.CoreType(c.Value.Type()).(*types.Signature)
}

// StaticCallee returns the callee if this is a trivially static
// "call"-mode call to a function.
func (c *CallCommon) StaticCallee() *Function {
	switch fn := c.Value.(type) {
	case *Function:
		return fn
	case *MakeClosure:
		return fn.Fn.(*Function)
	}
	return nil
}

// Description returns a description of the mode of this call suitable
// for a user interface, e.g., "static method call".
func (c *CallCommon) Description() string {
	switch fn := c.Value.(type) {
	case *Builtin:
		return "built-in function call"
	case *MakeClosure:
		return "static function closure call"
	case *Function:
		if fn.Signature.Recv() != nil {
			return "static method call"
		}
		return "static function call"
	}
	if c.IsInvoke() {
		return "dynamic method call" // ("invoke" mode)
	}
	return "dynamic function call"
}

// The CallInstruction interface, implemented by *Go, *Defer and *Call,
// exposes the common parts of function-calling instructions,
// yet provides a way back to the Value defined by *Call alone.
type CallInstruction interface {
	Instruction
	Common() *CallCommon // returns the common parts of the call
	Value() *Call        // returns the result value of the call (*Call) or nil (*Go, *Defer)
}

func (s *Call) Common() *CallCommon  { return &s.Call }
func (s *Defer) Common() *CallCommon { return &s.Call }
func (s *Go) Common() *CallCommon    { return &s.Call }

func (s *Call) Value() *Call  { return s }
func (s *Defer) Value() *Call { return nil }
func (s *Go) Value() *Call    { return nil }

func (v *Builtin) Type() types.Type        { return v.sig }
func (v *Builtin) Name() string            { return v.name }
func (*Builtin) Referrers() *[]Instruction { return nil }
func (v *Builtin) Pos() token.Pos          { return token.NoPos }
func (v *Builtin) Object() types.Object    { return types.Universe.Lookup(v.name) }
func (v *Builtin) Parent() *Function       { return nil }

func (v *FreeVar) Type() types.Type          { return v.typ }
func (v *FreeVar) Name() string              { return v.name }
func (v *FreeVar) Referrers() *[]Instruction { return &v.referrers }
func (v *FreeVar) Pos() token.Pos            { return v.pos }
func (v *FreeVar) Parent() *Function         { return v.parent }

func (v *Global) Type() types.Type                     { return v.typ }
func (v *Global) Name() string                         { return v.name }
func (v *Global) Parent() *Function                    { return nil }
func (v *Global) Pos() token.Pos                       { return v.pos }
func (v *Global) Referrers() *[]Instruction            { return nil }
func (v *Global) Token() token.Token                   { return token.VAR }
func (v *Global) Object() types.Object                 { return v.object }
func (v *Global) String() string                       { return v.RelString(nil) }
func (v *Global) Package() *Package                    { return v.Pkg }
func (v *Global) RelString(from *types.Package) string { return relString(v, from) }

func (v *Function) Name() string       { return v.name }
func (v *Function) Type() types.Type   { return v.Signature }
func (v *Function) Pos() token.Pos     { return v.pos }
func (v *Function) Token() token.Token { return token.FUNC }
func (v *Function) Object() types.Object {
	if v.object != nil {
		return types.Object(v.object)
	}
	return nil
}
func (v *Function) String() string    { return v.RelString(nil) }
func (v *Function) Package() *Package { return v.Pkg }
func (v *Function) Parent() *Function { return v.parent }
func (v *Function) Referrers() *[]Instruction {
	if v.parent != nil {
		return &v.referrers
	}
	return nil
}

// TypeParams are the function's type parameters if generic or the
// type parameters that were instantiated if fn is an instantiation.
func (fn *Function) TypeParams() *types.TypeParamList {
	return fn.typeparams
}

// TypeArgs are the types that TypeParams() were instantiated by to create fn
// from fn.Origin().
func (fn *Function) TypeArgs() []types.Type { return fn.typeargs }

// Origin returns the generic function from which fn was instantiated,
// or nil if fn is not an instantiation.
func (fn *Function) Origin() *Function {
	if fn.parent != nil && len(fn.typeargs) > 0 {
		// Nested functions are BUILT at a different time than their instances.
		// Build declared package if not yet BUILT. This is not an expected use
		// case, but is simple and robust.
		fn.declaredPackage().Build()
	}
	return origin(fn)
}

// origin is the function that fn is an instantiation of. Returns nil if fn is
// not an instantiation.
//
// Precondition: fn and the origin function are done building.
func origin(fn *Function) *Function {
	if fn.parent != nil && len(fn.typeargs) > 0 {
		return origin(fn.parent).AnonFuncs[fn.anonIdx]
	}
	return fn.topLevelOrigin
}

func (v *Parameter) Type() types.Type          { return v.typ }
func (v *Parameter) Name() string              { return v.name }
func (v *Parameter) Object() types.Object      { return v.object }
func (v *Parameter) Referrers() *[]Instruction { return &v.referrers }
func (v *Parameter) Pos() token.Pos            { return v.object.Pos() }
func (v *Parameter) Parent() *Function         { return v.parent }

func (v *Alloc) Type() types.Type          { return v.typ }
func (v *Alloc) Referrers() *[]Instruction { return &v.referrers }
func (v *Alloc) Pos() token.Pos            { return v.pos }

func (v *register) Type() types.Type          { return v.typ }
func (v *register) setType(typ types.Type)    { v.typ = typ }
func (v *register) Name() string              { return fmt.Sprintf("t%d", v.num) }
func (v *register) setNum(num int)            { v.num = num }
func (v *register) Referrers() *[]Instruction { return &v.referrers }
func (v *register) Pos() token.Pos            { return v.pos }
func (v *register) setPos(pos token.Pos)      { v.pos = pos }

func (v *anInstruction) Parent() *Function          { return v.block.parent }
func (v *anInstruction) Block() *BasicBlock         { return v.block }
func (v *anInstruction) setBlock(block *BasicBlock) { v.block = block }
func (v *anInstruction) Referrers() *[]Instruction  { return nil }

func (t *Type) Name() string                         { return t.object.Name() }
func (t *Type) Pos() token.Pos                       { return t.object.Pos() }
func (t *Type) Type() types.Type                     { return t.object.Type() }
func (t *Type) Token() token.Token                   { return token.TYPE }
func (t *Type) Object() types.Object                 { return t.object }
func (t *Type) String() string                       { return t.RelString(nil) }
func (t *Type) Package() *Package                    { return t.pkg }
func (t *Type) RelString(from *types.Package) string { return relString(t, from) }

func (c *NamedConst) Name() string                         { return c.object.Name() }
func (c *NamedConst) Pos() token.Pos                       { return c.object.Pos() }
func (c *NamedConst) String() string                       { return c.RelString(nil) }
func (c *NamedConst) Type() types.Type                     { return c.object.Type() }
func (c *NamedConst) Token() token.Token                   { return token.CONST }
func (c *NamedConst) Object() types.Object                 { return c.object }
func (c *NamedConst) Package() *Package                    { return c.pkg }
func (c *NamedConst) RelString(from *types.Package) string { return relString(c, from) }

func (d *DebugRef) Object() types.Object { return d.object }

// Func returns the package-level function of the specified name,
// or nil if not found.
func (p *Package) Func(name string) (f *Function) {
	f, _ = p.Members[name].(*Function)
	return
}

// Var returns the package-level variable of the specified name,
// or nil if not found.
func (p *Package) Var(name string) (g *Global) {
	g, _ = p.Members[name].(*Global)
	return
}

// Const returns the package-level constant of the specified name,
// or nil if not found.
func (p *Package) Const(name string) (c *NamedConst) {
	c, _ = p.Members[name].(*NamedConst)
	return
}

// Type returns the package-level type of the specified name,
// or nil if not found.
func (p *Package) Type(name string) (t *Type) {
	t, _ = p.Members[name].(*Type)
	return
}

func (v *Call) Pos() token.Pos      { return v.Call.pos }
func (s *Defer) Pos() token.Pos     { return s.pos }
func (s *Go) Pos() token.Pos        { return s.pos }
func (s *MapUpdate) Pos() token.Pos { return s.pos }
func (s *Panic) Pos() token.Pos     { return s.pos }
func (s *Return) Pos() token.Pos    { return s.pos }
func (s *Send) Pos() token.Pos      { return s.pos }
func (s *Store) Pos() token.Pos     { return s.pos }
func (s *If) Pos() token.Pos        { return token.NoPos }
func (s *Jump) Pos() token.Pos      { return token.NoPos }
func (s *RunDefers) Pos() token.Pos { return token.NoPos }
func (s *DebugRef) Pos() token.Pos  { return s.Expr.Pos() }

// Operands.

func (v *Alloc) Operands(rands []*Value) []*Value {
	return rands
}

func (v *BinOp) Operands(rands []*Value) []*Value {
	return append(rands, &v.X, &v.Y)
}

func (c *CallCommon) Operands(rands []*Value) []*Value {
	rands = append(rands, &c.Value)
	for i := range c.Args {
		rands = append(rands, &c.Args[i])
	}
	return rands
}

func (s *Go) Operands(rands []*Value) []*Value {
	return s.Call.Operands(rands)
}

func (s *Call) Operands(rands []*Value) []*Value {
	return s.Call.Operands(rands)
}

func (s *Defer) Operands(rands []*Value) []*Value {
	return append(s.Call.Operands(rands), &s.DeferStack)
}

func (v *ChangeInterface) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (v *ChangeType) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (v *Convert) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (v *MultiConvert) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (v *SliceToArrayPointer) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (s *DebugRef) Operands(rands []*Value) []*Value {
	return append(rands, &s.X)
}

func (v *Extract) Operands(rands []*Value) []*Value {
	return append(rands, &v.Tuple)
}

func (v *Field) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (v *FieldAddr) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (s *If) Operands(rands []*Value) []*Value {
	return append(rands, &s.Cond)
}

func (v *Index) Operands(rands []*Value) []*Value {
	return append(rands, &v.X, &v.Index)
}

func (v *IndexAddr) Operands(rands []*Value) []*Value {
	return append(rands, &v.X, &v.Index)
}

func (*Jump) Operands(rands []*Value) []*Value {
	return rands
}

func (v *Lookup) Operands(rands []*Value) []*Value {
	return append(rands, &v.X, &v.Index)
}

func (v *MakeChan) Operands(rands []*Value) []*Value {
	return append(rands, &v.Size)
}

func (v *MakeClosure) Operands(rands []*Value) []*Value {
	rands = append(rands, &v.Fn)
	for i := range v.Bindings {
		rands = append(rands, &v.Bindings[i])
	}
	return rands
}

func (v *MakeInterface) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (v *MakeMap) Operands(rands []*Value) []*Value {
	return append(rands, &v.Reserve)
}

func (v *MakeSlice) Operands(rands []*Value) []*Value {
	return append(rands, &v.Len, &v.Cap)
}

func (v *MapUpdate) Operands(rands []*Value) []*Value {
	return append(rands, &v.Map, &v.Key, &v.Value)
}

func (v *Next) Operands(rands []*Value) []*Value {
	return append(rands, &v.Iter)
}

func (s *Panic) Operands(rands []*Value) []*Value {
	return append(rands, &s.X)
}

func (v *Phi) Operands(rands []*Value) []*Value {
	for i := range v.Edges {
		rands = append(rands, &v.Edges[i])
	}
	return rands
}

func (v *Range) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (s *Return) Operands(rands []*Value) []*Value {
	for i := range s.Results {
		rands = append(rands, &s.Results[i])
	}
	return rands
}

func (*RunDefers) Operands(rands []*Value) []*Value {
	return rands
}

func (v *Select) Operands(rands []*Value) []*Value {
	for i := range v.States {
		rands = append(rands, &v.States[i].Chan, &v.States[i].Send)
	}
	return rands
}

func (s *Send) Operands(rands []*Value) []*Value {
	return append(rands, &s.Chan, &s.X)
}

func (v *Slice) Operands(rands []*Value) []*Value {
	return append(rands, &v.X, &v.Low, &v.High, &v.Max)
}

func (s *Store) Operands(rands []*Value) []*Value {
	return append(rands, &s.Addr, &s.Val)
}

func (v *TypeAssert) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

func (v *UnOp) Operands(rands []*Value) []*Value {
	return append(rands, &v.X)
}

// Non-Instruction Values:
func (v *Builtin) Operands(rands []*Value) []*Value   { return rands }
func (v *FreeVar) Operands(rands []*Value) []*Value   { return rands }
func (v *Const) Operands(rands []*Value) []*Value     { return rands }
func (v *Function) Operands(rands []*Value) []*Value  { return rands }
func (v *Global) Operands(rands []*Value) []*Value    { return rands }
func (v *Parameter) Operands(rands []*Value) []*Value { return rands }
