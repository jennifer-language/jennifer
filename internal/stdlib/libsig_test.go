// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package stdlib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/libsig"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestLibsigCoversInstalledBuiltins keeps the static signature table that
// `jennifer lint` reads (internal/libsig) in lock-step with what InstallAll
// registers: every line parses, every installed builtin has a signature, every
// signature names an installed builtin, and every library struct type a
// signature mentions is registered.
func TestLibsigCoversInstalledBuiltins(t *testing.T) {
	for _, err := range libsig.Errors() {
		t.Error(err)
	}
	in := interpreter.New()
	InstallAll(in)
	installed := map[string]map[string]bool{}
	for k := range in.NSBuiltins {
		if installed[k.NS] == nil {
			installed[k.NS] = map[string]bool{}
		}
		installed[k.NS][k.Name] = true
		if libsig.Lookup(k.NS, k.Name) == nil {
			t.Errorf("no libsig signature for %s.%s", k.NS, k.Name)
		}
	}
	structs := map[string]bool{}
	for k := range in.NSStructs {
		structs[k.NS+"."+k.Name] = true
	}
	var checkType func(where string, ty parser.Type)
	checkType = func(where string, ty parser.Type) {
		switch ty.Kind {
		case parser.TypeStruct:
			if ty.StructNS != "" && !structs[ty.StructNS+"."+ty.StructName] {
				t.Errorf("%s: unregistered library type %s.%s", where, ty.StructNS, ty.StructName)
			}
		case parser.TypeList, parser.TypeTask, parser.TypeChannel:
			if ty.Element != nil {
				checkType(where, *ty.Element)
			}
		case parser.TypeMap:
			if ty.KeyType != nil {
				checkType(where, *ty.KeyType)
				checkType(where, *ty.ValType)
			}
		}
	}
	for _, lib := range libsig.Libraries() {
		for _, name := range libsig.Names(lib) {
			where := lib + "." + name
			if !installed[lib][name] {
				t.Errorf("libsig lists %s, which InstallAll does not register", where)
			}
			for _, sig := range libsig.Lookup(lib, name) {
				for _, p := range sig.Params {
					for _, ty := range p.Alts {
						checkType(where, ty)
					}
				}
				for _, ty := range sig.Ret {
					checkType(where, ty)
				}
			}
		}
	}
}
