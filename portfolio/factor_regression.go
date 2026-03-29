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
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"gonum.org/v1/gonum/mat"
)

var (
	ErrTooFewObservations = errors.New("fewer than 12 overlapping observations")
	ErrNoFactors          = errors.New("factor DataFrame contains no metrics")
)

// FactorRegression holds the result of an OLS regression of portfolio excess
// returns against one or more factor return series.
type FactorRegression struct {
	Alpha    float64            // intercept: return not explained by factors
	RSquared float64            // fraction of variance explained by the model
	AIC      float64            // Akaike Information Criterion
	Betas    map[string]float64 // factor metric name -> coefficient
}

// StepwiseResult holds the output of forward stepwise factor selection.
type StepwiseResult struct {
	Best  FactorRegression   // the final selected model
	Steps []FactorRegression // one per round (each adds one factor)
}

// olsRegress runs ordinary least squares regression of the response vector
// (portfolio excess returns) against the factor columns in the DataFrame.
// The DataFrame must have asset.Factor as its sole asset; each metric is a
// factor. Returns alpha (intercept), per-factor betas, R-squared, and AIC.
func olsRegress(response []float64, factors *data.DataFrame) (*FactorRegression, error) {
	metrics := factors.MetricList()
	if len(metrics) == 0 {
		return nil, ErrNoFactors
	}

	nn := min(len(response), factors.Len())
	if nn < 12 {
		return nil, ErrTooFewObservations
	}

	kk := len(metrics) // number of factors
	response = response[:nn]

	// Build the design matrix X with an intercept column (first column = 1).
	// Layout: X is nn x (kk+1), stored row-major for gonum.
	ncols := kk + 1

	xData := make([]float64, nn*ncols)
	for ii := range nn {
		xData[ii*ncols] = 1.0 // intercept column
	}

	for jj, metric := range metrics {
		col := factors.Column(asset.Factor, metric)
		for ii := range nn {
			xData[ii*ncols+(jj+1)] = col[ii]
		}
	}

	xMat := mat.NewDense(nn, kk+1, xData)
	yVec := mat.NewVecDense(nn, response)

	// Solve X'X * beta = X'y via QR decomposition.
	var qr mat.QR
	qr.Factorize(xMat)

	var betaVec mat.VecDense
	if err := qr.SolveVecTo(&betaVec, false, yVec); err != nil {
		return nil, fmt.Errorf("OLS solve failed: %w", err)
	}

	// Extract coefficients.
	alphaVal := betaVec.AtVec(0)

	betas := make(map[string]float64, kk)
	for jj, metric := range metrics {
		betas[string(metric)] = betaVec.AtVec(jj + 1)
	}

	// Compute R-squared = 1 - SS_res / SS_tot.
	yMean := 0.0
	for ii := range nn {
		yMean += response[ii]
	}

	yMean /= float64(nn)

	ssTot := 0.0
	ssRes := 0.0

	var predicted mat.VecDense
	predicted.MulVec(xMat, &betaVec)

	for ii := range nn {
		residual := response[ii] - predicted.AtVec(ii)
		ssRes += residual * residual
		deviation := response[ii] - yMean
		ssTot += deviation * deviation
	}

	rSquared := 0.0
	if ssTot > 0 {
		rSquared = 1.0 - ssRes/ssTot
	}

	// AIC = n*ln(SS_res/n) + 2*(k+1)
	// where k+1 counts all parameters including the intercept.
	aic := float64(nn)*math.Log(ssRes/float64(nn)) + 2.0*float64(kk+1)

	return &FactorRegression{
		Alpha:    alphaVal,
		RSquared: rSquared,
		AIC:      aic,
		Betas:    betas,
	}, nil
}

// alignedFactors holds the result of aligning portfolio excess returns with
// factor return series on overlapping dates.
type alignedFactors struct {
	response   []float64                 // aligned portfolio excess returns
	factorCols map[data.Metric][]float64 // metric -> aligned factor returns
	times      []time.Time               // dummy time axis for DataFrame construction
}

// alignWithFactors extracts portfolio excess returns from the Account, aligns
// them with the factor DataFrame on overlapping dates, and filters out NaN
// values (e.g. the first row produced by Pct()).
func (a *Account) alignWithFactors(factors *data.DataFrame) (*alignedFactors, error) {
	ctx := context.Background()

	excessDF := a.ExcessReturns(ctx, nil)
	if excessDF == nil {
		return nil, fmt.Errorf("no excess returns available: portfolio may lack price history or risk-free data")
	}

	excessMetrics := excessDF.MetricList()
	excessCol := excessDF.Column(portfolioAsset, excessMetrics[0])
	excessTimes := excessDF.Times()

	factorTimes := factors.Times()

	factorDateIdx := make(map[time.Time]int, len(factorTimes))
	for ii, tt := range factorTimes {
		factorDateIdx[tt] = ii
	}

	allMetrics := factors.MetricList()

	// Pre-fetch factor columns to avoid repeated lookups.
	rawFactorCols := make(map[data.Metric][]float64, len(allMetrics))
	for _, metric := range allMetrics {
		rawFactorCols[metric] = factors.Column(asset.Factor, metric)
	}

	var response []float64

	factorCols := make(map[data.Metric][]float64, len(allMetrics))

	for ii, tt := range excessTimes {
		fi, ok := factorDateIdx[tt]
		if !ok {
			continue
		}

		if math.IsNaN(excessCol[ii]) {
			continue
		}

		// Skip rows where any factor value is NaN.
		hasNaN := false

		for _, metric := range allMetrics {
			if math.IsNaN(rawFactorCols[metric][fi]) {
				hasNaN = true

				break
			}
		}

		if hasNaN {
			continue
		}

		response = append(response, excessCol[ii])
		for _, metric := range allMetrics {
			factorCols[metric] = append(factorCols[metric], rawFactorCols[metric][fi])
		}
	}

	nn := len(response)

	dummyTimes := make([]time.Time, nn)
	for ii := range nn {
		dummyTimes[ii] = time.Date(2000+ii, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	return &alignedFactors{
		response:   response,
		factorCols: factorCols,
		times:      dummyTimes,
	}, nil
}

// FactorAnalysis regresses the portfolio's excess returns against the factor
// return series in the provided DataFrame. The DataFrame must use asset.Factor
// as its asset, with one metric per factor return series. The method aligns
// dates between the portfolio and factor time axes, using only overlapping
// dates for the regression.
func (a *Account) FactorAnalysis(factors *data.DataFrame) (*FactorRegression, error) {
	aligned, err := a.alignWithFactors(factors)
	if err != nil {
		return nil, err
	}

	metrics := factors.MetricList()

	cols := make([][]float64, len(metrics))
	for jj, metric := range metrics {
		cols[jj] = aligned.factorCols[metric]
	}

	alignedDF, err := data.NewDataFrame(
		aligned.times,
		[]asset.Asset{asset.Factor},
		metrics,
		data.Daily,
		cols,
	)
	if err != nil {
		return nil, fmt.Errorf("building aligned factor DataFrame: %w", err)
	}

	return olsRegress(aligned.response, alignedDF)
}

// StepwiseFactorAnalysis uses forward stepwise AIC selection to find the best
// factor subset from the candidates. At each step it tries adding each
// remaining factor and keeps the one that produces the lowest AIC, stopping
// when no addition improves the model.
func (a *Account) StepwiseFactorAnalysis(factors *data.DataFrame) (*StepwiseResult, error) {
	allMetrics := factors.MetricList()
	if len(allMetrics) == 0 {
		return nil, ErrNoFactors
	}

	aligned, err := a.alignWithFactors(factors)
	if err != nil {
		return nil, err
	}

	// Track which factors are selected and which remain.
	// Sort once for deterministic tie-breaking (slices.Delete preserves order).
	selected := make([]data.Metric, 0, len(allMetrics))
	remaining := make([]data.Metric, len(allMetrics))
	copy(remaining, allMetrics)
	slices.Sort(remaining)

	var steps []FactorRegression

	bestAIC := math.Inf(1)

	for len(remaining) > 0 {
		var (
			bestCandidate data.Metric
			bestResult    *FactorRegression
			bestIdx       int
		)

		candidateAIC := math.Inf(1)

		// Try adding each remaining factor.
		for idx, metric := range remaining {
			trial := make([]data.Metric, len(selected)+1)
			copy(trial, selected)
			trial[len(selected)] = metric

			trialCols := make([][]float64, len(trial))
			for jj, mm := range trial {
				trialCols[jj] = aligned.factorCols[mm]
			}

			trialDF, dfErr := data.NewDataFrame(
				aligned.times,
				[]asset.Asset{asset.Factor},
				trial,
				data.Daily,
				trialCols,
			)
			if dfErr != nil {
				return nil, fmt.Errorf("building trial DataFrame: %w", dfErr)
			}

			result, regErr := olsRegress(aligned.response, trialDF)
			if regErr != nil {
				continue
			}

			if result.AIC < candidateAIC {
				candidateAIC = result.AIC
				bestCandidate = metric
				bestResult = result
				bestIdx = idx
			}
		}

		// Stop if no candidate improves AIC.
		if candidateAIC >= bestAIC {
			break
		}

		bestAIC = candidateAIC

		selected = append(selected, bestCandidate)
		remaining = slices.Delete(remaining, bestIdx, bestIdx+1)

		steps = append(steps, *bestResult)
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("no factor improved the model")
	}

	return &StepwiseResult{
		Best:  steps[len(steps)-1],
		Steps: steps,
	}, nil
}
