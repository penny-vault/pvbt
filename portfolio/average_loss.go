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

type averageLoss struct{}

func (averageLoss) Name() string { return "AverageLoss" }

func (averageLoss) Description() string {
	return "Average loss in currency units on losing round-trip trades, computed from " +
		"FIFO-matched buy/sell pairs. The value is negative; closer to zero means smaller " +
		"average losses. Because the value depends on position sizing, compare within the " +
		"same portfolio rather than across different ones."
}

func (averageLoss) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	trips, _ := roundTrips(stats.TradeDetailsView(ctx), stats.TransactionsView(ctx))

	var (
		losses  int
		sumLoss float64
	)

	for _, rt := range trips {
		if rt.pnl <= 0 {
			losses++
			sumLoss += rt.pnl
		}
	}

	if losses == 0 {
		return 0, nil
	}

	return sumLoss / float64(losses), nil
}

func (averageLoss) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// AverageLoss is the average loss on losing round-trip trades (negative value).
var AverageLoss PerformanceMetric = averageLoss{}
