# `ml` - classical machine learning

Enable with `use ml;`. Classical / predictive machine learning on tabular data -
the scikit-learn-lite core companion to [`stats`](stats.md) and
[`linalg`](linalg.md). Models follow a **fit / predict** shape: a fit function
(`ml.kMeans`, `ml.linearRegression`, ...) trains and returns an opaque `ml.Model`
handle, and `ml.predict` / `ml.transform` apply it. Pure Go stdlib, TinyGo-clean,
both binaries.

It is **not** a deep-learning framework: tensors, autodiff, and training deep
nets are native GPU / C++ territory a tree-walker cannot usefully replace, and
"run a pre-trained model" is already [`http`](../modules/http.md) / `os.run`.

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

Each returns an opaque `ml.Model` handle. A fitted model is immutable, so a
handle is safe to share across value-copies and `spawn`ed tasks (read-only).

| Fit call | Kind | Notes |
| -------- | ---- | ----- |
| `ml.linearRegression(X, y)` | regression | Ordinary least squares (normal equations). |
| `ml.ridge(X, y, alpha)` | regression | L2-regularized OLS; `alpha >= 0` shrinks the coefficients (not the intercept). |
| `ml.kNN(X, y, k)` | classifier | k-nearest-neighbours majority vote (Euclidean). |
| `ml.naiveBayes(X, y)` | classifier | Gaussian naive Bayes. |
| `ml.logisticRegression(X, y [, lr [, epochs]])` | classifier | Binary (labels `0` / `1`); gradient descent (`lr` default `0.1`, `epochs` `1000`). |
| `ml.decisionTree(X, y [, maxDepth])` | classifier | CART with Gini impurity (`maxDepth` default `8`). |
| `ml.randomForest(X, y [, nTrees [, maxDepth]])` | classifier | Bagged trees with per-split feature subsampling (`nTrees` `10`). |
| `ml.kMeans(X, k [, maxIter])` | clustering | Lloyd's algorithm, k-means++ seeding. |
| `ml.pca(X, nComponents)` | transform | Principal component analysis (covariance eigendecomposition). |
| `ml.standardScaler(X)` | transform | Fit a z-score scaler (per-feature mean / stddev). |
| `ml.minMaxScaler(X)` | transform | Fit a `[0, 1]` min-max scaler. |

## Applying a model

| Call | Returns | |
| ---- | ------- | - |
| `ml.predict(model, X)` | `list of int` / `list of float` | Predicted labels (classifier / cluster) or values (regressor) for each row of `X`. |
| `ml.transform(model, X)` | `list of list of float` | Transformed features (scalers, PCA). |
| `ml.predictProba(model, X)` | `list of float` | Positive-class probability, for `logisticRegression`. |
| `ml.free(model)` | `null` | Drop the model to free its memory early (a handle otherwise lives for the run). |

The random models (`kMeans`, `randomForest`, `trainTestSplit`) draw from
`math`'s shared random source, so `math.randSeed(n)` makes a run reproducible.
This is deliberate - reproducibility, not secrecy, is what training needs; a seed
is not a secret, so `ml` uses `math`'s seedable source, never `crypto`'s
unseedable one (which would make a split or clustering impossible to reproduce).

A fitted model lives in a per-run registry behind its handle until the program
ends. When you fit **many** models in a loop (e.g. a large cross-validation),
call `ml.free(model)` on the ones you are done with so the registry does not grow
unbounded. The cost-driving hyper-parameters are bounded (tree `maxDepth` <= 64,
forest `nTrees` <= 1000, `kMeans` `maxIter` <= 10000, logistic `epochs` <= 1e6,
`kFold` `nSamples * k` <= 1e8); a value above the ceiling is a catchable error.

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

## Model selection

| Call | Returns | |
| ---- | ------- | - |
| `ml.trainTestSplit(X, y, testFraction)` | `ml.Split` | Shuffle and split into `{trainX, trainY, testX, testY}`; `testFraction` in `(0, 1)`. |
| `ml.kFold(nSamples, k)` | `list of ml.Fold` | `k` contiguous folds, each `{trainIdx, testIdx}` (index lists into your data). |

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
| `ml.rmse(yTrue, yPred)` / `ml.mae` | Root-mean-square / mean-absolute error (regression). |
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
regression design (collinear features), a diverged logistic fit, or magnitudes
that overflow the computation all raise a positioned error.

`ml` targets the modest tabular data a tree-walker handles - native loops over
thousands of rows, not millions. Large-scale training stays a native-tool job.
Multiclass logistic regression, model serialization, and gradient-boosted trees
are out of scope for this tier.

## See also

[`stats`](stats.md) (distributions + inference - the estimate / test companion to
`ml`'s fit / predict), [`linalg`](linalg.md) (linear algebra), and
[`math`](math.md). The module [index](index.md).
