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
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/penny-vault/pvbt/universe"
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

// Fetch implements universe.DataSource.
func (e *Engine) Fetch(ctx context.Context, assets []asset.Asset, lookback portfolio.Period, metrics []data.Metric) (*data.DataFrame, error) {
	// Stub -- will be implemented in Task 8.
	return nil, fmt.Errorf("engine: Fetch not yet implemented")
}

// FetchAt implements universe.DataSource.
func (e *Engine) FetchAt(ctx context.Context, assets []asset.Asset, t time.Time, metrics []data.Metric) (*data.DataFrame, error) {
	// Stub -- will be implemented in Task 8.
	return nil, fmt.Errorf("engine: FetchAt not yet implemented")
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

// Backtest executes a backtest over the given time range.
// Stub -- will be implemented in Task 10.
func (e *Engine) Backtest(ctx context.Context, acct *portfolio.Account, start, end time.Time) (*portfolio.Account, error) {
	return acct, fmt.Errorf("engine: Backtest not yet implemented")
}

// Run is a deprecated alias for Backtest. It maintains compilation of existing
// callers (e.g., cli/backtest.go) until they are updated in Task 12.
// Deprecated: Use Backtest instead.
func (e *Engine) Run(ctx context.Context, acct *portfolio.Account, start, end time.Time) (*portfolio.Account, error) {
	return e.Backtest(ctx, acct, start, end)
}

// RunLive starts continuous execution.
// Stub -- will be implemented in Task 11.
func (e *Engine) RunLive(ctx context.Context, acct *portfolio.Account) (<-chan *portfolio.Account, error) {
	return nil, fmt.Errorf("engine: RunLive not yet implemented")
}

// Compile-time check that Engine implements universe.DataSource.
var _ universe.DataSource = (*Engine)(nil)

// dataCache is a placeholder for the sliding-window cache (implemented in Task 7).
type dataCache struct{}
