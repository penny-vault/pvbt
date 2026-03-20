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
	"github.com/penny-vault/pvbt/tradecron"
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

	// 4b. If Setup did not set schedule/benchmark, try Describe().
	if desc, ok := e.strategy.(Descriptor); ok {
		description := desc.Describe()

		if e.schedule == nil && description.Schedule != "" {
			tc, tcErr := tradecron.New(description.Schedule, tradecron.RegularHours)
			if tcErr != nil {
				return nil, fmt.Errorf("engine: parsing schedule from Describe(): %w", tcErr)
			}

			e.schedule = tc
		}

		if e.benchmark == (asset.Asset{}) && description.Benchmark != "" {
			e.benchmark = e.assets[description.Benchmark]
			if e.benchmark == (asset.Asset{}) {
				e.benchmark = asset.Asset{Ticker: description.Benchmark}
			}
		}
	}

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

	// 7. Initialize data cache (before DGS3MO resolution which may use fetchRange).
	e.cache = newDataCache(e.cacheMaxBytes)

	// Resolve DGS3MO as the system risk-free rate.
	dgs3mo, rfErr := e.assetProvider.LookupAsset(ctx, "DGS3MO")
	if rfErr != nil {
		zerolog.Ctx(ctx).Warn().Msg("risk-free rate data (DGS3MO) not available, using 0%")
	} else {
		e.riskFreeResolved = true
		e.riskFreeAssetDGS = dgs3mo
	}

	// Pre-fetch the raw risk-free yield series with a 5-year lookback.
	if e.riskFreeResolved {
		lookbackStart := time.Now().AddDate(-5, 0, 0)
		rfDF, rfFetchErr := e.fetchRange(ctx, []asset.Asset{e.riskFreeAssetDGS}, []data.Metric{data.MetricClose}, lookbackStart, time.Now())

		if rfFetchErr == nil && rfDF.Len() > 0 {
			rfCol := rfDF.Column(e.riskFreeAssetDGS, data.MetricClose)
			e.riskFreeTimes = make([]time.Time, rfDF.Len())
			copy(e.riskFreeTimes, rfDF.Times())
			e.riskFreeValues = make([]float64, rfDF.Len())
			copy(e.riskFreeValues, rfCol)
			e.riskFreeIndex = make(map[time.Time]int, rfDF.Len())

			for idx, t := range e.riskFreeTimes {
				e.riskFreeIndex[time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)] = idx
			}
		}
	}

	e.riskFreeCumulative = 0

	// PHASE 2: GOROUTINE

	portfolioCh := make(chan portfolio.Portfolio, 1)

	go func() {
		defer close(portfolioCh)

		dailySchedule, dailyErr := tradecron.New("@close * * *", tradecron.RegularHours)
		if dailyErr != nil {
			zerolog.Ctx(ctx).Error().Err(dailyErr).Msg("failed to create daily equity schedule")
			return
		}

		housekeepMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
		priceMetrics := []data.Metric{data.MetricClose, data.AdjClose}
		step := 0

		for {
			// a. Compute next fire time for both schedules.
			now := time.Now()
			nextStrategy := e.schedule.Next(now)
			nextDaily := dailySchedule.Next(now)

			// Pick whichever fires sooner.
			nextTime := nextDaily
			isStrategy := false

			if !nextStrategy.After(nextDaily) {
				nextTime = nextStrategy
				isStrategy = true
			}

			// If both fall on the same calendar day, treat as a strategy day
			// and use the later timestamp for equity recording.
			if nextStrategy.Format("2006-01-02") == nextDaily.Format("2006-01-02") {
				isStrategy = true

				if nextDaily.After(nextStrategy) {
					nextTime = nextDaily
				}
			}

			wait := time.Until(nextTime)

			// b. Wait for next fire time or context cancellation.
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
				Bool("strategy_day", isStrategy).
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

			var housekeepDF *data.DataFrame

			if len(housekeepAssets) > 0 {
				var fetchErr error

				housekeepDF, fetchErr = e.Fetch(stepCtx, housekeepAssets, portfolio.Days(1), housekeepMetrics)
				if fetchErr != nil {
					zerolog.Ctx(stepCtx).Error().Err(fetchErr).Msg("housekeeping fetch failed")
					continue
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

			// g-h. Run strategy only on strategy-schedule days.
			if isStrategy {
				if sb, ok := e.broker.(*SimulatedBroker); ok {
					sb.SetPriceProvider(e, e.currentDate)
				}

				if err := e.strategy.Compute(stepCtx, e, acct); err != nil {
					zerolog.Ctx(stepCtx).Error().Err(err).Msg("strategy compute failed")
					continue
				}
			}

			// i. Mark-to-market: fetch prices and record equity.
			// Retry up to 18 times with 1-hour waits for delayed prices
			// (mutual fund NAVs may not be available until 1-3 AM next day).
			var priceAssets []asset.Asset

			acct.Holdings(func(a asset.Asset, _ float64) {
				priceAssets = append(priceAssets, a)
			})

			if e.benchmark != (asset.Asset{}) {
				priceAssets = append(priceAssets, e.benchmark)
			}

			// Convert DGS3MO yield to cumulative risk-free value.
			if e.riskFreeResolved {
				rfDF, rfFetchErr := e.FetchAt(stepCtx, []asset.Asset{e.riskFreeAssetDGS}, e.currentDate, []data.Metric{data.MetricClose})
				if rfFetchErr == nil {
					yield := rfDF.Value(e.riskFreeAssetDGS, data.MetricClose)
					if !math.IsNaN(yield) && yield > 0 {
						e.riskFreeCumulative = portfolio.YieldToCumulative(yield, e.riskFreeCumulative)
					} else if e.riskFreeCumulative == 0 {
						e.riskFreeCumulative = 100.0
					}

					// Append the raw yield for RiskAdjustedPct.
					e.riskFreeTimes = append(e.riskFreeTimes, e.currentDate)
					e.riskFreeValues = append(e.riskFreeValues, yield)

					rfKey := time.Date(e.currentDate.Year(), e.currentDate.Month(), e.currentDate.Day(), 0, 0, 0, 0, time.UTC)
					if e.riskFreeIndex == nil {
						e.riskFreeIndex = make(map[time.Time]int)
					}

					e.riskFreeIndex[rfKey] = len(e.riskFreeValues) - 1
				}
			}

			acct.SetRiskFreeValue(e.riskFreeCumulative)

			if len(priceAssets) > 0 {
				var (
					priceDF  *data.DataFrame
					fetchErr error
				)

				for attempt := range 18 {
					priceDF, fetchErr = e.FetchAt(stepCtx, priceAssets, e.currentDate, priceMetrics)
					if fetchErr == nil {
						break
					}

					zerolog.Ctx(stepCtx).Warn().
						Err(fetchErr).
						Int("attempt", attempt+1).
						Msg("price fetch failed, retrying in 1 hour")

					select {
					case <-time.After(time.Hour):
					case <-ctx.Done():
						return
					}
				}

				if fetchErr != nil {
					zerolog.Ctx(stepCtx).Error().Err(fetchErr).Msg("price fetch failed after retries")
				} else {
					acct.UpdatePrices(priceDF)
				}
			} else {
				// No assets to price -- record cash-only portfolio value.
				cashDF, cashErr := data.NewDataFrame([]time.Time{e.currentDate}, nil, nil, data.Daily, nil)
				if cashErr == nil {
					acct.UpdatePrices(cashDF)
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
