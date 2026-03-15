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

// Package data provides the core types for working with financial time-series
// data in pvbt. It is built around three concepts:
//
//   - Metrics -- externally-sourced measurements such as prices, volumes, and
//     fundamental accounting figures.
//   - Data providers -- adapters that connect to external sources (databases,
//     APIs, live feeds) and deliver metrics to the engine.
//   - DataFrames -- the primary data structure for time-series data, stored in
//     column-major layout with gonum-compatible []float64 columns.
//
// # Metrics
//
// A [Metric] is a named type backed by string that identifies a single
// measurement. The package declares well-known metrics for each data domain:
//
//   - End-of-day prices: [MetricOpen], [MetricHigh], [MetricLow], [MetricClose],
//     [AdjClose], [Volume], [Dividend], [SplitFactor]
//   - Live prices: [Price], [Bid], [Ask]
//   - Fundamentals: [Revenue], [NetIncome], [EarningsPerShare], [TotalDebt],
//     [TotalAssets], [FreeCashFlow], [BookValue]
//   - Valuation: [MarketCap], [EnterpriseValue], [PE], [PB], [PS]
//   - Ratios: [GrossMargin], [ProfitMargin], [ROA], [ROE], [DebtToEquity],
//     [DividendYield]
//
// Custom metrics can be created by converting any string to a [Metric]:
//
//	custom := data.Metric("MySignal")
//
// Economic indicators such as unemployment and CPI are not tied to a specific
// asset. They use the sentinel asset.EconomicIndicator in requests and
// DataFrames. From the DataFrame's perspective they look like any other asset;
// the data layout stays uniform.
//
// # Data Providers
//
// The [DataProvider] interface is the base for all providers. Every provider
// must declare the metrics it can supply via Provides and release resources
// via Close.
//
// Two specializations exist:
//
//   - [BatchProvider] -- fetches historical data in bulk via Fetch. Used during
//     backtesting where the engine requests a complete time range upfront.
//   - [StreamProvider] -- delivers data in real-time via Subscribe. Used during
//     live trading where the engine reacts to incoming market data.
//
// Provider lifecycle:
//
//  1. The caller constructs a provider (e.g. connecting to a database).
//  2. The caller registers it with the engine via WithDataProvider.
//  3. The engine routes [DataRequest] values to providers based on the metrics
//     each provider advertises.
//  4. The engine calls Close when it is finished with the provider.
//
// [AssetProvider] supplies asset metadata. The engine bulk-loads all known
// assets at startup via Assets and uses LookupAsset as a fallback for cache
// misses.
//
// The DataSource interface (defined in the universe package) decouples the
// universe from the engine. It exposes Fetch, FetchAt, and CurrentDate so
// that universe implementations can obtain data without depending on the
// engine type directly.
//
// [IndexProvider] supplies historical index membership. A provider that has
// access to historical index composition (e.g. S&P 500 additions and
// removals) implements this interface alongside [BatchProvider] or
// [StreamProvider]. The IndexMembers method returns the list of assets that
// belonged to the index at a given point in time.
//
// [DataRequest] describes a batch of data to fetch. It specifies the assets,
// metrics, time range, and [Frequency].
//
// # Time Zones
//
// Timestamps returned by a [BatchProvider] must use the same time zone as the
// tradecron schedule driving the backtest. Market-aware tradecron directives
// (@monthend, @open, etc.) produce America/New_York timestamps. If the
// provider returns UTC timestamps instead, the engine's time-range filtering
// will silently produce empty DataFrames and no trades will execute. See the
// tradecron package documentation for details.
//
// # DataFrame
//
// [DataFrame] stores contiguous float64 data in column-major order. Each
// column represents a single (asset, metric) pair across all timestamps.
// Columns are stored contiguously so that time-series operations can work on
// a single uninterrupted []float64 slice. The [DataFrame.Column] method
// returns the raw slice, which is directly usable with gonum routines.
//
// For a frame with T timestamps, A assets, and M metrics the total length
// of the internal data slab is T*A*M. The column for asset index a and metric
// index m starts at offset (a*M + m) * T and runs for T elements.
//
// # Accessors
//
// [DataFrame.Start], [DataFrame.End], [DataFrame.Duration], and [DataFrame.Len]
// describe the time axis. [DataFrame.ColCount] returns the total number of
// columns (assets * metrics). [DataFrame.Value] returns the last value for an
// (asset, metric) pair and [DataFrame.ValueAt] returns the value at a specific
// time. [DataFrame.Column] returns the full []float64 slice for a column.
// [DataFrame.At] narrows to a single timestamp and [DataFrame.Last] narrows to
// the final row. [DataFrame.Copy] produces a deep copy. [DataFrame.Table]
// returns a human-readable text table.
//
// # Narrowing and Filtering
//
// [DataFrame.Assets] restricts the frame to a subset of assets.
// [DataFrame.Metrics] restricts to a subset of metrics. [DataFrame.Between]
// restricts to a time range. [DataFrame.Drop] removes rows where any column
// matches a sentinel value. [DataFrame.Filter] keeps only rows for which a
// predicate returns true.
//
// # Arithmetic
//
// Element-wise operations between two DataFrames:
//
//   - [DataFrame.Add], [DataFrame.Sub], [DataFrame.Mul], [DataFrame.Div]
//
// Scalar operations applied to every element:
//
//   - [DataFrame.AddScalar], [DataFrame.SubScalar], [DataFrame.MulScalar],
//     [DataFrame.DivScalar]
//
// # Per-Column Aggregation
//
// Each method collapses the time axis to a single row:
//
//   - [DataFrame.Mean], [DataFrame.Sum], [DataFrame.Max], [DataFrame.Min],
//     [DataFrame.Variance], [DataFrame.Std], [DataFrame.Covariance]
//
// # Cross-Asset Aggregation
//
// These methods collapse the asset axis:
//
//   - [DataFrame.MaxAcrossAssets] -- per-timestamp maximum across assets
//   - [DataFrame.MinAcrossAssets] -- per-timestamp minimum across assets
//   - [DataFrame.IdxMaxAcrossAssets] -- per-timestamp asset with the maximum value
//
// # Transforms
//
// Each method returns a new [DataFrame] with the transformation applied to
// every column:
//
//   - [DataFrame.Pct] -- percentage change (optionally over n periods)
//   - [DataFrame.Diff] -- first difference
//   - [DataFrame.Log] -- natural logarithm
//   - [DataFrame.CumSum] -- cumulative sum
//   - [DataFrame.CumMax] -- cumulative maximum
//   - [DataFrame.Shift] -- shift values forward or backward by n rows
//
// # Resampling
//
// [DataFrame.Downsample] groups timestamps by the target frequency and returns
// a [DownsampledDataFrame]. Aggregation methods on the result include Last,
// Sum, Max, Min, Mean, First, Std, and Variance.
//
// [DataFrame.Upsample] fills gaps when converting to a higher frequency and
// returns an [UpsampledDataFrame]. Fill strategies include ForwardFill,
// BackFill, and Interpolate.
//
// # Rolling Windows
//
// [DataFrame.Rolling] returns a [RollingDataFrame] that applies rolling-window
// operations to each column:
//
//   - [RollingDataFrame.Mean], [RollingDataFrame.Sum], [RollingDataFrame.Max],
//     [RollingDataFrame.Min], [RollingDataFrame.Std], [RollingDataFrame.Variance],
//     [RollingDataFrame.Percentile]
//
// # Extensibility
//
// [DataFrame.Apply] maps a function over each column, returning a new
// [DataFrame]. [DataFrame.Reduce] collapses each column to a single value.
// For direct gonum usage, call [DataFrame.Column] to obtain the raw []float64
// slice and pass it to any gonum function.
//
// # Chaining
//
// All operations that produce a new [DataFrame] return a pointer, so calls
// chain naturally:
//
//	pct := df.Assets(spy).Metrics(data.MetricClose).Pct(1)
//
// If any step encounters an error the resulting [DataFrame] carries the error
// (retrievable via [DataFrame.Err]) and subsequent operations short-circuit.
//
// # Annotations
//
// [DataFrame.Annotate] pushes every non-NaN cell as a key-value annotation to
// an [Annotator] destination. Keys are formatted as "TICKER/Metric" and values
// are the float formatted as a string. This allows a strategy to annotate its
// reasoning with a single call:
//
//	momentumDF.Annotate(portfolio)
//
// The [Annotator] interface is intentionally narrow (a single Annotate method)
// to avoid a circular dependency with the portfolio package. The portfolio
// [Portfolio] interface satisfies it.
//
// # Frequency
//
// The [Frequency] type represents data publication frequency. Constants
// range from [Tick] (sub-second) through [Daily], [Weekly], [Monthly],
// [Quarterly], and [Yearly]. Frequency is used in [DataRequest], resampling,
// and upsampling operations.
package data
