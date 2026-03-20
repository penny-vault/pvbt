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

package portfolio

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/rs/zerolog"
)

const (
	riskParityMaxIter   = 1000
	riskParityTolerance = 1e-10
	riskParityStepSize  = 0.5
)

// RiskParity builds a PortfolioPlan using iterative optimization to equalize
// each asset's contribution to total portfolio risk. Uses Newton's method
// with simplex projection. A zero-value lookback defaults to 60 calendar days.
//
// Returns the best result found after riskParityMaxIter iterations. Logs a
// warning via zerolog if convergence is not reached. Falls back to equal
// weight when the covariance matrix degenerates.
func RiskParity(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error) {
	if !HasSelectedColumn(df) {
		return nil, ErrMissingSelected("RiskParity")
	}

	lookback = defaultLookback(lookback)
	times := df.Times()
	assets := df.AssetList()
	log := zerolog.Ctx(ctx)

	priceDF, err := ensureMetric(ctx, df, assets, lookback, data.AdjClose)
	if err != nil {
		return nil, fmt.Errorf("RiskParity: %w", err)
	}

	plan := make(PortfolioPlan, len(times))

	for timeIdx, timestamp := range times {
		chosen := CollectSelected(df, timestamp)

		if len(chosen) <= 1 {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		window := priceDF.Between(lookback.Before(timestamp), timestamp)
		returns := window.Pct()

		covMatrix := computeCovMatrix(returns, chosen)
		if covMatrix == nil {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		weights, converged := solveRiskParity(covMatrix, len(chosen))
		if !converged {
			log.Warn().
				Time("date", timestamp).
				Int("maxIter", riskParityMaxIter).
				Msg("RiskParity: did not converge, using best result")
		}

		if weights == nil {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		members := make(map[asset.Asset]float64, len(chosen))

		for idx, currentAsset := range chosen {
			if weights[idx] > 0 {
				members[currentAsset] = weights[idx]
			}
		}

		if len(members) == 0 {
			members = equalWeightMembers(chosen)
		}

		plan[timeIdx] = Allocation{Date: timestamp, Members: members}
	}

	return plan, nil
}

// solveRiskParity runs Newton's method to find weights where each asset
// contributes equally to total portfolio risk. Returns the weight vector and
// whether convergence was achieved.
func solveRiskParity(covMatrix []float64, numAssets int) ([]float64, bool) {
	targetRC := 1.0 / float64(numAssets)

	// Initialize to equal weight.
	weights := make([]float64, numAssets)
	for idx := range numAssets {
		weights[idx] = targetRC
	}

	bestWeights := make([]float64, numAssets)
	copy(bestWeights, weights)

	bestError := math.Inf(1)

	for range riskParityMaxIter {
		// Compute portfolio variance: w^T C w.
		portVar := quadForm(covMatrix, weights, numAssets)
		if portVar <= 0 {
			return nil, false
		}

		// Compute risk contributions: rc_i = w_i * (C @ w)_i / portVar.
		marginal := matVecMul(covMatrix, weights, numAssets)
		riskContrib := make([]float64, numAssets)
		maxErr := 0.0

		for idx := range numAssets {
			riskContrib[idx] = weights[idx] * marginal[idx] / portVar
			errVal := math.Abs(riskContrib[idx] - targetRC)

			if errVal > maxErr {
				maxErr = errVal
			}
		}

		// Track best solution.
		if maxErr < bestError {
			bestError = maxErr

			copy(bestWeights, weights)
		}

		if maxErr < riskParityTolerance {
			return weights, true
		}

		// Gradient step: move weights toward equal risk contribution.
		// Use the formula: w_i_new = w_i * (targetRC / rc_i)^stepSize.
		newWeights := make([]float64, numAssets)
		sumNew := 0.0

		for idx := range numAssets {
			if riskContrib[idx] > 0 {
				ratio := targetRC / riskContrib[idx]
				newWeights[idx] = weights[idx] * math.Pow(ratio, riskParityStepSize)
			} else {
				newWeights[idx] = weights[idx]
			}

			if newWeights[idx] < 0 {
				newWeights[idx] = 0
			}

			sumNew += newWeights[idx]
		}

		// Normalize to simplex.
		if sumNew <= 0 {
			return nil, false
		}

		for idx := range numAssets {
			weights[idx] = newWeights[idx] / sumNew
		}
	}

	// Did not converge; return best found.
	return bestWeights, false
}

// quadForm computes w^T M w for a flat NxN matrix M.
func quadForm(matrix, vec []float64, size int) float64 {
	result := 0.0

	for idx := range size {
		for jdx := range size {
			result += vec[idx] * matrix[idx*size+jdx] * vec[jdx]
		}
	}

	return result
}

// matVecMul computes M @ v for a flat NxN matrix M.
func matVecMul(matrix, vec []float64, size int) []float64 {
	result := make([]float64, size)

	for idx := range size {
		for jdx := range size {
			result[idx] += matrix[idx*size+jdx] * vec[jdx]
		}
	}

	return result
}
