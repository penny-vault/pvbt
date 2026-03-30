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

package portfolio

import (
	"context"
	"errors"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

var (
	ErrNoRiskFreeRate        = errors.New("risk-free rate not configured")
	ErrNoBenchmark           = errors.New("benchmark not configured")
	ErrBenchmarkNotSupported = errors.New("metric does not support benchmark targeting")
)

// PortfolioStats provides read-only access to the data a performance metric
// needs to compute its value. Account implements this interface directly.
// The lazy-computed columns (Returns, ExcessReturns, Drawdown,
// BenchmarkReturns) are cached on Account and invalidated when new price
// rows arrive.
type PortfolioStats interface {
	Returns(ctx context.Context, window *Period) *data.DataFrame
	ExcessReturns(ctx context.Context, window *Period) *data.DataFrame
	Drawdown(ctx context.Context, window *Period) *data.DataFrame
	BenchmarkReturns(ctx context.Context, window *Period) *data.DataFrame
	EquitySeries(ctx context.Context, window *Period) *data.DataFrame
	TransactionsView(ctx context.Context) []Transaction
	TradeDetailsView(ctx context.Context) []TradeDetail
	PricesView(ctx context.Context) *data.DataFrame
	TaxLotsView(ctx context.Context) map[asset.Asset][]TaxLot
	ShortLotsView(ctx context.Context, fn func(asset.Asset, []TaxLot))
	PerfDataView(ctx context.Context) *data.DataFrame

	// AnnualReturns computes calendar-year returns for the given metric
	// column (e.g. PortfolioEquity, PortfolioBenchmark).
	AnnualReturns(metric data.Metric) ([]int, []float64, error)

	// DrawdownDetails returns the top-N drawdown periods sorted by depth.
	DrawdownDetails(topN int) ([]DrawdownDetail, error)

	// MonthlyReturns computes a year x month grid of returns for the
	// given metric column.
	MonthlyReturns(metric data.Metric) ([]int, [][]float64, error)
}

// BenchmarkTargetable is a marker interface for metrics that can be
// computed against a benchmark equity curve. Metrics that embed both
// portfolio and benchmark data (Beta, Alpha, etc.) or that rely on
// transaction history (WinRate, etc.) should NOT implement this.
type BenchmarkTargetable interface {
	PerformanceMetric
	BenchmarkTargetable()
}

// Rankable extends PerformanceMetric with sort-direction metadata for
// optimization ranking. Only metrics that implement Rankable can be used
// as optimization objectives. This forces any new rankable metric to
// explicitly declare its sort direction, preventing silent sorting bugs.
type Rankable interface {
	PerformanceMetric
	HigherIsBetter() bool
}

// PerformanceMetric is implemented by each metric type (Sharpe, Beta,
// etc.). Each implementation lives in its own file with an unexported
// struct and an exported package-level var. The PortfolioStats interface
// is passed to give metrics access to everything they might need: the
// transaction log, performance data (perfData DataFrame), and current
// positions. Anyone can define custom metrics by implementing this
// interface.
type PerformanceMetric interface {
	// Name returns a human-readable name for the metric (e.g. "Sharpe").
	Name() string

	// Description returns a short explanation of what the metric
	// measures and how to interpret its values.
	Description() string

	// Compute calculates a single scalar value for the metric. The
	// PortfolioStats provides access to transaction history, equity curve,
	// benchmark data, and risk-free data. If window is nil, the full
	// history is used.
	Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error)

	// ComputeSeries calculates a rolling time series of the metric.
	// If window is nil, the full history is used.
	ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error)
}

// PerformanceMetricQuery is the builder returned by
// Portfolio.PerformanceMetric(). It captures which metric to compute
// and an optional window, then executes the computation on Value() or
// Series().
type PerformanceMetricQuery struct {
	account   *Account
	metric    PerformanceMetric
	window    *Period
	benchmark bool
	absStart  *time.Time
	absEnd    *time.Time
}

// Window sets the lookback period for the metric computation.
func (query PerformanceMetricQuery) Window(period Period) PerformanceMetricQuery {
	query.window = &period
	return query
}

// Benchmark tells the query to compute the metric against the benchmark
// equity curve instead of the portfolio equity curve.
func (query PerformanceMetricQuery) Benchmark() PerformanceMetricQuery {
	query.benchmark = true
	return query
}

// AbsoluteWindow restricts the metric computation to the inclusive date
// range [start, end]. The underlying PortfolioStats is wrapped in a
// windowedStats that filters all DataFrame-returning methods to this range.
func (query PerformanceMetricQuery) AbsoluteWindow(start, end time.Time) PerformanceMetricQuery {
	query.absStart = &start
	query.absEnd = &end

	return query
}

// Value computes and returns a single scalar value for the metric.
func (query PerformanceMetricQuery) Value() (float64, error) {
	stats, err := query.resolveStats()
	if err != nil {
		return 0, err
	}

	return query.metric.Compute(context.Background(), stats, query.window)
}

// Series computes and returns a rolling time series for the metric.
func (query PerformanceMetricQuery) Series() (*data.DataFrame, error) {
	stats, err := query.resolveStats()
	if err != nil {
		return nil, err
	}

	return query.metric.ComputeSeries(context.Background(), stats, query.window)
}

// resolveStats returns the PortfolioStats to compute against. When the
// benchmark flag is set it verifies the metric supports benchmark
// targeting, then returns a view account with the benchmark equity curve.
// When absStart/absEnd are set, the returned stats are wrapped in a
// windowedStats that restricts DataFrames to the given date range.
func (query PerformanceMetricQuery) resolveStats() (PortfolioStats, error) {
	var stats PortfolioStats

	if !query.benchmark {
		stats = query.account
	} else {
		if _, ok := query.metric.(BenchmarkTargetable); !ok {
			return nil, ErrBenchmarkNotSupported
		}

		bv, err := query.account.benchmarkView()
		if err != nil {
			return nil, err
		}

		stats = bv
	}

	if query.absStart != nil && query.absEnd != nil {
		stats = newWindowedStats(stats, *query.absStart, *query.absEnd)
	}

	return stats, nil
}
