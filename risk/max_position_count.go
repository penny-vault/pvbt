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
	"sort"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

type maxPositionCount struct {
	limit int
}

// MaxPositionCount returns a middleware that limits the number of concurrent
// positions to the given count. When the projected holdings exceed the limit,
// the smallest positions by dollar value are sold first and the proceeds go to
// cash.
func MaxPositionCount(limit int) portfolio.Middleware {
	return &maxPositionCount{limit: limit}
}

type positionEntry struct {
	asset asset.Asset
	value float64
	qty   float64
}

func (m *maxPositionCount) Process(_ context.Context, batch *portfolio.Batch) error {
	weights := batch.ProjectedWeights()
	if len(weights) <= m.limit {
		return nil
	}

	totalValue := batch.ProjectedValue()

	// Build a list of positions with their dollar values.
	positions := make([]positionEntry, 0, len(weights))
	holdings := batch.ProjectedHoldings()

	for ast, weight := range weights {
		positions = append(positions, positionEntry{
			asset: ast,
			value: weight * totalValue,
			qty:   holdings[ast],
		})
	}

	// Sort ascending by dollar value so the smallest positions are first.
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].value < positions[j].value
	})

	dropCount := len(positions) - m.limit

	for idx := 0; idx < dropCount; idx++ {
		pos := positions[idx]

		batch.Orders = append(batch.Orders, broker.Order{
			Asset:       pos.asset,
			Side:        broker.Sell,
			Qty:         pos.qty,
			OrderType:   broker.Market,
			TimeInForce: broker.Day,
		})

		batch.Annotate("risk:max-position-count",
			fmt.Sprintf("dropped %s ($%.0f, smallest position) to stay within %d position limit",
				pos.asset.Ticker, pos.value, m.limit))
	}

	return nil
}
