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
)

type treynor struct{}

func (treynor) Name() string { return "Treynor" }

func (treynor) Description() string {
	return "Excess return per unit of systematic risk (beta). Similar to Sharpe but uses beta instead of standard deviation. Appropriate for well-diversified portfolios where unsystematic risk has been eliminated."
}

func (treynor) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, nil
	}

	pd := stats.PerfDataView(ctx)
	if pd == nil {
		return 0, nil
	}

	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}

	eqCol := df.Column(portfolioAsset, data.PortfolioEquity)
	rfWinCol := pd.Window(window).Column(portfolioAsset, data.PortfolioRiskFree)

	if len(eqCol) < 2 || len(rfWinCol) < 2 {
		return 0, nil
	}

	// Compute time span in years; return 0 for very short backtests.
	times := df.Times()

	calendarDays := times[len(times)-1].Sub(times[0]).Hours() / 24
	if calendarDays < 30 {
		return 0, nil
	}

	years := calendarDays / 365.25

	// Annualize returns using CAGR.
	cagrPortfolio := math.Pow(eqCol[len(eqCol)-1]/eqCol[0], 1/years) - 1
	cagrRiskFree := math.Pow(rfWinCol[len(rfWinCol)-1]/rfWinCol[0], 1/years) - 1

	betaValue, err := Beta.Compute(ctx, stats, window)
	if err != nil {
		return 0, err
	}

	if betaValue == 0 {
		return 0, nil
	}

	return (cagrPortfolio - cagrRiskFree) / betaValue, nil
}

func (treynor) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// Treynor is the Treynor ratio: excess return per unit of systematic
// risk (beta).
var Treynor PerformanceMetric = treynor{}
