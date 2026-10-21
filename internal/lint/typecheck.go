// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/libsig"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// typedWalk runs a scope-aware walk with the typer wired in: for-each iterators
// take their collection's element type, and the hooks see a resolver for the
// current scope. Match-arm heads are skipped (a `when Variant(bind)` pattern
// parses as a call but is not one), as in L107 / L109.
func typedWalk(c *checkCtx, onStmt func(*typer, parser.Stmt, func(string) *binding), onExpr func(*typer, parser.Expr, func(string) *binding)) {
	t := newTyper(c)
	skip := matchArmHeads(c.prog)
	s := &scoped{
		iterType: func(coll parser.Expr, resolve func(string) *binding) parser.Type {
			ct, ok := t.typeOf(coll, resolve)
			if !ok {
				return parser.Type{}
			}
			switch ct.Kind {
			case parser.TypeList:
				if ct.Element != nil {
					return *ct.Element
				}
			case parser.TypeMap:
				if ct.KeyType != nil {
					return *ct.KeyType
				}
			}
			return parser.Type{}
		},
	}
	if onStmt != nil {
		s.onStmt = func(st parser.Stmt, resolve func(string) *binding) { onStmt(t, st, resolve) }
	}
	if onExpr != nil {
		s.onExpr = func(e parser.Expr, resolve func(string) *binding) {
			if !skip[e] {
				onExpr(t, e, resolve)
			}
		}
	}
	s.program(c.prog)
}

// matchArmHeads collects every `when` arm value, which checks that read calls
// must skip.
func matchArmHeads(p *parser.Program) map[parser.Expr]bool {
	skip := map[parser.Expr]bool{}
	w := walker{stmt: func(s parser.Stmt) {
		if m, ok := s.(*parser.MatchStmt); ok {
			for i := range m.Arms {
				for _, v := range m.Arms[i].Values {
					skip[v] = true
				}
			}
		}
	}}
	w.program(p)
	return skip
}

// checkArgTypes (L110) flags a call argument whose static type certainly
// differs from the parameter's: a same-file method's declared parameter type, an
// imported module's exported method's, or a library builtin's signature
// (internal/libsig). Only a type the typer is certain of is compared, so a call
// whose argument is a user method's result, a func-value call, or anything else
// without a declared type is never reported.
func checkArgTypes(c *checkCtx) {
	typedWalk(c, nil, func(t *typer, e parser.Expr, resolve func(string) *binding) {
		switch call := e.(type) {
		case *parser.CallExpr:
			if m := t.methods[call.Callee]; m != nil && len(call.Args) == len(m.Params) {
				for i, a := range call.Args {
					if want, ok := t.norm(m.Params[i].Type); ok {
						t.reportArg(a, i, call.Callee, want, false, resolve)
					}
				}
			}
		case *parser.QualifiedCallExpr:
			name := call.Prefix + "." + call.Callee
			if mod := c.cfg.Modules[call.Prefix]; mod != nil {
				m := exportedMethod(mod, call.Callee)
				if m == nil || len(call.Args) != len(m.Params) {
					return
				}
				for i, a := range call.Args {
					if want, ok := t.fromModule(mod, call.Prefix, m.Params[i].Type); ok {
						t.reportArg(a, i, name, want, false, resolve)
					}
				}
				return
			}
			if lib, ok := t.libCall(call.Prefix); ok {
				t.checkLibArgs(call, name, matchingSigs(libsig.Lookup(lib, call.Callee), len(call.Args)), resolve)
			}
		}
	})
}

// reportArg reports argument i (0-based) when it certainly mismatches want.
func (t *typer) reportArg(a parser.Expr, i int, callee string, want parser.Type, shallow bool, resolve func(string) *binding) {
	bad, w, got := t.mismatch(want, a, shallow, resolve)
	if bad == nil {
		return
	}
	if bad == a {
		t.c.report("L110", a, fmt.Sprintf("argument %d to `%s` must be %s, got %s", i+1, callee, typeText(want), typeText(got)))
		return
	}
	t.c.report("L110", bad, fmt.Sprintf("argument %d to `%s` must be %s, but this element is %s, not %s", i+1, callee, typeText(want), typeText(got), typeText(w)))
}

// checkLibArgs reports a library call no accepted shape fits. With one shape of
// the call's arity, each argument that fits none of its parameter's types is
// reported; with several (a builtin that dispatches on its arguments), the call
// is reported once when none fits.
func (t *typer) checkLibArgs(call *parser.QualifiedCallExpr, name string, sigs []libsig.Sig, resolve func(string) *binding) {
	if len(sigs) == 0 {
		return // no signature, or an arity mismatch (L109)
	}
	type badArg struct {
		i int
		a parser.Expr
		p libsig.Param
	}
	var first []badArg
	for si, s := range sigs {
		var bads []badArg
		for i, a := range call.Args {
			p, _ := s.ParamFor(i)
			if !t.fitsAny(p.Alts, a, resolve) {
				bads = append(bads, badArg{i, a, p})
			}
		}
		if len(bads) == 0 {
			return
		}
		if si == 0 {
			first = bads
		}
	}
	if len(sigs) > 1 {
		t.c.report("L110", call, fmt.Sprintf("no form of `%s` takes these argument types", name))
		return
	}
	for _, b := range first {
		got, _ := t.typeOf(b.a, resolve)
		if _, isList := b.a.(*parser.ListLit); isList && got.Kind == parser.TypeList {
			t.c.report("L110", b.a, fmt.Sprintf("argument %d to `%s` must be %s; this list literal has an element of another type", b.i+1, name, altsText(b.p.Alts)))
			continue
		}
		if _, isMap := b.a.(*parser.MapLit); isMap && got.Kind == parser.TypeMap {
			t.c.report("L110", b.a, fmt.Sprintf("argument %d to `%s` must be %s; this map literal has an entry of another type", b.i+1, name, altsText(b.p.Alts)))
			continue
		}
		t.c.report("L110", b.a, fmt.Sprintf("argument %d to `%s` must be %s, got %s", b.i+1, name, altsText(b.p.Alts), typeText(got)))
	}
}

// checkBindingTypes (L111) flags a value whose static type certainly differs
// from the declared type of the slot it is stored in: a `def ... init`
// initializer, a plain `$x = ...` assignment, a struct field write or literal
// field, a list / map element write, and a `$xs[] = ...` append. The runtime
// rejects each of these with a positioned error; this reports it without
// running anything.
func checkBindingTypes(c *checkCtx) {
	typedWalk(c, func(t *typer, st parser.Stmt, resolve func(string) *binding) {
		switch n := st.(type) {
		case *parser.DefineStmt:
			if want, ok := t.norm(n.VarType); ok {
				what := "variable"
				if n.IsConst {
					what = "constant"
				}
				t.reportBinding(n.InitExpr, want, resolve, func(got string) string {
					return fmt.Sprintf("cannot initialize %s %s `%s` with %s", typeText(want), what, n.VarName, got)
				})
			}
		case *parser.AssignStmt:
			if b := resolve(n.VarName); b != nil {
				if want, ok := t.norm(b.typ); ok {
					t.reportBinding(n.Value, want, resolve, func(got string) string {
						return fmt.Sprintf("cannot assign %s to %s variable `$%s`", got, typeText(want), n.VarName)
					})
				}
			}
		case *parser.FieldAssignStmt:
			if st, ok := t.typeOf(n.Target.Target, resolve); ok {
				if want, ok := t.fieldType(st, n.Target.Field); ok {
					t.reportBinding(n.Value, want, resolve, func(got string) string {
						return fmt.Sprintf("field `%s` of %s expects %s, got %s", n.Target.Field, typeText(st), typeText(want), got)
					})
				}
			}
		case *parser.IndexAssignStmt:
			if ct, ok := t.typeOf(n.Target.Target, resolve); ok {
				if want, ok := elementSlot(ct, false); ok {
					t.reportBinding(n.Value, want, resolve, func(got string) string {
						return fmt.Sprintf("cannot store %s in an element of %s", got, typeText(ct))
					})
				}
			}
		case *parser.AppendStmt:
			if ct, ok := t.typeOf(n.Target, resolve); ok {
				if want, ok := elementSlot(ct, true); ok {
					t.reportBinding(n.Value, want, resolve, func(got string) string {
						return fmt.Sprintf("cannot append %s to %s", got, typeText(ct))
					})
				}
			}
		}
	}, func(t *typer, e parser.Expr, resolve func(string) *binding) {
		lit, ok := e.(*parser.StructLit)
		if !ok {
			return
		}
		fields, conv, what := t.litFields(lit)
		for _, lf := range lit.Fields {
			for _, f := range fields {
				if f.Name != lf.Name {
					continue
				}
				if want, ok := conv(f.Type); ok {
					t.reportBinding(lf.Expr, want, resolve, func(got string) string {
						return fmt.Sprintf("field `%s` of %s expects %s, got %s", f.Name, what, typeText(want), got)
					})
				}
			}
		}
	})
}

// elementSlot returns the type an element write into ct must have: a list's
// element type, a map's value type, an int for bytes. append is the `$xs[]`
// form, which a map does not take.
func elementSlot(ct parser.Type, appendForm bool) (parser.Type, bool) {
	switch ct.Kind {
	case parser.TypeList:
		if ct.Element != nil {
			return *ct.Element, true
		}
	case parser.TypeMap:
		if !appendForm && ct.ValType != nil {
			return *ct.ValType, true
		}
	case parser.TypeBytes:
		return parser.Type{Kind: parser.TypeInt}, true
	}
	return parser.Type{}, false
}

// litFields returns the declared fields of a struct literal's struct or enum
// variant, a converter into the linted file's terms, and a display name.
func (t *typer) litFields(lit *parser.StructLit) ([]parser.StructField, func(parser.Type) (parser.Type, bool), string) {
	variant := func(ed *parser.EnumDef) []parser.StructField {
		for _, v := range ed.Variants {
			if v.Name == lit.Name {
				return v.Fields
			}
		}
		return nil
	}
	if lit.Enum != "" {
		if mod := t.c.cfg.Modules[lit.NS]; mod != nil {
			if ed := exportedEnum(mod, lit.Enum); ed != nil {
				return variant(ed), func(ty parser.Type) (parser.Type, bool) { return t.fromModule(mod, lit.NS, ty) },
					fmt.Sprintf("variant `%s.%s.%s`", lit.NS, lit.Enum, lit.Name)
			}
		}
		return nil, nil, ""
	}
	if lit.NS != "" && t.c.cfg.Modules[lit.NS] == nil && t.libs[lit.NS] == "" {
		if ed := t.enums[lit.NS]; ed != nil {
			return variant(ed), t.norm, fmt.Sprintf("variant `%s.%s`", lit.NS, lit.Name)
		}
		return nil, nil, ""
	}
	st, ok := t.litType(lit)
	if !ok {
		return nil, nil, ""
	}
	fields, conv := t.fieldsOf(st)
	return fields, conv, "struct `" + typeText(st) + "`"
}

// reportBinding reports value (or an element of a literal value) that
// certainly mismatches want; msg renders the finding from the "got" phrase.
func (t *typer) reportBinding(value parser.Expr, want parser.Type, resolve func(string) *binding, msg func(got string) string) {
	bad, w, got := t.mismatch(want, value, false, resolve)
	if bad == nil {
		return
	}
	if bad == value {
		t.c.report("L111", value, msg(typeText(got)))
		return
	}
	t.c.report("L111", bad, msg(fmt.Sprintf("an element of type %s (want %s)", typeText(got), typeText(w))))
}

// typeText renders a type for a message; an open collection element prints as
// the bare collection kind ("list"), not "list of <?>".
func typeText(t parser.Type) string {
	switch t.Kind {
	case parser.TypeList, parser.TypeTask, parser.TypeChannel:
		if t.Element == nil {
			return t.Kind.String()
		}
	case parser.TypeMap:
		if t.KeyType == nil {
			return "map"
		}
	}
	return t.String()
}

// altsText renders a parameter's accepted types: "string or bytes".
func altsText(alts []parser.Type) string {
	parts := make([]string, len(alts))
	for i, a := range alts {
		parts[i] = typeText(a)
	}
	return strings.Join(parts, " or ")
}
