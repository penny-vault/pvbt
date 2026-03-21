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

// grossExposureLimit rejects orders that would push gross exposure
// (longMV + shortMV) / equity above the configured maximum.
type grossExposureLimit struct {
	maxGross float64
}

// GrossExposureLimit returns a middleware that filters out orders when the
// resulting gross exposure (longMV + shortMV) / equity would exceed maxGross.
// Orders are evaluated sequentially; an order that would breach the limit is
// dropped while subsequent orders that fit are kept.
func GrossExposureLimit(maxGross float64) portfolio.Middleware {
	return &grossExposureLimit{maxGross: maxGross}
}

func (g *grossExposureLimit) Process(_ context.Context, batch *portfolio.Batch) error {
	port := batch.Portfolio()
	equity := port.Equity()

	if equity <= 0 {
		return nil
	}

	longMV := port.LongMarketValue()
	shortMV := port.ShortMarketValue()

	var kept []broker.Order

	for _, order := range batch.Orders {
		projLong, projShort := projectExposureChange(port, order, longMV, shortMV)
		projGross := (projLong + projShort) / equity

		if projGross > g.maxGross {
			batch.Annotate("risk:gross-exposure-limit",
				fmt.Sprintf("dropped %v %s: gross exposure would be %.1f%% (limit %.1f%%)",
					order.Side, order.Asset.Ticker, projGross*100, g.maxGross*100))

			continue
		}

		kept = append(kept, order)
		longMV = projLong
		shortMV = projShort
	}

	batch.Orders = kept

	return nil
}

// netExposureLimit constrains abs(longMV - shortMV) / equity.
type netExposureLimit struct {
	maxNet float64
}

// NetExposureLimit returns a middleware that filters out orders when the
// resulting net exposure abs(longMV - shortMV) / equity would exceed maxNet.
func NetExposureLimit(maxNet float64) portfolio.Middleware {
	return &netExposureLimit{maxNet: maxNet}
}

func (n *netExposureLimit) Process(_ context.Context, batch *portfolio.Batch) error {
	port := batch.Portfolio()
	equity := port.Equity()

	if equity <= 0 {
		return nil
	}

	longMV := port.LongMarketValue()
	shortMV := port.ShortMarketValue()

	var kept []broker.Order

	for _, order := range batch.Orders {
		projLong, projShort := projectExposureChange(port, order, longMV, shortMV)
		projNet := math.Abs(projLong-projShort) / equity

		if projNet > n.maxNet {
			batch.Annotate("risk:net-exposure-limit",
				fmt.Sprintf("dropped %v %s: net exposure would be %.1f%% (limit %.1f%%)",
					order.Side, order.Asset.Ticker, projNet*100, n.maxNet*100))

			continue
		}

		kept = append(kept, order)
		longMV = projLong
		shortMV = projShort
	}

	batch.Orders = kept

	return nil
}

// projectExposureChange computes what longMV and shortMV would become if the
// given order were executed. It uses the portfolio's current position to
// determine whether a sell opens or extends a short vs closes a long.
func projectExposureChange(port portfolio.Portfolio, order broker.Order, longMV, shortMV float64) (float64, float64) {
	currentQty := port.Position(order.Asset)

	// Determine the dollar amount of the order.
	var dollarAmount float64

	switch {
	case order.Amount > 0:
		dollarAmount = order.Amount
	case order.Qty > 0:
		positionValue := port.PositionValue(order.Asset)
		if currentQty != 0 {
			pricePerShare := math.Abs(positionValue / currentQty)
			dollarAmount = order.Qty * pricePerShare
		}
	}

	switch order.Side {
	case broker.Buy:
		if currentQty < 0 {
			// Covering a short: reduce shortMV. If we cover more than the
			// short position, the remainder increases longMV.
			shortPositionValue := math.Abs(currentQty) * math.Abs(port.PositionValue(order.Asset)/currentQty)
			if dollarAmount <= shortPositionValue {
				shortMV -= dollarAmount
			} else {
				shortMV -= shortPositionValue
				longMV += dollarAmount - shortPositionValue
			}
		} else {
			// Opening or extending a long.
			longMV += dollarAmount
		}

	case broker.Sell:
		if currentQty > 0 {
			// Closing a long: reduce longMV. If we sell more than the long
			// position, the remainder increases shortMV.
			longPositionValue := currentQty * (port.PositionValue(order.Asset) / currentQty)
			if dollarAmount <= longPositionValue {
				longMV -= dollarAmount
			} else {
				longMV -= longPositionValue
				shortMV += dollarAmount - longPositionValue
			}
		} else {
			// Opening or extending a short.
			shortMV += dollarAmount
		}
	}

	return longMV, shortMV
}
