// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
)

// bundledModuleSrc is a module as it would ship in the system module dir: a
// struct whose field names a sibling struct, so a left-out field's zero must
// carry the importer-visible identity.
const bundledModuleSrc = `
export def struct Inner { n as int };
export def struct Opts { a as int, inner as Inner, tags as list of string };
export func innerN(o as Opts) { return $o.inner.n; }
`

// runWithSysmod writes sysmod (into a system module dir) and local (beside
// main.j) and runs main with the system module dir first on the search path,
// as the CLI does.
func runWithSysmod(t *testing.T, sysmod, local map[string]string, main string) (string, error) {
	t.Helper()
	root := t.TempDir()
	sysDir := filepath.Join(root, "sys")
	appDir := filepath.Join(root, "app")
	write := func(dir string, files map[string]string) {
		for name, content := range files {
			full := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(sysDir, sysmod)
	write(appDir, local)
	write(appDir, map[string]string{"main.j": main})
	var buf bytes.Buffer
	setup := func(s *interpreter.Interpreter) {
		s.Out = &buf
		iolib.Install(s)
	}
	in := interpreter.New()
	setup(in)
	in.EnableModules(appDir, []string{sysDir}, moduleProgram, setup)
	prog, err := moduleProgram(filepath.Join(appDir, "main.j"))
	if err != nil {
		t.Fatalf("parse main: %v", err)
	}
	runErr := in.Run(prog)
	return buf.String(), runErr
}

// A literal of a struct from a module in the system module dir may leave
// fields out; each takes its zero value, as `def x as m.T;` gives it, nested
// module structs included. That module ships with the interpreter and may add
// a field in any release, which would otherwise break every literal of it.
func TestBundledModuleLitMayLeaveFieldsOut(t *testing.T) {
	out, err := runWithSysmod(t, map[string]string{"m.j": bundledModuleSrc}, nil, `use io;
import "m.j";
def o as m.Opts init m.Opts{a: 7};
def z as m.Opts;
$z.a = 7;
io.printf("%d %d %d %t; ", $o.a, m.innerN($o), len($o.tags), $o == $z);
def i as m.Inner init $o.inner;
io.printf("%d", $i.n);`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "7 0 0 true; 0"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// The exemption follows where the file lives, not how it is imported: the same
// module beside the program stays strict, and an unknown field is still an
// error for a bundled one.
func TestBundledModuleLitKeepsOtherChecks(t *testing.T) {
	cases := []struct {
		name   string
		sysmod map[string]string
		local  map[string]string
		main   string
		want   string
	}{
		{"local module stays strict", nil, map[string]string{"m.j": bundledModuleSrc},
			`import "./m.j"; def o as m.Opts init m.Opts{a: 7};`, `missing field "inner" in struct "Opts" literal`},
		{"module in a local subdirectory stays strict", nil, map[string]string{"lib/m.j": bundledModuleSrc},
			`import "./lib/m.j"; def o as m.Opts init m.Opts{a: 7};`, `missing field "inner"`},
		{"bundled unknown field", map[string]string{"m.j": bundledModuleSrc}, nil,
			`import "m.j"; def o as m.Opts init m.Opts{a: 7, b: 1};`, `unknown field "b" in struct "Opts"`},
		{"bundled field type still checked", map[string]string{"m.j": bundledModuleSrc}, nil,
			`import "m.j"; def o as m.Opts init m.Opts{a: "x"};`, `field "a" of struct "Opts" expects int`},
		{"bundled module's own literal stays strict", map[string]string{"m.j": bundledModuleSrc +
			`export func make() { return Opts{a: 1}; }`}, nil,
			`import "m.j"; def o as m.Opts init m.make();`, `missing field "inner" in struct "Opts" literal`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := runWithSysmod(t, c.sysmod, c.local, c.main)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want %q", err, c.want)
			}
		})
	}
}
