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

type recoveryFactor struct{}

func (recoveryFactor) Name() string { return "RecoveryFactor" }

func (recoveryFactor) Description() string {
	return "Total compounded return divided by the absolute value of the maximum drawdown. Measures how many times over the strategy recovered from its worst decline. Higher values indicate greater resilience. A value of 3.0 means the strategy earned 3x its worst drawdown."
}

func (recoveryFactor) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}

	equity := pd.Window(window).Metrics(data.PortfolioEquity)

	eqCol := equity.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) < 2 {
		return 0, nil
	}

	totalReturn := eqCol[len(eqCol)-1]/eqCol[0] - 1

	peak := equity.CumMax()
	dd := equity.Sub(peak).Div(peak)
	ddCol := dd.Column(portfolioAsset, data.PortfolioEquity)

	minDD := 0.0
	for _, v := range ddCol {
		if v < minDD {
			minDD = v
		}
	}

	if minDD == 0 {
		return 0, nil
	}

	return totalReturn / math.Abs(minDD), nil
}

func (recoveryFactor) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// RecoveryFactor is the total compounded return divided by the absolute
// maximum drawdown. It tells you how many times the strategy has earned
// back its worst loss.
var RecoveryFactor PerformanceMetric = recoveryFactor{}
