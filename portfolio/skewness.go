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

type skewness struct{}

func (skewness) Name() string { return "Skewness" }

func (skewness) Description() string {
	return "Measures the asymmetry of the return distribution. Positive skew means more extreme positive returns. Negative skew means more extreme negative returns (the common case for equities). Zero indicates a symmetric distribution."
}

func (skewness) Compute(a *Account, window *Period) (float64, error) {
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

	numValues := len(col)
	if numValues < 3 {
		return 0, nil
	}

	stdDev := stat.StdDev(col, nil)
	if stdDev == 0 {
		return 0, nil
	}

	meanVal := stat.Mean(col, nil)
	sum := 0.0

	for _, v := range col {
		d := v - meanVal
		sum += d * d * d
	}

	return sum / float64(numValues) / (stdDev * stdDev * stdDev), nil
}

func (skewness) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// Skewness measures the asymmetry of the return distribution.
// Negative skew means the left tail is longer (more large losses
// than large gains). Positive skew means the right tail is longer.
func (skewness) BenchmarkTargetable() {}

var Skewness PerformanceMetric = skewness{}
