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

type longProfitFactor struct{}

func (longProfitFactor) Name() string { return "LongProfitFactor" }

func (longProfitFactor) Description() string {
	return "Gross profit divided by gross loss from long round-trip trades only. " +
		"A value above 1.0 means long trades are profitable overall."
}

func (longProfitFactor) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	trades := stats.TradeDetailsView(ctx)
	trips, _ := roundTrips(trades, stats.TransactionsView(ctx))

	var sumWin, sumLoss float64

	for idx, td := range trades {
		if td.Direction != TradeLong {
			continue
		}

		if trips[idx].pnl > 0 {
			sumWin += trips[idx].pnl
		} else {
			sumLoss += trips[idx].pnl
		}
	}

	if sumLoss == 0 {
		return math.NaN(), nil
	}

	return sumWin / math.Abs(sumLoss), nil
}

func (longProfitFactor) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) { return nil, nil }

// LongProfitFactor is the ratio of gross profit to gross loss from long round-trip trades.
var LongProfitFactor PerformanceMetric = longProfitFactor{}
