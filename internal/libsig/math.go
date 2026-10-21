// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("math", `
abs(int|float) int|float
min(int|float, int|float) int|float
max(int|float, int|float) int|float
sqrt(int|float) float
pow(int|float, int|float) float
floor(int|float) int
ceil(int|float) int
round(int|float) int
rand() float
randInt(int, int) int
randSeed(int) null
sin(int|float) float
cos(int|float) float
tan(int|float) float
asin(int|float) float
acos(int|float) float
atan(int|float) float
atan2(int|float, int|float) float
sinh(int|float) float
cosh(int|float) float
tanh(int|float) float
asinh(int|float) float
acosh(int|float) float
atanh(int|float) float
exp(int|float) float
expm1(int|float) float
ln(int|float) float
log10(int|float) float
log2(int|float) float
log1p(int|float) float
log(int|float, int|float) float
cbrt(int|float) float
hypot(int|float, int|float) float
sign(int|float) int|float
trunc(int|float) int
factorial(int) int
comb(int, int) int
perm(int, int) int
gcd(int, int) int
lcm(int, int) int
erf(int|float) float
erfc(int|float) float
gamma(int|float) float
lgamma(int|float) float
beta(int|float, int|float) float
lbeta(int|float, int|float) float
regGammaP(int|float, int|float) float
regGammaQ(int|float, int|float) float
regBetaI(int|float, int|float, int|float) float
`)
}
