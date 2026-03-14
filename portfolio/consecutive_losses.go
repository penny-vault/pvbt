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

type consecutiveLosses struct{}

func (consecutiveLosses) Name() string { return "ConsecutiveLosses" }

func (consecutiveLosses) Description() string {
	return "Longest streak of consecutive periods with negative returns. Critical for understanding drawdown psychology -- longer losing streaks are harder to endure even if total loss is small. The value is a count of periods."
}

func (consecutiveLosses) Compute(a *Account, window *Period) (float64, error) {
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

	maxStreak := 0
	current := 0
	for _, v := range col {
		if v < 0 {
			current++
			if current > maxStreak {
				maxStreak = current
			}
		} else {
			current = 0
		}
	}

	return float64(maxStreak), nil
}

func (consecutiveLosses) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// ConsecutiveLosses is the longest streak of consecutive negative-return
// periods. Important for assessing the psychological difficulty of
// running a strategy during its worst stretches.
var ConsecutiveLosses PerformanceMetric = consecutiveLosses{}
