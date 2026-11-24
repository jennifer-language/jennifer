# `ml` - classical machine learning

Enable with `use ml;`. Classical / predictive machine learning on tabular data,
a scikit-learn-lite companion to [`stats`](stats.md) and
[`linalg`](linalg.md). Models follow a **fit / predict** shape: a fit function
(`ml.kMeans`, `ml.linearRegression`, ...) trains and returns an `ml.Model`
value, and `ml.predict` / `ml.transform` apply it. Pure Go stdlib, TinyGo-clean,
both binaries.

It is **not** a deep-learning framework: tensors, autodiff and deep-net
training belong to native GPU / C++ tools, and running a pre-trained model is
already possible via [`http`](../modules/http.md) / `os.run`.

```jennifer
use ml;
use io;

# Fit a model, then apply it.
def X as list of list of float init [[1.0, 1.0], [2.0, 1.0], [1.0, 2.0], [3.0, 2.0]];
def y as list of float init [6.0, 8.0, 9.0, 13.0];   # y = 2*x1 + 3*x2 + 1
def model as ml.Model init ml.linearRegression($X, $y);
io.printf("%v\n", ml.predict($model, [[5.0, 5.0]]));  # ~[26.0]
```

## Data shape

A **feature matrix** `X` is a `list of list of float/int` (one inner list per
sample, one entry per feature); **labels / targets** `y` are a `list of
float/int`. Class labels are integers (`0`, `1`, `2`, ...); regression targets
are any number. `ml.predict` returns a `list of int` for a classifier / cluster
model and a `list of float` for a regressor.

## Fitting models

Each returns an `ml.Model`: an opaque, immutable value the garbage collector
frees once no copy is left. Copies (assignments, struct fields, list elements,
`spawn`ed tasks) share the fitted model; a model `==` itself and its copies, and
two separate fits are never equal. The zero value (`def m as ml.Model;`) holds
no model, and every call refuses it.

| Fit call | Kind | Notes |
| -------- | ---- | ----- |
| `ml.linearRegression(X, y)` | regression | Ordinary least squares (normal equations). |
| `ml.ridge(X, y, alpha)` | regression | L2-regularized OLS; `alpha >= 0` shrinks the coefficients (not the intercept). |
| `ml.lasso(X, y, alpha)` | regression | L1-regularized (coordinate descent); drives small coefficients to exactly `0`. |
| `ml.kNN(X, y, k)` | classifier | k-nearest-neighbours majority vote (Euclidean). |
| `ml.kNNRegressor(X, y, k)` | regression | k-NN averaging the `k` nearest targets. |
| `ml.naiveBayes(X, y)` | classifier | Gaussian naive Bayes (multiclass). |
| `ml.logisticRegression(X, y)` / `ml.logisticRegressionWith(X, y, ml.LogisticOptions)` | classifier | Binary or **multiclass** (one-vs-rest); gradient descent. Only the binary case has `predictProba`. |
| `ml.decisionTree(X, y)` / `ml.decisionTreeWith(X, y, ml.TreeOptions)` | classifier | CART with Gini impurity. |
| `ml.decisionTreeRegressor(X, y)` / `ml.decisionTreeRegressorWith(X, y, ml.TreeOptions)` | regression | CART with variance-reduction splits, leaf = mean target. |
| `ml.randomForest(X, y)` / `ml.randomForestWith(X, y, ml.ForestOptions)` | classifier | Bagged trees; each split considers a fresh random `sqrt(features)` subset. |
| `ml.randomForestRegressor(X, y)` / `ml.randomForestRegressorWith(X, y, ml.ForestOptions)` | regression | Bagged regression trees with the same per-split feature subsets (mean of tree predictions). |
| `ml.kMeans(X, k)` / `ml.kMeansWith(X, k, ml.KMeansOptions)` | clustering | Lloyd's algorithm, k-means++ seeding. |
| `ml.pca(X, nComponents)` | transform | Principal component analysis (covariance eigendecomposition). |
| `ml.standardScaler(X)` | transform | Fit a z-score scaler (per-feature mean / stddev). |
| `ml.minMaxScaler(X)` | transform | Fit a `[0, 1]` min-max scaler. |

### Options

The base form is the `With` form with zero options. A zero field is the
default, a negative one throws (`"runtime"`), and a count past its ceiling
throws `"limit"`.

| Struct | Field | Type | Default | Ceiling |
| --- | --- | --- | --- | --- |
| `ml.LogisticOptions` | `learningRate` | `float` | `0.1` | - |
| | `epochs` | `int` | `1000` | 1000000 |
| `ml.TreeOptions` | `maxDepth` | `int` | `8` | 64 |
| `ml.ForestOptions` | `nTrees` | `int` | `10` | 1000 |
| | `maxDepth` | `int` | `8` | 64 |
| `ml.KMeansOptions` | `maxIter` | `int` | `100` | 10000 |

```jennifer
use ml;
def X as list of list of float init [[0.0], [1.0], [5.0], [6.0]];
def y as list of int init [0, 0, 1, 1];
def f as ml.Model init ml.randomForestWith($X, $y, ml.ForestOptions{nTrees: 50, maxDepth: 4});
```

## Inspecting a fitted model

A model is opaque, but the parameters it learned can be read:

| Call | Applies to | Returns |
| ---- | ---------- | ------- |
| `ml.coefficients(model)` | linear / ridge / lasso / logistic | `list of float` (per feature), or `list of list of float` (per class) for multiclass logistic. |
| `ml.intercept(model)` | linear / ridge / lasso / logistic | `float`, or `list of float` per class for multiclass logistic. |
| `ml.centroids(model)` | kMeans | `list of list of float` (cluster centres). |
| `ml.components(model)` | pca | `list of list of float` (principal axes, one row per component). |
| `ml.explainedVariance(model)` | pca | `list of float` - the variance ratio each component captures (choose `nComponents` from these). |
| `ml.featureImportances(model)` | decisionTree / randomForest | `list of float` (Gini importances, summing to `1`). |

```jennifer
use ml;
use io;
def m as ml.Model init ml.linearRegression([[1.0, 1.0], [2.0, 1.0], [1.0, 2.0], [3.0, 2.0]], [6.0, 8.0, 9.0, 13.0]);
io.printf("y = %v . x + %v\n", ml.coefficients($m), ml.intercept($m));   # [2, 3], 1
```

## Applying a model

| Call | Returns | |
| ---- | ------- | - |
| `ml.predict(model, X)` | `list of int` / `list of float` | Predicted labels (classifier / cluster) or values (regressor) for each row of `X`. |
| `ml.transform(model, X)` | `list of list of float` | Transformed features (scalers, PCA). |
| `ml.predictProba(model, X)` | `list of float` | Positive-class probability, for `logisticRegression`. |

The random models (`kMeans`, `randomForest`, `trainTestSplit`) draw from
`math`'s shared random source, so `math.randSeed(n)` makes a run reproducible.
Training needs reproducibility, not secrecy, so `ml` never uses `crypto`'s
unseedable source.

The cost-driving hyper-parameters are bounded; a value above the ceiling is a
catchable error, as is a `polynomialFeatures` product that overflows to a
non-finite value.

| Parameter | Ceiling | Kind |
| --- | --- | --- |
| tree `maxDepth`, forest `nTrees`, `kMeans` `maxIter`, logistic `epochs` | see [Options](#options) | `"limit"` (`"runtime"` in the deprecated argument forms) |
| multiclass logistic classes | 100 | `"limit"` |
| `kFold` `nSamples * k` | 2097152 (524288 on `jennifer-tiny`) | `"limit"` |
| `pca` features, `confusionMatrix` distinct labels | 1024 (256 on `jennifer-tiny`): the `linalg` element limit on the d x d result | `"limit"` |
| `polynomialFeatures` degree | 8 | `"runtime"` |
| `polynomialFeatures` output width | 100000 | `"limit"` |
| `polynomialFeatures` cells (rows x columns) | 2097152 (524288 on `jennifer-tiny`) | `"limit"` |

`ml.lasso`, unlike ordinary least squares, accepts more features than rows
(p > n), its feature-selection use case. A very wide `polynomialFeatures` design
is allowed but does dense O(sweeps x features x rows) work; keep the expansion
modest.
`ml.logLoss` requires each probability in `[0, 1]`; an out-of-range value errors
instead of being clamped.

```jennifer
use ml;
use math;
use io;

math.randSeed(1);
def data as list of list of float init [[0.0, 0.0], [0.2, 0.1], [5.0, 5.0], [5.1, 4.8]];
def km as ml.Model init ml.kMeans($data, 2);
io.printf("clusters: %v\n", ml.predict($km, $data));   # e.g. [0, 0, 1, 1]

# Scale features, then reduce dimensionality.
def scaler as ml.Model init ml.standardScaler($data);
def scaled as list of list of float init ml.transform($scaler, $data);
```

## Deprecated

Each still works and is removed in 0.29.0; `jennifer lint` reports a call
(L301).

| Deprecated | Use instead |
| --- | --- |
| `ml.free(model)` | nothing: a model is a value the garbage collector frees (`ml.free` is a no-op) |
| `ml.logisticRegression(X, y, lr[, epochs])` | `ml.logisticRegressionWith(X, y, ml.LogisticOptions{learningRate: ..., epochs: ...})` |
| `ml.decisionTree(X, y, maxDepth)`, `ml.decisionTreeRegressor(...)` | `ml.decisionTreeWith(X, y, ml.TreeOptions{maxDepth: ...})`, `ml.decisionTreeRegressorWith(...)` |
| `ml.randomForest(X, y, nTrees[, maxDepth])`, `ml.randomForestRegressor(...)` | `ml.randomForestWith(X, y, ml.ForestOptions{nTrees: ..., maxDepth: ...})`, `ml.randomForestRegressorWith(...)` |
| `ml.kMeans(X, k, maxIter)` | `ml.kMeansWith(X, k, ml.KMeansOptions{maxIter: ...})` |

In the deprecated forms an argument must be positive (`lr > 0`, counts `>= 1`).

## Model selection

| Call | Returns | |
| ---- | ------- | - |
| `ml.trainTestSplit(X, y, testFraction)` | `ml.Split` | Shuffle and split into `{trainX, trainY, testX, testY}`; `testFraction` in `(0, 1)`. |
| `ml.kFold(nSamples, k)` | `list of ml.Fold` | `k` contiguous folds, each `{trainIdx, testIdx}` (index lists into your data). |
| `ml.polynomialFeatures(X, degree)` | `list of list of float` | Expand to all monomials up to `degree` (with a leading bias column); stateless, so the same call fits train and test. |

```jennifer
export def struct Split { trainX as list of list of float, trainY as list of float, testX as list of list of float, testY as list of float };
export def struct Fold  { trainIdx as list of int, testIdx as list of int };
```

## Metrics

Pure functions over label / value lists (no model). Classification metrics take
`(yTrue, yPred)`; `precision` / `recall` / `f1` take an optional third
positive-label argument (default `1`).

| Call | Meaning |
| ---- | ------- |
| `ml.accuracy(yTrue, yPred)` | Fraction of exact matches. |
| `ml.precision(yTrue, yPred [, positive])` | TP / (TP + FP) for the positive label. |
| `ml.recall(yTrue, yPred [, positive])` | TP / (TP + FN). |
| `ml.f1(yTrue, yPred [, positive])` | Harmonic mean of precision and recall. |
| `ml.confusionMatrix(yTrue, yPred)` | `ml.Confusion{labels, matrix}` (rows = true, columns = predicted). |
| `ml.rocAuc(yTrue, scores)` | Binary ROC-AUC from `0` / `1` labels and predicted scores (tie-aware). |
| `ml.logLoss(yTrue, probas)` | Binary cross-entropy of `0` / `1` labels against predicted probabilities. |
| `ml.rmse(yTrue, yPred)` / `ml.mse` / `ml.mae` | Root-mean-square / mean-square / mean-absolute error (regression). |
| `ml.r2(yTrue, yPred)` | Coefficient of determination R^2 (zero-variance targets error). |

```jennifer
use ml;
use io;
def yt as list of float init [1.0, 0.0, 1.0, 1.0, 0.0, 1.0];
def yp as list of float init [1.0, 0.0, 0.0, 1.0, 0.0, 1.0];
io.printf("accuracy %v, F1 %v\n", ml.accuracy($yt, $yp), ml.f1($yt, $yp, 1));
```

## Strictness and scope

Like the rest of the numeric stack, a degenerate input is a catchable error, not
a silent NaN: an empty / ragged matrix, mismatched `X` / `y` lengths, a singular
regression design (features collinear, also up to rounding), a diverged
logistic fit, or magnitudes that overflow the computation all raise a
positioned error.

`ml` targets modest tabular data: thousands of rows, not millions. Large-scale
training stays a native-tool job. Support-vector machines, gradient-boosted trees, density-based clustering
(DBSCAN), and model serialization are out of scope for this tier.

## See also

[`stats`](stats.md) (distributions + inference - the estimate / test companion to
`ml`'s fit / predict), [`linalg`](linalg.md) (linear algebra), and
[`math`](math.md). The module [index](index.md).

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Numbers and data: decimal, stats, linalg, ml, plot and dot](https://academy.jennifer-lang.dev/talks/mplx/numbers/) | regression with two features, k-means and k-nearest neighbours |
