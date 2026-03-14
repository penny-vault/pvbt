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
	"math"

	"github.com/penny-vault/pvbt/data"
	"gonum.org/v1/gonum/stat"
)

type probabilisticSharpe struct{}

func (probabilisticSharpe) Name() string { return "ProbabilisticSharpe" }

func (probabilisticSharpe) Description() string {
	return "Probability that the true Sharpe ratio exceeds zero, accounting for skewness and kurtosis of returns. Based on Bailey and Lopez de Prado (2012). Values near 1.0 indicate high confidence the strategy has positive risk-adjusted returns. Values near 0.5 indicate no statistical evidence of skill."
}

func (probabilisticSharpe) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}
	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}
	perfDF := pd.Window(window)
	returns := perfDF.Pct().Drop(math.NaN())
	er := returns.Metrics(data.PortfolioEquity).Sub(returns, data.PortfolioRiskFree)
	if er.Len() == 0 {
		return 0, nil
	}
	erCol := er.Column(portfolioAsset, data.PortfolioEquity)

	n := len(erCol)
	if n < 4 {
		return 0, nil
	}

	// Compute sample Sharpe (not annualized -- PSR works on per-period).
	sd := stat.StdDev(erCol, nil)
	if sd == 0 {
		return 0, nil
	}
	sr := stat.Mean(erCol, nil) / sd

	// Compute skewness and kurtosis of excess returns.
	m := stat.Mean(erCol, nil)
	var sum3, sum4 float64
	for _, v := range erCol {
		d := v - m
		d2 := d * d
		sum3 += d2 * d
		sum4 += d2 * d2
	}
	nf := float64(n)
	skew := (sum3 / nf) / (sd * sd * sd)
	kurt := (sum4/nf)/(sd*sd*sd*sd) - 3

	// Standard error of the Sharpe ratio (Lo, 2002 / Bailey & Lopez de Prado).
	// se(SR) = sqrt((1 - skew*SR + (kurt/4)*SR^2) / (n - 1))
	sr2 := sr * sr
	inner := (1 - skew*sr + (kurt/4)*sr2) / (nf - 1)
	if inner <= 0 {
		return 0, nil
	}
	se := math.Sqrt(inner)

	if se == 0 {
		return 0, nil
	}

	// PSR = Phi(SR / se) where Phi is the standard normal CDF.
	// Benchmark Sharpe is 0.
	z := sr / se
	return normalCDF(z), nil
}

func (probabilisticSharpe) ComputeSeries(a *Account, window *Period) ([]float64, error) {
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
