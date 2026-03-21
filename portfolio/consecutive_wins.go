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

	"github.com/penny-vault/pvbt/data"
)

type consecutiveWins struct{}

func (consecutiveWins) Name() string { return "ConsecutiveWins" }

func (consecutiveWins) Description() string {
	return "Longest streak of consecutive periods with positive returns. Useful for behavioral analysis -- longer winning streaks suggest momentum or trending conditions. The value is a count of periods."
}

func (consecutiveWins) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return 0, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioReturns))
	if len(col) == 0 {
		return 0, nil
	}

	maxStreak := 0
	current := 0

	for _, v := range col {
		if v > 0 {
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

func (consecutiveWins) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// ConsecutiveWins is the longest streak of consecutive positive-return
// periods. Useful for understanding momentum characteristics and the
// behavioral experience of running a strategy.
var ConsecutiveWins PerformanceMetric = consecutiveWins{}
