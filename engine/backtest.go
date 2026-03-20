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

	// 1b. Load market holidays from the first provider that supports it.
	if !tradecron.HolidaysInitialized() {
		if hp := e.findHolidayProvider(); hp != nil {
			holidays, err := hp.FetchMarketHolidays(ctx)
			if err != nil {
				return nil, fmt.Errorf("engine: loading market holidays: %w", err)
			}

			tradecron.SetMarketHolidays(holidays)
		}
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

	// 4c. CLI benchmark override (WithBenchmarkTicker) takes priority.
	if e.benchmarkTicker != "" {
		e.benchmark = e.assets[e.benchmarkTicker]
		if e.benchmark == (asset.Asset{}) {
			e.benchmark = asset.Asset{Ticker: e.benchmarkTicker}
		}
	}

	// 5. Validate: error if schedule is nil.
	if e.schedule == nil {
		return nil, fmt.Errorf("engine: strategy %q did not set a schedule during Setup", e.strategy.Name())
	}

	// 6. Create and configure account.
	acct := e.createAccount(start)

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

	// Pre-fetch the raw risk-free yield series extending before the backtest
	// start so strategies with lookback windows have risk-free data available.
	if e.riskFreeResolved {
		rfStart := start.AddDate(-2, 0, 0)

		rfDF, rfFetchErr := e.fetchRange(ctx, []asset.Asset{e.riskFreeAssetDGS}, []data.Metric{data.MetricClose}, rfStart, end)
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

	// 8. Store start/end on engine.
	e.start = start
	e.end = end

	// PHASE 2: DATE ENUMERATION

	// 9. Create a daily schedule for equity recording on every trading day.
	dailySchedule, dailyErr := tradecron.New("@close * * *", tradecron.RegularHours)
	if dailyErr != nil {
		return nil, fmt.Errorf("engine: creating daily equity schedule: %w", dailyErr)
	}

	// Collect strategy dates by calendar date for matching.
	strategyCalDates := make(map[string]bool)
	cur := e.schedule.Next(start.Add(-time.Nanosecond))

	for !cur.After(end) {
		strategyCalDates[cur.Format("2006-01-02")] = true
		cur = e.schedule.Next(cur.Add(time.Nanosecond))
	}

	// Walk all trading days via the daily schedule.
	type backtestStep struct {
		date       time.Time
		isStrategy bool
	}

	var steps []backtestStep

	cur = dailySchedule.Next(start.Add(-time.Nanosecond))
	for !cur.After(end) {
		calKey := cur.Format("2006-01-02")
		steps = append(steps, backtestStep{
			date:       cur,
			isStrategy: strategyCalDates[calKey],
		})
		cur = dailySchedule.Next(cur.Add(time.Nanosecond))
	}

	// PHASE 3: STEP LOOP

	housekeepMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
	priceMetrics := []data.Metric{data.MetricClose, data.AdjClose}

	for stepIdx, step := range steps {
		// 10. Check context cancellation.
		if err := ctx.Err(); err != nil {
			return acct, err
		}

		date := step.date

		// 11. Set current date.
		e.currentDate = date

		// 12. Build step context with zerolog.
		stepLogger := zerolog.Ctx(ctx).With().
			Str("strategy", e.strategy.Name()).
			Time("date", date).
			Int("step", stepIdx+1).
			Int("total", len(steps)).
			Bool("strategy_day", step.isStrategy).
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

		// 15-16. Run strategy only on strategy-schedule dates.
		if step.isStrategy {
			// 15. Update simulated broker with price provider and date.
			if sb, ok := e.broker.(*SimulatedBroker); ok {
				sb.SetPriceProvider(e, date)
			}

			// 16. Call strategy.Compute.
			if err := e.strategy.Compute(stepCtx, e, acct); err != nil {
				return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
					e.strategy.Name(), date, err)
			}
		}

		// 17. Build price DataFrame for all held assets (including any
		// newly acquired positions from Compute).
		var priceAssets []asset.Asset

		acct.Holdings(func(a asset.Asset, _ float64) {
			priceAssets = append(priceAssets, a)
		})

		if e.benchmark != (asset.Asset{}) {
			priceAssets = append(priceAssets, e.benchmark)
		}

		// Convert DGS3MO yield to cumulative risk-free value.
		if e.riskFreeResolved {
			rfDF, rfFetchErr := e.FetchAt(stepCtx, []asset.Asset{e.riskFreeAssetDGS}, date, []data.Metric{data.MetricClose})
			if rfFetchErr == nil {
				yield := rfDF.Value(e.riskFreeAssetDGS, data.MetricClose)
				if !math.IsNaN(yield) && yield > 0 {
					e.riskFreeCumulative = portfolio.YieldToCumulative(yield, e.riskFreeCumulative)
				} else if e.riskFreeCumulative == 0 {
					e.riskFreeCumulative = 100.0
				}
			}
		}

		acct.SetRiskFreeValue(e.riskFreeCumulative)

		if len(priceAssets) > 0 {
			priceDF, err := e.FetchAt(stepCtx, priceAssets, date, priceMetrics)
			if err != nil {
				return nil, fmt.Errorf("engine: price fetch on %v: %w", date, err)
			}

			// 18. Record equity.
			acct.UpdatePrices(priceDF)
		} else {
			// No assets to price -- record cash-only portfolio value.
			cashDF, cashErr := data.NewDataFrame([]time.Time{date}, nil, nil, data.Daily, nil)
			if cashErr != nil {
				return nil, fmt.Errorf("engine: cash-only DataFrame on %v: %w", date, cashErr)
			}

			acct.UpdatePrices(cashDF)
		}

		// 18b. Compute registered metrics only on strategy dates.
		if step.isStrategy {
			computeMetrics(acct, date)
		}

		// 19. Evict old cache data.
		e.cache.evictBefore(date)
	}

	// PHASE 4: RETURN
	return acct, nil
}
