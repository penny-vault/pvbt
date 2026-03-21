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

type tradeGainLossRatio struct{}

func (tradeGainLossRatio) Name() string { return "TradeGainLossRatio" }

func (tradeGainLossRatio) Description() string {
	return "Average winning trade PnL divided by average losing trade PnL from FIFO-matched " +
		"round-trip trades. Measures the quality of individual trade decisions independent of " +
		"portfolio-level effects like cash drag or unrealized gains. A value above 1.0 means " +
		"winning trades are larger than losing trades on average. Compare with GainLossRatio " +
		"which uses equity curve period returns instead."
}

func (tradeGainLossRatio) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	trips, _ := roundTrips(stats.TradeDetailsView(ctx), stats.TransactionsView(ctx))

	var (
		wins, losses    int
		sumWin, sumLoss float64
	)

	for _, roundTrip := range trips {
		if roundTrip.pnl > 0 {
			wins++
			sumWin += roundTrip.pnl
		} else if roundTrip.pnl < 0 {
			losses++
			sumLoss += roundTrip.pnl
		}
	}

	if wins == 0 || losses == 0 {
		return math.NaN(), nil
	}

	avgWin := sumWin / float64(wins)
	avgLoss := sumLoss / float64(losses)

	return avgWin / math.Abs(avgLoss), nil
}

func (tradeGainLossRatio) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// TradeGainLossRatio is the average winning trade PnL divided by the average
// losing trade PnL, computed from FIFO-matched round-trip trades.
var TradeGainLossRatio PerformanceMetric = tradeGainLossRatio{}
