// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// checkStructLits validates the field set of every struct and enum-variant
// literal collected by resolveQualifiedRefs, before any statement runs. A
// literal names its type statically and must name each declared field exactly
// once, so an unknown or missing field is knowable from the source alone; this
// reports it at load, with the same positioned message evalStructLit /
// evalEnumLit would raise when the literal ran. A literal whose type does not
// resolve here (an unknown struct, a bad alias, a private module struct) is left
// to the runtime path, which owns those messages.
func (i *Interpreter) checkStructLits() error {
	lits := i.structLits
	i.structLits = nil
	for _, ex := range lits {
		if err := i.checkStructLit(ex); err != nil {
			return err
		}
	}
	return nil
}

func (i *Interpreter) checkStructLit(ex *parser.StructLit) error {
	if ex.Enum != "" || (ex.NS != "" && i.enums[ex.NS] != nil && !i.isNamespaceOrAlias(ex.NS)) {
		return i.checkEnumLit(ex)
	}
	var def *parser.StructDef
	library := false // a library struct: any field may be left out, taking its zero
	if mod, ok := i.moduleAliases[ex.NS]; ok {
		d, exists := mod.interp.structs[ex.Name]
		if !exists || !mod.exports[ex.Name] {
			return nil
		}
		def = d
	} else if ex.NS != "" {
		canonical, err := i.resolveNamespacePrefix(ex.NS)
		if err != nil {
			return nil
		}
		def = i.NSStructs[nsKey{NS: canonical, Name: ex.Name}]
		library = true
	} else {
		def = i.structs[ex.Name]
	}
	if def == nil {
		return nil
	}
	return litFieldsError(ex, def.Fields,
		func(name string) string { return fmt.Sprintf("unknown field %q in struct %q", name, ex.Name) },
		func(name string) string {
			if _, optional := errorLocationZero(ex, name); optional || library {
				return ""
			}
			return fmt.Sprintf("missing field %q in struct %q literal", name, ex.Name)
		})
}

func (i *Interpreter) checkEnumLit(ex *parser.StructLit) error {
	var def *parser.EnumDef
	if ex.Enum != "" {
		mod, ok := i.moduleAliases[ex.NS]
		if !ok || !mod.exports[ex.Enum] {
			return nil
		}
		def = mod.interp.enums[ex.Enum]
	} else {
		def = i.enums[ex.NS]
	}
	if def == nil {
		return nil
	}
	var vdef *parser.EnumVariant
	for k := range def.Variants {
		if def.Variants[k].Name == ex.Name {
			vdef = &def.Variants[k]
			break
		}
	}
	file, line, col := posFor(ex)
	switch {
	case vdef == nil:
		return &runtimeError{Msg: fmt.Sprintf("enum %s has no variant %q", def.Name, ex.Name), File: file, Line: line, Col: col}
	case ex.Bare && len(vdef.Fields) > 0:
		return &runtimeError{Msg: fmt.Sprintf("variant %s.%s carries a payload; construct it as %s.%s{ ... }", def.Name, ex.Name, def.Name, ex.Name), File: file, Line: line, Col: col}
	case ex.Bare:
		return nil
	case len(vdef.Fields) == 0:
		return &runtimeError{Msg: fmt.Sprintf("variant %s.%s has no payload; write it as %s.%s, without braces", def.Name, ex.Name, def.Name, ex.Name), File: file, Line: line, Col: col}
	}
	return litFieldsError(ex, vdef.Fields,
		func(name string) string {
			return fmt.Sprintf("unknown field %q in variant %s.%s", name, def.Name, ex.Name)
		},
		func(name string) string {
			return fmt.Sprintf("missing field %q in variant %s.%s literal", name, def.Name, ex.Name)
		})
}

// litFieldsError compares a literal's named fields with the declared ones: an
// unknown field is reported at that field, a missing one at the literal (unless
// missing returns "", marking a field the literal may leave out).
// Unknown fields are reported first, as the evaluators do.
func litFieldsError(ex *parser.StructLit, declared []parser.StructField, unknown, missing func(string) string) error {
	known := make(map[string]bool, len(declared))
	for _, f := range declared {
		known[f.Name] = true
	}
	provided := make(map[string]bool, len(ex.Fields))
	for _, f := range ex.Fields {
		if !known[f.Name] {
			return &runtimeError{Msg: unknown(f.Name), File: f.File, Line: f.Line, Col: f.Col}
		}
		provided[f.Name] = true
	}
	for _, f := range declared {
		if !provided[f.Name] {
			msg := missing(f.Name)
			if msg == "" {
				continue // a field the literal may leave out
			}
			file, line, col := posFor(ex)
			return &runtimeError{Msg: msg, File: file, Line: line, Col: col}
		}
	}
	return nil
}
