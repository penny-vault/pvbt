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

type kellyCriterion struct{}

func (kellyCriterion) Name() string { return "KellyCriterion" }

func (kellyCriterion) Description() string {
	return "Optimal fraction of capital to risk per period based on historical win rate and payoff ratio. Computed as W - (1-W)/R where W is win rate and R is average win / average loss. Values above 0 suggest positive edge; values near 0 or negative suggest the strategy should not be traded."
}

func (kellyCriterion) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return 0, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioEquity))
	if len(col) == 0 {
		return 0, nil
	}

	var (
		wins, losses    int
		avgWin, avgLoss float64
	)

	for _, val := range col {
		if val > 0 {
			wins++
			avgWin += val
		} else if val < 0 {
			losses++
			avgLoss += math.Abs(val)
		}
	}

	if wins == 0 || losses == 0 {
		return 0, nil
	}

	avgWin /= float64(wins)
	avgLoss /= float64(losses)

	w := float64(wins) / float64(len(col))
	// Kelly = W - (1 - W) / R, where R = avgWin / avgLoss
	return w - (1-w)/(avgWin/avgLoss), nil
}

func (kellyCriterion) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// KellyCriterion computes the optimal capital allocation fraction using
// the Kelly formula: W - (1-W)/R where W is win rate and R is the ratio
// of average win to average loss. Widely used for position sizing.
var KellyCriterion PerformanceMetric = kellyCriterion{}
