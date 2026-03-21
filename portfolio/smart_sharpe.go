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

type smartSharpe struct{}

func (smartSharpe) Name() string { return "SmartSharpe" }

func (smartSharpe) Description() string {
	return "Sharpe ratio penalized for autocorrelation in returns. When returns are serially correlated, the standard Sharpe overstates risk-adjusted performance. The penalty factor is 1 + 2*sum(autocorrelations), following Lo (2002). Lower than Sharpe when returns exhibit positive autocorrelation."
}

func (smartSharpe) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
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
	if len(erCol) < 2 {
		return 0, nil
	}

	stdDev := stat.StdDev(erCol, nil)
	if stdDev == 0 || math.IsNaN(stdDev) {
		return 0, nil
	}

	af := annualizationFactor(df.Times())
	rawSharpe := stat.Mean(erCol, nil) / stdDev * math.Sqrt(af)

	penalty := autocorrelationPenalty(erCol)
	if penalty == 0 {
		return 0, nil
	}

	return rawSharpe / penalty, nil
}

func (smartSharpe) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// autocorrelationPenalty computes the Lo (2002) correction factor:
// sqrt(1 + 2*sum(rho_k)) where rho_k is the autocorrelation at lag k.
// Uses lags 1 through min(n/4, 6) to avoid noise from high-lag estimates.
func autocorrelationPenalty(returns []float64) float64 {
	count := len(returns)
	if count < 4 {
		return 1
	}

	meanReturn := stat.Mean(returns, nil)
	maxLag := min(count/4, 6)

	// Compute variance (denominator for autocorrelation).
	var varSum float64

	for _, v := range returns {
		d := v - meanReturn
		varSum += d * d
	}

	if varSum == 0 {
		return 1
	}

	// Sum autocorrelations at lags 1..maxLag.
	var acSum float64

	for lag := 1; lag <= maxLag; lag++ {
		var covSum float64
		for idx := lag; idx < count; idx++ {
			covSum += (returns[idx] - meanReturn) * (returns[idx-lag] - meanReturn)
		}

		acSum += covSum / varSum
	}

	// Penalty = sqrt(1 + 2*sum(rho_k)).
	inner := 1 + 2*acSum
	if inner <= 0 {
		return 1
	}

	return math.Sqrt(inner)
}

// SmartSharpe is the Sharpe ratio corrected for autocorrelation in
// returns using the Lo (2002) method. It divides the standard Sharpe
// by a penalty factor derived from return serial correlation.
var SmartSharpe PerformanceMetric = smartSharpe{}
