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

type gainToPain struct{}

func (gainToPain) Name() string { return "GainToPainRatio" }

func (gainToPain) Description() string {
	return "Jack Schwager's Gain-to-Pain Ratio: sum of all returns divided by the absolute sum of negative returns. Measures total gains per unit of total pain endured. Values above 1.0 are good; above 2.0 is excellent. Unlike GainLossRatio (average win/loss), this uses total sums."
}

func (gainToPain) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return 0, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioEquity))
	if len(col) == 0 {
		return 0, nil
	}

	var totalReturn, negativeSum float64
	for _, v := range col {
		totalReturn += v
		if v < 0 {
			negativeSum += math.Abs(v)
		}
	}

	if negativeSum == 0 {
		return 0, nil
	}

	return totalReturn / negativeSum, nil
}

func (gainToPain) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// GainToPainRatio is Jack Schwager's metric: the sum of all returns
// divided by the absolute sum of negative returns. It captures total
// profit relative to total pain, making it an intuitive alternative
// to Sharpe for evaluating strategy performance.
var GainToPainRatio PerformanceMetric = gainToPain{}
