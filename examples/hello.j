// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>
//
// hello.j - the canonical Jennifer M1 program.
// Prints 42.
import stdlib;

def app() {
    define $x as int init 21;
    printf($x + $x);
}
