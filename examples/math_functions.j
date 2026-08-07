# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# The math library's function surface: trigonometry, exponentials and
# logarithms, combinatorics, and the special functions a probability layer is
# built on. Every result is deterministic - no randomness here - so the output
# is a stable golden.

use io;
use math;

# Trigonometry (radians) and its inverses.
io.printf("sin(PI/2)   = %v\n", math.sin(math.PI / 2));
io.printf("cos(0)      = %v\n", math.cos(0));
io.printf("atan2(1, 1) = %v\n", math.atan2(1, 1) * 4);        # = PI

# Exponentials and logarithms.
io.printf("exp(0)      = %v\n", math.exp(0));
io.printf("ln(E)       = %v\n", math.ln(math.E));
io.printf("log2(1024)  = %v\n", math.log2(1024));
io.printf("log(81, 3)  = %v\n", math.log(81, 3));

# Roots, magnitude, sign, and toward-zero truncation.
io.printf("cbrt(27)    = %v\n", math.cbrt(27));
io.printf("hypot(3, 4) = %v\n", math.hypot(3, 4));
io.printf("sign(-8)    = %v\n", math.sign(0 - 8));
io.printf("trunc(-2.9) = %v\n", math.trunc(0.0 - 2.9));

# Combinatorics (exact integers).
io.printf("factorial(10) = %v\n", math.factorial(10));
io.printf("comb(52, 5)   = %v\n", math.comb(52, 5));
io.printf("perm(6, 3)    = %v\n", math.perm(6, 3));
io.printf("gcd(48, 36)   = %v\n", math.gcd(48, 36));
io.printf("lcm(4, 6)     = %v\n", math.lcm(4, 6));

# Special functions: the error / gamma / beta family and the regularized
# incomplete forms every distribution CDF is built on.
io.printf("gamma(5)             = %v\n", math.gamma(5));       # = 4!
io.printf("beta(2, 3)           = %v\n", math.beta(2, 3));     # = 1/12
io.printf("regGammaP(1, 1)      = %v\n", math.regGammaP(1, 1)); # 1 - 1/e
io.printf("regBetaI(0.5, 2, 2)  = %v\n", math.regBetaI(0.5, 2, 2));

# The TAU constant, and strictness: a domain slip is a catchable error.
io.printf("TAU = %v\n", math.TAU);
try {
    def bad as float init math.ln(0);
} catch (e) {
    io.printf("ln(0) rejected: %s\n", $e.kind);
}
