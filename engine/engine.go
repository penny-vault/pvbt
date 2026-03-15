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
	"github.com/penny-vault/pvbt/data"
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
	riskFree      asset.Asset
	benchmark     asset.Asset

	// configuration (set via options, used during init)
	cacheMaxBytes  int64
	initialDeposit float64
	broker         broker.Broker
	snapshot       portfolio.PortfolioSnapshot

	account *portfolio.Account

	// populated during initialization
	assets         map[string]asset.Asset
	cache          *dataCache
	currentDate    time.Time
	start          time.Time
	end            time.Time
	metricProvider map[data.Metric]data.BatchProvider
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
func (e *Engine) createAccount(start time.Time) *portfolio.Account {
	if e.broker == nil {
		e.broker = NewSimulatedBroker()
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

// Schedule sets the trading schedule for the engine. Called by the
// strategy during Setup.
func (e *Engine) Schedule(s *tradecron.TradeCron) {
	e.schedule = s
}

// SetBenchmark sets the benchmark asset. Called by the strategy during Setup.
func (e *Engine) SetBenchmark(a asset.Asset) {
	e.benchmark = a
}

// RiskFreeAsset sets the risk-free asset. Called by the strategy during Setup.
func (e *Engine) RiskFreeAsset(a asset.Asset) {
	e.riskFree = a
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

// CurrentDate returns the current simulation date.
func (e *Engine) CurrentDate() time.Time {
	return e.currentDate
}

// periodToTime returns base minus the given period.
func periodToTime(base time.Time, period portfolio.Period) time.Time {
	switch period.Unit {
	case portfolio.UnitDay:
		return base.AddDate(0, 0, -period.N)
	case portfolio.UnitMonth:
		return base.AddDate(0, -period.N, 0)
	case portfolio.UnitYear:
		return base.AddDate(-period.N, 0, 0)
	default:
		return base
	}
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

// Fetch implements universe.DataSource.
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

	return e.fetchRange(ctx, assets, metrics, rangeStart, rangeEnd)
}

// FetchAt implements universe.DataSource.
func (e *Engine) FetchAt(ctx context.Context, assets []asset.Asset, timestamp time.Time, metrics []data.Metric) (*data.DataFrame, error) {
	if !e.currentDate.IsZero() && timestamp.After(e.currentDate) {
		return nil, fmt.Errorf("FetchAt: requested future date %s (current simulation date is %s)",
			timestamp.Format("2006-01-02"), e.currentDate.Format("2006-01-02"))
	}

	if e.cache == nil {
		e.cache = newDataCache(e.cacheMaxBytes)
	}

	return e.fetchRange(ctx, assets, metrics, timestamp, timestamp)
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

	assembled, err := data.NewDataFrame(unionTimes, assets, metrics, data.Daily, slab)
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
// all registered data providers.
func (e *Engine) Close() error {
	var firstErr error
	for _, p := range e.providers {
		if err := p.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// Prices implements broker.PriceProvider. It returns close prices for
// the requested assets at the engine's current simulation date.
func (e *Engine) Prices(ctx context.Context, assets ...asset.Asset) (*data.DataFrame, error) {
	return e.FetchAt(ctx, assets, e.currentDate, []data.Metric{data.MetricClose})
}

// Compile-time check that Engine implements universe.DataSource.
var _ universe.DataSource = (*Engine)(nil)

// Compile-time check that Engine implements broker.PriceProvider.
var _ broker.PriceProvider = (*Engine)(nil)

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
