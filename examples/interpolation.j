# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * String interpolation: a cooked "..." string interpolates each `{expr}` slot -
 * a single expression evaluated in the current scope and stringified in place, so
 * a value sits beside its label with no `sprintf` verb / arg matching. A raw
 * '...' string never interpolates, and `\{` / `\}` are literal braces. Slots stay
 * simple here (variables, index, arithmetic) - the recommended style; a call
 * belongs in a variable first (see `total` below).
 * @module interpolation
 */

use io;
use strings;

def name as string init "Jennifer";
def n as int init 41;
def xs as list of int init [1, 2, 3];
def count as int init len($xs);

# A variable, an arithmetic expression, an index, and a list value - each a slot.
io.printf("hello {$name}, next is {$n + 1}\n");
io.printf("list = {$xs}, first = {$xs[0]}, count = {$count}\n");

# Compute a call into a variable, then interpolate the variable (the tidy style;
# `lint` flags a call placed directly in a slot as L204).
def shout as string init strings.upper($name);
io.printf("shout = {$shout}\n");

# A raw '...' string stays literal - no interpolation.
io.printf("a raw template: {$name} stays literal -> " + '{$name}' + "\n");

# `\{` / `\}` are literal braces in a cooked string.
io.printf("literal braces: \{ {$n} \}\n");
