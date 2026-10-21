// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"
	"strings"

	"jennifer-lang.dev/jennifer/internal/libsig"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// checkUnknownMember (L114) flags a `ns.name` whose namespace is declared but
// has no member by that name: a library builtin, constant, or type the library
// does not register (`time.millis(10)`, `math.PII`, `def t as time.Tme`), or a
// name an imported module does not export (unknown, or private to it). The
// runtime reports these only when the reference executes, so a typo in a rarely
// taken branch survives until that branch runs.
//
// The surfaces are exact: a library's builtins come from the signature table
// (kept complete against InstallAll by a test), its constants and types from
// the registered tables the caller supplies in Config, and a module's from its
// parsed program. A surface that is not available - no Config tables, a module
// that did not load, a library the signature table does not cover - is skipped,
// never guessed. An undeclared namespace is L112's; an enum's variants are
// checked when the program loads.
func checkUnknownMember(c *checkCtx) {
	t := newTyper(c)
	report := func(n parser.Node, what, prefix, name string, candidates []string) {
		line, col := n.Pos()
		reportTypeAt(c, n.Filename(), line, col, what, prefix, name, candidates)
	}

	// call checks `prefix.name(...)`.
	call := func(n *parser.QualifiedCallExpr) {
		if mod := c.cfg.Modules[n.Prefix]; mod != nil {
			if exportedMethod(mod, n.Callee) == nil {
				report(n, moduleWhat(mod, n.Callee, "function"), n.Prefix, n.Callee, moduleMethods(mod))
			}
			return
		}
		lib, ok := t.libCall(n.Prefix)
		if !ok || !libsig.HasLibrary(lib) || len(libsig.Lookup(lib, n.Callee)) > 0 {
			return
		}
		report(n, "function", n.Prefix, n.Callee, libsig.Names(lib))
	}

	// constRef checks `prefix.NAME` (a constant, or a module function used as a
	// value).
	constRef := func(n *parser.QualifiedConstRefExpr) {
		if mod := c.cfg.Modules[n.Prefix]; mod != nil {
			if exportedConst(mod, n.Name) == nil && exportedMethod(mod, n.Name) == nil {
				report(n, moduleWhat(mod, n.Name, "constant"), n.Prefix, n.Name, moduleConsts(mod))
			}
			return
		}
		lib, ok := t.libCall(n.Prefix)
		if !ok || c.cfg.LibConsts == nil {
			return
		}
		if _, ok := c.cfg.LibConsts[lib+"."+n.Name]; ok {
			return
		}
		if len(libsig.Lookup(lib, n.Name)) > 0 {
			return // a builtin named without a call; the runtime explains that
		}
		report(n, "constant", n.Prefix, n.Name, libConsts(c, lib))
	}

	// typeName checks a struct / enum / opaque type `prefix.Name`, used in a
	// literal or a declared type; n is nil for a declared type, which reports at
	// file:line:col instead.
	typeName := func(n parser.Node, file string, line, col int, prefix, name string) {
		if mod := c.cfg.Modules[prefix]; mod != nil {
			if exportedStruct(mod, name) == nil && exportedEnum(mod, name) == nil {
				msg := moduleWhat(mod, name, "type")
				if n != nil {
					report(n, msg, prefix, name, moduleTypes(mod))
				} else {
					reportTypeAt(c, file, line, col, msg, prefix, name, moduleTypes(mod))
				}
			}
			return
		}
		lib, ok := t.libCall(prefix)
		if !ok || c.cfg.LibStructs == nil || t.libStruct(lib, name) != nil {
			return
		}
		if n != nil {
			report(n, "type", prefix, name, libTypes(c, lib))
		} else {
			reportTypeAt(c, file, line, col, "type", prefix, name, libTypes(c, lib))
		}
	}

	var declType func(ty parser.Type, file string, line, col int)
	declType = func(ty parser.Type, file string, line, col int) {
		if ty.Kind == parser.TypeStruct && ty.StructNS != "" {
			typeName(nil, file, line, col, ty.StructNS, ty.StructName)
		}
		for _, sub := range []*parser.Type{ty.Element, ty.KeyType, ty.ValType} {
			if sub != nil {
				declType(*sub, file, line, col)
			}
		}
	}

	w := walker{
		stmt: func(s parser.Stmt) {
			if d, ok := s.(*parser.DefineStmt); ok {
				line, col := d.Pos()
				declType(d.VarType, d.Filename(), line, col)
			}
		},
		expr: func(e parser.Expr) {
			switch n := e.(type) {
			case *parser.QualifiedCallExpr:
				call(n)
			case *parser.QualifiedConstRefExpr:
				constRef(n)
			case *parser.StructLit:
				switch {
				case n.NS == "":
				case n.Enum != "":
					typeName(n, "", 0, 0, n.NS, n.Enum) // `m.Enum.Variant`: the variant is load-checked
				default:
					typeName(n, "", 0, 0, n.NS, n.Name)
				}
			}
		},
	}
	w.program(c.prog)
	for _, m := range c.prog.Methods {
		for _, p := range m.Params {
			declType(p.Type, p.File, p.Line, p.Col)
		}
	}
	for _, sd := range c.prog.Structs {
		for _, f := range sd.Fields {
			declType(f.Type, f.File, f.Line, f.Col)
		}
	}
	for _, ed := range c.prog.Enums {
		for _, v := range ed.Variants {
			for _, f := range v.Fields {
				declType(f.Type, f.File, f.Line, f.Col)
			}
		}
	}
}

// reportTypeAt reports an unknown member at an explicit position (a parameter or
// field is not a Node).
func reportTypeAt(c *checkCtx, file string, line, col int, what, prefix, name string, candidates []string) {
	if kind, ok := strings.CutPrefix(what, "private "); ok {
		c.reportAt("L114", file, line, col, fmt.Sprintf("%s `%s.%s` is not exported by the module", kind, prefix, name))
		return
	}
	msg := fmt.Sprintf("%s `%s.%s` does not exist", what, prefix, name)
	if s := closestName(name, candidates); s != "" {
		msg += fmt.Sprintf(" - did you mean `%s.%s`?", prefix, s)
	}
	c.reportAt("L114", file, line, col, msg)
}

// moduleWhat names the kind of a missing module member, saying "is not
// exported" when the module does define it privately.
func moduleWhat(mod *parser.Program, name, kind string) string {
	for _, m := range mod.Methods {
		if m.Name == name {
			return "private function"
		}
	}
	for _, st := range mod.TopLevel {
		if d, ok := st.(*parser.DefineStmt); ok && d.IsConst && d.VarName == name {
			return "private constant"
		}
	}
	for _, sd := range mod.Structs {
		if sd.Name == name {
			return "private type"
		}
	}
	for _, ed := range mod.Enums {
		if ed.Name == name {
			return "private type"
		}
	}
	return kind
}

// exportedConst returns the exported top-level constant of a module program,
// or nil.
func exportedConst(mod *parser.Program, name string) *parser.DefineStmt {
	for _, st := range mod.TopLevel {
		if d, ok := st.(*parser.DefineStmt); ok && d.IsConst && d.Exported && d.VarName == name {
			return d
		}
	}
	return nil
}

func moduleMethods(mod *parser.Program) []string {
	var out []string
	for _, m := range mod.Methods {
		if m.Exported {
			out = append(out, m.Name)
		}
	}
	return out
}

func moduleConsts(mod *parser.Program) []string {
	var out []string
	for _, st := range mod.TopLevel {
		if d, ok := st.(*parser.DefineStmt); ok && d.IsConst && d.Exported {
			out = append(out, d.VarName)
		}
	}
	return out
}

func moduleTypes(mod *parser.Program) []string {
	var out []string
	for _, sd := range mod.Structs {
		if sd.Exported {
			out = append(out, sd.Name)
		}
	}
	for _, ed := range mod.Enums {
		if ed.Exported {
			out = append(out, ed.Name)
		}
	}
	return out
}

func libConsts(c *checkCtx, lib string) []string {
	return libMembers(c.cfg.LibConsts, lib)
}

func libTypes(c *checkCtx, lib string) []string {
	return libMembers(c.cfg.LibStructs, lib)
}

// libMembers returns the names a `lib.NAME`-keyed table holds for lib.
func libMembers[V any](m map[string]V, lib string) []string {
	var out []string
	for k := range m {
		if l, name, ok := strings.Cut(k, "."); ok && l == lib {
			out = append(out, name)
		}
	}
	return out
}

// closestName returns the candidate nearest to name for a "did you mean" hint:
// the same name in another case, or the unique closest within a small edit
// distance. Empty when nothing is close enough or two candidates tie.
func closestName(name string, candidates []string) string {
	limit := 2
	if len(name) <= 3 {
		limit = 1
	}
	best, bestD, tie := "", limit+1, false
	for _, cand := range candidates {
		if strings.EqualFold(cand, name) {
			return cand
		}
		d := editDistance(name, cand)
		switch {
		case d < bestD:
			best, bestD, tie = cand, d, false
		case d == bestD && cand != best:
			tie = true
		}
	}
	if tie {
		return ""
	}
	return best
}

// editDistance is the Levenshtein distance between a and b (bytes; names are
// ASCII identifiers).
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
