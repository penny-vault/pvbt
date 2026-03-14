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
	"sort"

	"github.com/penny-vault/pvbt/data"
)

type tailRatio struct{}

func (tailRatio) Name() string { return "TailRatio" }

func (tailRatio) Description() string {
	return "Ratio of the 95th percentile return to the absolute 5th percentile return. Measures the asymmetry between upside and downside tails. Values above 1.0 indicate the upside tail is fatter than the downside. Higher is better."
}

func (tailRatio) Compute(a *Account, window *Period) (float64, error) {
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

	sorted := make([]float64, len(col))
	copy(sorted, col)
	sort.Float64s(sorted)

	n := len(sorted)
	p5 := sorted[int(math.Floor(0.05*float64(n)))]
	p95 := sorted[int(math.Floor(0.95*float64(n)))]

	if p5 == 0 {
		return 0, nil
	}

	return p95 / math.Abs(p5), nil
}

func (tailRatio) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// TailRatio is the 95th percentile return divided by the absolute 5th
// percentile return. It measures the asymmetry between extreme gains
// and extreme losses.
var TailRatio PerformanceMetric = tailRatio{}
