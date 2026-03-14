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

type avgDrawdownDays struct{}

func (avgDrawdownDays) Name() string { return "AvgDrawdownDays" }

func (avgDrawdownDays) Description() string {
	return "Mean duration of drawdown periods in trading days. A drawdown period starts when the equity falls below its peak and ends when a new peak is reached. Complements AvgDrawdown (magnitude) by measuring how long recoveries take."
}

func (avgDrawdownDays) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}
	eq := pd.Window(window).Metrics(data.PortfolioEquity)
	if eq.Len() < 2 {
		return 0, nil
	}
	peak := eq.CumMax()
	dd := eq.Sub(peak).Div(peak)
	ddCol := dd.Column(portfolioAsset, data.PortfolioEquity)

	// Count drawdown episodes and their durations.
	var durations []int
	current := 0
	for _, v := range ddCol {
		if v < 0 {
			current++
		} else if current > 0 {
			durations = append(durations, current)
			current = 0
		}
	}
	// If still in a drawdown at the end, count it.
	if current > 0 {
		durations = append(durations, current)
	}

	if len(durations) == 0 {
		return 0, nil
	}

	total := 0
	for _, d := range durations {
		total += d
	}

	return float64(total) / float64(len(durations)), nil
}

func (avgDrawdownDays) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// AvgDrawdownDays is the mean duration of drawdown episodes in trading
// days. It measures how long the strategy typically spends recovering
// from a decline before reaching a new peak.
var AvgDrawdownDays PerformanceMetric = avgDrawdownDays{}
