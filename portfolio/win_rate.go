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

type winRate struct{}

func (winRate) Name() string { return "WinRate" }

func (winRate) Description() string {
	return "Percentage of round-trip trades that were profitable, computed from FIFO-matched " +
		"buy/sell pairs. A value of 1.0 means all trades won; 0.0 means none did. Best " +
		"interpreted alongside GainLossRatio since a low win rate with large average wins " +
		"can still be profitable."
}

func (winRate) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	trips, _ := roundTrips(stats.TradeDetailsView(ctx), stats.TransactionsView(ctx))
	if len(trips) == 0 {
		return 0, nil
	}

	wins := 0

	for _, rt := range trips {
		if rt.pnl > 0 {
			wins++
		}
	}

	return float64(wins) / float64(len(trips)), nil
}

func (winRate) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// WinRate is the percentage of round-trip trades that were profitable.
var WinRate PerformanceMetric = winRate{}
