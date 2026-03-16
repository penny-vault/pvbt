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

import "github.com/penny-vault/pvbt/data"

type maxDrawdown struct{}

func (maxDrawdown) Name() string { return "MaxDrawdown" }

func (maxDrawdown) Description() string {
	return "Largest peak-to-trough decline in the equity curve as a decimal fraction. A value of -0.20 means the portfolio fell 20% from its peak. More negative values indicate larger drawdowns."
}

func (maxDrawdown) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}

	equity := pd.Window(window).Metrics(data.PortfolioEquity)
	if equity.Len() == 0 {
		return 0, nil
	}

	peak := equity.CumMax()
	dd := equity.Sub(peak).Div(peak)
	ddCol := dd.Column(portfolioAsset, data.PortfolioEquity)

	minDD := 0.0
	for _, v := range ddCol {
		if v < minDD {
			minDD = v
		}
	}

	return minDD, nil
}

func (maxDrawdown) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return nil, nil
	}

	equity := pd.Window(window).Metrics(data.PortfolioEquity)
	if equity.Len() == 0 {
		return nil, nil
	}

	peak := equity.CumMax()
	dd := equity.Sub(peak).Div(peak)

	return dd.Column(portfolioAsset, data.PortfolioEquity), nil
}

// MaxDrawdown is the largest peak-to-trough decline in portfolio value.
func (maxDrawdown) BenchmarkTargetable() {}

var MaxDrawdown PerformanceMetric = maxDrawdown{}
