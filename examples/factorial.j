// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>
//
// factorial.j - recursion + parameters + multi-arg printf.
// Demonstrates M3 features: method parameters, return values, recursion,
// and format-string printf.
use stdlib;

func fact(n as int) {
    if ($n == 0) { return 1; }
    return $n * fact($n - 1);
}

for (def i as int init 0; $i <= 8; $i = $i + 1) {
    printf("%d! = %d\n", $i, fact($i));
}
