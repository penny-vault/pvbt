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
	// Get the current drawdown from the running peak: the last value of the
	// drawdown series. Using the since-inception MaxDrawdown scalar instead
	// would latch the breaker permanently after a single historical breach,
	// even once the portfolio has recovered to a new peak.
	ddSeries, err := batch.Portfolio().PerformanceMetric(portfolio.MaxDrawdown).Series()
	if err != nil || ddSeries == nil || ddSeries.Len() == 0 {
		// No performance data yet (early in backtest) -- no action.
		return nil
	}

	ddCol := ddSeries.Column(ddSeries.AssetList()[0], ddSeries.MetricList()[0])
	if len(ddCol) == 0 {
		return nil
	}

	// Drawdown values are negative (e.g., -0.15 for 15% below peak).
	dd := ddCol[len(ddCol)-1]
	if math.IsNaN(dd) || math.Abs(dd) < m.threshold {
		return nil
	}

	// Collect sell orders for all existing long positions.
	var sells []broker.Order

	for ast, qty := range batch.Portfolio().Holdings() {
		if qty > 0 {
			sells = append(sells, broker.Order{
				Asset:       ast,
				Side:        broker.Sell,
				Qty:         qty,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
		}
	}

	// Replace the strategy's orders with the liquidation sells. Keeping the
	// strategy's own sell orders alongside the full-position sells would
	// oversell held positions into unintended shorts.
	batch.Orders = sells

	batch.Annotate("risk:drawdown-circuit-breaker",
		fmt.Sprintf("drawdown %.1f%% exceeds %.1f%% threshold, force-liquidating all positions",
			math.Abs(dd)*100, m.threshold*100))

	return nil
}
