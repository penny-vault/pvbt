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
	cacheChunkSize time.Duration
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
		e.cache = newDataCache(e.cacheMaxBytes, e.cacheChunkSize)
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

	assetsHash := hashAssets(assets)
	metricsHash := hashMetrics(metrics)

	boundaries := e.cache.chunkBoundaries(rangeStart, rangeEnd)
	var chunks []*data.DataFrame

	for _, chunkStart := range boundaries {
		key := dataCacheKey{
			assetsHash:  assetsHash,
			metricsHash: metricsHash,
			chunkStart:  chunkStart,
		}
		if df, ok := e.cache.get(key); ok {
			chunks = append(chunks, df)
			continue
		}

		chunkEnd := chunkStart.Add(e.cache.chunkSize)
		if chunkEnd.After(rangeEnd) {
			chunkEnd = rangeEnd
		}

		df, err := e.fetchFromProviders(ctx, assets, metrics, chunkStart, chunkEnd)
		if err != nil {
			return nil, err
		}
		log.Debug().
			Int("len", df.Len()).
			Int("assets", len(df.AssetList())).
			Int("metrics", len(df.MetricList())).
			Time("chunkStart", chunkStart).
			Time("chunkEnd", chunkEnd).
			Msg("engine.Fetch chunk result")
		e.cache.put(key, df)
		chunks = append(chunks, df)
	}

	if len(chunks) == 0 {
		return data.NewDataFrame(nil, nil, nil, nil)
	}

	var merged *data.DataFrame
	if len(chunks) == 1 {
		merged = chunks[0]
	} else {
		var err error
		merged, err = data.MergeTimes(chunks...)
		if err != nil {
			return nil, fmt.Errorf("engine: merge chunks: %w", err)
		}
	}

	result := merged.Between(rangeStart, rangeEnd)
	log.Debug().
		Int("len", result.Len()).
		Int("assets", len(result.AssetList())).
		Int("metrics", len(result.MetricList())).
		Msg("engine.Fetch final result")
	return result, nil
}

// FetchAt implements universe.DataSource.
func (e *Engine) FetchAt(ctx context.Context, assets []asset.Asset, t time.Time, metrics []data.Metric) (*data.DataFrame, error) {
	return e.fetchFromProviders(ctx, assets, metrics, t, t)
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
