# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# Scientific-notation float literals: an [eE][+-]? exponent, the natural way to
# write physical magnitudes and small probabilities. The exponent makes a
# literal a float even with no fractional part, so 1e10 is a float where a bare
# 1 is an int.

use io;
use convert;

# Physical constants, written the way a scientist writes them.
def avogadro as float init 6.022e23;
def electronCharge as float init 1.6e-19;
def planck as float init 6.626e-34;
io.printf("Avogadro       = %v\n", $avogadro);
io.printf("electron charge= %v\n", $electronCharge);
io.printf("Planck         = %v\n", $planck);

# An exponent alone (no fraction) still makes a float; uppercase E and an
# explicit + sign both work; the mantissa keeps its _ separators.
io.printf("1e10 is a %s = %v\n", convert.typeOf(1e10), 1e10);
io.printf("2.5E8 = %v, 1e+6 = %v, 1_000.5e3 = %v\n", 2.5E8, 1e+6, 1_000.5e3);

# Scientific literals are ordinary floats: arithmetic and comparison work.
io.printf("1.5e3 + 500.0 = %v\n", 1.5e3 + 500.0);
io.printf("2e-3 < 3e-3 ? %v\n", 2e-3 < 3e-3);

# The interpreter also prints extreme magnitudes in this form, so output now
# reads back as source.
io.printf("tiny = %v\n", 1e-300);
