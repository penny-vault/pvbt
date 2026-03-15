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

type omegaRatio struct{}

func (omegaRatio) Name() string { return "OmegaRatio" }

func (omegaRatio) Description() string {
	return "Probability-weighted ratio of gains over losses relative to a zero threshold. Captures the entire return distribution, not just the first two moments like Sharpe. A value above 1.0 means gains outweigh losses. Higher is better."
}

func (omegaRatio) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}

	eq := pd.Window(window).Metrics(data.PortfolioEquity)

	r := eq.Pct().Drop(math.NaN())
	if r.Len() == 0 {
		return 0, nil
	}

	col := r.Column(portfolioAsset, data.PortfolioEquity)

	// Omega = sum(max(r_i, 0)) / sum(max(-r_i, 0))
	// Threshold is 0 (no risk-free subtraction).
	var gains, losses float64

	for _, v := range col {
		if v > 0 {
			gains += v
		} else {
			losses += math.Abs(v)
		}
	}

	if losses == 0 {
		return 0, nil
	}

	return gains / losses, nil
}

func (omegaRatio) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// OmegaRatio is the probability-weighted ratio of gains over losses.
// Unlike Sharpe, it considers the entire return distribution including
// higher moments (skewness, kurtosis). A ratio above 1.0 indicates
// that gains outweigh losses on a probability-weighted basis.
var OmegaRatio PerformanceMetric = omegaRatio{}
