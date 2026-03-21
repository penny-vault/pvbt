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

type shortWinRate struct{}

func (shortWinRate) Name() string { return "ShortWinRate" }

func (shortWinRate) Description() string {
	return "Percentage of short round-trip trades that were profitable, computed from " +
		"FIFO-matched short-sell/buy-cover pairs. Only includes trades with Direction == TradeShort."
}

func (shortWinRate) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	trades := stats.TradeDetailsView(ctx)
	trips, _ := roundTrips(trades, stats.TransactionsView(ctx))

	var shortTrips []roundTrip

	for idx, td := range trades {
		if td.Direction == TradeShort {
			shortTrips = append(shortTrips, trips[idx])
		}
	}

	if len(shortTrips) == 0 {
		return 0, nil
	}

	wins := 0

	for _, rt := range shortTrips {
		if rt.pnl > 0 {
			wins++
		}
	}

	return float64(wins) / float64(len(shortTrips)), nil
}

func (shortWinRate) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) { return nil, nil }

// ShortWinRate is the percentage of short round-trip trades that were profitable.
var ShortWinRate PerformanceMetric = shortWinRate{}
