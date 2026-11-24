// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"fmt"
	"strings"
	"testing"
)

// TestRuneCursorScanLoop: the scanning loop the rune cursor speeds up (find
// the next delimiter from the last one, slice the piece between) gives the
// same pieces as strings.split on a non-ASCII string; a backward slice after
// the scan, a slice of another string, and a slice and search of the modified
// string (one rune prepended, so every byte offset moves) are exact too.
func TestRuneCursorScanLoop(t *testing.T) {
	out, err := runAllLibs(t, `use io;
use strings;
def parts as list of string init [];
for (def k in 0..300) {
    $parts[] = "é" + strings.replicate("ü", $k % 5) + "\\H";
}
def s as string init strings.join($parts, "\\");
def pos as int init 0;
def n as int init len($s);
def got as list of string init [];
while ($pos <= $n) {
    def at as int init strings.indexOf($s, "\\", $pos);
    if ($at < 0) {
        $at = $n;
    }
    $got[] = $s[$pos..$at];
    $pos = $at + 1;
}
io.printf("same=%v\n", $got == strings.split($s, "\\"));
io.printf("back=%s\n", $s[10..14]);
def other as string init strings.replicate("漢", 400) + $s;
io.printf("other=%s\n", $other[400..404]);
io.printf("last=%s\n", $s[$n - 4..$n]);
$s = "ß" + $s;
io.printf("moved=%s\n", $s[$n - 3..$n + 1]);
io.printf("from=%d\n", strings.indexOf($s, "H", $n - 3));`)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for k := 0; k < 300; k++ {
		parts = append(parts, "é"+strings.Repeat("ü", k%5)+`\H`)
	}
	rs := []rune(strings.Join(parts, `\`))
	n := len(rs)
	want := fmt.Sprintf("same=true\nback=%s\nother=%s\nlast=%s\nmoved=%s\nfrom=%d\n",
		string(rs[10:14]), string(rs[0:4]), string(rs[n-4:n]), string(rs[n-4:n]), n)
	if out != want {
		t.Fatalf("scan loop:\n got %q\nwant %q", out, want)
	}
}

// TestRuneCursorSpawnedScans: tasks scanning the same non-ASCII string at once
// (a spawn snapshot shares the string's bytes, so they share its cursor) each
// count every piece; run with -race.
func TestRuneCursorSpawnedScans(t *testing.T) {
	out, err := runAllLibs(t, `use io;
use task;
use strings;
def s as string init strings.replicate("äöü,", 2000);
def ts as list of task of int init [];
for (def w in 0..8) {
    $ts[] = spawn {
        def pos as int init 0;
        def n as int init len($s);
        def count as int init 0;
        while ($pos < $n) {
            def at as int init strings.indexOf($s, ",", $pos);
            if ($s[$pos..$at] == "äöü") {
                $count = $count + 1;
            }
            $pos = $at + 1;
        }
        return $count;
    };
}
def total as int init 0;
for (def t in $ts) {
    $total = $total + task.wait($t);
}
io.printf("%d\n", $total);`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "16000\n" {
		t.Fatalf("got %q, want 16000", out)
	}
}
