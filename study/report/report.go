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

package report

import (
	"time"

	"github.com/penny-vault/pvbt/portfolio"
)

// ReportablePortfolio is the interface required by the summary builder.
// It composes read-only portfolio access with statistical queries needed
// for the full report.
type ReportablePortfolio interface {
	portfolio.Portfolio
	portfolio.PortfolioStats
}

// ---------------------------------------------------------------------------
// View model types -- plain structs shared by builders and renderers.
// ---------------------------------------------------------------------------

// Header contains the headline information for the report.
type Header struct {
	StrategyName    string
	StrategyVersion string
	Benchmark       string
	StartDate       time.Time
	EndDate         time.Time
	InitialCash     float64
	FinalValue      float64
	Elapsed         time.Duration
	Steps           int
}

// EquityCurve holds the time series for the strategy and benchmark equity.
type EquityCurve struct {
	Times           []time.Time
	StrategyValues  []float64
	BenchmarkValues []float64 // normalized to InitialCash
}

// ReturnTable holds return figures for named periods.
type ReturnTable struct {
	SectionName string
	AsOf        time.Time
	Periods     []string
	Strategy    []float64
	Benchmark   []float64
}

// AnnualReturns holds year-by-year return figures.
type AnnualReturns struct {
	Years     []int
	Strategy  []float64
	Benchmark []float64
}

// Risk holds paired (strategy, benchmark) risk metrics.
// Each [2]float64 is {Strategy, Benchmark}.
type Risk struct {
	HasBenchmark      bool
	MaxDrawdown       [2]float64
	Volatility        [2]float64
	DownsideDeviation [2]float64
	Sharpe            [2]float64
	Sortino           [2]float64
	Calmar            [2]float64
	UlcerIndex        [2]float64
	ValueAtRisk       [2]float64
	Skewness          [2]float64
	ExcessKurtosis    [2]float64
}

// RiskVsBenchmark holds relative-risk metrics that only make sense when
// a benchmark is configured.
type RiskVsBenchmark struct {
	Beta             float64
	Alpha            float64
	RSquared         float64
	TrackingError    float64
	InformationRatio float64
	Treynor          float64
	UpsideCapture    float64
	DownsideCapture  float64
}

// DrawdownEntry describes a single drawdown episode.
type DrawdownEntry struct {
	Start    time.Time
	End      time.Time
	Recovery time.Time
	Depth    float64
	Days     int
}

// Drawdowns holds the top drawdown episodes.
type Drawdowns struct {
	Entries []DrawdownEntry
}

// MonthlyReturns holds a year x month grid of returns.
type MonthlyReturns struct {
	Years  []int
	Values [][]float64 // [yearIdx][monthIdx], NaN for missing
}

// TradeEntry describes a single trade.
type TradeEntry struct {
	Date   time.Time
	Action string
	Ticker string
	Shares float64
	Price  float64
	Amount float64
}

// Trades holds trade analysis data.
type Trades struct {
	TotalTransactions int
	RoundTrips        int
	WinRate           float64
	AvgHolding        float64
	AvgWin            float64
	AvgLoss           float64
	ProfitFactor      float64
	GainLossRatio     float64
	Turnover          float64
	PositivePeriods   float64
	Trades            []TradeEntry
}
