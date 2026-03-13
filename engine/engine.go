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
	e := &Engine{
		strategy: strategy,
		assets:   make(map[string]asset.Asset),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// createAccount builds a portfolio.Account from the engine's configuration.
// If a snapshot is set, the account is restored from it; otherwise a fresh
// account is created with the initial deposit.
// createAccount builds a portfolio.Account from the engine's configuration.
// If a snapshot is set, the account is restored from it; otherwise a fresh
// account is created with the initial deposit. If no broker was provided
// via WithBroker, a SimulatedBroker is created and stored on e.broker.
func (e *Engine) createAccount() *portfolio.Account {
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
		opts = append(opts, portfolio.WithCash(e.initialDeposit))
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

// CurrentDate returns the current simulation date.
func (e *Engine) CurrentDate() time.Time {
	return e.currentDate
}

// periodToTime returns base minus the given period.
func periodToTime(base time.Time, p portfolio.Period) time.Time {
	switch p.Unit {
	case portfolio.UnitDay:
		return base.AddDate(0, 0, -p.N)
	case portfolio.UnitMonth:
		return base.AddDate(0, -p.N, 0)
	case portfolio.UnitYear:
		return base.AddDate(-p.N, 0, 0)
	default:
		return base
	}
}

// buildProviderRouting populates e.metricProvider by iterating e.providers
// and mapping each metric to the BatchProvider that serves it.
// Returns an error if a provider does not implement BatchProvider.
func (e *Engine) buildProviderRouting() error {
	e.metricProvider = make(map[data.Metric]data.BatchProvider)
	for _, p := range e.providers {
		bp, ok := p.(data.BatchProvider)
		if !ok {
			return fmt.Errorf("engine: provider %T does not implement BatchProvider", p)
		}
		for _, m := range p.Provides() {
			e.metricProvider[m] = bp
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
	for _, m := range metrics {
		bp, ok := e.metricProvider[m]
		if !ok {
			return nil, fmt.Errorf("engine: no provider registered for metric %q", m)
		}
		providerMetrics[bp] = append(providerMetrics[bp], m)
	}

	var frames []*data.DataFrame
	for bp, provMetrics := range providerMetrics {
		req := data.DataRequest{
			Assets:    assets,
			Metrics:   provMetrics,
			Start:     start,
			End:       end,
			Frequency: data.Daily,
		}
		df, err := bp.Fetch(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("engine: provider fetch: %w", err)
		}
		frames = append(frames, df)
	}

	if len(frames) == 0 {
		return data.NewDataFrame(nil, nil, nil, nil)
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
func (e *Engine) FetchAt(ctx context.Context, assets []asset.Asset, t time.Time, metrics []data.Metric) (*data.DataFrame, error) {
	if e.cache == nil {
		e.cache = newDataCache(e.cacheMaxBytes)
	}
	return e.fetchRange(ctx, assets, metrics, t, t)
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

	for _, yr := range years {
		for _, a := range assets {
			for _, m := range metrics {
				key := colCacheKey{figi: a.CompositeFigi, metric: m, chunkStart: yr}
				if _, ok := e.cache.get(key); !ok {
					cm, exists := misses[yr]
					if !exists {
						cm = &chunkMiss{
							assets:  make(map[string]asset.Asset),
							metrics: make(map[data.Metric]bool),
						}
						misses[yr] = cm
					}
					cm.assets[a.CompositeFigi] = a
					cm.metrics[m] = true
				}
			}
		}
	}

	// Fetch misses: one bulk call per chunk year.
	for yr, cm := range misses {
		missAssets := make([]asset.Asset, 0, len(cm.assets))
		for _, a := range cm.assets {
			missAssets = append(missAssets, a)
		}
		missMetrics := make([]data.Metric, 0, len(cm.metrics))
		for m := range cm.metrics {
			missMetrics = append(missMetrics, m)
		}

		chunkStart := time.Unix(yr, 0).In(nyc)
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
		dfTimes := df.Times()
		for _, a := range df.AssetList() {
			for _, m := range df.MetricList() {
				col := df.Column(a, m)
				if col == nil {
					continue
				}
				colCopy := make([]float64, len(col))
				copy(colCopy, col)
				timesCopy := make([]time.Time, len(dfTimes))
				copy(timesCopy, dfTimes)
				key := colCacheKey{figi: a.CompositeFigi, metric: m, chunkStart: yr}
				e.cache.put(key, &colCacheEntry{times: timesCopy, values: colCopy})
			}
		}
	}

	// Assemble the requested DataFrame from cached columns.
	// Build the union time axis.
	timeSet := make(map[int64]time.Time)
	for _, yr := range years {
		for _, a := range assets {
			for _, m := range metrics {
				key := colCacheKey{figi: a.CompositeFigi, metric: m, chunkStart: yr}
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
		return data.NewDataFrame(nil, nil, nil, nil)
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
	T := len(unionTimes)
	M := len(metrics)
	slab := make([]float64, T*len(assets)*M)
	for i := range slab {
		slab[i] = math.NaN()
	}

	// Scatter cached values into slab.
	for aIdx, a := range assets {
		for mIdx, m := range metrics {
			colStart := (aIdx*M + mIdx) * T
			for _, yr := range years {
				key := colCacheKey{figi: a.CompositeFigi, metric: m, chunkStart: yr}
				entry, ok := e.cache.get(key)
				if !ok {
					continue
				}
				for i, t := range entry.times {
					ti, ok := timeIdx[t.Unix()]
					if !ok {
						continue
					}
					slab[colStart+ti] = entry.values[i]
				}
			}
		}
	}

	assembled, err := data.NewDataFrame(unionTimes, assets, metrics, slab)
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
