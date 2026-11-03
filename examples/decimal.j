# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Exact decimal arithmetic with the decimal library: no float rounding,
 * division that either is exact or rounds where it is told to, and the
 * rounding modes money code needs.
 * @module decimal
 */
use io;
use decimal;

# Floats cannot hold 0.1 exactly; decimals can.
io.printf("float   0.1 + 0.2 = %v\n", 0.1 + 0.2);
io.printf("decimal 0.1 + 0.2 = %v\n", decimal.add("0.1", "0.2"));

# An invoice: operands may be Decimals, ints or decimal strings.
def lines as list of decimal.Decimal init [
    decimal.mul("19.99", 3),
    decimal.mul("4.50", 2),
    decimal.from("0.99")
];
def net as decimal.Decimal init decimal.sum($lines);
def tax as decimal.Decimal init decimal.round(decimal.mul($net, "0.20"), 2);
io.printf("net %v + tax %v = %v\n", $net, $tax, decimal.add($net, $tax));

# The scale is kept for display, but == compares values.
io.printf(
    "1.0 == 1.00: %t, scale of 1.00 is %d\n",
    decimal.from("1.0") == decimal.from("1.00"),
    decimal.scale("1.00"));

# Division: exact when it can be, otherwise at a scale you name.
io.printf("1 / 8 = %v\n", decimal.div(1, 8));
io.printf("10 / 3 at 4 places = %v\n", decimal.div(10, 3, 4));
try {
    decimal.div(10, 3);
} catch (e) {
    io.printf("%s: %s\n", $e.kind, $e.message);
}

# Rounding modes: half-even is the default.
for (def mode in ["half-even", "half-up", "half-down", "up", "down", "ceiling", "floor"]) {
    io.printf(
        "%s|pad=9|align=left  2.5 -> %v   -2.5 -> %v\n",
        $mode,
        decimal.round("2.5", 0, $mode),
        decimal.round("-2.5", 0, $mode));
}

# Big integers are decimals with scale 0.
io.printf("2^100 = %v\n", decimal.pow(2, 100));
io.printf("toInt(42.00) = %d, toFloat(0.1) = %v\n", decimal.toInt("42.00"), decimal.toFloat("0.1"));
