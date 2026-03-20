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
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

type drawdownCircuitBreaker struct {
	threshold float64
}

// DrawdownCircuitBreaker returns a middleware that force-liquidates all
// equity positions to cash when the portfolio's drawdown from peak exceeds
// the given threshold (e.g., 0.15 for 15%).
func DrawdownCircuitBreaker(threshold float64) portfolio.Middleware {
	return &drawdownCircuitBreaker{threshold: threshold}
}

func (m *drawdownCircuitBreaker) Process(_ context.Context, batch *portfolio.Batch) error {
	// Get current drawdown from the portfolio's performance metrics.
	dd, err := batch.Portfolio().PerformanceMetric(portfolio.MaxDrawdown).Value()
	if err != nil {
		// No performance data yet (early in backtest) -- no action.
		return nil
	}

	// MaxDrawdown is typically negative (e.g., -0.15 for 15% drawdown).
	if math.Abs(dd) < m.threshold {
		return nil
	}

	// Collect sell orders for all existing long positions.
	var sells []broker.Order

	batch.Portfolio().Holdings(func(ast asset.Asset, qty float64) {
		if qty > 0 {
			sells = append(sells, broker.Order{
				Asset:       ast,
				Side:        broker.Sell,
				Qty:         qty,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
		}
	})

	// Keep only sell orders from the batch (remove any buy orders).
	filtered := batch.Orders[:0]

	for _, order := range batch.Orders {
		if order.Side == broker.Sell {
			filtered = append(filtered, order)
		}
	}

	batch.Orders = append(filtered, sells...)

	batch.Annotate("risk:drawdown-circuit-breaker",
		fmt.Sprintf("drawdown %.1f%% exceeds %.1f%% threshold, force-liquidating all positions",
			math.Abs(dd)*100, m.threshold*100))

	return nil
}
