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

type averageMAE struct{}

func (averageMAE) Name() string { return "AverageMAE" }

func (averageMAE) Description() string {
	return "Mean Maximum Adverse Excursion (MAE) across all round-trip trades, " +
		"expressed as a fraction of entry price. Values are typically negative; " +
		"closer to zero indicates tighter risk control."
}

func (averageMAE) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	trades := stats.TradeDetailsView(ctx)
	if len(trades) == 0 {
		return 0, nil
	}

	var sumMAE float64
	for _, trade := range trades {
		sumMAE += trade.MAE
	}

	return sumMAE / float64(len(trades)), nil
}

func (averageMAE) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// AverageMAE is the mean Maximum Adverse Excursion across all round-trip trades.
var AverageMAE PerformanceMetric = averageMAE{}
