// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/stdlib"
)

// runModuleMainAll is runModuleMain with the whole standard library installed
// in every interpreter (these programs use meta, task, channel and lists).
func runModuleMainAll(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	var buf bytes.Buffer
	setup := func(s *interpreter.Interpreter) {
		s.Out = &buf
		stdlib.InstallAll(s)
	}
	in := interpreter.New()
	setup(in)
	in.EnableModules(dir, nil, moduleProgram, setup)
	prog, err := moduleProgram(filepath.Join(dir, "main.j"))
	if err != nil {
		t.Fatalf("parse main: %v", err)
	}
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return buf.String()
}

// An argument is the value it had when evaluated: a later argument that mutates
// the global an earlier one names must not show through, whether the callee
// borrows the parameter (a GlobalSafe function), copies it, or is a builtin, a
// module function, or a func value.
func TestArgumentSnapshotBeforeLaterArgumentMutates(t *testing.T) {
	out := runModuleMainAll(t, map[string]string{
		"m.j": `export func first(xs as list of int, z as int) { return $xs[0]; }`,
		"main.j": `
			use io;
			use lists;
			import "./m.j" as m;
			def struct P { xs as list of int };
			def g as list of int init [1, 2, 3];
			def p as P init P{xs: [1]};
			func reset() { $g[0] = 1; $p.xs[0] = 1; return 0; }
			func mut() { $g[0] = 99; $p.xs[0] = 99; return 0; }
			func f(xs as list of int, z as int) { return $xs[0]; }
			func fp(q as P, z as int) { return $q.xs[0]; }
			def fv as func init f;
			io.printf("%d ", f($g, mut()));
			reset();
			io.printf("%d ", fp($p, mut()));
			reset();
			io.printf("%d ", f($p.xs, mut()));
			reset();
			io.printf("%d ", $fv($g, mut()));
			reset();
			io.printf("%d ", m.first($g, mut()));
			reset();
			io.printf("%t", lists.contains($g, 99 + mut()));
		`,
	})
	if out != "1 1 1 1 1 false" {
		t.Fatalf("got %q, want %q", out, "1 1 1 1 1 false")
	}
}

// A spawned task that re-enters the host (module -> meta.callMain) writes the
// live host globals, so no function may borrow on the strength of its own body:
// both when the callee spawns the task and when a task started elsewhere runs
// between two reads.
func TestBorrowOffWhenSpawnReentersHost(t *testing.T) {
	files := map[string]string{
		"m.j": `use meta; export func poke() { meta.callMain("bump"); return 0; }`,
		"main.j": `
			use io;
			use task;
			use channel;
			import "./m.j" as m;
			def g as list of int init [1, 2, 3];
			func bump() { $g[0] = 99; }
			func viaSpawn(p as list of int) {
				def w as task of int init spawn { return m.poke(); };
				task.wait($w);
				return $p[0];
			}
			func viaChannel(p as list of int, go as channel of int, ack as channel of int) {
				def before as int init $p[0];
				channel.send($go, 1);
				channel.recv($ack);
				return $before * 1000 + $p[0];
			}
			io.printf("%d ", viaSpawn($g));
			$g[0] = 1;
			def go as channel of int init channel.make(0);
			def ack as channel of int init channel.make(0);
			def t as task of int init spawn { channel.recv($go); m.poke(); channel.send($ack, 1); return 0; };
			io.printf("%d", viaChannel($g, $go, $ack));
			task.wait($t);
		`,
	}
	if out := runModuleMainAll(t, files); out != "1 1001" {
		t.Fatalf("got %q, want %q", out, "1 1001")
	}
}
