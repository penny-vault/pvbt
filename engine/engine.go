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
	strategy  Strategy
	providers []data.DataProvider
	schedule  *tradecron.TradeCron
	universes []universe.Universe
	riskFree  asset.Asset
}

// New creates a new engine for the given strategy.
func New(strategy Strategy, opts ...Option) *Engine {
	e := &Engine{strategy: strategy}
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

// Register adds a universe to the engine. Called by the strategy during
// Setup so the engine can prefetch data and resolve membership.
func (e *Engine) Register(u universe.Universe) {
	e.universes = append(e.universes, u)
}

// Asset looks up an asset by ticker. Used during Setup to reference
// assets that aren't part of a universe, such as a risk-free rate
// instrument for risk adjustment.
func (e *Engine) Asset(ticker string) asset.Asset {
	// Resolve the ticker to an Asset. The engine may look this up
	// from the data providers or from a known asset registry.
	return asset.Asset{Ticker: ticker}
}

// RiskFreeAsset sets the risk-free asset for this strategy. When set,
// the engine fetches the risk-free asset's data alongside the
// universe's members, and signals that compute returns can
// automatically adjust for the risk-free rate. Called by the strategy
// during Setup.
func (e *Engine) RiskFreeAsset(a asset.Asset) {
	e.riskFree = a
}

// DataFrame returns a DataFrame for the given universe and metrics.
// The engine checks its cache first and fetches from providers if
// needed. Called by the strategy during Compute.
func (e *Engine) DataFrame(u universe.Universe, metrics ...data.Metric) *data.DataFrame {
	// Resolve universe membership for the current simulation date,
	// build a DataRequest for the requested metrics, check the
	// DataFrame cache, and fetch from the appropriate provider if
	// not cached. Return the populated DataFrame.
	return nil
}

// Run executes a backtest over the given time range. The account holds
// the initial cash balance and optional broker configuration. Returns
// the account with its full history after the backtest completes.
func (e *Engine) Run(ctx context.Context, acct *portfolio.Account, start, end time.Time) (*portfolio.Account, error) {
	// 1. Parse the strategy TOML and build a Config.
	// 2. Use reflection to populate the strategy struct's fields from
	//    TOML arguments. Match fields by pvbt struct tag or by name.
	//    Validate that each field's Go type is compatible with the
	//    TOML argument's typecode. Panic on mismatch.
	//    Supported mappings:
	//      float64           <-> TypeNumber
	//      string            <-> TypeString, TypeChoice
	//      bool              <-> TypeBool
	//      time.Duration     <-> TypeDuration
	//      asset.Asset       <-> TypeStock
	//      universe.Universe <-> TypeStockList (builds StaticUniverse)
	//    For universe.Universe fields, automatically register the
	//    universe with the engine after populating it.
	// 3. Call Strategy.Setup(e, config).
	// 4. Prefetch all registered universes for [start, end].
	// 5. Enumerate schedule dates in [start, end] using tradecron.
	// 6. For each scheduled date:
	//    a. Attach a zerolog logger to the context with step metadata
	//       (strategy name, date, step number).
	//    b. Resolve universe membership at this date.
	//    c. Populate/update cached DataFrames.
	//    d. Record dividends for held assets via acct.Record().
	//    e. Call Strategy.Compute(ctx, acct) (acct passed as Portfolio).
	//    f. Process any resulting trades (commission, slippage).
	//    g. Call acct.UpdatePrices() with current market prices.
	// 7. Return the account with its full history.
	return acct, nil
}

// RunLive starts continuous execution. The account holds the initial
// cash balance and broker configuration. The engine waits for each
// scheduled time, calls Compute, and sends the account on the returned
// channel after each step. The channel is closed when the context is
// cancelled.
func (e *Engine) RunLive(ctx context.Context, acct *portfolio.Account) (<-chan *portfolio.Account, error) {
	// 1. Parse the strategy TOML and build a Config.
	// 2. Populate strategy struct fields via reflection (same as Run).
	//    Auto-register any universe.Universe fields.
	// 3. Call Strategy.Setup(e, config).
	// 4. Start a goroutine that:
	//    a. Computes the next scheduled time from tradecron.
	//    b. Sleeps until that time (or ctx is cancelled).
	//    c. Attaches a zerolog logger to the context with step metadata.
	//    d. Resolves universe membership at the current time.
	//    e. Fetches fresh data from stream or batch providers.
	//    f. Records dividends for held assets via acct.Record().
	//    g. Calls Strategy.Compute(ctx, acct) (acct passed as Portfolio).
	//    h. Calls acct.UpdatePrices() with current market prices.
	//    i. Sends the account on the channel.
	//    i. Repeats from (a).
	// 5. Return the channel immediately.
	return nil, nil
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
