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
	"sort"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/config"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/fill"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/penny-vault/pvbt/universe"

	"github.com/rs/zerolog"
)

// Engine orchestrates data access, computation scheduling, and portfolio
// management for both backtesting and live trading.
type Engine struct {
	strategy      Strategy
	providers     []data.DataProvider
	assetProvider data.AssetProvider
	schedule      *tradecron.TradeCron
	benchmark     asset.Asset

	// Risk-free rate (DGS3MO) state.
	riskFreeResolved   bool
	riskFreeAssetDGS   asset.Asset
	riskFreeCumulative float64
	riskFreeTimes      []time.Time
	riskFreeValues     []float64
	riskFreeIndex      map[time.Time]int // date -> index into riskFreeValues, built once during init

	// configuration (set via options, used during init)
	cacheMaxBytes    int64
	initialDeposit   float64
	broker           broker.Broker
	snapshot         portfolio.PortfolioSnapshot
	dateRangeMode    DateRangeMode
	warmup           int
	benchmarkTicker  string
	fillBaseModel    fill.BaseModel
	fillAdjusters    []fill.Adjuster
	middlewareConfig *config.Config

	account portfolio.PortfolioManager

	// populated during initialization
	assets         map[string]asset.Asset
	cache          *dataCache
	currentDate    time.Time
	start          time.Time
	end            time.Time
	metricProvider map[data.Metric]data.BatchProvider
	predicting     bool

	children       []*childEntry
	childrenByName map[string]*childEntry
}

// New creates a new engine for the given strategy.
func New(strategy Strategy, opts ...Option) *Engine {
	eng := &Engine{
		strategy: strategy,
		assets:   make(map[string]asset.Asset),
	}
	for _, opt := range opts {
		opt(eng)
	}

	return eng
}

// createAccount builds a portfolio.Account from the engine's configuration.
// If a snapshot is set, the account is restored from it; otherwise a fresh
// account is created with the initial deposit.
// createAccount builds a portfolio.Account from the engine's configuration.
// If a snapshot is set, the account is restored from it; otherwise a fresh
// account is created with the initial deposit. If no broker was provided
// via WithBroker, a SimulatedBroker is created and stored on e.broker.
func (e *Engine) createAccount(start time.Time) portfolio.PortfolioManager {
	if e.broker == nil {
		sb := NewSimulatedBroker()
		if e.fillBaseModel != nil {
			sb.SetFillPipeline(fill.NewPipeline(e.fillBaseModel, e.fillAdjusters))
		}

		e.broker = sb
	}

	// Use pre-configured account if provided.
	if e.account != nil {
		if e.initialDeposit != 0 || e.snapshot != nil {
			zerolog.Ctx(context.Background()).Warn().Msg("WithAccount set: ignoring WithInitialDeposit and WithPortfolioSnapshot")
		}

		if !e.account.HasBroker() {
			e.account.SetBroker(e.broker)
		}

		return e.account
	}

	var opts []portfolio.Option
	if e.snapshot != nil {
		opts = append(opts, portfolio.WithPortfolioSnapshot(e.snapshot))
	} else {
		opts = append(opts, portfolio.WithCash(e.initialDeposit, start))
	}

	opts = append(opts, portfolio.WithBroker(e.broker))

	return portfolio.New(opts...)
}

// SetBenchmark sets the benchmark asset for performance comparison.
// Typically called by the runner (CLI) rather than the strategy itself.
// Strategies should suggest a benchmark via Describe() instead.
func (e *Engine) SetBenchmark(a asset.Asset) {
	e.benchmark = a
}

// Asset looks up an asset by ticker from the pre-loaded registry.
// Panics if the ticker cannot be resolved.
func (e *Engine) Asset(ticker string) asset.Asset {
	if a, ok := e.assets[ticker]; ok {
		return a
	}

	if e.assetProvider != nil {
		a, err := e.assetProvider.LookupAsset(context.Background(), ticker)
		if err == nil {
			e.assets[ticker] = a
			return a
		}
	}

	panic(fmt.Sprintf("engine: unknown asset ticker %q", ticker))
}

// findHolidayProvider returns the first registered provider that implements
// HolidayProvider, or nil if none do.
func (e *Engine) findHolidayProvider() data.HolidayProvider {
	for _, provider := range e.providers {
		if hp, ok := provider.(data.HolidayProvider); ok {
			return hp
		}
	}

	if hp, ok := e.assetProvider.(data.HolidayProvider); ok {
		return hp
	}

	return nil
}

// Universe creates a static universe wired to this engine for data fetching.
func (e *Engine) Universe(assets ...asset.Asset) universe.Universe {
	return universe.NewStaticWithSource(assets, e)
}

// RatedUniverse creates a universe whose membership is determined by analyst
// ratings. The engine finds a RatingProvider from its registered providers,
// creates the universe, and wires it with the engine's data source.
func (e *Engine) RatedUniverse(analyst string, filter data.RatingFilter) universe.Universe {
	for _, p := range e.providers {
		if rp, ok := p.(data.RatingProvider); ok {
			u := universe.NewRated(rp, analyst, filter)
			u.SetDataSource(e)

			return u
		}
	}

	panic(fmt.Sprintf("engine: no provider implements RatingProvider (needed for analyst %q)", analyst))
}

// IndexUniverse creates a universe whose membership is determined by index
// composition (e.g. S&P 500, Nasdaq 100). The engine finds an IndexProvider
// from its registered providers, creates the universe, and wires it with the
// engine's data source.
func (e *Engine) IndexUniverse(indexName string) universe.Universe {
	for _, p := range e.providers {
		if ip, ok := p.(data.IndexProvider); ok {
			u := universe.NewIndex(ip, indexName)
			u.SetDataSource(e)

			return u
		}
	}

	panic(fmt.Sprintf("engine: no provider implements IndexProvider (needed for index %q)", indexName))
}

// CurrentDate returns the current simulation date.
func (e *Engine) CurrentDate() time.Time {
	return e.currentDate
}

// periodToTime returns base minus the given period.
func periodToTime(base time.Time, period portfolio.Period) time.Time {
	return period.Before(base)
}

// buildProviderRouting populates e.metricProvider by iterating e.providers
// and mapping each metric to the BatchProvider that serves it.
// Returns an error if a provider does not implement BatchProvider.
func (e *Engine) buildProviderRouting() error {
	e.metricProvider = make(map[data.Metric]data.BatchProvider)
	for _, provider := range e.providers {
		batchProvider, ok := provider.(data.BatchProvider)
		if !ok {
			return fmt.Errorf("engine: provider %T does not implement BatchProvider", provider)
		}

		for _, metric := range provider.Provides() {
			e.metricProvider[metric] = batchProvider
		}
	}

	return nil
}

// fetchFromProviders groups metrics by their provider, issues one Fetch call
// per provider, and merges the results with MergeColumns.
func (e *Engine) fetchFromProviders(ctx context.Context, assets []asset.Asset, metrics []data.Metric, start, end time.Time) (*data.DataFrame, error) {
	if e.metricProvider == nil {
		return nil, fmt.Errorf("engine: provider routing not initialized; call buildProviderRouting first")
	}

	// Group metrics by provider.
	providerMetrics := make(map[data.BatchProvider][]data.Metric)

	for _, metric := range metrics {
		batchProvider, ok := e.metricProvider[metric]
		if !ok {
			return nil, fmt.Errorf("engine: no provider registered for metric %q", metric)
		}

		providerMetrics[batchProvider] = append(providerMetrics[batchProvider], metric)
	}

	var frames []*data.DataFrame

	for batchProvider, provMetrics := range providerMetrics {
		req := data.DataRequest{
			Assets:    assets,
			Metrics:   provMetrics,
			Start:     start,
			End:       end,
			Frequency: data.Daily,
		}

		df, err := batchProvider.Fetch(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("engine: provider fetch: %w", err)
		}

		frames = append(frames, df)
	}

	if len(frames) == 0 {
		return data.NewDataFrame(nil, nil, nil, data.Daily, nil)
	}

	if len(frames) == 1 {
		return frames[0], nil
	}

	return data.MergeColumns(frames...)
}

// Fetch implements data.DataSource.
func (e *Engine) Fetch(ctx context.Context, assets []asset.Asset, lookback portfolio.Period, metrics []data.Metric) (*data.DataFrame, error) {
	log := zerolog.Ctx(ctx)

	if e.cache == nil {
		e.cache = newDataCache(e.cacheMaxBytes)
	}

	rangeStart := periodToTime(e.currentDate, lookback)
	rangeEnd := e.currentDate

	tickers := make([]string, len(assets))

	figis := make([]string, len(assets))
	for i, a := range assets {
		tickers[i] = a.Ticker
		figis[i] = a.CompositeFigi
	}

	log.Debug().
		Strs("tickers", tickers).
		Strs("figis", figis).
		Int("metrics", len(metrics)).
		Time("rangeStart", rangeStart).
		Time("rangeEnd", rangeEnd).
		Time("currentDate", e.currentDate).
		Msg("engine.Fetch")

	assembled, fetchErr := e.fetchRange(ctx, assets, metrics, rangeStart, rangeEnd)
	if fetchErr != nil {
		return nil, fetchErr
	}

	if e.predicting && assembled.Len() > 0 && assembled.End().Before(e.currentDate) {
		filled, fillErr := ForwardFillTo(assembled, e.currentDate)
		if fillErr != nil {
			return nil, fmt.Errorf("engine: forward-fill in Fetch: %w", fillErr)
		}

		assembled = filled
	}

	if e.riskFreeResolved && len(e.riskFreeTimes) > 0 && assembled.Len() > 0 {
		rfSlice := e.sliceRiskFree(assembled.Times())
		if rfSlice != nil {
			if err := assembled.SetRiskFreeRates(rfSlice); err != nil {
				return nil, fmt.Errorf("engine: set risk-free rates on assembled frame: %w", err)
			}
		}
	}

	assembled.SetSource(e)

	return assembled, nil
}

// FetchAt implements data.DataSource.
func (e *Engine) FetchAt(ctx context.Context, assets []asset.Asset, timestamp time.Time, metrics []data.Metric) (*data.DataFrame, error) {
	if !e.predicting && !e.currentDate.IsZero() && timestamp.After(e.currentDate) {
		return nil, fmt.Errorf("FetchAt: requested future date %s (current simulation date is %s)",
			timestamp.Format("2006-01-02"), e.currentDate.Format("2006-01-02"))
	}

	if e.cache == nil {
		e.cache = newDataCache(e.cacheMaxBytes)
	}

	result, err := e.fetchRange(ctx, assets, metrics, timestamp, timestamp)
	if err != nil {
		return nil, err
	}

	if e.predicting && result.Len() > 0 && result.End().Before(timestamp) {
		filled, fillErr := ForwardFillTo(result, timestamp)
		if fillErr != nil {
			return nil, fmt.Errorf("engine: forward-fill in FetchAt: %w", fillErr)
		}

		result = filled
	}

	if e.riskFreeResolved && len(e.riskFreeTimes) > 0 && result.Len() > 0 {
		rfSlice := e.sliceRiskFree(result.Times())
		if rfSlice != nil {
			if err := result.SetRiskFreeRates(rfSlice); err != nil {
				return nil, fmt.Errorf("engine: set risk-free rates on result frame: %w", err)
			}
		}
	}

	result.SetSource(e)

	return result, nil
}

// sliceRiskFree returns the cumulative risk-free values for the given
// timestamps. Returns nil if no risk-free series is available or if
// timestamps fall outside the precomputed range. Uses the pre-built
// riskFreeIndex for O(1) lookups with binary-search forward-fill fallback.
func (e *Engine) sliceRiskFree(timestamps []time.Time) []float64 {
	if len(e.riskFreeTimes) == 0 || len(timestamps) == 0 {
		return nil
	}

	result := make([]float64, len(timestamps))

	for tsIdx, t := range timestamps {
		key := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		if idx, ok := e.riskFreeIndex[key]; ok {
			result[tsIdx] = e.riskFreeValues[idx]
		} else {
			// Forward-fill: binary search for the most recent prior date.
			searchKey := key
			bestIdx := sort.Search(len(e.riskFreeTimes), func(j int) bool {
				rfDate := e.riskFreeTimes[j]
				return rfDate.After(searchKey)
			}) - 1

			if bestIdx >= 0 {
				result[tsIdx] = e.riskFreeValues[bestIdx]
			} else {
				result[tsIdx] = math.NaN()
			}
		}
	}

	return result
}

// fetchRange is the shared implementation for Fetch and FetchAt.
// It checks the per-column cache, bulk-fetches misses grouped by
// calendar-year chunk, and assembles the result.
func (e *Engine) fetchRange(ctx context.Context, assets []asset.Asset, metrics []data.Metric, rangeStart, rangeEnd time.Time) (*data.DataFrame, error) {
	log := zerolog.Ctx(ctx)

	years := chunkYears(rangeStart, rangeEnd)

	// Identify cache misses grouped by chunk year.
	type chunkMiss struct {
		assets  map[string]asset.Asset // figi -> asset
		metrics map[data.Metric]bool
	}

	misses := make(map[int64]*chunkMiss)

	for _, year := range years {
		for _, assetItem := range assets {
			for _, metric := range metrics {
				key := colCacheKey{figi: assetItem.CompositeFigi, metric: metric, chunkStart: year}
				if _, ok := e.cache.get(key); !ok {
					cacheMiss, exists := misses[year]
					if !exists {
						cacheMiss = &chunkMiss{
							assets:  make(map[string]asset.Asset),
							metrics: make(map[data.Metric]bool),
						}
						misses[year] = cacheMiss
					}

					cacheMiss.assets[assetItem.CompositeFigi] = assetItem
					cacheMiss.metrics[metric] = true
				}
			}
		}
	}

	// Fetch misses: one bulk call per chunk year.
	for year, cacheMiss := range misses {
		missAssets := make([]asset.Asset, 0, len(cacheMiss.assets))
		for _, assetItem := range cacheMiss.assets {
			missAssets = append(missAssets, assetItem)
		}

		missMetrics := make([]data.Metric, 0, len(cacheMiss.metrics))
		for metric := range cacheMiss.metrics {
			missMetrics = append(missMetrics, metric)
		}

		chunkStart := time.Unix(year, 0).In(nyc)
		chunkEnd := time.Date(chunkStart.Year()+1, 1, 1, 0, 0, 0, 0, nyc).Add(-time.Nanosecond)

		log.Debug().
			Int("missAssets", len(missAssets)).
			Int("missMetrics", len(missMetrics)).
			Time("chunkStart", chunkStart).
			Time("chunkEnd", chunkEnd).
			Msg("engine.fetchRange cache miss")

		df, err := e.fetchFromProviders(ctx, missAssets, missMetrics, chunkStart, chunkEnd)
		if err != nil {
			return nil, err
		}

		// Decompose the DataFrame into individual columns and cache them.
		// For empty results, cache empty entries so we don't re-fetch.
		dfTimes := df.Times()
		if len(dfTimes) == 0 {
			empty := &colCacheEntry{}

			for _, assetItem := range missAssets {
				for _, metric := range missMetrics {
					key := colCacheKey{figi: assetItem.CompositeFigi, metric: metric, chunkStart: year}
					e.cache.put(key, empty)
				}
			}
		} else {
			for _, assetItem := range df.AssetList() {
				for _, metric := range df.MetricList() {
					col := df.Column(assetItem, metric)
					if col == nil {
						continue
					}

					colCopy := make([]float64, len(col))
					copy(colCopy, col)

					timesCopy := make([]time.Time, len(dfTimes))
					copy(timesCopy, dfTimes)

					key := colCacheKey{figi: assetItem.CompositeFigi, metric: metric, chunkStart: year}
					e.cache.put(key, &colCacheEntry{times: timesCopy, values: colCopy})
				}
			}
		}
	}

	// Assemble the requested DataFrame from cached columns.
	// Build the union time axis.
	timeSet := make(map[int64]time.Time)

	for _, year := range years {
		for _, assetItem := range assets {
			for _, metric := range metrics {
				key := colCacheKey{figi: assetItem.CompositeFigi, metric: metric, chunkStart: year}

				entry, ok := e.cache.get(key)
				if !ok {
					continue
				}

				for _, t := range entry.times {
					timeSet[t.Unix()] = t
				}
			}
		}
	}

	if len(timeSet) == 0 {
		return data.NewDataFrame(nil, nil, nil, data.Daily, nil)
	}

	// Sort times.
	unionTimes := make([]time.Time, 0, len(timeSet))
	for _, t := range timeSet {
		unionTimes = append(unionTimes, t)
	}

	sort.Slice(unionTimes, func(i, j int) bool {
		return unionTimes[i].Before(unionTimes[j])
	})

	// Build time index.
	timeIdx := make(map[int64]int, len(unionTimes))
	for i, t := range unionTimes {
		timeIdx[t.Unix()] = i
	}

	// Allocate slab and fill with NaN.
	numTimes := len(unionTimes)
	numMetrics := len(metrics)

	slab := make([]float64, numTimes*len(assets)*numMetrics)
	for i := range slab {
		slab[i] = math.NaN()
	}

	// Scatter cached values into slab.
	for aIdx, a := range assets {
		for mIdx, m := range metrics {
			colStart := (aIdx*numMetrics + mIdx) * numTimes

			for _, year := range years {
				key := colCacheKey{figi: a.CompositeFigi, metric: m, chunkStart: year}

				entry, ok := e.cache.get(key)
				if !ok {
					continue
				}

				for ii, t := range entry.times {
					ti, ok := timeIdx[t.Unix()]
					if !ok {
						continue
					}

					slab[colStart+ti] = entry.values[ii]
				}
			}
		}
	}

	assembled, err := data.NewDataFrame(unionTimes, assets, metrics, data.Daily,
		data.SlabToColumns(slab, len(assets)*len(metrics), len(unionTimes)))
	if err != nil {
		return nil, fmt.Errorf("engine: assemble cached data: %w", err)
	}

	result := assembled.Between(rangeStart, rangeEnd)
	log.Debug().
		Int("len", result.Len()).
		Int("assets", len(result.AssetList())).
		Int("metrics", len(result.MetricList())).
		Msg("engine.Fetch final result")

	return result, nil
}

// Close releases all resources held by the engine, including closing
// the broker and all registered data providers.
func (e *Engine) Close() error {
	var firstErr error

	// Close the broker.
	if e.broker != nil {
		if err := e.broker.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, p := range e.providers {
		if err := p.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// Prices implements broker.PriceProvider. It returns close, high, and low
// prices for the requested assets at the engine's current simulation date.
// High and low are needed by EvaluatePending for intrabar bracket order
// evaluation.
func (e *Engine) Prices(ctx context.Context, assets ...asset.Asset) (*data.DataFrame, error) {
	return e.FetchAt(ctx, assets, e.currentDate, []data.Metric{
		data.MetricClose, data.MetricHigh, data.MetricLow,
		data.Dividend, data.SplitFactor,
	})
}

// PredictedPortfolio runs the strategy's Compute against a shadow copy of the
// current portfolio using the next scheduled trade date. Data is forward-filled
// from the last available date to the predicted date. The strategy is unaware
// it is a prediction run. Child strategy accounts are also cloned so that
// ChildAllocations returns correct weights during the prediction run.
func (e *Engine) PredictedPortfolio(ctx context.Context) (portfolio.Portfolio, error) {
	if e.schedule == nil {
		return nil, fmt.Errorf("engine: PredictedPortfolio requires a schedule")
	}

	if e.account == nil {
		return nil, fmt.Errorf("engine: PredictedPortfolio requires an initialized account")
	}

	// Determine the next trade date.
	predictedDate := e.schedule.Next(e.currentDate)

	// Clone the current account.
	clone := e.account.Clone()

	// Set up the shadow broker.
	shadowBroker := NewSimulatedBroker()
	clone.SetBroker(shadowBroker)

	// Save original children so we can restore them after the prediction run.
	savedChildren := e.children
	savedChildrenByName := e.childrenByName

	// Clone children for prediction so ChildAllocations uses shadow state.
	if len(e.children) > 0 {
		clonedChildren := make([]*childEntry, len(e.children))
		clonedByName := make(map[string]*childEntry, len(e.childrenByName))

		for idx, child := range e.children {
			clonedAccount := child.account.Clone()
			clonedBroker := NewSimulatedBroker()
			clonedAccount.SetBroker(clonedBroker)

			clonedChildren[idx] = &childEntry{
				strategy: child.strategy,
				name:     child.name,
				weight:   child.weight,
				schedule: child.schedule,
				account:  clonedAccount,
				broker:   clonedBroker,
			}
			clonedByName[child.name] = clonedChildren[idx]
		}

		e.children = clonedChildren
		e.childrenByName = clonedByName
	}

	// Save and restore engine state (including children).
	savedDate := e.currentDate
	e.predicting = true
	e.currentDate = predictedDate

	defer func() {
		e.currentDate = savedDate
		e.predicting = false
		e.children = savedChildren
		e.childrenByName = savedChildrenByName
	}()

	// Set the parent shadow broker's price provider.
	shadowBroker.SetPriceProvider(e, predictedDate)

	// Build compute context.
	computeLogger := zerolog.Ctx(ctx).With().
		Str("strategy", e.strategy.Name()).
		Time("date", predictedDate).
		Logger()
	computeCtx := computeLogger.WithContext(ctx)

	// Run any child strategies that are scheduled on the predicted date so that
	// ChildAllocations reflects up-to-date child portfolios.
	for _, child := range e.children {
		if child.schedule == nil {
			continue
		}

		nextChildDate := child.schedule.Next(predictedDate.Add(-time.Nanosecond))
		if nextChildDate.Format("2006-01-02") != predictedDate.Format("2006-01-02") {
			continue
		}

		child.broker.SetPriceProvider(e, predictedDate)

		childBatch := child.account.NewBatch(predictedDate)
		if err := child.strategy.Compute(computeCtx, e, child.account, childBatch); err != nil {
			return nil, fmt.Errorf("engine: PredictedPortfolio child %q compute on %v: %w",
				child.name, predictedDate, err)
		}

		if err := child.account.ExecuteBatch(computeCtx, childBatch); err != nil {
			return nil, fmt.Errorf("engine: PredictedPortfolio child %q execute on %v: %w",
				child.name, predictedDate, err)
		}
	}

	// Run the parent strategy Compute on the cloned account.
	batch := clone.NewBatch(predictedDate)
	if err := e.strategy.Compute(computeCtx, e, clone, batch); err != nil {
		return nil, fmt.Errorf("engine: PredictedPortfolio compute on %v: %w",
			predictedDate, err)
	}

	if err := clone.ExecuteBatch(computeCtx, batch); err != nil {
		return nil, fmt.Errorf("engine: PredictedPortfolio execute batch on %v: %w",
			predictedDate, err)
	}

	return clone, nil
}

// Compile-time check that Engine implements data.DataSource.
var _ data.DataSource = (*Engine)(nil)

// Compile-time check that Engine implements broker.PriceProvider.
var _ broker.PriceProvider = (*Engine)(nil)

// Compile-time check that Engine implements fill.DataFetcher.
var _ fill.DataFetcher = (*Engine)(nil)

// ForwardFillTo extends a DataFrame by copying the last row's values forward
// to the target date, spaced according to the DataFrame's frequency. Returns
// the original DataFrame unchanged if it already covers the target date.
// Returns an error for Tick frequency since it has no regular interval.
func ForwardFillTo(df *data.DataFrame, targetDate time.Time) (*data.DataFrame, error) {
	if df.Len() == 0 {
		return df, nil
	}

	if !df.End().Before(targetDate) {
		return df, nil
	}

	freq := df.Frequency()
	if freq == data.Tick {
		return nil, fmt.Errorf("cannot forward-fill tick-frequency data: no regular interval")
	}

	// Extract the last row's values.
	assets := df.AssetList()
	metrics := df.MetricList()

	lastRow := make([]float64, len(assets)*len(metrics))
	for assetIdx, currentAsset := range assets {
		for metricIdx, metric := range metrics {
			lastRow[assetIdx*len(metrics)+metricIdx] = df.Value(currentAsset, metric)
		}
	}

	// Generate fill timestamps and append rows.
	cursor := df.End()
	for {
		cursor = nextTimestamp(cursor, freq)
		if cursor.After(targetDate) {
			break
		}

		if err := df.AppendRow(cursor, lastRow); err != nil {
			return nil, fmt.Errorf("forward-fill append at %v: %w", cursor, err)
		}
	}

	return df, nil
}

// nextTimestamp advances a timestamp by one frequency step.
func nextTimestamp(current time.Time, freq data.Frequency) time.Time {
	switch freq {
	case data.Daily:
		return current.AddDate(0, 0, 1)
	case data.Weekly:
		return current.AddDate(0, 0, 7)
	case data.Monthly:
		return current.AddDate(0, 1, 0)
	case data.Quarterly:
		return current.AddDate(0, 3, 0)
	case data.Yearly:
		return current.AddDate(1, 0, 0)
	default:
		return current.AddDate(0, 0, 1)
	}
}
