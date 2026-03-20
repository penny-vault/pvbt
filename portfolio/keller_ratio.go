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
)

type kellerRatio struct{}

func (kellerRatio) Name() string { return "KellerRatio" }

func (kellerRatio) Description() string {
	return "Risk-adjusted return that penalizes drawdowns non-linearly: K = R * (1 - D/(1-D)), where R is total return and D is maximum drawdown. When R < 0 or D > 50% the ratio is 0. Small drawdowns have limited impact; as drawdowns approach 50% the penalty grows sharply, making this useful for evaluating tactical allocation strategies."
}

func (kellerRatio) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}

	equity := pd.Window(window).Metrics(data.PortfolioEquity)

	eqCol := equity.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) < 2 {
		return 0, nil
	}

	totalReturn := (eqCol[len(eqCol)-1] / eqCol[0]) - 1

	peak := equity.CumMax()
	dd := equity.Sub(peak).Div(peak)
	ddCol := dd.Column(portfolioAsset, data.PortfolioEquity)

	// Find max drawdown as a positive number (abs of most negative drawdown).
	minDD := 0.0
	for _, d := range ddCol {
		if d < minDD {
			minDD = d
		}
	}

	maxDD := math.Abs(minDD)

	if totalReturn >= 0 && maxDD <= 0.5 {
		return totalReturn * (1 - maxDD/(1-maxDD)), nil
	}

	return 0, nil
}

func (kellerRatio) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// KellerRatio adjusts return for drawdown severity:
// K = R * (1 - D/(1-D)) when R >= 0 and D <= 50%, else 0.
// Small drawdowns have limited impact; large drawdowns amplify the
// penalty, making this a useful risk-adjusted measure.
var KellerRatio PerformanceMetric = kellerRatio{}
