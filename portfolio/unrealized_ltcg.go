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

	"github.com/penny-vault/pvbt/data"
)

type unrealizedLTCG struct{}

func (unrealizedLTCG) Name() string { return "UnrealizedLTCG" }

func (unrealizedLTCG) Description() string {
	return "Unrealized long-term capital gains from open positions held longer than 365 days. Computed by comparing current market prices to cost basis of existing tax lots. These gains become realized (and taxable) when positions are sold."
}

func (unrealizedLTCG) Compute(a *Account, _ *Period) float64 {
	prices := a.Prices()
	times := a.EquityTimes()

	if prices == nil || len(times) == 0 {
		return 0
	}

	now := times[len(times)-1]
	var total float64

	for ast, lots := range a.TaxLots() {
		currentPrice := prices.Value(ast, data.MetricClose)
		if math.IsNaN(currentPrice) {
			continue
		}
		for _, lot := range lots {
			holdingDays := now.Sub(lot.Date).Hours() / 24
			if holdingDays > 365 {
				total += (currentPrice - lot.Price) * lot.Qty
			}
		}
	}

	return total
}

func (unrealizedLTCG) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// UnrealizedLTCGMetric is unrealized long-term capital gains from positions
// held longer than 365 days.
var UnrealizedLTCGMetric PerformanceMetric = unrealizedLTCG{}
