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
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// Option configures the engine.
type Option func(*Engine)

// WithDataProvider registers one or more data providers with the engine.
func WithDataProvider(providers ...data.DataProvider) Option {
	return func(e *Engine) {
		e.providers = append(e.providers, providers...)
	}
}

// WithAssetProvider sets the asset provider for ticker resolution.
func WithAssetProvider(p data.AssetProvider) Option {
	return func(e *Engine) {
		e.assetProvider = p
	}
}

// WithCacheMaxBytes sets the maximum memory for the data cache.
// Default is 512MB.
func WithCacheMaxBytes(n int64) Option {
	return func(e *Engine) {
		e.cacheMaxBytes = n
	}
}

// WithInitialDeposit sets the starting cash balance for the portfolio.
// Mutually exclusive with WithPortfolioSnapshot.
func WithInitialDeposit(amount float64) Option {
	return func(e *Engine) {
		e.initialDeposit = amount
	}
}

// WithBroker sets the broker used for order execution. If not set,
// the engine defaults to a SimulatedBroker.
func WithBroker(b broker.Broker) Option {
	return func(e *Engine) {
		e.broker = b
	}
}

// WithPortfolioSnapshot restores the portfolio from a previous run's
// snapshot. Mutually exclusive with WithInitialDeposit.
func WithPortfolioSnapshot(snap portfolio.PortfolioSnapshot) Option {
	return func(e *Engine) {
		e.snapshot = snap
	}
}

// WithBenchmarkTicker sets the benchmark asset by ticker. This is resolved
// to an asset during engine initialization and takes priority over any
// benchmark suggested by the strategy's Describe() method.
func WithBenchmarkTicker(ticker string) Option {
	return func(e *Engine) {
		e.benchmarkTicker = ticker
	}
}

// WithAccount sets a pre-configured portfolio Account for the engine
// to use. When set, this takes priority over WithInitialDeposit,
// WithPortfolioSnapshot, and WithBroker.
func WithAccount(acct *portfolio.Account) Option {
	return func(e *Engine) {
		e.account = acct
	}
}

// WithDateRangeMode sets how the engine handles date ranges when warmup
// data is insufficient. Default is DateRangeModeStrict.
func WithDateRangeMode(mode DateRangeMode) Option {
	return func(e *Engine) {
		e.dateRangeMode = mode
	}
}
