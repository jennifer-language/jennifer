# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# The ml library: classical machine learning with a fit / predict shape. This
# demo uses only the deterministic models (regression, k-NN, decision tree,
# scaling, metrics), so the output is a stable golden - the random models
# (kMeans, randomForest, trainTestSplit) honor math.randSeed instead.

use io;
use ml;

# --- Regression: fit y = 2*x1 + 3*x2 + 1 exactly, then predict. ---
def X as list of list of float init [[1.0, 1.0], [2.0, 1.0], [1.0, 2.0], [3.0, 2.0], [2.0, 3.0], [4.0, 1.0]];
def y as list of float init [6.0, 8.0, 9.0, 13.0, 14.0, 12.0];
def reg as ml.Model init ml.linearRegression($X, $y);
io.printf("linreg predict [5,5] = %f|prec=1\n", ml.predict($reg, [[5.0, 5.0]])[0]);
io.printf("linreg r2 on train  = %f|prec=3\n", ml.r2($y, ml.predict($reg, $X)));
# Introspection: read the learned coefficients / intercept (y = 2*x1 + 3*x2 + 1).
io.printf("linreg coefficients  = %v intercept = %f|prec=0\n", ml.coefficients($reg), ml.intercept($reg));

# --- Ridge regularization shrinks the fit (higher alpha, smaller coefficients). ---
def rdg as ml.Model init ml.ridge($X, $y, 5.0);
io.printf("ridge  predict [5,5] = %f|prec=2\n", ml.predict($rdg, [[5.0, 5.0]])[0]);

# --- Classification: a clean two-class problem, k-NN and a decision tree. ---
def CX as list of list of float init [[1.0, 1.0], [1.5, 2.0], [2.0, 1.0], [8.0, 8.0], [8.5, 9.0], [9.0, 8.0]];
def cy as list of float init [0.0, 0.0, 0.0, 1.0, 1.0, 1.0];
def query as list of list of float init [[1.2, 1.2], [8.7, 8.5]];
io.printf("kNN(k=3) predict     = %v\n", ml.predict(ml.kNN($CX, $cy, 3), $query));
io.printf("decisionTree predict = %v\n", ml.predict(ml.decisionTree($CX, $cy), $query));

# --- Feature scaling: standardize to mean 0 / unit variance. ---
def scaler as ml.Model init ml.standardScaler($CX);
io.printf("standardized [1,1]   = %v\n", ml.transform($scaler, [[1.0, 1.0]]));

# --- Metrics on a set of predictions. ---
def yt as list of float init [1.0, 0.0, 1.0, 1.0, 0.0, 1.0];
def yp as list of float init [1.0, 0.0, 0.0, 1.0, 0.0, 1.0];
io.printf("accuracy=%f|prec=3 precision=%f|prec=3 recall=%f|prec=3 f1=%f|prec=3\n",
    ml.accuracy($yt, $yp), ml.precision($yt, $yp, 1), ml.recall($yt, $yp, 1), ml.f1($yt, $yp, 1));
io.printf("rocAuc(perfect)      = %f|prec=1\n", ml.rocAuc([1.0, 0.0, 1.0, 0.0], [0.9, 0.1, 0.8, 0.4]));
