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
	"sort"

	"github.com/penny-vault/pvbt/data"
)

type valueAtRisk struct{}

func (valueAtRisk) Name() string { return "ValueAtRisk" }

func (valueAtRisk) Description() string {
	return "The 5th percentile of historical returns, representing the worst-case loss expected 95% of the time. A value of -0.02 means there is a 5% chance of losing more than 2% in a single period. More negative values indicate higher risk."
}

func (valueAtRisk) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return 0, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioReturns))
	if len(col) == 0 {
		return 0, nil
	}

	sorted := make([]float64, len(col))
	copy(sorted, col)
	sort.Float64s(sorted)

	idx := int(math.Floor(0.05 * float64(len(sorted))))

	return sorted[idx], nil
}

func (valueAtRisk) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// ValueAtRisk estimates the maximum expected loss over a given time
// horizon at a specified confidence level (e.g., 95%). A VaR of 5%
// at 95% confidence means there is a 5% chance the portfolio loses
// more than 5% in the period.
func (valueAtRisk) BenchmarkTargetable() {}

var ValueAtRisk PerformanceMetric = valueAtRisk{}
