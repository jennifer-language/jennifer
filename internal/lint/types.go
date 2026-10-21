// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"jennifer-lang.dev/jennifer/internal/libsig"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// types.go is the static typer behind L110 (argument type) and L111 (binding
// type). It answers "what type does this expression certainly have?" only where
// the answer needs no inference: a literal, a reference to a binding with a
// declared type, a field / index / slice of one, an operator over such operands,
// a struct or enum literal, and a library call whose signature table
// (internal/libsig) names one return type. Everything else - a user method call
// (methods declare no return type), a func-value call, a module call - is
// unknown, and an unknown type never produces a finding. Types are compared the
// way the runtime's MatchesDeclared compares them (kinds are strict: an int is
// not a float), so a reported mismatch is one the runtime would raise.
//
// Struct identity is kept in the linted file's own terms, normalised so two
// spellings of one type compare equal: a same-file struct or enum has no
// namespace, a library struct carries its canonical library name (whatever
// alias the file `use`s it under), and an imported module's struct carries the
// prefix the file imports the module under. A struct name the typer cannot place
// (an unresolved include, an alias it does not know) makes the type unknown.

// typer holds the per-file tables the static typer resolves names against.
type typer struct {
	c       *checkCtx
	structs map[string]*parser.StructDef
	enums   map[string]*parser.EnumDef
	methods map[string]*parser.MethodDef
	libs    map[string]string // `use` prefix (alias or name) -> canonical library
}

func newTyper(c *checkCtx) *typer {
	t := &typer{
		c:       c,
		structs: map[string]*parser.StructDef{},
		enums:   map[string]*parser.EnumDef{},
		methods: map[string]*parser.MethodDef{},
		libs:    map[string]string{},
	}
	for _, sd := range c.prog.Structs {
		t.structs[sd.Name] = sd
	}
	for _, ed := range c.prog.Enums {
		t.enums[ed.Name] = ed
	}
	for _, m := range c.prog.Methods {
		t.methods[m.Name] = m
	}
	for _, im := range c.prog.Imports {
		if im.AsName != "" {
			t.libs[im.AsName] = im.Name
		} else {
			t.libs[im.Name] = im.Name
		}
	}
	return t
}

// errorFields is the reserved Error struct's shape.
var errorFields = []parser.StructField{
	{Name: "kind", Type: parser.Type{Kind: parser.TypeString}},
	{Name: "message", Type: parser.Type{Kind: parser.TypeString}},
	{Name: "file", Type: parser.Type{Kind: parser.TypeString}},
	{Name: "line", Type: parser.Type{Kind: parser.TypeInt}},
	{Name: "col", Type: parser.Type{Kind: parser.TypeInt}},
}

// known reports whether t carries a type (TypeInvalid is "unknown").
func known(t parser.Type) bool { return t.Kind != parser.TypeInvalid }

// libStruct returns a library struct's declaration, when the CLI supplied the
// library surface.
func (t *typer) libStruct(lib, name string) *parser.StructDef {
	return t.c.cfg.LibStructs[lib+"."+name]
}

// isLibType reports whether lib.name is a library struct / opaque type lint
// can stand behind: one the CLI registered, or (with no registered surface) any
// type of a library the signature table covers.
func (t *typer) isLibType(lib, name string) bool {
	if t.c.cfg.LibStructs != nil {
		return t.libStruct(lib, name) != nil
	}
	return libsig.HasLibrary(lib)
}

// norm normalises a type written in the linted file. ok=false when a struct in
// it cannot be placed; a collection whose element cannot be placed stays known
// with an open element.
func (t *typer) norm(ty parser.Type) (parser.Type, bool) {
	switch ty.Kind {
	case parser.TypeInvalid:
		return ty, false
	case parser.TypeStruct:
		switch ns := ty.StructNS; {
		case ns == "":
			if ty.StructName == "Error" || t.structs[ty.StructName] != nil || t.enums[ty.StructName] != nil {
				return parser.Type{Kind: parser.TypeStruct, StructName: ty.StructName}, true
			}
		case t.c.cfg.Modules[ns] != nil:
			mod := t.c.cfg.Modules[ns]
			if exportedStruct(mod, ty.StructName) != nil || exportedEnum(mod, ty.StructName) != nil {
				return parser.Type{Kind: parser.TypeStruct, StructNS: ns, StructName: ty.StructName}, true
			}
		case t.libs[ns] != "":
			lib := t.libs[ns]
			if t.isLibType(lib, ty.StructName) {
				return parser.Type{Kind: parser.TypeStruct, StructNS: lib, StructName: ty.StructName}, true
			}
		}
		return parser.Type{}, false
	}
	return mapElems(ty, t.norm), true
}

// mapElems rebuilds a collection type with f applied to its element / key /
// value types; an element f cannot place becomes open (nil).
func mapElems(ty parser.Type, f func(parser.Type) (parser.Type, bool)) parser.Type {
	sub := func(p *parser.Type) *parser.Type {
		if p == nil {
			return nil
		}
		if n, ok := f(*p); ok {
			return &n
		}
		return nil
	}
	switch ty.Kind {
	case parser.TypeList, parser.TypeTask, parser.TypeChannel:
		return parser.Type{Kind: ty.Kind, Element: sub(ty.Element)}
	case parser.TypeMap:
		k, v := sub(ty.KeyType), sub(ty.ValType)
		if k == nil || v == nil {
			return parser.Type{Kind: parser.TypeMap}
		}
		return parser.Type{Kind: parser.TypeMap, KeyType: k, ValType: v}
	}
	return ty
}

// fromModule translates a type written inside the module imported under
// prefix into the linted file's terms: the module's own struct / enum gains the
// prefix, a library type its canonical name (through the module's own `use`).
// ok=false for a struct the translation cannot place (a nested module's type).
func (t *typer) fromModule(mod *parser.Program, prefix string, ty parser.Type) (parser.Type, bool) {
	if ty.Kind != parser.TypeStruct {
		if ty.Kind == parser.TypeInvalid {
			return ty, false
		}
		return mapElems(ty, func(e parser.Type) (parser.Type, bool) { return t.fromModule(mod, prefix, e) }), true
	}
	if ty.StructNS == "" {
		if ty.StructName == "Error" {
			return ty, true
		}
		if exportedStruct(mod, ty.StructName) != nil || exportedEnum(mod, ty.StructName) != nil {
			return parser.Type{Kind: parser.TypeStruct, StructNS: prefix, StructName: ty.StructName}, true
		}
		return parser.Type{}, false
	}
	for _, im := range mod.Imports {
		if im.AsName == ty.StructNS || (im.AsName == "" && im.Name == ty.StructNS) {
			if t.isLibType(im.Name, ty.StructName) {
				return parser.Type{Kind: parser.TypeStruct, StructNS: im.Name, StructName: ty.StructName}, true
			}
		}
	}
	return parser.Type{}, false
}

// fromLib translates a library struct's field type: the library's own structs
// are written bare in its registration.
func (t *typer) fromLib(lib string, ty parser.Type) (parser.Type, bool) {
	if ty.Kind != parser.TypeStruct {
		if ty.Kind == parser.TypeInvalid {
			return ty, false
		}
		return mapElems(ty, func(e parser.Type) (parser.Type, bool) { return t.fromLib(lib, e) }), true
	}
	ns := ty.StructNS
	if ns == "" {
		if ty.StructName == "Error" {
			return ty, true
		}
		ns = lib
	}
	if t.libStruct(ns, ty.StructName) != nil {
		return parser.Type{Kind: parser.TypeStruct, StructNS: ns, StructName: ty.StructName}, true
	}
	return parser.Type{}, false
}

// fieldsOf returns the declared fields of a (normalised) struct type, already
// in the linted file's terms via conv, or nil when the shape is not known (an
// enum, an opaque library type, an unplaced struct).
func (t *typer) fieldsOf(st parser.Type) ([]parser.StructField, func(parser.Type) (parser.Type, bool)) {
	if st.Kind != parser.TypeStruct {
		return nil, nil
	}
	switch ns := st.StructNS; {
	case ns == "":
		if st.StructName == "Error" {
			return errorFields, t.norm
		}
		if sd := t.structs[st.StructName]; sd != nil {
			return sd.Fields, t.norm
		}
	case t.c.cfg.Modules[ns] != nil:
		mod := t.c.cfg.Modules[ns]
		if sd := exportedStruct(mod, st.StructName); sd != nil {
			return sd.Fields, func(ty parser.Type) (parser.Type, bool) { return t.fromModule(mod, ns, ty) }
		}
	default:
		if sd := t.libStruct(ns, st.StructName); sd != nil {
			return sd.Fields, func(ty parser.Type) (parser.Type, bool) { return t.fromLib(ns, ty) }
		}
	}
	return nil, nil
}

// fieldType returns the declared type of field name of struct type st.
func (t *typer) fieldType(st parser.Type, name string) (parser.Type, bool) {
	fields, conv := t.fieldsOf(st)
	for _, f := range fields {
		if f.Name == name {
			return conv(f.Type)
		}
	}
	return parser.Type{}, false
}

// litType returns the type a struct / enum literal builds, normalised.
func (t *typer) litType(lit *parser.StructLit) (parser.Type, bool) {
	if lit.Enum != "" {
		if mod := t.c.cfg.Modules[lit.NS]; mod != nil && exportedEnum(mod, lit.Enum) != nil {
			return parser.Type{Kind: parser.TypeStruct, StructNS: lit.NS, StructName: lit.Enum}, true
		}
		return parser.Type{}, false
	}
	if lit.NS == "" {
		return t.norm(parser.Type{Kind: parser.TypeStruct, StructName: lit.Name})
	}
	if t.c.cfg.Modules[lit.NS] != nil || t.libs[lit.NS] != "" {
		return t.norm(parser.Type{Kind: parser.TypeStruct, StructNS: lit.NS, StructName: lit.Name})
	}
	if ed := t.enums[lit.NS]; ed != nil && hasVariant(ed, lit.Name) {
		return parser.Type{Kind: parser.TypeStruct, StructName: ed.Name}, true
	}
	return parser.Type{}, false
}

func hasVariant(ed *parser.EnumDef, name string) bool {
	for _, v := range ed.Variants {
		if v.Name == name {
			return true
		}
	}
	return false
}

// libCall returns the library a qualified call's prefix names, if it is one
// (and not an imported module).
func (t *typer) libCall(prefix string) (string, bool) {
	if t.c.cfg.Modules[prefix] != nil {
		return "", false
	}
	lib, ok := t.libs[prefix]
	return lib, ok
}

// matchingSigs returns the call shapes of lib.name that accept n arguments.
func matchingSigs(sigs []libsig.Sig, n int) []libsig.Sig {
	var out []libsig.Sig
	for _, s := range sigs {
		if s.Accepts(n) {
			out = append(out, s)
		}
	}
	return out
}

// typeOf returns the type e certainly has, or ok=false. resolve finds the
// binding a `$name` / bare name refers to in the current scope.
func (t *typer) typeOf(e parser.Expr, resolve func(string) *binding) (parser.Type, bool) {
	prim := func(k parser.TypeKind) (parser.Type, bool) { return parser.Type{Kind: k}, true }
	switch n := e.(type) {
	case *parser.IntLit:
		return prim(parser.TypeInt)
	case *parser.FloatLit:
		return prim(parser.TypeFloat)
	case *parser.StringLit, *parser.InterpStringExpr:
		return prim(parser.TypeString)
	case *parser.BoolLit:
		return prim(parser.TypeBool)
	case *parser.NullLit:
		return prim(parser.TypeNull)
	case *parser.LenExpr:
		return prim(parser.TypeInt)
	case *parser.ListLit:
		return prim(parser.TypeList) // a literal is generic: no recorded element type
	case *parser.MapLit:
		return prim(parser.TypeMap)
	case *parser.RangeExpr:
		el := parser.Type{Kind: parser.TypeInt}
		return parser.Type{Kind: parser.TypeList, Element: &el}, true
	case *parser.SpawnExpr:
		return prim(parser.TypeTask)
	case *parser.VarExpr:
		if b := resolve(n.Name); b != nil {
			return t.norm(b.typ)
		}
	case *parser.ConstRefExpr:
		if b := resolve(n.Name); b != nil {
			return t.norm(b.typ)
		}
		if t.methods[n.Name] != nil {
			return prim(parser.TypeFunc)
		}
	case *parser.QualifiedConstRefExpr:
		if mod := t.c.cfg.Modules[n.Prefix]; mod != nil {
			for _, st := range mod.TopLevel {
				if d, ok := st.(*parser.DefineStmt); ok && d.IsConst && d.Exported && d.VarName == n.Name {
					return t.fromModule(mod, n.Prefix, d.VarType)
				}
			}
			return parser.Type{}, false
		}
		if lib := t.libs[n.Prefix]; lib != "" {
			ty, ok := t.c.cfg.LibConsts[lib+"."+n.Name]
			return ty, ok && known(ty)
		}
		if ed := t.enums[n.Prefix]; ed != nil && hasVariant(ed, n.Name) {
			return parser.Type{Kind: parser.TypeStruct, StructName: ed.Name}, true
		}
	case *parser.StructLit:
		return t.litType(n)
	case *parser.FieldAccessExpr:
		if st, ok := t.typeOf(n.Target, resolve); ok {
			return t.fieldType(st, n.Field)
		}
	case *parser.IndexExpr:
		if ct, ok := t.typeOf(n.Target, resolve); ok {
			switch ct.Kind {
			case parser.TypeList:
				if ct.Element != nil {
					return *ct.Element, true
				}
			case parser.TypeMap:
				if ct.ValType != nil {
					return *ct.ValType, true
				}
			case parser.TypeBytes:
				return prim(parser.TypeInt)
			}
		}
	case *parser.SliceExpr:
		if ct, ok := t.typeOf(n.Target, resolve); ok {
			switch ct.Kind {
			case parser.TypeList, parser.TypeBytes, parser.TypeString:
				return ct, true
			}
		}
	case *parser.QualifiedCallExpr:
		lib, ok := t.libCall(n.Prefix)
		if !ok {
			return parser.Type{}, false
		}
		var ret *parser.Type
		for _, s := range matchingSigs(libsig.Lookup(lib, n.Callee), len(n.Args)) {
			if len(s.Ret) != 1 || (ret != nil && !ret.Equal(s.Ret[0])) {
				return parser.Type{}, false
			}
			r := s.Ret[0]
			ret = &r
		}
		if ret != nil {
			return *ret, true
		}
	case *parser.UnaryExpr:
		switch n.Op {
		case parser.OpNot:
			return prim(parser.TypeBool)
		case parser.OpBitNot:
			return prim(parser.TypeInt)
		case parser.OpNeg:
			if ot, ok := t.typeOf(n.Operand, resolve); ok && (ot.Kind == parser.TypeInt || ot.Kind == parser.TypeFloat) {
				return ot, true
			}
		}
	case *parser.BinaryExpr:
		return t.binaryType(n, resolve)
	}
	return parser.Type{}, false
}

// binaryType types an operator expression: comparisons and logic give bool,
// bitwise ops int, `/` float; `+ - * // %` follow their operands (int with int
// stays int, a float operand makes float, `+` of two strings is a string).
func (t *typer) binaryType(n *parser.BinaryExpr, resolve func(string) *binding) (parser.Type, bool) {
	switch {
	case n.Op.IsComparison(), n.Op.IsLogical():
		return parser.Type{Kind: parser.TypeBool}, true
	}
	switch n.Op {
	case parser.OpBitOr, parser.OpBitXor, parser.OpBitAnd, parser.OpShl, parser.OpShr:
		return parser.Type{Kind: parser.TypeInt}, true
	case parser.OpDiv:
		return parser.Type{Kind: parser.TypeFloat}, true
	}
	l, lok := t.typeOf(n.Left, resolve)
	r, rok := t.typeOf(n.Right, resolve)
	if !lok || !rok {
		return parser.Type{}, false
	}
	num := func(k parser.TypeKind) bool { return k == parser.TypeInt || k == parser.TypeFloat }
	switch {
	case l.Kind == parser.TypeInt && r.Kind == parser.TypeInt:
		return parser.Type{Kind: parser.TypeInt}, true
	case num(l.Kind) && num(r.Kind):
		return parser.Type{Kind: parser.TypeFloat}, true
	case n.Op == parser.OpAdd && l.Kind == parser.TypeString && r.Kind == parser.TypeString:
		return parser.Type{Kind: parser.TypeString}, true
	}
	return parser.Type{}, false
}

// typeClash reports whether a value of type a certainly fails a slot of type d.
// Kinds must match; struct identities must match; element types are compared
// only when both sides record one, and not at all when shallow (a library
// builtin checks a collection's elements one by one, so an empty collection of
// another element type still passes it).
func typeClash(d, a parser.Type, shallow bool) bool {
	if !known(d) || !known(a) {
		return false
	}
	if d.Kind != a.Kind {
		return true
	}
	elem := func(x, y *parser.Type) bool {
		return !shallow && x != nil && y != nil && typeClash(*x, *y, false)
	}
	switch d.Kind {
	case parser.TypeList, parser.TypeChannel:
		return elem(d.Element, a.Element)
	case parser.TypeMap:
		return elem(d.KeyType, a.KeyType) || elem(d.ValType, a.ValType)
	case parser.TypeStruct:
		return d.StructNS != a.StructNS || d.StructName != a.StructName
	}
	return false
}

// mismatch checks e against slot type d. It returns the offending expression
// (e itself, or an element of a list / map literal) and that expression's type,
// or nil when e may fit. A literal collection is checked element by element,
// the way the runtime validates a generic collection against a declared type.
func (t *typer) mismatch(d parser.Type, e parser.Expr, shallow bool, resolve func(string) *binding) (parser.Expr, parser.Type, parser.Type) {
	if !known(d) || e == nil {
		return nil, parser.Type{}, parser.Type{}
	}
	switch lit := e.(type) {
	case *parser.ListLit:
		if d.Kind == parser.TypeList && d.Element != nil {
			for _, el := range lit.Elements {
				if bad, want, got := t.mismatch(*d.Element, el, shallow, resolve); bad != nil {
					return bad, want, got
				}
			}
		}
	case *parser.MapLit:
		if d.Kind == parser.TypeMap && d.KeyType != nil {
			for i := range lit.Keys {
				if bad, want, got := t.mismatch(*d.KeyType, lit.Keys[i], shallow, resolve); bad != nil {
					return bad, want, got
				}
				if bad, want, got := t.mismatch(*d.ValType, lit.Values[i], shallow, resolve); bad != nil {
					return bad, want, got
				}
			}
		}
	}
	a, ok := t.typeOf(e, resolve)
	if ok && typeClash(d, a, shallow) {
		return e, d, a
	}
	return nil, parser.Type{}, parser.Type{}
}

// fitsAny reports whether e may fit at least one of alts (nil alts: any value).
func (t *typer) fitsAny(alts []parser.Type, e parser.Expr, resolve func(string) *binding) bool {
	if alts == nil {
		return true
	}
	for _, d := range alts {
		if bad, _, _ := t.mismatch(d, e, true, resolve); bad == nil {
			return true
		}
	}
	return false
}
