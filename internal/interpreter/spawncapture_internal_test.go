// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"bytes"
	"sort"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// computeSpawnCaptures stamps each SpawnExpr with the global names its body
// needs (directly and transitively through named callees) so snapshotForSpawn
// copies only those. A missed global is a silent wrong result, so every
// uncertain shape (dynamic dispatch, module call, callback builtin, unknown
// node) must fall back to AllGlobals = copy everything. These cases pin the
// analysis; the value-semantics isolation it must preserve is covered end-to-end
// in spawn_test.go.
func TestComputeSpawnCaptures(t *testing.T) {
	// capturesOf runs src, then collects every stamped spawn (top-level and in
	// method bodies). Each case declares exactly one spawn.
	capturesOf := func(t *testing.T, src string) *parser.SpawnCaptures {
		t.Helper()
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		in := New()
		var buf bytes.Buffer
		in.Out = &buf
		if err := in.Run(prog); err != nil {
			t.Fatalf("run: %v", err)
		}
		var spawns []*parser.SpawnExpr
		spawns = append(spawns, in.scanGlobalRefs(prog.TopLevel, nil).spawns...)
		for _, m := range in.methods {
			spawns = append(spawns, in.scanGlobalRefs(m.Body.Stmts, m.Params).spawns...)
		}
		if len(spawns) != 1 {
			t.Fatalf("want exactly one spawn, found %d", len(spawns))
		}
		if spawns[0].Captures == nil {
			t.Fatalf("spawn left unstamped (Captures nil)")
		}
		return spawns[0].Captures
	}
	names := func(c *parser.SpawnCaptures) []string {
		var ns []string
		for n := range c.Globals {
			ns = append(ns, n)
		}
		sort.Strings(ns)
		return ns
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	cases := []struct {
		name string
		src  string
		all  bool     // expect AllGlobals (copy everything)
		want []string // expected Globals set when !all
	}{
		{
			"needs nothing",
			`def g as int init 1; def t as task of int init spawn { return 1; };`,
			false, nil,
		},
		{
			"one direct global",
			`def g1 as int init 1; def g2 as int init 2; def t as task of int init spawn { return $g1; };`,
			false, []string{"g1"},
		},
		{
			"transitive through a method",
			`def g as int init 2; func r() { return $g; } def t as task of int init spawn { return r(); };`,
			false, []string{"g"},
		},
		{
			"const reference is captured",
			`def const K as int init 5; def t as task of int init spawn { return K; };`,
			false, []string{"K"},
		},
		{
			"transitive union of several methods",
			`def a as int init 1; def b as int init 2; def c as int init 3;
			 func ra() { return $a; } func rb() { return $b; }
			 def t as task of int init spawn { return ra() + rb(); };`,
			false, []string{"a", "b"},
		},
		{
			"func-value call falls back to all",
			`def g as int init 1; func r() { return $g; } def f as func init r;
			 def t as task of int init spawn { return $f(); };`,
			true, nil,
		},
		{
			"recursion stays bounded",
			`def g as int init 0;
			 func countdown(n as int) { if ($n <= 0) { return $g; } return countdown($n - 1); }
			 def t as task of int init spawn { return countdown(3); };`,
			false, []string{"g"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := capturesOf(t, c.src)
			if caps.AllGlobals != c.all {
				t.Fatalf("AllGlobals = %v, want %v", caps.AllGlobals, c.all)
			}
			if c.all {
				return
			}
			got := names(caps)
			if !eq(got, c.want) {
				t.Errorf("Globals = %v, want %v", got, c.want)
			}
		})
	}
}
