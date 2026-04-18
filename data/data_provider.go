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

package data

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/tradecron"
)

// DataProvider is the base interface for all data providers. A provider
// supplies externally-sourced metrics (e.g. price data from a database,
// economic indicators from an API). Providers are constructed by the caller
// and registered with the engine via WithDataProvider.
type DataProvider interface {
	// Provides returns the set of metrics this provider can supply.
	Provides() []Metric

	// Close releases any resources held by the provider (database
	// connections, open files, etc.). The engine calls Close when it
	// is finished with the provider.
	Close() error
}

// BatchProvider fetches historical data in bulk. Used during backtesting
// where the engine requests a complete time range upfront.
type BatchProvider interface {
	DataProvider

	// Fetch retrieves historical data for the given request. The returned
	// DataFrame contains all requested assets, metrics, and timestamps.
	Fetch(ctx context.Context, req DataRequest) (*DataFrame, error)
}

// StreamProvider delivers data in real-time. Used during live trading
// where the engine reacts to incoming market data. The engine manages
// subscriptions by cancelling the context and re-subscribing when the
// requested assets or metrics change.
type StreamProvider interface {
	DataProvider

	// Subscribe opens a real-time data stream. Each DataPoint is delivered
	// on the returned channel. The provider closes the channel when the
	// context is cancelled.
	Subscribe(ctx context.Context, req DataRequest) (<-chan DataPoint, error)
}

// IndexConstituent is an asset with its weight in an index at a point in time.
type IndexConstituent struct {
	Asset  asset.Asset
	Weight float64
}

// IndexProvider supplies index membership data. A provider that has
// access to historical index composition (e.g. S&P 500 additions and
// removals) implements this interface alongside BatchProvider or
// StreamProvider.
type IndexProvider interface {
	IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, []IndexConstituent, error)
}

// HolidayProvider supplies market holiday data. Providers that have
// access to a holiday calendar (database, snapshot file) implement this
// so the engine can initialize tradecron automatically during Backtest.
type HolidayProvider interface {
	FetchMarketHolidays(ctx context.Context) ([]tradecron.MarketHoliday, error)
}

// FundamentalsByDateKeyProvider is implemented by providers that can
// return fundamentals filtered to a specific reporting period (date_key).
// The engine type-asserts on this interface from FetchByDateKey.
type FundamentalsByDateKeyProvider interface {
	// FetchFundamentalsByDateKey returns one row per asset for the given
	// date_key + dimension. Only filings with event_date <= maxEventDate
	// are included (point-in-time correctness). For dimensions where a
	// single (figi, date_key) can have multiple filings (MR restatements),
	// the row with the maximum event_date wins.
	//
	// metrics must contain only fundamental metrics. Metadata metrics
	// (FundamentalsDateKey, FundamentalsReportPeriod) populate from the
	// row's date_key/report_period columns.
	FetchFundamentalsByDateKey(
		ctx context.Context,
		assets []asset.Asset,
		metrics []Metric,
		dateKey time.Time,
		dimension string,
		maxEventDate time.Time,
	) (*DataFrame, error)
}
