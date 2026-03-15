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

// RunLive starts continuous live trading execution. It performs the same
// initialization as Backtest (loading assets, hydrating fields, building
// provider routing, calling Setup), then launches a goroutine that fires on
// each scheduled time. The returned channel receives the portfolio after
// each step; sends are non-blocking so a slow consumer does not block the loop.
// Cancel the context to stop execution and close the channel.
func (e *Engine) RunLive(ctx context.Context) (<-chan portfolio.Portfolio, error) {
	// PHASE 1: INITIALIZATION

	// 1. Load asset registry from assetProvider.
	if e.assetProvider == nil {
		return nil, fmt.Errorf("engine: assetProvider is required for RunLive")
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

	// 5. Validate.
	if e.schedule == nil {
		return nil, fmt.Errorf("engine: strategy %q did not set a schedule during Setup", e.strategy.Name())
	}

	// 6. Create and configure account.
	acct := e.createAccount(time.Now())
	e.account = acct
	if e.benchmark != (asset.Asset{}) {
		acct.SetBenchmark(e.benchmark)
	}

	if e.riskFree != (asset.Asset{}) {
		acct.SetRiskFree(e.riskFree)
	}

	// 7. Initialize data cache.
	e.cache = newDataCache(e.cacheMaxBytes)

	// PHASE 2: GOROUTINE

	portfolioCh := make(chan portfolio.Portfolio, 1)

	go func() {
		defer close(portfolioCh)

		housekeepMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
		priceMetrics := []data.Metric{data.MetricClose, data.AdjClose}
		step := 0

		for {
			// a. Compute next scheduled time.
			nextTime := e.schedule.Next(time.Now())
			wait := time.Until(nextTime)

			// b. Wait for next scheduled time or context cancellation.
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return
			}

			step++

			// c. Set current date.
			e.currentDate = time.Now()

			// d. Build step context with zerolog.
			stepLogger := zerolog.Ctx(ctx).With().
				Str("strategy", e.strategy.Name()).
				Time("date", e.currentDate).
				Int("step", step).
				Logger()
			stepCtx := stepLogger.WithContext(ctx)

			// e. Collect held assets plus benchmark and risk-free for housekeeping fetch.
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
					zerolog.Ctx(stepCtx).Error().Err(fetchErr).Msg("housekeeping fetch failed")
					continue // skip this step, try again at next scheduled time
				}
			}

			// f. Record dividends for held assets.
			if housekeepDF != nil {
				for _, heldAsset := range heldAssets {
					qty := acct.Position(heldAsset)
					if qty <= 0 {
						continue
					}

					divPerShare := housekeepDF.ValueAt(heldAsset, data.Dividend, e.currentDate)
					if !math.IsNaN(divPerShare) && divPerShare > 0 {
						acct.Record(portfolio.Transaction{
							Date:   e.currentDate,
							Asset:  heldAsset,
							Type:   portfolio.DividendTransaction,
							Amount: divPerShare * qty,
							Qty:    qty,
							Price:  divPerShare,
						})
					}
				}
			}

			// g. Update simulated broker with price provider if applicable.
			if sb, ok := e.broker.(*SimulatedBroker); ok {
				sb.SetPriceProvider(e, e.currentDate)
			}

			// h. Call strategy.Compute.
			if err := e.strategy.Compute(stepCtx, e, acct); err != nil {
				zerolog.Ctx(stepCtx).Error().Err(err).Msg("strategy compute failed")
				continue
			}

			// i. Build price DataFrame for post-Compute holdings and call UpdatePrices.
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
				priceDF, fetchErr := e.FetchAt(stepCtx, priceAssets, e.currentDate, priceMetrics)
				if fetchErr != nil {
					zerolog.Ctx(stepCtx).Error().Err(fetchErr).Msg("price fetch failed")
				} else {
					acct.UpdatePrices(priceDF)
				}
			}

			// j. Non-blocking send of updated portfolio.
			select {
			case portfolioCh <- acct:
			default:
			}
		}
	}()

	return portfolioCh, nil
}
