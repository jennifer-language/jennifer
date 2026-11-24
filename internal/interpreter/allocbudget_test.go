// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// The per-task allocation budget (task.setAllocBudget, M29.3): a task counts the
// bytes it allocates, and past its limit the next charged operation raises a
// catchable Error{kind: "limit"} in that task only.

func runBudget(t *testing.T, src string) string {
	t.Helper()
	out, err := runAllLibs(t, src)
	if err != nil {
		t.Fatalf("run: %v\noutput: %s", err, out)
	}
	return out
}

// A task over its budget raises "limit" at the next charged operation, catches
// it and goes on; the error names the budget and the count.
func TestAllocBudgetTripsAndIsCaught(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		def t as task of string init spawn {
			task.setAllocBudget(100000);
			def xs as list of int init [];
			try {
				while (true) { $xs[] = 1; }
			} catch (e) {
				return $e.kind + "|" + $e.message;
			}
			return "no trip";
		};
		io.printf("%s", task.wait($t));
	`)
	if !strings.HasPrefix(out, "limit|task allocation budget exceeded: ") ||
		!strings.Contains(out, "over the 100000-byte budget set by task.setAllocBudget") {
		t.Fatalf("got %q", out)
	}
}

// A sibling task without a budget is unaffected by one that trips, and counts
// nothing.
func TestAllocBudgetSiblingUnaffected(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		def a as task of string init spawn {
			task.setAllocBudget(10000);
			def xs as list of int init [];
			try {
				while (true) { $xs[] = 1; }
			} catch (e) {
				return $e.kind;
			}
			return "no trip";
		};
		def b as task of string init spawn {
			def xs as list of int init [];
			for (def i in 0..100000) { $xs[] = $i; }
			return "built " + convert.toString(len($xs)) + " counted " + convert.toString(task.allocated());
		};
		use convert;
		io.printf("%s / %s", task.wait($a), task.wait($b));
	`)
	if out != "limit / built 100000 counted 0" {
		t.Fatalf("got %q", out)
	}
}

// A task spawned under a budget shares its counter and limit: the child trips
// on the parent's budget, and the parent sees the child's bytes.
func TestAllocBudgetSpawnedChildShares(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		task.setAllocBudget(50000);
		def before as int init task.allocated();
		def c as task of string init spawn {
			def xs as list of int init [];
			try {
				while (true) { $xs[] = 1; }
			} catch (e) {
				return $e.kind;
			}
			return "no trip";
		};
		io.printf("%s %t", task.wait($c), task.allocated() > 50000 + $before);
	`)
	if out != "limit true" {
		t.Fatalf("got %q", out)
	}
}

// math.MAX_INT lifts the limit and keeps counting; a second call changes the
// limit and keeps the count. The 100000 appends charge at least 3 MB: a Value
// cell is 48 bytes on 64-bit targets and 32 on 32-bit ones, past the lifted
// 2000000 either way.
func TestAllocBudgetMaxIntLiftsAndKeepsCount(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		use math;
		task.setAllocBudget(1000000);
		def xs as list of int init [];
		for (def i in 0..100) { $xs[] = $i; }
		def n1 as int init task.allocated();
		task.setAllocBudget(2000000);
		def n2 as int init task.allocated();
		task.setAllocBudget(math.MAX_INT);
		for (def i in 0..100000) { $xs[] = $i; }
		io.printf("%t %t %t", $n1 > 0, $n1 == $n2, task.allocated() > 3000000);
	`)
	if out != "true true true" {
		t.Fatalf("got %q", out)
	}
}

// Lowering the limit below the count raises at the next charged operation.
func TestAllocBudgetLoweredBelowCount(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		task.setAllocBudget(1000000);
		def xs as list of int init [];
		for (def i in 0..100) { $xs[] = $i; }
		task.setAllocBudget(10);
		try {
			$xs[] = 1;
			io.printf("no trip");
		} catch (e) {
			io.printf("%s", $e.kind);
		}
	`)
	if out != "limit" {
		t.Fatalf("got %q", out)
	}
}

// 0 and negative are caller errors of the task library's kind, "runtime".
func TestAllocBudgetRejectsZeroAndNegative(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		for (def n in [0, -1]) {
			try {
				task.setAllocBudget($n);
			} catch (e) {
				io.printf("%s: %s\n", $e.kind, $e.message);
			}
		}
		io.printf("%d", task.allocated());
	`)
	want := "runtime: task.setAllocBudget: bytes must be > 0 (math.MAX_INT for no limit), got 0\n" +
		"runtime: task.setAllocBudget: bytes must be > 0 (math.MAX_INT for no limit), got -1\n0"
	if out != want {
		t.Fatalf("got %q", out)
	}
}

// Without a budget nothing is counted.
func TestAllocBudgetNoneCountsNothing(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		use strings;
		def s as string init "";
		def xs as list of string init [];
		for (def i in 0..1000) {
			$s = $s + "ab";
			$xs[] = strings.replicate("x", 100);
		}
		io.printf("%d", task.allocated());
	`)
	if out != "0" {
		t.Fatalf("got %q", out)
	}
}

// Each charged operation grows the count, and each one alone trips a budget in
// a loop.
func TestAllocBudgetChargedOperations(t *testing.T) {
	ops := map[string]string{
		"list append":     `$xs[] = 1;`,
		"bytes append":    `$b[] = 1;`,
		"map insert":      `$m["k" + convert.toString($i)] = $i;`,
		"concat":          `$s = $s + "abcdef";`,
		"interpolation":   `def t as string init "{$big}";`,
		"builtin result":  `def r as string init strings.replicate("x", 1000);`,
		"define copy":     `def c as list of int init $big;`,
		"assign copy":     `$c2 = $big;`,
		"param copy":      `keep($big);`,
		"field write":     `$p.items = $big;`,
		"range":           `def r as list of int init 0..100;`,
		"slice":           `def r as list of int init $big[0..50];`,
		"list literal":    `def r as list of list of int init [$big];`,
		"map literal":     `def r as map of string to list of int init {"a": $big};`,
		"struct literal":  `def r as Box init Box{items: $big};`,
		"index overwrite": `$grid[0] = $big;`,
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			src := `
				use io;
				use task;
				use strings;
				use convert;
				def struct Box { items as list of int };
				func keep(v as list of int) {
					$v[0] = 2;
					return len($v);
				}
				def big as list of int init [];
				for (def k in 0..100) { $big[] = $k; }
				def xs as list of int init [];
				def b as bytes;
				def m as map of string to int init {};
				def s as string init "";
				def c2 as list of int init [];
				def p as Box;
				def grid as list of list of int init [[]];
				def i as int init 0;
				task.setAllocBudget(200000);
				def start as int init task.allocated();
				try {
					while (true) {
						` + op + `
						$i = $i + 1;
					}
				} catch (e) {
					io.printf("%s %t", $e.kind, $i > 0);
				}
			`
			if out := runBudget(t, src); out != "limit true" {
				t.Fatalf("got %q", out)
			}
		})
	}
}

// A task that catches the error and goes on gets room to recover; one that
// keeps allocating raises again.
func TestAllocBudgetRaisesAgainAfterRecovery(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		task.setAllocBudget(10000);
		def xs as list of int init [];
		def trips as int init 0;
		while ($trips < 3) {
			try {
				while (true) { $xs[] = 1; }
			} catch (e) {
				$trips = $trips + 1;
				def note as string init "caught " + $e.kind;
			}
		}
		io.printf("%d %t", $trips, len($xs) > 40000);
	`)
	if out != "3 true" {
		t.Fatalf("got %q", out)
	}
}

// A task that sets a budget under an inherited one nests it: its bytes count
// toward both, the outer limit still holds, and allocated() reads its own.
func TestAllocBudgetNestedUnderInherited(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		task.setAllocBudget(60000);
		def c as task of string init spawn {
			task.setAllocBudget(1000000000);
			def own as int init task.allocated();
			def xs as list of int init [];
			try {
				while (true) { $xs[] = 1; }
			} catch (e) {
				return $e.message;
			}
			return "no trip";
		};
		io.printf("%s", task.wait($c));
	`)
	if !strings.Contains(out, "over the 60000-byte budget") {
		t.Fatalf("got %q", out)
	}
}

// The budget follows the task into a module's functions: bytes a module function
// allocates are charged, and its trip is catchable in the caller.
func TestAllocBudgetAcrossModuleBoundary(t *testing.T) {
	out := runModuleMainAll(t, map[string]string{
		"grow.j": `
			export func grow() {
				def xs as list of int init [];
				while (true) { $xs[] = 1; }
				return len($xs);
			}
		`,
		"main.j": `
			use io;
			use task;
			import "./grow.j" as g;
			def t as task of string init spawn {
				task.setAllocBudget(30000);
				try {
					g.grow();
				} catch (e) {
					return $e.kind;
				}
				return "no trip";
			};
			io.printf("%s", task.wait($t));
		`,
	})
	if out != "limit" {
		t.Fatalf("got %q", out)
	}
}

// An eager copy and a literal element are charged their full size, every
// nesting level included: before, a two-level estimate charged a copy of a
// list of structs holding lists a fraction of its cells, and a budgeted task
// could allocate hundreds of times its budget.
func TestAllocBudgetChargesDeepCopiesInFull(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		def struct Rec { id as int, vals as list of int };
		def struct Box { rows as list of list of int };
		def recs as list of Rec;
		for (def i in 0..50) { $recs[] = Rec{id: $i, vals: 0..40}; }
		def b as list of list of int init [[1, 2, 3], [4, 5, 6]];
		task.setAllocBudget(1000000000);
		def n0 as int init task.allocated();
		def c as list of Rec init $recs;
		def n1 as int init task.allocated();
		def l as list of list of list of int init [$b, $b];
		def n2 as int init task.allocated();
		def x as Box init Box{rows: $b};
		def n3 as int init task.allocated();
		io.printf("%d %d %d", $n1 - $n0, $n2 - $n1, $n3 - $n2);
	`)
	cell := int(unsafe.Sizeof(interpreter.Value{}))
	// list of 50 Rec: 50 cells, 2 fields each, 40 ints each; [$b, $b]: 2 cells
	// plus two copies of b (2 + 6 cells); Box{rows: $b}: 1 field plus b.
	want := fmt.Sprintf("%d %d %d", (50+50*2+50*40)*cell, (2+2*8)*cell, (1+8)*cell)
	if out != want {
		t.Fatalf("charged %q, want %q", out, want)
	}
}

// The review's reproduction: 20 copies of a ~48 MB list of structs under a
// 100 MB budget raise "limit" (before the fix: 5.7 MB charged, no raise).
func TestAllocBudgetDeepCopiesTrip(t *testing.T) {
	out := runBudget(t, `
		use io;
		use task;
		def struct Rec { id as int, vals as list of int };
		def recs as list of Rec;
		def row as list of int init 0..200;
		for (def i in 0..200) { $recs[] = Rec{id: $i, vals: $row}; }
		task.setAllocBudget(4000000);
		def keep as list of list of Rec;
		try {
			for (def k in 0..20) { def c as list of Rec init $recs; $keep[] = $c; }
			io.printf("no trip at %d", task.allocated());
		} catch (e) {
			io.printf("%s %d", $e.kind, len($keep));
		}
	`)
	if !strings.HasPrefix(out, "limit ") || out == "limit 20" {
		t.Fatalf("got %q", out)
	}
}
