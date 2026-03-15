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
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/rs/zerolog"
)

// Backtest executes a full backtest over [start, end] using the engine's
// configured strategy and data providers. It returns the portfolio
// after running every scheduled trading date.
func (e *Engine) Backtest(ctx context.Context, start, end time.Time) (portfolio.Portfolio, error) {
	// PHASE 1: INITIALIZATION

	// 1. Load asset registry from assetProvider.
	if e.assetProvider == nil {
		return nil, fmt.Errorf("engine: assetProvider is required for Backtest")
	}

	allAssets, err := e.assetProvider.Assets(ctx)
	if err != nil {
		return nil, fmt.Errorf("engine: loading asset registry: %w", err)
	}

	for _, a := range allAssets {
		e.assets[a.Ticker] = a
	}

	// 2. Hydrate strategy fields from default tags.
	if err := hydrateFields(e, e.strategy); err != nil {
		return nil, fmt.Errorf("engine: %w", err)
	}

	// 3. Build provider routing table.
	if err := e.buildProviderRouting(); err != nil {
		return nil, fmt.Errorf("engine: building provider routing: %w", err)
	}

	// 4. Call strategy.Setup.
	e.strategy.Setup(e)

	// 5. Validate: error if schedule is nil.
	if e.schedule == nil {
		return nil, fmt.Errorf("engine: strategy %q did not set a schedule during Setup", e.strategy.Name())
	}

	// 6. Create and configure account.
	acct := e.createAccount(start)
	if e.benchmark != (asset.Asset{}) {
		acct.SetBenchmark(e.benchmark)
	}

	if e.riskFree != (asset.Asset{}) {
		acct.SetRiskFree(e.riskFree)
	}

	// 7. Initialize data cache.
	e.cache = newDataCache(e.cacheMaxBytes)

	// 8. Store start/end on engine.
	e.start = start
	e.end = end

	// PHASE 2: DATE ENUMERATION

	// 9. Walk tradecron.Next() from start until past end.
	// Subtract one nanosecond so that a date exactly equal to start is included.
	var dates []time.Time

	cur := e.schedule.Next(start.Add(-time.Nanosecond))
	for !cur.After(end) {
		dates = append(dates, cur)
		cur = e.schedule.Next(cur.Add(time.Nanosecond))
	}

	// PHASE 3: STEP LOOP

	housekeepMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
	priceMetrics := []data.Metric{data.MetricClose, data.AdjClose}

	for dateIdx, date := range dates {
		// 10. Check context cancellation.
		if err := ctx.Err(); err != nil {
			return acct, err
		}

		// 11. Set current date.
		e.currentDate = date

		// 12. Build step context with zerolog.
		stepLogger := zerolog.Ctx(ctx).With().
			Str("strategy", e.strategy.Name()).
			Time("date", date).
			Int("step", dateIdx+1).
			Int("total", len(dates)).
			Logger()
		stepCtx := stepLogger.WithContext(ctx)

		// 13. Fetch housekeeping data for held assets.
		var heldAssets []asset.Asset

		acct.Holdings(func(a asset.Asset, _ float64) {
			heldAssets = append(heldAssets, a)
		})

		var housekeepAssets []asset.Asset

		housekeepAssets = append(housekeepAssets, heldAssets...)
		if e.benchmark != (asset.Asset{}) {
			housekeepAssets = append(housekeepAssets, e.benchmark)
		}

		if e.riskFree != (asset.Asset{}) {
			housekeepAssets = append(housekeepAssets, e.riskFree)
		}

		var housekeepDF *data.DataFrame

		if len(housekeepAssets) > 0 {
			var fetchErr error

			housekeepDF, fetchErr = e.Fetch(stepCtx, housekeepAssets, portfolio.Days(1), housekeepMetrics)
			if fetchErr != nil {
				return nil, fmt.Errorf("engine: housekeeping fetch on %v: %w", date, fetchErr)
			}
		}

		// 14. Record dividends for held assets.
		if housekeepDF != nil {
			for _, heldAsset := range heldAssets {
				qty := acct.Position(heldAsset)
				if qty <= 0 {
					continue
				}

				divPerShare := housekeepDF.ValueAt(heldAsset, data.Dividend, date)
				if !math.IsNaN(divPerShare) && divPerShare > 0 {
					acct.Record(portfolio.Transaction{
						Date:   date,
						Asset:  heldAsset,
						Type:   portfolio.DividendTransaction,
						Amount: divPerShare * qty,
						Qty:    qty,
						Price:  divPerShare,
					})
				}
			}
		}

		// 15. Update simulated broker with price provider and date.
		if sb, ok := e.broker.(*SimulatedBroker); ok {
			sb.SetPriceProvider(e, date)
		}

		// 16. Call strategy.Compute.
		if err := e.strategy.Compute(stepCtx, e, acct); err != nil {
			return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
				e.strategy.Name(), date, err)
		}

		// 17. Build price DataFrame for all assets seen this step (including any
		// newly acquired positions from Compute).
		var priceAssets []asset.Asset

		acct.Holdings(func(a asset.Asset, _ float64) {
			priceAssets = append(priceAssets, a)
		})

		if e.benchmark != (asset.Asset{}) {
			priceAssets = append(priceAssets, e.benchmark)
		}

		if e.riskFree != (asset.Asset{}) {
			priceAssets = append(priceAssets, e.riskFree)
		}

		if len(priceAssets) > 0 {
			priceDF, err := e.FetchAt(stepCtx, priceAssets, date, priceMetrics)
			if err != nil {
				return nil, fmt.Errorf("engine: price fetch on %v: %w", date, err)
			}

			// 18. Call acct.UpdatePrices.
			acct.UpdatePrices(priceDF)
		}

		// 18b. Compute registered metrics across all standard windows.
		computeMetrics(acct, date)

		// 19. Evict old cache data.
		e.cache.evictBefore(date)
	}

	// PHASE 4: RETURN
	return acct, nil
}
