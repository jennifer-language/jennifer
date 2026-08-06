# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# Descriptive statistics over a list with the `stats` library.

use io;
use stats;

def scores as list of int init [72, 88, 91, 88, 65, 88, 79, 95, 88, 70];

io.printf("n=%d  mean=%v  median=%v  mode=%d\n",
    len($scores), stats.mean($scores), stats.median($scores), stats.mode($scores));
io.printf("min=%d  max=%d  sum=%d\n",
    stats.min($scores), stats.max($scores), stats.sum($scores));
io.printf("range=%d  variance=%v  stddev=%v\n",
    stats.range($scores), stats.variance($scores), stats.stddev($scores));
io.printf("sample variance=%v  sample stddev=%v\n",
    stats.sampleVariance($scores), stats.sampleStddev($scores));
io.printf("quartiles=%v  iqr=%v  p90=%v\n",
    stats.quartiles($scores), stats.iqr($scores), stats.percentile($scores, 90));

# Pearson correlation between two series (hours studied vs score).
def hours as list of float init [1.0, 2.0, 3.0, 4.0, 5.0];
def result as list of float init [50.0, 60.0, 65.0, 80.0, 95.0];
io.printf("study-vs-score correlation = %v\n", stats.correlation($hours, $result));
