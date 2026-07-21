// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package parser

import "fmt"

// Node is the root interface for all AST nodes. Pos returns the source line/col
// where the node starts; Filename returns the source file (or "" if unknown).
type Node interface {
	Pos() (line, col int)
	Filename() string
	astNode()
}

type Stmt interface {
	Node
	stmtNode()
}

type Expr interface {
	Node
	exprNode()
}

// pos carries source position; embedded into every node.
type pos struct {
	File string
	Line int
	Col  int
}

func (p pos) Pos() (int, int)  { return p.Line, p.Col }
func (p pos) Filename() string { return p.File }
func (p pos) astNode()         {}

// TypeKind tags the static kind of a declared type. Primitive kinds
// (TypeInt..TypeNull) don't need any payload; compound kinds (TypeList,
// TypeMap) carry their element / key+value type via the pointers on the
// surrounding Type struct.
type TypeKind int

const (
	TypeInvalid TypeKind = iota
	TypeInt
	TypeFloat
	TypeString
	TypeBool
	TypeNull
	TypeBytes // mutable byte sequence; elements are int in [0, 255]
	TypeList
	TypeMap
	TypeStruct // user-defined record; carries the struct's name in Type.StructName
	TypeTask   // `task of T` - a pending or completed spawned computation
)

func (k TypeKind) String() string {
	switch k {
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeString:
		return "string"
	case TypeBool:
		return "bool"
	case TypeNull:
		return "null"
	case TypeBytes:
		return "bytes"
	case TypeList:
		return "list"
	case TypeMap:
		return "map"
	case TypeStruct:
		return "struct"
	case TypeTask:
		return "task"
	default:
		return "<invalid>"
	}
}

// Type is the declared static type of a variable, constant, or method
// parameter. For primitives only `Kind` matters; for compound kinds the
// pointer fields are non-nil and themselves point at full Type values,
// which is how `list of list of int` and `map of string to list of int`
// fall out without special casing.
type Type struct {
	Kind       TypeKind
	Element    *Type  // TypeList: element type
	KeyType    *Type  // TypeMap:  key type
	ValType    *Type  // TypeMap:  value type
	StructName string // TypeStruct: name of the struct definition
	StructNS   string // TypeStruct: display namespace prefix (library name or a
	//                  module's file stem). Empty for user-defined structs
	//                  (`def struct Name`); set for library-registered structs
	//                  (`os.Result`) and stamped module structs.
	ModPath string // TypeStruct: module identity (the module's canonical path)
	//                stamped onto a module-struct type by resolveDeclaredStructNS;
	//                empty for library / user structs. Keeps two same-stem module
	//                types distinct while StructNS stays the readable stem.
	// Resolved is an interpreter annotation, not parser output: set once
	// resolveDeclaredStructNS has stamped a declared type's namespace
	// (importer alias -> module stem, or library alias -> canonical). It makes
	// re-resolution an idempotent no-op, so a shared type node reached from
	// concurrent spawn / method-call goroutines is only read, never re-stamped
	// (the write-write race that would otherwise be), and distinguishes a
	// stamped node from a user-written canonical-when-aliased name. Equal()
	// ignores it.
	Resolved bool
}

func (t Type) String() string {
	switch t.Kind {
	case TypeList:
		if t.Element == nil {
			return "list of <?>"
		}
		return "list of " + t.Element.String()
	case TypeMap:
		if t.KeyType == nil || t.ValType == nil {
			return "map of <?> to <?>"
		}
		return "map of " + t.KeyType.String() + " to " + t.ValType.String()
	case TypeStruct:
		if t.StructName == "" {
			return "struct"
		}
		if t.StructNS != "" {
			return t.StructNS + "." + t.StructName
		}
		return t.StructName
	case TypeTask:
		if t.Element == nil {
			return "task of <?>"
		}
		return "task of " + t.Element.String()
	}
	return t.Kind.String()
}

// Equal reports whether two types are structurally identical (same kind
// and, for compound kinds, recursively equal element / key / value types).
// Used by the type-mismatch checks at assignment and call sites.
func (t Type) Equal(o Type) bool {
	if t.Kind != o.Kind {
		return false
	}
	switch t.Kind {
	case TypeList:
		if (t.Element == nil) != (o.Element == nil) {
			return false
		}
		if t.Element == nil {
			return true
		}
		return t.Element.Equal(*o.Element)
	case TypeMap:
		if (t.KeyType == nil) != (o.KeyType == nil) || (t.ValType == nil) != (o.ValType == nil) {
			return false
		}
		if t.KeyType == nil {
			return true
		}
		return t.KeyType.Equal(*o.KeyType) && t.ValType.Equal(*o.ValType)
	case TypeStruct:
		return t.StructName == o.StructName && t.StructNS == o.StructNS && t.ModPath == o.ModPath
	case TypeTask:
		if (t.Element == nil) != (o.Element == nil) {
			return false
		}
		if t.Element == nil {
			return true
		}
		return t.Element.Equal(*o.Element)
	}
	return true
}

// Primitive type-value constructors. Compound types are built from these
// at parse time: `Type{Kind: TypeList, Element: &elem}`,
// `Type{Kind: TypeMap, KeyType: &k, ValType: &v}`.
func PrimitiveType(k TypeKind) Type { return Type{Kind: k} }
func ListType(elem Type) Type       { return Type{Kind: TypeList, Element: &elem} }
func MapType(k, v Type) Type        { return Type{Kind: TypeMap, KeyType: &k, ValType: &v} }
func StructType(name string) Type   { return Type{Kind: TypeStruct, StructName: name} }
func TaskType(elem Type) Type       { return Type{Kind: TypeTask, Element: &elem} }

// NamespacedStructType is a struct type registered by a
// library and reachable behind that library's namespace prefix
// (`os.Result`). `ns` is the library prefix at the use site; the
// interpreter resolves aliases (`use os as o;` -> `o.Result`) when
// looking the type up.
func NamespacedStructType(ns, name string) Type {
	return Type{Kind: TypeStruct, StructNS: ns, StructName: name}
}

// ---- Top-level program ----

type Program struct {
	pos
	Imports       []*ImportStmt       // `use LIB;` library imports
	ModuleImports []*ModuleImportStmt // `import "path.j" [as NAME];` module imports
	Methods       []*MethodDef
	Structs       []*StructDef // top-level `def struct Name { ... };` declarations
	TopLevel      []Stmt       // top-level statements executed in source order after method hoisting
	// Number of slots the global frame needs. Populated by
	// Resolve() during parse. Zero when Resolve() hasn't run - the
	// interpreter falls back to name-based lookup in that case, which
	// keeps unit tests that hand-build AST fragments working without
	// running the resolver.
	NumGlobals int
}

// ---- Statements ----

// ImportStmt is a `use NAME;` or `use NAME as ALIAS;` library import. AsName
// is empty for the plain form. When AsName is set, only `ALIAS.` resolves
// qualified calls into the library's namespace; the canonical NAME is
// shadowed at the use site (matches Python's `import foo as bar`).
type ImportStmt struct {
	pos
	Name   string
	AsName string // empty when there's no `as ALIAS` clause
}

func (*ImportStmt) stmtNode() {}

// ModuleImportStmt is a `import "path.j" [as NAME];` module import - a real
// module boundary (unlike `include`'s textual splice). Path is the logical,
// `/`-separated import string; AsName is the namespace prefix, empty for the
// bare form (the prefix is then the file stem, resolved by the interpreter).
type ModuleImportStmt struct {
	pos
	Path   string
	AsName string // empty when there's no `as NAME` clause
}

func (*ModuleImportStmt) stmtNode() {}

// Param is one formal parameter of a method.
type Param struct {
	Name string
	Type Type
	File string
	Line int
	Col  int
}

// StructField is one declared field of a struct definition.
// Mirrors Param but is owned by StructDef rather than MethodDef.
type StructField struct {
	Name string
	Type Type
	File string
	Line int
	Col  int
}

// StructDef is a top-level `def struct Name { field as type, ... };`
// declaration. Hoisted at Run() time alongside method
// definitions so order of declaration doesn't matter.
type StructDef struct {
	pos
	Name     string
	Fields   []StructField
	Exported bool // `export def struct ...` - published from the enclosing module
}

func (*StructDef) stmtNode() {}

type MethodDef struct {
	pos
	Name     string
	Params   []Param
	Body     *Block
	Exported bool // `export func ...` - published from the enclosing module
}

func (*MethodDef) stmtNode() {}

type Block struct {
	pos
	Stmts []Stmt
	// Number of slots this block's fresh frame needs.
	// Populated by Resolve() during parse. Zero when Resolve() hasn't
	// run - the interpreter's fallback path grows the slot slice on
	// demand in that case.
	NumSlots int
}

func (*Block) stmtNode() {}

// DefineStmt: `define $x as int [init <expr>];` or `define const NAME as int init <expr>;`
// InitExpr is nil for uninitialized variables (interpreter uses ZeroFor in that case).
// Constants always have InitExpr != nil (the parser enforces this).
type DefineStmt struct {
	pos
	IsConst  bool
	Exported bool   // `export def const ...` - published from the enclosing module
	VarName  string // for variables: the $-name; for constants: the UPPERCASE name
	VarType  Type
	InitExpr Expr // may be nil for uninitialized variables; never nil for constants
	// Slot the binding will occupy in its enclosing frame.
	// Populated by Resolve(); default -1 means "not resolved".
	Slot int
}

func (*DefineStmt) stmtNode() {}

// AssignStmt: `$x = <expr>;`
type AssignStmt struct {
	pos
	VarName string
	Value   Expr
	// (Depth, Slot) address the binding to update. Default -1.
	Depth int
	Slot  int
}

func (*AssignStmt) stmtNode() {}

// IndexAssignStmt: `$xs[i] = <expr>;`, `$m["k"] = <expr>;`, or a chain
// `$xs[i][j] = <expr>;`. Target is always an IndexExpr (an l-value
// chain rooted on a VarExpr). Distinguished from AssignStmt because the
// interpreter has to walk the index chain to find the slot to write,
// and because const-target enforcement runs only against the *root*
// binding (the VarExpr at the bottom of the chain).
type IndexAssignStmt struct {
	pos
	Target *IndexExpr
	Value  Expr
}

func (*IndexAssignStmt) stmtNode() {}

// AppendStmt: `$xs[] = <expr>;` - the syntax-level append. The empty
// brackets are a write-only target meaning "the position just past the
// end of the list"; reading `$xs[]` is a parse error. Only valid on a
// plain VARREF root (no chained `$xs[0][]` form); the targeted
// variable must be a list at runtime, value's type is checked against
// the list's declared element type, and const targets are rejected as
// usual.
type AppendStmt struct {
	pos
	Target *VarExpr
	Value  Expr
}

func (*AppendStmt) stmtNode() {}

// FieldAssignStmt: `$p.field = <expr>;`. Same shape as
// IndexAssignStmt: the target chain is rooted on a VarExpr so the
// const-target rejection and write-back-to-binding bookkeeping live
// here. Only valid on a struct value; the assigned value's type is
// checked against the struct definition's declared field type.
type FieldAssignStmt struct {
	pos
	Target *FieldAccessExpr
	Value  Expr
}

func (*FieldAssignStmt) stmtNode() {}

// IfStmt: `if (cond) { body } [elseif (cond) { body }]* [else { body }]?`
// ElseIfs is parallel to ElseIfBodies. Else may be nil.
type IfStmt struct {
	pos
	Cond         Expr
	Then         *Block
	ElseIfs      []Expr
	ElseIfBodies []*Block
	Else         *Block // nil if absent
}

func (*IfStmt) stmtNode() {}

// WhileStmt: `while (cond) { body }`
type WhileStmt struct {
	pos
	Cond Expr
	Body *Block
}

func (*WhileStmt) stmtNode() {}

// ForStmt: `for (init; cond; step) { body }`
// Init may be a DefineStmt or an AssignStmt; Step is an AssignStmt.
// Any field may be nil (e.g. `for (; cond ;) { ... }` would mean empty init/step).
type ForStmt struct {
	pos
	Init Stmt // *DefineStmt or *AssignStmt, or nil
	Cond Expr // must produce bool; nil means "true forever"
	Step Stmt // *AssignStmt or nil
	Body *Block
	// HeaderSlots is the number of bindings the resolver placed in the loop's
	// own header frame (the Init `def`). The interpreter pre-sizes that frame
	// so Init doesn't grow it, and the body frame stays sized to just its own
	// slots rather than header+body.
	HeaderSlots int
}

func (*ForStmt) stmtNode() {}

// ForEachStmt: `for (def NAME in EXPR) { body }`. The iteration variable
// is bound in the body's scope. For lists the value of NAME is each
// element in order; for maps it's each key in insertion order. EXPR is
// evaluated once at loop entry. Distinct from ForStmt because the two
// constructs share nothing semantically beyond the keyword - it's
// cleaner to have two AST nodes than one big union.
type ForEachStmt struct {
	pos
	VarName string
	Coll    Expr
	Body    *Block
	// Slot the iterator variable takes in the per-iteration
	// frame. Default -1 = not resolved. The iterator lives in a fresh
	// frame chained on top of the enclosing scope; the body's own
	// defs get slots after IterSlot in the same frame (via Body.NumSlots).
	IterSlot int
}

func (*ForEachStmt) stmtNode() {}

// ReturnStmt: `return;` (returns null) or `return EXPR;`.
type ReturnStmt struct {
	pos
	Value Expr // nil for bare `return;`
}

func (*ReturnStmt) stmtNode() {}

// BreakStmt: `break;` - exit the innermost enclosing loop. Errors at
// runtime if no loop is active.
type BreakStmt struct {
	pos
}

func (*BreakStmt) stmtNode() {}

// ContinueStmt: `continue;` - skip to the next iteration of the
// innermost enclosing loop. Same rule as BreakStmt; errors outside a
// loop.
type ContinueStmt struct {
	pos
}

func (*ContinueStmt) stmtNode() {}

// RepeatStmt: `repeat { ... } until (cond);` - post-test loop that
// runs the body at least once and stops when `cond` evaluates true.
// New keywords `repeat` and `until` were chosen over the
// `do { } while ...` shape so the inversion ("loop until done")
// reads as English and matches Jennifer's word-operator style.
type RepeatStmt struct {
	pos
	Body *Block
	Cond Expr
}

func (*RepeatStmt) stmtNode() {}

// ExitStmt: `exit;` (exit code 0) or `exit EXPR;` (EXPR must evaluate
// to int). Terminates the program immediately, skipping the rest of
// the current method's body, any caller frames, and any remaining
// top-level statements. Distinct from `return` (which is
// method-scoped).
type ExitStmt struct {
	pos
	Code Expr // nil for `exit;` -> 0
}

func (*ExitStmt) stmtNode() {}

// TryStmt: `try { body } catch (NAME) { handler }` - catchable error
// block. The body runs first; if any `throw` (user-issued or runtime)
// reaches this `try`, the handler runs with `NAME` bound to the
// thrown value in a fresh per-handler scope.
// TryStmt: `try { body } catch (name) { handler };`. CatchName
// is the identifier the handler binds. CatchSlot is the slot the caught
// value takes in the handler's fresh frame. Default -1 = not
// resolved.
type TryStmt struct {
	pos
	Body      *Block
	CatchName string
	CatchBody *Block
	CatchFile string // position of the `catch (NAME)` introducer for diagnostics
	CatchLine int
	CatchCol  int
	CatchSlot int // slot for CatchName in the handler frame; -1 = unresolved
}

func (*TryStmt) stmtNode() {}

// ThrowStmt: `throw EXPR;` - raises a catchable error. EXPR may
// produce any value (the convention is an `Error` struct - see
// docs/milestones.md).
type ThrowStmt struct {
	pos
	Value Expr
}

func (*ThrowStmt) stmtNode() {}

// DeferStmt: `defer CALL(args);` schedules a single call to run when the
// enclosing block exits, on every exit path (fall-through, return, break,
// continue, throw, exit), last-registered-first. Call is a *CallExpr or
// *QualifiedCallExpr (the parser rejects anything else); its arguments are
// evaluated at the defer site, the call runs at block exit.
//
// OnError marks the `errdefer` variant: the call runs only when the block is
// exiting because an error is propagating (a `throw` or a runtime error) -
// it is skipped on fall-through, `return`, `break`, `continue`, and `exit`.
// The two variants share this node so every statement walker (resolver, lint,
// AST dump) treats them identically; only the interpreter's frame teardown
// consults the flag.
type DeferStmt struct {
	pos
	Call    Expr
	OnError bool
}

func (*DeferStmt) stmtNode() {}

// PreEval wraps an already-computed runtime value so it can stand in an
// expression position. It exists only for the interpreter's `defer` machinery:
// arguments are evaluated at the defer site, then the captured values are spliced
// back into the call's argument list and fed through the normal call path. The
// payload is stored as `any` so the parser package stays free of an interpreter
// import; the interpreter's evalExpr type-asserts it back to a Value.
type PreEval struct {
	pos
	Value any
}

func (*PreEval) exprNode() {}

// NewPreEval builds a PreEval carrying value v at the given source position.
// Exported so the interpreter can construct one (the pos field is unexported).
func NewPreEval(v any, file string, line, col int) *PreEval {
	return &PreEval{pos: pos{File: file, Line: line, Col: col}, Value: v}
}

// ExprStmt: a bare expression terminated by `;` (used for calls like `printf(...)`).
type ExprStmt struct {
	pos
	Expr Expr
}

func (*ExprStmt) stmtNode() {}

// ---- Expressions ----

type IntLit struct {
	pos
	Value int64
}

func (*IntLit) exprNode() {}

type FloatLit struct {
	pos
	Value float64
}

func (*FloatLit) exprNode() {}

type StringLit struct {
	pos
	Value string
}

func (*StringLit) exprNode() {}

type BoolLit struct {
	pos
	Value bool
}

func (*BoolLit) exprNode() {}

type NullLit struct {
	pos
}

func (*NullLit) exprNode() {}

// ListLit: a `[expr, expr, ...]` literal. Element types are checked
// against the declared variable type at assignment time; the parser
// doesn't constrain element types itself.
type ListLit struct {
	pos
	Elements []Expr
}

func (*ListLit) exprNode() {}

// MapLit: a `{key: value, key: value, ...}` literal. Keys and values are
// arbitrary expressions; the interpreter enforces that keys are of the
// declared key type and hashable. Insertion order is preserved.
type MapLit struct {
	pos
	Keys, Values []Expr // parallel slices
}

func (*MapLit) exprNode() {}

// IndexExpr: `$xs[i]` for lists or `$m[k]` for maps. The same node
// covers both - the interpreter dispatches at evaluation time based on
// the kind of Target.
//
// Used as both an r-value (read) in expressions and as an l-value
// (write target) in AssignStmt-with-index. We attach the operation site
// for positioned error messages on out-of-bounds reads / missing-key
// reads / write-type-mismatch.
type IndexExpr struct {
	pos
	Target Expr
	Index  Expr
}

func (*IndexExpr) exprNode() {}

// StructLit is `Name{ field: expr, ... }` or
// `lib.Name{ field: expr, ... }`. The struct's name must
// reference a top-level `def struct` declaration (bare form) or a
// library-registered namespaced struct (qualified form). Field
// expressions are evaluated in source order; the interpreter
// type-checks each value against the struct definition's declared
// type.
type StructLit struct {
	pos
	NS     string // optional library prefix at the use site; empty for user-defined structs
	Name   string
	Fields []StructLitField
}

// StructLitField is one `field: expr` entry in a struct literal.
type StructLitField struct {
	Name string
	Expr Expr
	File string
	Line int
	Col  int
}

func (*StructLit) exprNode() {}

// FieldAccessExpr is `$p.field`. The parser produces this
// when it sees `.` after a value expression (after the existing
// qualified-call special case rules out namespace-prefix usage).
// Used as both r-value (read) and l-value (assignment target).
type FieldAccessExpr struct {
	pos
	Target Expr
	Field  string
}

func (*FieldAccessExpr) exprNode() {}

type VarExpr struct {
	pos
	Name string // without the leading $
	// (Depth, Slot) locate the binding in the runtime scope
	// chain. Depth is how many parent pointers to walk from the
	// current environment; Slot is the slot index in that frame.
	// Both default to -1, meaning "not resolved" - the interpreter
	// falls back to name-based lookup so ad-hoc AST fragments in
	// tests keep working. Populated by Resolve() during parse.
	Depth int
	Slot  int
}

func (*VarExpr) exprNode() {}

// ConstRefExpr is a bare-identifier reference in an expression context
// (e.g. `printf(MAX)`). The interpreter resolves it to a constant in scope.
// A bare-identifier reference to a variable is rejected at runtime with a hint
// to use the `$` sigil.
type ConstRefExpr struct {
	pos
	Name string
	// Same shape as VarExpr - see the comment there.
	Depth int
	Slot  int
}

func (*ConstRefExpr) exprNode() {}

type CallExpr struct {
	pos
	Callee string
	Args   []Expr
	// Pre-resolved pointer to the top-level user method
	// this call targets. Filled in by Resolve() when Callee matches
	// a hoisted method name. nil for builtin calls (dispatched
	// through the Builtins / NSBuiltins registries) and for
	// resolver-less paths (REPL, tests that bypass Resolve). The
	// interpreter's evalCall consults this before the methods-map
	// lookup, so the hot recursion path (fib, walk) skips a hash
	// lookup per call.
	Method *MethodDef
}

func (*CallExpr) exprNode() {}

// LenExpr is the `len(EXPR)` language built-in. Polymorphic
// over string (rune count), list (element count), map (entry count),
// and bytes (byte count); any other kind is a positioned runtime
// error. `len` is a reserved keyword in expression position, not a
// library function - removing the `core` library dropped the
// last name registered via the global-builtin path.
type LenExpr struct {
	pos
	Operand Expr
}

func (*LenExpr) exprNode() {}

// SpawnExpr is the `spawn { ... }` block primary expression. The
// body is a statement list run in a separate goroutine (Phase 2); the
// expression's value is a `task of T` where `T` is the body's return
// type. Variables referenced from the enclosing scope are deep-copied
// into the spawned frame at spawn time, matching the value-semantics
// capture of a method call. `return EXPR;` inside the body produces
// the task's value; bare `return;` produces null.
type SpawnExpr struct {
	pos
	Body []Stmt
}

func (*SpawnExpr) exprNode() {}

// QualifiedCallExpr is a namespaced call: `IDENT . IDENT ( args )`. The
// Prefix is the use-site identifier (the library's namespace or its
// alias if `use lib as alias;` was set). The interpreter looks up
// (Prefix, Callee) against the namespaced-builtin registry.
type QualifiedCallExpr struct {
	pos
	Prefix string
	Callee string
	Args   []Expr
	// Pre-resolved namespaced-builtin descriptor stamped by
	// Interpreter.resolveQualifiedRefs after processImports runs.
	// Kept `any` to avoid a parser -> interpreter import cycle; the
	// interpreter treats it as an *builtinEntry-shaped value and
	// falls back to the resolveNamespacePrefix + NSBuiltins map
	// lookup when this is nil (REPL turns, hand-built ASTs,
	// resolver-less paths).
	Fn any
}

func (*QualifiedCallExpr) exprNode() {}

// QualifiedConstRefExpr is a namespaced constant reference: `IDENT . IDENT`
// without a following `(`. Resolution mirrors QualifiedCallExpr.
type QualifiedConstRefExpr struct {
	pos
	Prefix string
	Name   string
	// Pre-resolved namespaced-constant Value pointer stamped
	// by Interpreter.resolveQualifiedRefs. Kept `any` for the same
	// import-cycle reason as QualifiedCallExpr.Fn; the interpreter
	// dereferences it as a *Value.
	Const any
}

func (*QualifiedConstRefExpr) exprNode() {}

type BinaryOp int

const (
	OpAdd BinaryOp = iota
	OpSub
	OpMul
	OpDiv      // `/` - always returns float
	OpFloorDiv // `div` - floor division (int//int -> int; involves float -> float floor)
	OpMod
	OpLt
	OpGt
	OpLe
	OpGe
	OpEq
	OpNeq // != - the negation of OpEq (same operand rules)
	OpAnd
	OpOr

	// Bitwise - int only, never short-circuit.
	OpBitOr  // |
	OpBitXor // ^
	OpBitAnd // &
	OpShl    // <<
	OpShr    // >> (arithmetic / sign-extending on signed int)
)

func (o BinaryOp) String() string {
	switch o {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpFloorDiv:
		return "//"
	case OpMod:
		return "%"
	case OpLt:
		return "<"
	case OpGt:
		return ">"
	case OpLe:
		return "<="
	case OpGe:
		return ">="
	case OpEq:
		return "=="
	case OpNeq:
		return "!="
	case OpAnd:
		return "and"
	case OpOr:
		return "or"
	case OpBitOr:
		return "|"
	case OpBitXor:
		return "^"
	case OpBitAnd:
		return "&"
	case OpShl:
		return "<<"
	case OpShr:
		return ">>"
	}
	return "?"
}

// IsComparison reports whether the op is a comparison (result type is bool).
func (o BinaryOp) IsComparison() bool {
	switch o {
	case OpLt, OpGt, OpLe, OpGe, OpEq, OpNeq:
		return true
	}
	return false
}

// IsLogical reports whether the op is a short-circuit logical operator.
func (o BinaryOp) IsLogical() bool {
	return o == OpAnd || o == OpOr
}

// UnaryOp identifies a prefix-unary operator.
type UnaryOp int

const (
	OpNeg    UnaryOp = iota // -x  (numeric)
	OpNot                   // not x  (bool)
	OpBitNot                // ~x  (bitwise NOT on int)
)

func (o UnaryOp) String() string {
	switch o {
	case OpNeg:
		return "-"
	case OpNot:
		return "not"
	case OpBitNot:
		return "~"
	}
	return "?"
}

// UnaryExpr is a prefix-unary expression: `-EXPR` or `not EXPR`.
type UnaryExpr struct {
	pos
	Op      UnaryOp
	Operand Expr
	// Pre-computed constant-fold result stamped by Resolve
	// when Operand collapses to a compile-time literal. The
	// interpreter's evalUnary returns Folded's value directly when
	// present, skipping the operand walk + op switch. nil for
	// runtime-only expressions and resolver-less paths.
	Folded Expr
}

func (*UnaryExpr) exprNode() {}

type BinaryExpr struct {
	pos
	Op    BinaryOp
	Left  Expr
	Right Expr
	// Pre-computed constant-fold result. Same convention
	// as UnaryExpr.Folded - the resolver stamps this when both
	// Left and Right collapse to literals (transitively through
	// their own Folded fields, so `(1+2)*3` folds to 9). Ops that
	// would error at fold time (division by zero, shift-count
	// negative) are left unfolded so the runtime hits them with
	// proper source-position information.
	Folded Expr
}

func (*BinaryExpr) exprNode() {}

// Sprint produces a stable, readable representation of any AST node - used in tests.
func Sprint(n Node) string {
	switch v := n.(type) {
	case *Program:
		s := "Program{"
		for _, im := range v.Imports {
			s += Sprint(im) + " "
		}
		for _, m := range v.Methods {
			s += Sprint(m) + " "
		}
		for _, st := range v.TopLevel {
			s += Sprint(st) + " "
		}
		return s + "}"
	case *ImportStmt:
		if v.AsName != "" {
			return fmt.Sprintf("Import(%s as %s)", v.Name, v.AsName)
		}
		return fmt.Sprintf("Import(%s)", v.Name)
	case *MethodDef:
		params := ""
		for i, p := range v.Params {
			if i > 0 {
				params += ", "
			}
			params += fmt.Sprintf("%s as %s", p.Name, p.Type)
		}
		return fmt.Sprintf("Method(%s(%s), %s)", v.Name, params, Sprint(v.Body))
	case *Block:
		s := "Block["
		for i, st := range v.Stmts {
			if i > 0 {
				s += "; "
			}
			s += Sprint(st)
		}
		return s + "]"
	case *DefineStmt:
		kind := "Define"
		if v.IsConst {
			kind = "Const"
		}
		name := "$" + v.VarName
		if v.IsConst {
			name = v.VarName
		}
		if v.InitExpr == nil {
			return fmt.Sprintf("%s(%s as %s)", kind, name, v.VarType)
		}
		return fmt.Sprintf("%s(%s as %s = %s)", kind, name, v.VarType, Sprint(v.InitExpr))
	case *AssignStmt:
		return fmt.Sprintf("Assign($%s = %s)", v.VarName, Sprint(v.Value))
	case *IfStmt:
		s := fmt.Sprintf("If(%s, %s", Sprint(v.Cond), Sprint(v.Then))
		for i, c := range v.ElseIfs {
			s += fmt.Sprintf(", ElseIf(%s, %s)", Sprint(c), Sprint(v.ElseIfBodies[i]))
		}
		if v.Else != nil {
			s += fmt.Sprintf(", Else(%s)", Sprint(v.Else))
		}
		return s + ")"
	case *WhileStmt:
		return fmt.Sprintf("While(%s, %s)", Sprint(v.Cond), Sprint(v.Body))
	case *ForStmt:
		initS := "<nil>"
		if v.Init != nil {
			initS = Sprint(v.Init)
		}
		condS := "<nil>"
		if v.Cond != nil {
			condS = Sprint(v.Cond)
		}
		stepS := "<nil>"
		if v.Step != nil {
			stepS = Sprint(v.Step)
		}
		return fmt.Sprintf("For(%s; %s; %s, %s)", initS, condS, stepS, Sprint(v.Body))
	case *ReturnStmt:
		if v.Value == nil {
			return "Return"
		}
		return fmt.Sprintf("Return(%s)", Sprint(v.Value))
	case *BreakStmt:
		return "Break"
	case *ContinueStmt:
		return "Continue"
	case *RepeatStmt:
		return fmt.Sprintf("Repeat(%s, until %s)", Sprint(v.Body), Sprint(v.Cond))
	case *ExitStmt:
		if v.Code == nil {
			return "Exit"
		}
		return fmt.Sprintf("Exit(%s)", Sprint(v.Code))
	case *TryStmt:
		return fmt.Sprintf("Try(%s, catch %s %s)", Sprint(v.Body), v.CatchName, Sprint(v.CatchBody))
	case *ThrowStmt:
		return fmt.Sprintf("Throw(%s)", Sprint(v.Value))
	case *DeferStmt:
		if v.OnError {
			return fmt.Sprintf("Errdefer(%s)", Sprint(v.Call))
		}
		return fmt.Sprintf("Defer(%s)", Sprint(v.Call))
	case *ExprStmt:
		return fmt.Sprintf("ExprStmt(%s)", Sprint(v.Expr))
	case *IntLit:
		return fmt.Sprintf("Int(%d)", v.Value)
	case *FloatLit:
		return fmt.Sprintf("Float(%g)", v.Value)
	case *StringLit:
		return fmt.Sprintf("Str(%q)", v.Value)
	case *BoolLit:
		return fmt.Sprintf("Bool(%t)", v.Value)
	case *NullLit:
		return "Null"
	case *VarExpr:
		return fmt.Sprintf("Var($%s)", v.Name)
	case *ConstRefExpr:
		return fmt.Sprintf("Const(%s)", v.Name)
	case *CallExpr:
		s := fmt.Sprintf("Call(%s", v.Callee)
		for _, a := range v.Args {
			s += ", " + Sprint(a)
		}
		return s + ")"
	case *LenExpr:
		return fmt.Sprintf("Len(%s)", Sprint(v.Operand))
	case *SpawnExpr:
		s := "Spawn{"
		for i, st := range v.Body {
			if i > 0 {
				s += "; "
			}
			s += Sprint(st)
		}
		return s + "}"
	case *QualifiedCallExpr:
		s := fmt.Sprintf("QCall(%s.%s", v.Prefix, v.Callee)
		for _, a := range v.Args {
			s += ", " + Sprint(a)
		}
		return s + ")"
	case *QualifiedConstRefExpr:
		return fmt.Sprintf("QConst(%s.%s)", v.Prefix, v.Name)
	case *BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", Sprint(v.Left), v.Op, Sprint(v.Right))
	case *UnaryExpr:
		return fmt.Sprintf("(%s %s)", v.Op, Sprint(v.Operand))
	case *ListLit:
		s := "List["
		for i, e := range v.Elements {
			if i > 0 {
				s += ", "
			}
			s += Sprint(e)
		}
		return s + "]"
	case *MapLit:
		s := "Map{"
		for i := range v.Keys {
			if i > 0 {
				s += ", "
			}
			s += Sprint(v.Keys[i]) + ": " + Sprint(v.Values[i])
		}
		return s + "}"
	case *IndexExpr:
		return fmt.Sprintf("Index(%s, %s)", Sprint(v.Target), Sprint(v.Index))
	case *IndexAssignStmt:
		return fmt.Sprintf("IndexAssign(%s = %s)", Sprint(v.Target), Sprint(v.Value))
	case *AppendStmt:
		return fmt.Sprintf("Append(%s = %s)", Sprint(v.Target), Sprint(v.Value))
	case *StructLit:
		s := v.Name + "{"
		for i, f := range v.Fields {
			if i > 0 {
				s += ", "
			}
			s += f.Name + ": " + Sprint(f.Expr)
		}
		return s + "}"
	case *FieldAccessExpr:
		return fmt.Sprintf("Field(%s.%s)", Sprint(v.Target), v.Field)
	case *FieldAssignStmt:
		return fmt.Sprintf("FieldAssign(%s = %s)", Sprint(v.Target), Sprint(v.Value))
	case *StructDef:
		s := "Struct(" + v.Name + "{"
		for i, f := range v.Fields {
			if i > 0 {
				s += ", "
			}
			s += f.Name + " as " + f.Type.String()
		}
		return s + "})"
	case *ForEachStmt:
		return fmt.Sprintf("ForEach($%s in %s, %s)", v.VarName, Sprint(v.Coll), Sprint(v.Body))
	}
	return fmt.Sprintf("<unknown %T>", n)
}
