// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("decimal", `
from(string|int|float) decimal.Decimal
add(decimal.Decimal|int|string, decimal.Decimal|int|string) decimal.Decimal
sub(decimal.Decimal|int|string, decimal.Decimal|int|string) decimal.Decimal
mul(decimal.Decimal|int|string, decimal.Decimal|int|string) decimal.Decimal
div(decimal.Decimal|int|string, decimal.Decimal|int|string, int?, string?) decimal.Decimal
neg(decimal.Decimal|int|string) decimal.Decimal
abs(decimal.Decimal|int|string) decimal.Decimal
normalize(decimal.Decimal|int|string) decimal.Decimal
round(decimal.Decimal|int|string, int, string?) decimal.Decimal
pow(decimal.Decimal|int|string, int) decimal.Decimal
sum(list of any) decimal.Decimal
compare(decimal.Decimal|int|string, decimal.Decimal|int|string) int
sign(decimal.Decimal|int|string) int
scale(decimal.Decimal|int|string) int
isInteger(decimal.Decimal|int|string) bool
toString(decimal.Decimal|int|string) string
toInt(decimal.Decimal|int|string) int
toFloat(decimal.Decimal|int|string) float
`)
}
