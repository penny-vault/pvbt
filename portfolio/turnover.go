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

type turnover struct{}

func (turnover) Name() string { return "Turnover" }

func (turnover) Description() string {
	return "Annualized portfolio turnover rate computed as total sell value divided by mean " +
		"portfolio value, scaled to a year. Higher values indicate more active trading. A " +
		"value of 1.0 means the entire portfolio was turned over once per year."
}

func (turnover) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	_, totalSellValue := roundTrips(stats.TradeDetailsView(ctx), stats.TransactionsView(ctx))

	perfData := stats.PerfDataView(ctx)
	if perfData == nil {
		return 0, nil
	}

	equityCol := perfData.Column(portfolioAsset, data.PortfolioEquity)
	equityTimes := perfData.Times()

	if len(equityCol) < 2 || totalSellValue == 0 {
		return 0, nil
	}

	var sum float64
	for _, v := range equityCol {
		sum += v
	}

	meanValue := sum / float64(len(equityCol))

	if meanValue <= 0 {
		return 0, nil
	}

	periodDays := equityTimes[len(equityTimes)-1].Sub(equityTimes[0]).Hours() / 24.0
	if periodDays <= 0 {
		return 0, nil
	}

	return (totalSellValue / meanValue) * (365.25 / periodDays), nil
}

func (turnover) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) { return nil, nil }

// Turnover is the annualized portfolio turnover rate, computed as
// total sell value divided by mean portfolio value, scaled to a year.
var Turnover PerformanceMetric = turnover{}
