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

package engine

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/rs/zerolog"
)

// MarginCallHandler is an optional interface that strategies may implement
// to handle margin calls. When a margin deficiency is detected, the engine
// calls OnMarginCall before falling back to automatic liquidation.
type MarginCallHandler interface {
	OnMarginCall(ctx context.Context, eng *Engine, port portfolio.Portfolio, batch *portfolio.Batch) error
}

// checkAndHandleMarginCall checks if maintenance margin is breached and
// handles via strategy handler or auto-liquidation.
func (eng *Engine) checkAndHandleMarginCall(ctx context.Context, acct portfolio.PortfolioManager, date time.Time) error {
	deficiency := acct.MarginDeficiency()
	if deficiency == 0 {
		return nil
	}

	zerolog.Ctx(ctx).Warn().
		Float64("deficiency", deficiency).
		Float64("margin_ratio", acct.MarginRatio()).
		Msg("margin call triggered")

	// Try strategy handler first.
	if handler, ok := eng.strategy.(MarginCallHandler); ok {
		batch := acct.NewBatch(date)
		batch.SkipMiddleware = true

		if err := handler.OnMarginCall(ctx, eng, acct, batch); err != nil {
			return fmt.Errorf("engine: margin call handler: %w", err)
		}

		if err := acct.ExecuteBatch(ctx, batch); err != nil {
			return fmt.Errorf("engine: execute margin call batch: %w", err)
		}

		if acct.MarginDeficiency() == 0 {
			return nil
		}
	}

	return eng.autoLiquidateShorts(ctx, acct, date)
}

// setMarginPrices fetches current close prices for held assets and stores
// them on the account via SetPrices (without recording an equity point).
// This makes margin ratio calculations available before the full
// updateAccountPrices call that records performance data.
func (eng *Engine) setMarginPrices(ctx context.Context, acct portfolio.PortfolioManager, date time.Time) error {
	var heldAssets []asset.Asset

	acct.Holdings(func(held asset.Asset, _ float64) {
		heldAssets = append(heldAssets, held)
	})

	if len(heldAssets) == 0 {
		return nil
	}

	priceDF, err := eng.FetchAt(ctx, heldAssets, date, []data.Metric{data.MetricClose})
	if err != nil {
		return fmt.Errorf("engine: margin price fetch on %v: %w", date, err)
	}

	acct.SetPrices(priceDF)

	return nil
}

// autoLiquidateShorts covers short positions proportionally to restore
// maintenance margin.
func (eng *Engine) autoLiquidateShorts(ctx context.Context, acct portfolio.PortfolioManager, date time.Time) error {
	deficiency := acct.MarginDeficiency()
	if deficiency == 0 {
		return nil
	}

	shortMarketValue := acct.ShortMarketValue()
	if shortMarketValue == 0 {
		return nil
	}

	coverFraction := deficiency / shortMarketValue
	if coverFraction > 1 {
		coverFraction = 1
	}

	batch := acct.NewBatch(date)
	batch.SkipMiddleware = true

	acct.Holdings(func(ast asset.Asset, qty float64) {
		if qty >= 0 {
			return
		}

		coverQty := math.Ceil(math.Abs(qty) * coverFraction)
		if coverQty > math.Abs(qty) {
			coverQty = math.Abs(qty)
		}

		batch.Orders = append(batch.Orders, broker.Order{
			Asset:         ast,
			Side:          broker.Buy,
			Qty:           coverQty,
			OrderType:     broker.Market,
			TimeInForce:   broker.Day,
			Justification: "margin call auto-liquidation",
		})
	})

	if len(batch.Orders) == 0 {
		return nil
	}

	if err := acct.ExecuteBatch(ctx, batch); err != nil {
		return fmt.Errorf("engine: auto-liquidate shorts: %w", err)
	}

	if acct.MarginDeficiency() > 0 {
		zerolog.Ctx(ctx).Error().
			Float64("remaining_deficiency", acct.MarginDeficiency()).
			Msg("margin still breached after auto-liquidation")
	}

	return nil
}
