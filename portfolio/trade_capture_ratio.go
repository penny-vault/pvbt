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

type tradeCaptureRatio struct{}

func (tradeCaptureRatio) Name() string { return "TradeCaptureRatio" }

func (tradeCaptureRatio) Description() string {
	return "Ratio of mean realized return percentage to mean MFE across all round-trip " +
		"trades. Values close to 1.0 indicate the strategy captures most of the available " +
		"favorable excursion; values near 0 indicate premature exits."
}

func (tradeCaptureRatio) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	trades := stats.TradeDetailsView(ctx)
	if len(trades) == 0 {
		return math.NaN(), nil
	}

	var sumReturnPct, sumMFE float64

	for _, trade := range trades {
		returnPct := (trade.ExitPrice - trade.EntryPrice) / trade.EntryPrice
		sumReturnPct += returnPct
		sumMFE += trade.MFE
	}

	tradeCount := float64(len(trades))
	avgReturnPct := sumReturnPct / tradeCount
	avgMFE := sumMFE / tradeCount

	if avgMFE == 0 {
		return math.NaN(), nil
	}

	return avgReturnPct / avgMFE, nil
}

func (tradeCaptureRatio) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// TradeCaptureRatio is the ratio of mean realized return to mean MFE.
var TradeCaptureRatio PerformanceMetric = tradeCaptureRatio{}
