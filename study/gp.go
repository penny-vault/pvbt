// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package study

import (
	"fmt"
	"math"
)

// gaussianProcess implements a Gaussian process with a squared exponential (RBF) kernel.
// It is used as the surrogate model for Bayesian optimization.
type gaussianProcess struct {
	xTrain    [][]float64 // N x D normalized training inputs
	yTrain    []float64   // N observed scores
	variance  float64
	length    float64
	jitter    float64
	choleskyL [][]float64 // lower triangular Cholesky factor
	alpha     []float64   // L^T \ (L \ y)
}

// newGaussianProcess creates a GP with the given kernel hyperparameters.
func newGaussianProcess(variance, length, jitter float64) *gaussianProcess {
	return &gaussianProcess{
		variance: variance,
		length:   length,
		jitter:   jitter,
	}
}

// Fit trains the GP on the given data points.
func (gp *gaussianProcess) Fit(xTrain [][]float64, yTrain []float64) error {
	nn := len(xTrain)
	if nn == 0 {
		return fmt.Errorf("gaussian process fit: no training data provided")
	}

	if nn != len(yTrain) {
		return fmt.Errorf("gaussian process fit: xTrain length %d != yTrain length %d", nn, len(yTrain))
	}

	gp.xTrain = xTrain
	gp.yTrain = yTrain

	// Build the covariance matrix K + jitter*I.
	kMatrix := make([][]float64, nn)
	for ii := range nn {
		kMatrix[ii] = make([]float64, nn)
		for jj := range nn {
			kMatrix[ii][jj] = gp.kernel(xTrain[ii], xTrain[jj])
		}

		kMatrix[ii][ii] += gp.jitter
	}

	// Compute Cholesky decomposition K = L * L^T.
	lower, err := choleskyDecompose(kMatrix)
	if err != nil {
		return fmt.Errorf("gaussian process fit: cholesky decomposition failed: %w", err)
	}

	gp.choleskyL = lower

	// Solve L * L^T * alpha = y  =>  alpha = L^T \ (L \ y).
	solved := choleskySolve(lower, yTrain)
	gp.alpha = solved

	return nil
}

// Predict returns the posterior mean and standard deviation at a test point.
func (gp *gaussianProcess) Predict(xTest []float64) (mean, stddev float64) {
	nn := len(gp.xTrain)

	// k* = kernel between test point and each training point.
	kStar := make([]float64, nn)
	for ii := range nn {
		kStar[ii] = gp.kernel(xTest, gp.xTrain[ii])
	}

	// mean = k*^T * alpha
	for ii := range nn {
		mean += kStar[ii] * gp.alpha[ii]
	}

	// v = L \ k*
	vv := forwardSolve(gp.choleskyL, kStar)

	// variance = k(x*, x*) - v^T * v
	kSelf := gp.kernel(xTest, xTest)

	varSum := 0.0
	for ii := range nn {
		varSum += vv[ii] * vv[ii]
	}

	variance := kSelf + gp.jitter - varSum
	if variance < 0 {
		variance = 0
	}

	return mean, math.Sqrt(variance)
}

// kernel computes the squared exponential (RBF) kernel between two points.
// k(x, x') = variance * exp(-||x - x'||^2 / (2 * lengthScale^2))
func (gp *gaussianProcess) kernel(xa, xb []float64) float64 {
	sqDist := 0.0

	for ii := range xa {
		diff := xa[ii] - xb[ii]
		sqDist += diff * diff
	}

	return gp.variance * math.Exp(-sqDist/(2*gp.length*gp.length))
}

// choleskyDecompose computes the lower triangular Cholesky factor L such that A = L * L^T.
func choleskyDecompose(matrix [][]float64) ([][]float64, error) {
	nn := len(matrix)
	lower := make([][]float64, nn)

	for ii := range nn {
		lower[ii] = make([]float64, nn)
	}

	for ii := range nn {
		for jj := range ii + 1 {
			sum := 0.0
			for kk := range jj {
				sum += lower[ii][kk] * lower[jj][kk]
			}

			if ii == jj {
				val := matrix[ii][ii] - sum
				if val <= 0 {
					return nil, fmt.Errorf("matrix is not positive definite at index %d (value=%g)", ii, val)
				}

				lower[ii][jj] = math.Sqrt(val)
			} else {
				lower[ii][jj] = (matrix[ii][jj] - sum) / lower[jj][jj]
			}
		}
	}

	return lower, nil
}

// choleskySolve solves L * L^T * x = b given the lower triangular factor L.
func choleskySolve(lower [][]float64, bb []float64) []float64 {
	// Forward substitution: L * y = b
	yy := forwardSolve(lower, bb)

	// Back substitution: L^T * x = y
	nn := len(bb)
	xx := make([]float64, nn)

	for ii := nn - 1; ii >= 0; ii-- {
		sum := 0.0
		for jj := ii + 1; jj < nn; jj++ {
			sum += lower[jj][ii] * xx[jj] // L^T[ii][jj] = L[jj][ii]
		}

		xx[ii] = (yy[ii] - sum) / lower[ii][ii]
	}

	return xx
}

// forwardSolve solves L * x = b via forward substitution where L is lower triangular.
func forwardSolve(lower [][]float64, bb []float64) []float64 {
	nn := len(bb)
	xx := make([]float64, nn)

	for ii := range nn {
		sum := 0.0
		for jj := range ii {
			sum += lower[ii][jj] * xx[jj]
		}

		xx[ii] = (bb[ii] - sum) / lower[ii][ii]
	}

	return xx
}

// standardNormalPDF returns the standard normal probability density function at z.
func standardNormalPDF(zz float64) float64 {
	return math.Exp(-0.5*zz*zz) / math.Sqrt(2*math.Pi)
}

// standardNormalCDF returns the standard normal cumulative distribution function at z.
func standardNormalCDF(zz float64) float64 {
	return 0.5 * (1 + math.Erf(zz/math.Sqrt2))
}

// expectedImprovement computes the EI acquisition function value.
// EI(x) = (mu - bestScore) * Phi(Z) + sigma * phi(Z)
// where Z = (mu - bestScore) / sigma.
func expectedImprovement(mu, sigma, bestScore float64) float64 {
	if sigma < 1e-12 {
		if mu > bestScore {
			return mu - bestScore
		}

		return 0
	}

	zz := (mu - bestScore) / sigma

	return (mu-bestScore)*standardNormalCDF(zz) + sigma*standardNormalPDF(zz)
}
