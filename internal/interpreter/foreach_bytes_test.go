// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// TestForEachBytes: a for-each over bytes binds each byte as an int, in order,
// with break, continue and return working as for a list; empty bytes run the
// body never.
func TestForEachBytes(t *testing.T) {
	out, err := run(t, `
use io;
use convert;
def b as bytes init convert.bytesFromString("utf-8", "A.xyz");
$b[1] = 0;
$b[2] = 255;
$b[3] = 7;
$b[4] = 9;
for (def x in $b) {
    io.printf("%d ", $x);
}
io.printf("|");
for (def x in $b) {
    if ($x == 0) {
        continue;
    }
    if ($x == 7) {
        break;
    }
    io.printf("%d ", $x);
}
io.printf("|");
func firstOver(data as bytes, limit as int) {
    for (def x in $data) {
        if ($x > $limit) {
            return $x;
        }
    }
    return -1;
}
io.printf("%d ", firstOver($b, 100));
def none as bytes;
for (def x in $none) {
    io.printf("never");
}
io.printf("%d\n", firstOver($none, 0));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "65 0 255 7 9 |65 255 |255 -1\n" {
		t.Errorf("got %q", out)
	}
}

// TestForEachBytesIteratesSnapshot: writes to the bytes inside the loop do not
// change what later iterations see, and they stay after the loop.
func TestForEachBytesIteratesSnapshot(t *testing.T) {
	out, err := run(t, `
use io;
use convert;
def b as bytes init convert.bytesFromString("utf-8", "abc");
$b[0] = 1;
$b[1] = 2;
$b[2] = 3;
for (def x in $b) {
    $b[2] = 99;
    io.printf("%d ", $x);
}
io.printf("%d\n", $b[2]);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "1 2 3 99\n" {
		t.Errorf("got %q", out)
	}
}

// TestForEachBytesVariableIsInt: the loop variable is an int, so it is an int
// where one is declared, and a string cannot be assigned to it.
func TestForEachBytesVariableIsInt(t *testing.T) {
	out, err := run(t, `
use io;
use convert;
def b as bytes init convert.bytesFromString("utf-8", "a");
$b[0] = 200;
for (def x in $b) {
    def n as int init $x + 1;
    io.printf("%d\n", $n);
    $x = "s";
}
`)
	if err == nil || !strings.Contains(err.Error(), "string") {
		t.Fatalf("err = %v (out %q), want a type error assigning a string", err, out)
	}
	if out != "201\n" {
		t.Errorf("got %q", out)
	}
}

// TestForEachStringStaysRefused: a string is no for-each collection (bytes,
// code points or graphemes would be a silent choice); the message names what
// is accepted.
func TestForEachStringStaysRefused(t *testing.T) {
	_, err := run(t, `
for (def c in "ab") {
}
`)
	if err == nil || !strings.Contains(err.Error(), "for-each requires a list, map or bytes, got string") {
		t.Fatalf("err = %v", err)
	}
}
