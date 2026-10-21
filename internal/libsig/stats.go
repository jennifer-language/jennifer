// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	// Data lists accept int and float elements mixed, so they are `list of any`.
	add("stats", `
mean(list of any) float
median(list of any) float
mode(list of any) int|float
variance(list of any) float
stddev(list of any) float
percentile(list of any, int|float) float
min(list of any) int|float
max(list of any) int|float
sum(list of any) int|float
correlation(list of any, list of any) float
sampleVariance(list of any) float
sampleStddev(list of any) float
range(list of any) int|float
covariance(list of any, list of any) float
iqr(list of any) float
quartiles(list of any) list of float
zscore(list of any) list of float
geometricMean(list of any) float
harmonicMean(list of any) float
skewness(list of any) float
kurtosis(list of any) float
sampleCovariance(list of any, list of any) float
mad(list of any) float
weightedMean(list of any, list of any) float
modes(list of any) list of any
describe(list of any) stats.Summary
normalPdf(int|float, int|float, int|float) float
normalCdf(int|float, int|float, int|float) float
normalQuantile(int|float, int|float, int|float) float
normalSample(int|float, int|float) float
tPdf(int|float, int|float) float
tCdf(int|float, int|float) float
tQuantile(int|float, int|float) float
chiSquareCdf(int|float, int|float) float
chiSquareQuantile(int|float, int|float) float
fCdf(int|float, int|float, int|float) float
fQuantile(int|float, int|float, int|float) float
binomialPmf(int, int, int|float) float
binomialCdf(int, int, int|float) float
poissonPmf(int, int|float) float
poissonCdf(int, int|float) float
linearRegression(list of any, list of any) stats.Regression
multipleRegression(list of any, list of any) list of float
confidenceInterval(list of any, int|float) stats.Interval
proportionCi(int, int, int|float, string) stats.Interval
tTest(list of any, int|float) stats.Test
tTest2(list of any, list of any) stats.Test
chiSquareTest(list of any, list of any) stats.Test
fTest(list of any, list of any) stats.Test
anova(list of any) stats.Test
histogram(list of any, list of any) list of int
`)
}
