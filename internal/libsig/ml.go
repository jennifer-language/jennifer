// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("ml", `
# X is a list of rows of int/float features, y a list of int/float
linearRegression(list of list of any, list of any) ml.Model
ridge(list of list of any, list of any, int|float) ml.Model
lasso(list of list of any, list of any, int|float) ml.Model
kNN(list of list of any, list of any, int) ml.Model
kNNRegressor(list of list of any, list of any, int) ml.Model
naiveBayes(list of list of any, list of any) ml.Model
logisticRegression(list of list of any, list of any, int|float?, int?) ml.Model
kMeans(list of list of any, int, int?) ml.Model
pca(list of list of any, int) ml.Model
decisionTree(list of list of any, list of any, int?) ml.Model
decisionTreeRegressor(list of list of any, list of any, int?) ml.Model
randomForest(list of list of any, list of any, int?, int?) ml.Model
randomForestRegressor(list of list of any, list of any, int?, int?) ml.Model
standardScaler(list of list of any) ml.Model
minMaxScaler(list of list of any) ml.Model
predict(ml.Model, list of list of any) list of int|list of float
transform(ml.Model, list of list of any) list of list of float
predictProba(ml.Model, list of list of any) list of float
free(ml.Model) null
coefficients(ml.Model) list of float|list of list of float
intercept(ml.Model) float|list of float
centroids(ml.Model) list of list of float
components(ml.Model) list of list of float
explainedVariance(ml.Model) list of float
featureImportances(ml.Model) list of float
trainTestSplit(list of list of any, list of any, int|float) ml.Split
kFold(int, int) list of ml.Fold
polynomialFeatures(list of list of any, int) list of list of float
accuracy(list of any, list of any) float
precision(list of any, list of any, int|float?) float
recall(list of any, list of any, int|float?) float
f1(list of any, list of any, int|float?) float
confusionMatrix(list of any, list of any) ml.Confusion
rocAuc(list of any, list of any) float
rmse(list of any, list of any) float
mse(list of any, list of any) float
mae(list of any, list of any) float
r2(list of any, list of any) float
logLoss(list of any, list of any) float
`)
}
