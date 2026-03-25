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

	// 1c. Discover child strategies before hydrating parent.
	e.childrenByName = make(map[string]*childEntry)
	if err := e.discoverChildren(e.strategy, make(map[uintptr]bool)); err != nil {
		return nil, fmt.Errorf("engine: %w", err)
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

	// 4d. Initialize child strategies.
	for _, child := range e.children {
		if err := hydrateFields(e, child.strategy); err != nil {
			return nil, fmt.Errorf("engine: hydrating child %q: %w", child.name, err)
		}

		child.strategy.Setup(e)

		// Extract schedule from Describe().
		if desc, ok := child.strategy.(Descriptor); ok {
			description := desc.Describe()
			if description.Schedule != "" {
				tc, tcErr := tradecron.New(description.Schedule, tradecron.RegularHours)
				if tcErr != nil {
					return nil, fmt.Errorf("engine: child %q schedule: %w", child.name, tcErr)
				}

				child.schedule = tc
			}
		}

		if child.schedule == nil {
			return nil, fmt.Errorf("engine: child strategy %q did not set a schedule", child.name)
		}

		// Create child portfolio with simulated broker.
		childBroker := NewSimulatedBroker()
		child.broker = childBroker
		child.account = portfolio.New(
			portfolio.WithCash(100, start),
			portfolio.WithBroker(childBroker),
		)
	}

	// 5. Validate: error if schedule is nil.
	if e.schedule == nil {
		return nil, fmt.Errorf("engine: strategy %q did not set a schedule during Setup", e.strategy.Name())
	}

	// 5b. Initialize data cache early so validateWarmup can use fetchRange.
	e.cache = newDataCache(e.cacheMaxBytes)

	// 5c. Validate warmup data availability; may adjust start in permissive mode.
	adjustedStart, warmupErr := e.validateWarmup(ctx, start, end)
	if warmupErr != nil {
		return nil, warmupErr
	}

	start = adjustedStart

	// 6. Create and configure account.
	acct := e.createAccount(start)

	e.account = acct
	if e.benchmark != (asset.Asset{}) {
		acct.SetBenchmark(e.benchmark)
	}

	// 6b. Apply config-driven middleware if provided.
	if e.middlewareConfig != nil {
		if err := e.buildMiddlewareFromConfig(); err != nil {
			return nil, fmt.Errorf("engine: building middleware from config: %w", err)
		}
	}

	info := DescribeStrategy(e.strategy)
	if info.Name != "" {
		acct.SetMetadata(portfolio.MetaStrategyName, info.Name)
	}

	if info.ShortCode != "" {
		acct.SetMetadata(portfolio.MetaStrategyShortCode, info.ShortCode)
	}

	if info.Version != "" {
		acct.SetMetadata(portfolio.MetaStrategyVersion, info.Version)
	}

	if info.Description != "" {
		acct.SetMetadata(portfolio.MetaStrategyDesc, info.Description)
	}

	if info.Benchmark != "" {
		acct.SetMetadata(portfolio.MetaStrategyBenchmark, info.Benchmark)
	}

	// 7. Resolve DGS3MO as the system risk-free rate.
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

	// Wire portfolio to simulated broker for margin checks.
	if sb, ok := e.broker.(*SimulatedBroker); ok {
		sb.SetPortfolio(acct)
		sb.SetBorrowRate(acct.BorrowRate())
	}

	// Connect the broker (no-op for SimulatedBroker, authenticates for live brokers).
	if err := e.broker.Connect(ctx); err != nil {
		return nil, fmt.Errorf("engine: broker connect: %w", err)
	}

	// PHASE 2: DATE ENUMERATION

	// 9. Create a daily schedule for equity recording on every trading day.
	dailySchedule, dailyErr := tradecron.New("@close * * *", tradecron.RegularHours)
	if dailyErr != nil {
		return nil, fmt.Errorf("engine: creating daily equity schedule: %w", dailyErr)
	}

	// Collect parent strategy dates by calendar date for matching.
	parentCalDates := make(map[string]bool)
	cur := e.schedule.Next(start.Add(-time.Nanosecond))

	for !cur.After(end) {
		parentCalDates[cur.Format("2006-01-02")] = true
		cur = e.schedule.Next(cur.Add(time.Nanosecond))
	}

	// Collect child strategy dates.
	childCalDates := make(map[string]map[string]bool)

	for _, child := range e.children {
		if child.schedule == nil {
			continue
		}

		dates := make(map[string]bool)

		childCur := child.schedule.Next(start.Add(-time.Nanosecond))
		for !childCur.After(end) {
			dates[childCur.Format("2006-01-02")] = true
			childCur = child.schedule.Next(childCur.Add(time.Nanosecond))
		}

		childCalDates[child.name] = dates
	}

	// Walk all trading days via the daily schedule.
	type backtestStep struct {
		date             time.Time
		isParentStrategy bool
		childStrategies  []string
	}

	var steps []backtestStep

	cur = dailySchedule.Next(start.Add(-time.Nanosecond))
	for !cur.After(end) {
		calKey := cur.Format("2006-01-02")

		var scheduledChildren []string

		for _, child := range e.children {
			if childCalDates[child.name][calKey] {
				scheduledChildren = append(scheduledChildren, child.name)
			}
		}

		steps = append(steps, backtestStep{
			date:             cur,
			isParentStrategy: parentCalDates[calKey],
			childStrategies:  scheduledChildren,
		})
		cur = dailySchedule.Next(cur.Add(time.Nanosecond))
	}

	// PHASE 3: STEP LOOP

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
			Bool("strategy_day", step.isParentStrategy).
			Logger()
		stepCtx := stepLogger.WithContext(ctx)

		// Evaluate pending bracket/OCO orders against intrabar prices.
		if sb, ok := e.broker.(*SimulatedBroker); ok {
			sb.SetPriceProvider(e, date)
			sb.SetDataFetcher(e)
			sb.EvaluatePending()
		}

		// 13-14b. Housekeep parent account (dividends + fill draining).
		if err := e.housekeepAccount(stepCtx, acct, date, e.benchmark); err != nil {
			return nil, err
		}

		// Set prices for margin computation. The housekeeping step above
		// fetched close prices so this call hits the cache. SetPrices
		// stores the price DataFrame without recording an equity point;
		// the full UpdatePrices call later handles equity recording.
		if err := e.setMarginPrices(stepCtx, acct, date); err != nil {
			return nil, err
		}

		// Check margin and handle margin calls (runs every trading day).
		if err := e.checkAndHandleMarginCall(stepCtx, acct, date); err != nil {
			return nil, fmt.Errorf("engine: margin call on %v: %w", date, err)
		}

		// Run scheduled child strategies (children before parent).
		for _, childName := range step.childStrategies {
			child := e.childrenByName[childName]
			child.broker.SetPriceProvider(e, date)
			child.broker.EvaluatePending()

			if err := child.account.CancelOpenOrders(stepCtx); err != nil {
				return nil, fmt.Errorf("engine: child %q cancel orders on %v: %w", childName, date, err)
			}

			childBatch := child.account.NewBatch(date)
			if err := child.strategy.Compute(stepCtx, e, child.account, childBatch); err != nil {
				return nil, fmt.Errorf("engine: child %q compute on %v: %w", childName, date, err)
			}

			if err := child.account.ExecuteBatch(stepCtx, childBatch); err != nil {
				return nil, fmt.Errorf("engine: child %q execute batch on %v: %w", childName, date, err)
			}
		}

		// 15-16. Run strategy only on strategy-schedule dates.
		if step.isParentStrategy {
			// 15. Update simulated broker with price provider and date.
			if sb, ok := e.broker.(*SimulatedBroker); ok {
				sb.SetPriceProvider(e, date)
			}

			// Cancel open orders from previous frame.
			if err := acct.CancelOpenOrders(stepCtx); err != nil {
				return nil, fmt.Errorf("engine: cancel open orders on %v: %w", date, err)
			}

			// 16. Create batch and run strategy.
			batch := acct.NewBatch(date)
			if err := e.strategy.Compute(stepCtx, e, acct, batch); err != nil {
				return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
					e.strategy.Name(), date, err)
			}

			// Execute batch through middleware chain.
			if err := acct.ExecuteBatch(stepCtx, batch); err != nil {
				return nil, fmt.Errorf("engine: execute batch on %v: %w", date, err)
			}
		}

		// 17-18. Update parent account prices.
		if err := e.updateAccountPrices(stepCtx, acct, date, e.benchmark); err != nil {
			return nil, err
		}

		// Housekeep and update prices for all child portfolios at every step.
		for _, child := range e.children {
			if err := e.housekeepAccount(stepCtx, child.account, date, asset.Asset{}); err != nil {
				return nil, fmt.Errorf("engine: child %q housekeeping on %v: %w", child.name, date, err)
			}

			if err := e.updateAccountPrices(stepCtx, child.account, date, asset.Asset{}); err != nil {
				return nil, fmt.Errorf("engine: child %q price update on %v: %w", child.name, date, err)
			}
		}

		// 18b. Compute registered metrics only on strategy dates.
		if step.isParentStrategy {
			if statsProvider, ok := acct.(portfolio.PortfolioStats); ok {
				computeMetrics(statsProvider, date, acct.RegisteredMetrics(), acct.AppendMetric)
			}
		}

		// 19. Evict old cache data.
		e.cache.evictBefore(date)
	}

	// PHASE 4: RETURN
	acct.SetMetadata(portfolio.MetaRunMode, "backtest")
	acct.SetMetadata(portfolio.MetaRunStart, start.Format(time.RFC3339))
	acct.SetMetadata(portfolio.MetaRunEnd, end.Format(time.RFC3339))

	return acct, nil
}

// housekeepAccount records dividends for held assets and drains broker fills
// for the given account on date. benchmark controls whether the benchmark asset
// is included in the housekeeping data fetch; pass asset.Asset{} for child
// accounts that have no benchmark.
func (eng *Engine) housekeepAccount(ctx context.Context, acct portfolio.PortfolioManager, date time.Time, benchmark asset.Asset) error {
	housekeepMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.SplitFactor}

	var heldAssets []asset.Asset

	acct.Holdings(func(a asset.Asset, _ float64) {
		heldAssets = append(heldAssets, a)
	})

	var housekeepAssets []asset.Asset

	housekeepAssets = append(housekeepAssets, heldAssets...)
	if benchmark != (asset.Asset{}) {
		housekeepAssets = append(housekeepAssets, benchmark)
	}

	var housekeepDF *data.DataFrame

	if len(housekeepAssets) > 0 {
		var fetchErr error

		housekeepDF, fetchErr = eng.Fetch(ctx, housekeepAssets, portfolio.Days(1), housekeepMetrics)
		if fetchErr != nil {
			return fmt.Errorf("engine: housekeeping fetch on %v: %w", date, fetchErr)
		}
	}

	// Drain fills from previous step (before splits and dividends).
	if acct.HasBroker() {
		if drainErr := acct.DrainFills(ctx); drainErr != nil {
			return fmt.Errorf("engine: drain fills on %v: %w", date, drainErr)
		}
	}

	// Apply stock splits before dividends (dividend values are post-split).
	if housekeepDF != nil {
		for _, heldAsset := range heldAssets {
			splitFactor := housekeepDF.ValueAt(heldAsset, data.SplitFactor, date)
			if math.IsNaN(splitFactor) || splitFactor == 1.0 {
				continue
			}

			if err := acct.ApplySplit(heldAsset, date, splitFactor); err != nil {
				return fmt.Errorf("engine: apply split for %s on %v: %w", heldAsset.Ticker, date, err)
			}
		}
	}

	// Record borrow fees for short positions.
	if housekeepDF != nil {
		borrowRate := acct.BorrowRate()
		for _, heldAsset := range heldAssets {
			qty := acct.Position(heldAsset)
			if qty >= 0 {
				continue
			}

			closePrice := housekeepDF.ValueAt(heldAsset, data.MetricClose, date)
			if math.IsNaN(closePrice) || closePrice == 0 {
				continue
			}

			dailyFee := math.Abs(qty) * closePrice * (borrowRate / 252.0)
			acct.Record(portfolio.Transaction{
				Date:          date,
				Asset:         heldAsset,
				Type:          asset.FeeTransaction,
				Amount:        -dailyFee,
				Justification: fmt.Sprintf("borrow fee: %s %.2f%% annualized", heldAsset.Ticker, borrowRate*100),
			})
		}
	}

	// Record dividends for held assets.
	if housekeepDF != nil {
		for _, heldAsset := range heldAssets {
			qty := acct.Position(heldAsset)
			if qty == 0 {
				continue
			}

			divPerShare := housekeepDF.ValueAt(heldAsset, data.Dividend, date)
			if math.IsNaN(divPerShare) || divPerShare <= 0 {
				continue
			}

			if qty > 0 {
				// Long position: receive dividend.
				acct.Record(portfolio.Transaction{
					Date:   date,
					Asset:  heldAsset,
					Type:   asset.DividendTransaction,
					Amount: divPerShare * qty,
					Qty:    qty,
					Price:  divPerShare,
				})
			} else {
				// Short position: owe dividend.
				acct.Record(portfolio.Transaction{
					Date:          date,
					Asset:         heldAsset,
					Type:          asset.DividendTransaction,
					Amount:        divPerShare * qty, // negative (qty is negative)
					Qty:           qty,
					Price:         divPerShare,
					Justification: fmt.Sprintf("short dividend obligation: %s ex-date %s", heldAsset.Ticker, date.Format("2006-01-02")),
				})
			}
		}
	}

	return nil
}

// updateAccountPrices fetches current prices and updates equity for the given
// account on date. benchmark controls whether the benchmark asset is included
// in the price fetch; pass asset.Asset{} for child accounts. The risk-free
// rate logic (DGS3MO yield to cumulative conversion) only runs when benchmark
// is non-zero, matching the behavior of the parent account.
func (eng *Engine) updateAccountPrices(ctx context.Context, acct portfolio.PortfolioManager, date time.Time, benchmark asset.Asset) error {
	priceMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow}

	var priceAssets []asset.Asset

	acct.Holdings(func(a asset.Asset, _ float64) {
		priceAssets = append(priceAssets, a)
	})

	if benchmark != (asset.Asset{}) {
		priceAssets = append(priceAssets, benchmark)
	}

	// Convert DGS3MO yield to cumulative risk-free value (parent account only).
	if benchmark != (asset.Asset{}) && eng.riskFreeResolved {
		rfDF, rfFetchErr := eng.FetchAt(ctx, []asset.Asset{eng.riskFreeAssetDGS}, date, []data.Metric{data.MetricClose})
		if rfFetchErr == nil {
			yield := rfDF.Value(eng.riskFreeAssetDGS, data.MetricClose)
			if !math.IsNaN(yield) && yield > 0 {
				eng.riskFreeCumulative = portfolio.YieldToCumulative(yield, eng.riskFreeCumulative)
			} else if eng.riskFreeCumulative == 0 {
				eng.riskFreeCumulative = 100.0
			}
		}
	}

	acct.SetRiskFreeValue(eng.riskFreeCumulative)

	if len(priceAssets) > 0 {
		priceDF, fetchErr := eng.FetchAt(ctx, priceAssets, date, priceMetrics)
		if fetchErr != nil {
			return fmt.Errorf("engine: price fetch on %v: %w", date, fetchErr)
		}

		acct.UpdatePrices(priceDF)
		acct.UpdateExcursions(priceDF)
	} else {
		// No assets to price -- record cash-only portfolio value.
		cashDF, cashErr := data.NewDataFrame([]time.Time{date}, nil, nil, data.Daily, nil)
		if cashErr != nil {
			return fmt.Errorf("engine: cash-only DataFrame on %v: %w", date, cashErr)
		}

		acct.UpdatePrices(cashDF)
		acct.UpdateExcursions(cashDF)
	}

	return nil
}
