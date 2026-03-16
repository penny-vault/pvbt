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

import "errors"

var (
	ErrNoRiskFreeRate       = errors.New("risk-free rate not configured")
	ErrNoBenchmark          = errors.New("benchmark not configured")
	ErrBenchmarkNotSupported = errors.New("metric does not support benchmark targeting")
)

// BenchmarkTargetable is a marker interface for metrics that can be
// computed against a benchmark equity curve. Metrics that embed both
// portfolio and benchmark data (Beta, Alpha, etc.) or that rely on
// transaction history (WinRate, etc.) should NOT implement this.
type BenchmarkTargetable interface {
	PerformanceMetric
	BenchmarkTargetable()
}

// PerformanceMetric is implemented by each metric type (Sharpe, Beta,
// etc.). Each implementation lives in its own file with an unexported
// struct and an exported package-level var. The Account is passed to
// give metrics access to everything they might need: the transaction
// log, performance data (perfData DataFrame), and current positions.
// Anyone can define custom metrics by implementing this interface.
type PerformanceMetric interface {
	// Name returns a human-readable name for the metric (e.g. "Sharpe").
	Name() string

	// Description returns a short explanation of what the metric
	// measures and how to interpret its values.
	Description() string

	// Compute calculates a single scalar value for the metric. The
	// Account provides access to transaction history, equity curve,
	// benchmark data, and risk-free data. If window is nil, the full
	// history is used.
	Compute(a *Account, window *Period) (float64, error)

	// ComputeSeries calculates a rolling time series of the metric.
	// If window is nil, the full history is used.
	ComputeSeries(a *Account, window *Period) ([]float64, error)
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
}

// Window sets the lookback period for the metric computation.
func (q PerformanceMetricQuery) Window(p Period) PerformanceMetricQuery {
	q.window = &p
	return q
}

// Benchmark tells the query to compute the metric against the benchmark
// equity curve instead of the portfolio equity curve.
func (q PerformanceMetricQuery) Benchmark() PerformanceMetricQuery {
	q.benchmark = true
	return q
}

// Value computes and returns a single scalar value for the metric.
func (q PerformanceMetricQuery) Value() (float64, error) {
	account, err := q.resolveAccount()
	if err != nil {
		return 0, err
	}

	return q.metric.Compute(account, q.window)
}

// Series computes and returns a rolling time series for the metric.
func (q PerformanceMetricQuery) Series() ([]float64, error) {
	account, err := q.resolveAccount()
	if err != nil {
		return nil, err
	}

	return q.metric.ComputeSeries(account, q.window)
}

// resolveAccount returns the account to compute against. When the
// benchmark flag is set it verifies the metric supports benchmark
// targeting, then returns a view account with the benchmark equity curve.
func (q PerformanceMetricQuery) resolveAccount() (*Account, error) {
	if !q.benchmark {
		return q.account, nil
	}

	if _, ok := q.metric.(BenchmarkTargetable); !ok {
		return nil, ErrBenchmarkNotSupported
	}

	return q.account.benchmarkView()
}
