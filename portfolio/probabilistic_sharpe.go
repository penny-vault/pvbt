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
	"math"

	"github.com/penny-vault/pvbt/data"
	"gonum.org/v1/gonum/stat"
)

type probabilisticSharpe struct{}

func (probabilisticSharpe) Name() string { return "ProbabilisticSharpe" }

func (probabilisticSharpe) Description() string {
	return "Probability that the true Sharpe ratio exceeds zero, accounting for skewness and kurtosis of returns. Based on Bailey and Lopez de Prado (2012). Values near 1.0 indicate high confidence the strategy has positive risk-adjusted returns. Values near 0.5 indicate no statistical evidence of skill."
}

func (probabilisticSharpe) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	pd := stats.PerfDataView(ctx)
	if pd == nil {
		return 0, nil
	}

	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}

	df := stats.ExcessReturns(ctx, window)
	if df == nil {
		return 0, nil
	}

	erCol := removeNaN(df.Column(portfolioAsset, data.PortfolioEquity))

	count := len(erCol)
	if count < 4 {
		return 0, nil
	}

	// Compute sample Sharpe (not annualized -- PSR works on per-period).
	stdDev := stat.StdDev(erCol, nil)
	if stdDev == 0 {
		return 0, nil
	}

	sharpeRatio := stat.Mean(erCol, nil) / stdDev

	// Compute skewness and kurtosis of excess returns.
	m := stat.Mean(erCol, nil)

	var sum3, sum4 float64

	for _, v := range erCol {
		d := v - m
		d2 := d * d
		sum3 += d2 * d
		sum4 += d2 * d2
	}

	numPeriods := float64(count)
	skew := (sum3 / numPeriods) / (stdDev * stdDev * stdDev)
	kurt := (sum4/numPeriods)/(stdDev*stdDev*stdDev*stdDev) - 3

	// Standard error of the Sharpe ratio (Lo, 2002 / Bailey & Lopez de Prado).
	// se(SR) = sqrt((1 - skew*SR + (kurt/4)*SR^2) / (n - 1))
	sr2 := sharpeRatio * sharpeRatio

	inner := (1 - skew*sharpeRatio + (kurt/4)*sr2) / (numPeriods - 1)
	if inner <= 0 {
		return 0, nil
	}

	stdErr := math.Sqrt(inner)

	if stdErr == 0 {
		return 0, nil
	}

	// PSR = Phi(SR / se) where Phi is the standard normal CDF.
	// Benchmark Sharpe is 0.
	z := sharpeRatio / stdErr

	return normalCDF(z), nil
}

func (probabilisticSharpe) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// normalCDF approximates the standard normal cumulative distribution function.
func normalCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// ProbabilisticSharpe is the probability that the true Sharpe ratio
// exceeds zero, accounting for non-normality of returns. Based on
// Bailey and Lopez de Prado (2012). Values close to 1.0 indicate
// statistical confidence in positive risk-adjusted returns.
var ProbabilisticSharpe PerformanceMetric = probabilisticSharpe{}
