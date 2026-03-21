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
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

type unrealizedLTCG struct{}

func (unrealizedLTCG) Name() string { return "UnrealizedLTCG" }

func (unrealizedLTCG) Description() string {
	return "Unrealized long-term capital gains from open positions held longer than 365 days. Computed by comparing current market prices to cost basis of existing tax lots. These gains become realized (and taxable) when positions are sold."
}

func (unrealizedLTCG) Compute(acct *Account, _ *Period) (float64, error) {
	prices := acct.Prices()
	perfData := acct.PerfData()

	if prices == nil || perfData == nil {
		return 0, nil
	}

	times := perfData.Times()
	if len(times) == 0 {
		return 0, nil
	}

	now := times[len(times)-1]

	var total float64

	for ast, lots := range acct.TaxLots() {
		currentPrice := prices.Value(ast, data.MetricClose)
		if math.IsNaN(currentPrice) {
			return math.NaN(), nil
		}

		for _, lot := range lots {
			holdingDays := now.Sub(lot.Date).Hours() / 24
			if holdingDays > 365 {
				total += (currentPrice - lot.Price) * lot.Qty
			}
		}
	}

	// Include short lots: unrealized P&L = (entry price - current price) * qty
	acct.ShortLots(func(shortAsset asset.Asset, lots []TaxLot) {
		currentPrice := prices.Value(shortAsset, data.MetricClose)
		if math.IsNaN(currentPrice) {
			return
		}

		for _, lot := range lots {
			holdingDays := now.Sub(lot.Date).Hours() / 24
			if holdingDays > 365 {
				total += (lot.Price - currentPrice) * lot.Qty
			}
		}
	})

	return total, nil
}

func (unrealizedLTCG) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// UnrealizedLTCGMetric is unrealized long-term capital gains from positions
// held longer than 365 days.
var UnrealizedLTCGMetric PerformanceMetric = unrealizedLTCG{}
