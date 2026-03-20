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

package risk

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

type maxPositionSize struct {
	limit float64
}

// MaxPositionSize returns a middleware that caps any single position at the
// given weight (0.0 to 1.0) of total portfolio value. Excess goes to cash.
func MaxPositionSize(limit float64) portfolio.Middleware {
	return &maxPositionSize{limit: limit}
}

func (m *maxPositionSize) Process(_ context.Context, batch *portfolio.Batch) error {
	weights := batch.ProjectedWeights()
	totalValue := batch.ProjectedValue()
	modified := false

	for asset, weight := range weights {
		if weight <= m.limit {
			continue
		}

		excessWeight := weight - m.limit
		excessDollars := excessWeight * totalValue

		// Inject sell order to reduce position.
		batch.Orders = append(batch.Orders, broker.Order{
			Asset:       asset,
			Side:        broker.Sell,
			Amount:      excessDollars,
			OrderType:   broker.Market,
			TimeInForce: broker.Day,
		})

		batch.Annotate("risk:max-position-size",
			fmt.Sprintf("capped %s from %.1f%% to %.1f%%, $%.0f moved to cash",
				asset.Ticker, weight*100, m.limit*100, excessDollars))

		modified = true
	}

	_ = modified

	return nil
}
