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
)

// RiskParityFast builds a PortfolioPlan using a single-pass approximation of
// equal risk contribution. It starts with inverse volatility weights and
// adjusts for pairwise correlations via the naive risk parity formula:
//
//	w_i = (1/sigma_i) / (C @ w)_i, then normalize.
//
// A zero-value lookback defaults to 60 calendar days. Falls back to equal
// weight when all volatilities are zero or the covariance matrix degenerates.
func RiskParityFast(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error) {
	if !HasSelectedColumn(df) {
		return nil, ErrMissingSelected("RiskParityFast")
	}

	lookback = defaultLookback(lookback)
	times := df.Times()
	assets := df.AssetList()

	priceDF, err := ensureMetric(ctx, df, assets, lookback, data.AdjClose)
	if err != nil {
		return nil, fmt.Errorf("RiskParityFast: %w", err)
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

		members, fallback := riskParityFastWeights(returns, chosen)
		if fallback {
			members = equalWeightMembers(chosen)
		}

		plan[timeIdx] = Allocation{Date: timestamp, Members: members}
	}

	return plan, nil
}

// riskParityFastWeights computes the single-pass naive risk parity weights.
// Returns the weight map and a boolean indicating whether fallback to equal
// weight is needed.
func riskParityFastWeights(returns *data.DataFrame, chosen []asset.Asset) (map[asset.Asset]float64, bool) {
	numAssets := len(chosen)

	// Compute volatilities using NaN-safe helper (Pct produces NaN in first row).
	vols := make([]float64, numAssets)
	allZero := true

	for idx, currentAsset := range chosen {
		vol := nanSafeStd(returns, currentAsset, data.AdjClose)

		if vol <= 0 {
			vols[idx] = 0
		} else {
			vols[idx] = vol
			allZero = false
		}
	}

	if allZero {
		return nil, true
	}

	// Start with inverse volatility weights.
	weights := make([]float64, numAssets)
	for idx := range numAssets {
		if vols[idx] > 0 {
			weights[idx] = 1.0 / vols[idx]
		}
	}

	// Compute covariance matrix (as flat NxN).
	covMatrix := computeCovMatrix(returns, chosen)
	if covMatrix == nil {
		return nil, true
	}

	// Compute marginal risk contribution: (C @ w)_i.
	marginalRisk := make([]float64, numAssets)

	for idx := range numAssets {
		for jdx := range numAssets {
			marginalRisk[idx] += covMatrix[idx*numAssets+jdx] * weights[jdx]
		}
	}

	// Adjust: w_i = w_i / marginalRisk_i, then normalize.
	sumWeights := 0.0

	for idx := range numAssets {
		if marginalRisk[idx] > 0 {
			weights[idx] /= marginalRisk[idx]
		} else {
			weights[idx] = 0
		}

		sumWeights += weights[idx]
	}

	if sumWeights == 0 {
		return nil, true
	}

	members := make(map[asset.Asset]float64, numAssets)

	for idx, currentAsset := range chosen {
		weight := weights[idx] / sumWeights
		if weight > 0 {
			members[currentAsset] = weight
		}
	}

	return members, false
}

// computeCovMatrix computes a flat NxN covariance matrix from return data.
// Returns nil if computation fails.
func computeCovMatrix(returns *data.DataFrame, chosen []asset.Asset) []float64 {
	numAssets := len(chosen)
	covMatrix := make([]float64, numAssets*numAssets)
	returnTimes := returns.Times()

	if len(returnTimes) < 2 {
		return nil
	}

	// Extract return series for each asset.
	series := make([][]float64, numAssets)

	for idx, currentAsset := range chosen {
		vals := make([]float64, len(returnTimes))

		for tdx, timestamp := range returnTimes {
			val := returns.ValueAt(currentAsset, data.AdjClose, timestamp)
			if math.IsNaN(val) {
				val = 0
			}

			vals[tdx] = val
		}

		series[idx] = vals
	}

	// Compute sample covariance.
	for idx := range numAssets {
		for jdx := range numAssets {
			covMatrix[idx*numAssets+jdx] = sampleCovariance(series[idx], series[jdx])
		}
	}

	return covMatrix
}

// sampleCovariance computes sample covariance between two equal-length slices
// using N-1 denominator.
func sampleCovariance(seriesA, seriesB []float64) float64 {
	numObs := len(seriesA)
	if numObs < 2 {
		return 0
	}

	meanA := 0.0
	meanB := 0.0

	for idx := range numObs {
		meanA += seriesA[idx]
		meanB += seriesB[idx]
	}

	meanA /= float64(numObs)
	meanB /= float64(numObs)

	cov := 0.0
	for idx := range numObs {
		cov += (seriesA[idx] - meanA) * (seriesB[idx] - meanB)
	}

	return cov / float64(numObs-1)
}
