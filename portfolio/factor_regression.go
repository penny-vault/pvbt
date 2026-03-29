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

// OLSRegress runs ordinary least squares regression of the response vector
// (portfolio excess returns) against the factor columns in the DataFrame.
// The DataFrame must have asset.Factor as its sole asset; each metric is a
// factor. Returns alpha (intercept), per-factor betas, R-squared, and AIC.
func OLSRegress(response []float64, factors *data.DataFrame) (*FactorRegression, error) {
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

	// AICc (corrected AIC for finite samples):
	//   AIC  = n*ln(SS_res/n) + 2*p
	//   AICc = AIC + 2*p*(p+1)/(n-p-1)
	// where p = k+1 counts all parameters including the intercept.
	pp := float64(kk + 1)

	aic := float64(nn)*math.Log(ssRes/float64(nn)) + 2.0*pp
	if float64(nn)-pp-1 > 0 {
		aic += 2.0 * pp * (pp + 1) / (float64(nn) - pp - 1)
	}

	return &FactorRegression{
		Alpha:    alphaVal,
		RSquared: rSquared,
		AIC:      aic,
		Betas:    betas,
	}, nil
}

// FactorAnalysis regresses the portfolio's excess returns against the factor
// return series in the provided DataFrame. The DataFrame must use asset.Factor
// as its asset, with one metric per factor return series. The method aligns
// dates between the portfolio and factor time axes, using only overlapping
// dates for the regression.
func (a *Account) FactorAnalysis(factors *data.DataFrame) (*FactorRegression, error) {
	ctx := context.Background()

	excessDF := a.ExcessReturns(ctx, nil)
	if excessDF == nil {
		return nil, fmt.Errorf("no excess returns available: portfolio may lack price history or risk-free data")
	}

	excessMetrics := excessDF.MetricList()
	excessCol := excessDF.Column(portfolioAsset, excessMetrics[0])
	excessTimes := excessDF.Times()

	// Build a date -> index map for the factor DataFrame.
	factorTimes := factors.Times()

	factorDateIdx := make(map[time.Time]int, len(factorTimes))

	for ii, tt := range factorTimes {
		factorDateIdx[tt] = ii
	}

	// Align: walk the excess returns time axis and collect matching factor rows.
	metrics := factors.MetricList()

	var alignedResponse []float64

	alignedFactorCols := make([][]float64, len(metrics))

	for ii, tt := range excessTimes {
		fi, ok := factorDateIdx[tt]
		if !ok {
			continue
		}

		alignedResponse = append(alignedResponse, excessCol[ii])

		for jj, metric := range metrics {
			col := factors.Column(asset.Factor, metric)
			alignedFactorCols[jj] = append(alignedFactorCols[jj], col[fi])
		}
	}

	// Build an aligned factor DataFrame for OLSRegress.
	nn := len(alignedResponse)

	alignedTimes := make([]time.Time, nn)

	for ii := range nn {
		alignedTimes[ii] = time.Date(2000+ii, 1, 1, 0, 0, 0, 0, time.UTC) // dummy dates
	}

	alignedDF, err := data.NewDataFrame(
		alignedTimes,
		[]asset.Asset{asset.Factor},
		metrics,
		data.Daily,
		alignedFactorCols,
	)
	if err != nil {
		return nil, fmt.Errorf("building aligned factor DataFrame: %w", err)
	}

	return OLSRegress(alignedResponse, alignedDF)
}

// StepwiseFactorAnalysis uses forward stepwise AIC selection to find the best
// factor subset from the candidates. At each step it tries adding each
// remaining factor and keeps the one that produces the lowest AIC, stopping
// when no addition improves the model.
func (a *Account) StepwiseFactorAnalysis(factors *data.DataFrame) (*StepwiseResult, error) {
	ctx := context.Background()

	excessDF := a.ExcessReturns(ctx, nil)
	if excessDF == nil {
		return nil, fmt.Errorf("no excess returns available: portfolio may lack price history or risk-free data")
	}

	allMetrics := factors.MetricList()
	if len(allMetrics) == 0 {
		return nil, ErrNoFactors
	}

	// Align portfolio excess returns with factor dates.
	excessMetrics := excessDF.MetricList()
	excessCol := excessDF.Column(portfolioAsset, excessMetrics[0])
	excessTimes := excessDF.Times()

	factorTimes := factors.Times()

	factorDateIdx := make(map[time.Time]int, len(factorTimes))
	for ii, tt := range factorTimes {
		factorDateIdx[tt] = ii
	}

	var alignedResponse []float64

	alignedFactorCols := make(map[data.Metric][]float64)

	for ii, tt := range excessTimes {
		fi, ok := factorDateIdx[tt]
		if !ok {
			continue
		}

		alignedResponse = append(alignedResponse, excessCol[ii])

		for _, metric := range allMetrics {
			col := factors.Column(asset.Factor, metric)
			alignedFactorCols[metric] = append(alignedFactorCols[metric], col[fi])
		}
	}

	nn := len(alignedResponse)

	alignedTimes := make([]time.Time, nn)
	for ii := range nn {
		alignedTimes[ii] = time.Date(2000+ii, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	// Track which factors are selected and which remain.
	selected := make([]data.Metric, 0, len(allMetrics))

	remaining := make(map[data.Metric]bool, len(allMetrics))
	for _, metric := range allMetrics {
		remaining[metric] = true
	}

	var steps []FactorRegression

	bestAIC := math.Inf(1)

	for len(remaining) > 0 {
		var bestCandidate data.Metric

		var bestResult *FactorRegression

		candidateAIC := math.Inf(1)

		// Try adding each remaining factor.
		for metric := range remaining {
			trial := make([]data.Metric, len(selected)+1)
			copy(trial, selected)
			trial[len(selected)] = metric

			trialCols := make([][]float64, len(trial))
			for jj, mm := range trial {
				trialCols[jj] = alignedFactorCols[mm]
			}

			trialDF, err := data.NewDataFrame(
				alignedTimes,
				[]asset.Asset{asset.Factor},
				trial,
				data.Daily,
				trialCols,
			)
			if err != nil {
				return nil, fmt.Errorf("building trial DataFrame: %w", err)
			}

			result, err := OLSRegress(alignedResponse, trialDF)
			if err != nil {
				continue
			}

			if result.AIC < candidateAIC {
				candidateAIC = result.AIC
				bestCandidate = metric
				bestResult = result
			}
		}

		// Stop if no candidate improves AICc by at least 2 (Burnham & Anderson
		// threshold: models within 2 AICc units are essentially equivalent).
		if candidateAIC >= bestAIC-2.0 {
			break
		}

		bestAIC = candidateAIC

		selected = append(selected, bestCandidate)
		delete(remaining, bestCandidate)

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
