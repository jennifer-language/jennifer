#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * `def enum` - sum types. A value of an enum type is exactly one of its
 * variants; a variant is a payload-less tag or carries named fields (a
 * mini-struct). `match` consumes an enum by variant pattern, binds the payload,
 * and is checked for exhaustiveness at compile time.
 * @module enum
 */
use io;

def enum Shape {
    Circle { r as float },
    Rect { w as float, h as float },
    Empty
};

# A pattern `match`: each arm binds the variant payload into a fresh name. The
# match must cover every variant (or carry an `else`), so a forgotten variant is
# a compile error, not a silent no-op.
func area(s as Shape) {
    match ($s) {
        when Circle(c) {
            return 3.14159 * $c.r * $c.r;
        }
        when Rect(rc) {
            return $rc.w * $rc.h;
        }
        when Empty {
            return 0.0;
        }
    }
    return -1.0;
}

func name(s as Shape) {
    match ($s) {
        when Circle(c) { return "circle"; }
        when Rect(rc) { return "rect"; }
        when Empty { return "empty"; }
    }
    return "?";
}

def shapes as list of Shape init [
    Shape.Circle{ r: 2.0 },
    Shape.Rect{ w: 3.0, h: 4.0 },
    Shape.Empty
];

for (def s in $shapes) {
    io.printf("%s area = %f\n", name($s), area($s));
}

# Value semantics: a copy is independent; equality is by variant + payload.
def a as Shape init Shape.Circle{ r: 1.0 };
def b as Shape init $a;
io.printf("a == b: %t\n", $a == $b);
io.printf("a == Empty: %t\n", $a == Shape.Empty);

# The zero value of an enum is its first declared variant, payload zeroed.
def z as Shape;
io.printf("zero = %v\n", $z);
