# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# The stats library's distributions and inference layer: probability
# distributions (normal / t / chi-square / binomial) and classical inference
# (regression, confidence intervals, hypothesis tests). All deterministic - no
# sampling - so the output is a stable golden.

use io;
use stats;

# Distributions: cdf, quantile (inverse cdf), and a discrete mass function.
io.printf("normalCdf(1.96)      = %f|prec=6\n", stats.normalCdf(1.96, 0.0, 1.0));
io.printf("normalQuantile(.975) = %f|prec=6\n", stats.normalQuantile(0.975, 0.0, 1.0));
io.printf("tQuantile(.975, 10)  = %f|prec=6\n", stats.tQuantile(0.975, 10));
io.printf("chiSqQuantile(.95,1) = %f|prec=6\n", stats.chiSquareQuantile(0.95, 1));
io.printf("binomialPmf(5,10,.5) = %f|prec=6\n", stats.binomialPmf(5, 10, 0.5));
io.printf("poissonCdf(2, 3)     = %f|prec=6\n", stats.poissonCdf(2, 3.0));

# Linear regression on an exact line y = 2x + 1.
def rg as stats.Regression init stats.linearRegression([1, 2, 3, 4, 5], [3, 5, 7, 9, 11]);
io.printf("regression: slope=%f|prec=1 intercept=%f|prec=1 r2=%f|prec=3 n=%d\n",
    $rg.slope, $rg.intercept, $rg.r2, $rg.n);

# Confidence interval for the mean and an exact proportion interval.
def data as list of float init [2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0];
def ci as stats.Interval init stats.confidenceInterval($data, 0.95);
io.printf("mean 95pct CI: lower=%f|prec=4 upper=%f|prec=4\n", $ci.lower, $ci.upper);
def pc as stats.Interval init stats.proportionCi(8, 10, 0.95, "clopper-pearson");
io.printf("proportion 8/10 Clopper-Pearson: lower=%f|prec=4 upper=%f|prec=4\n", $pc.lower, $pc.upper);

# Hypothesis tests: one-sample t, chi-square goodness of fit, one-way ANOVA.
def tt as stats.Test init stats.tTest($data, 3.0);
io.printf("tTest vs 3: t=%f|prec=4 df=%f|prec=0 p=%f|prec=4\n", $tt.statistic, $tt.df1, $tt.pValue);
def cs as stats.Test init stats.chiSquareTest([10.0, 20.0, 30.0, 40.0], [25.0, 25.0, 25.0, 25.0]);
io.printf("chiSquareTest: stat=%f|prec=1 df=%f|prec=0 p=%f|prec=6\n", $cs.statistic, $cs.df1, $cs.pValue);
def av as stats.Test init stats.anova([[1.0, 2.0, 3.0], [2.0, 3.0, 4.0], [5.0, 6.0, 7.0]]);
io.printf("anova: F=%f|prec=1 df1=%f|prec=0 df2=%f|prec=0 p=%f|prec=6\n",
    $av.statistic, $av.df1, $av.df2, $av.pValue);

# Histogram (Excel FREQUENCY): k+1 edges give k bins, last closed on the right.
io.printf("histogram: %v\n", stats.histogram([1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0], [1.0, 2.0, 3.0, 4.0]));
