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

package summary

import "time"

// header contains the headline information for the report.
type header struct {
	strategyName    string
	strategyVersion string
	benchmark       string
	startDate       time.Time
	endDate         time.Time
	initialCash     float64
	finalValue      float64
	elapsed         time.Duration
	steps           int
}

// equityCurve holds the time series for the strategy and benchmark equity.
type equityCurve struct {
	times           []time.Time
	strategyValues  []float64
	benchmarkValues []float64 // normalized to initialCash
}

// returnTable holds return figures for named periods.
type returnTable struct {
	sectionName string
	asOf        time.Time
	periods     []string
	strategy    []float64
	benchmark   []float64
}

// annualReturns holds year-by-year return figures.
type annualReturns struct {
	years     []int
	strategy  []float64
	benchmark []float64
}

// risk holds paired (strategy, benchmark) risk metrics.
// Each [2]float64 is {strategy, benchmark}.
type risk struct {
	hasBenchmark      bool
	maxDrawdown       [2]float64
	volatility        [2]float64
	downsideDeviation [2]float64
	sharpe            [2]float64
	sortino           [2]float64
	calmar            [2]float64
	ulcerIndex        [2]float64
	valueAtRisk       [2]float64
	skewness          [2]float64
	excessKurtosis    [2]float64
}

// riskVsBenchmark holds relative-risk metrics that only make sense when
// a benchmark is configured.
type riskVsBenchmark struct {
	beta             float64
	alpha            float64
	rSquared         float64
	trackingError    float64
	informationRatio float64
	treynor          float64
	upsideCapture    float64
	downsideCapture  float64
}

// drawdownEntry describes a single drawdown episode.
type drawdownEntry struct {
	start    time.Time
	end      time.Time
	recovery time.Time
	depth    float64
	days     int
}

// drawdowns holds the top drawdown episodes.
type drawdowns struct {
	entries []drawdownEntry
}

// monthlyReturns holds a year x month grid of returns.
type monthlyReturns struct {
	years  []int
	values [][]float64 // [yearIdx][monthIdx], NaN for missing
}

// tradeEntry describes a single trade.
type tradeEntry struct {
	date   time.Time
	action string
	ticker string
	shares float64
	price  float64
	amount float64
}

// trades holds trade analysis data.
type trades struct {
	totalTransactions int
	roundTrips        int
	winRate           float64
	avgHolding        float64
	avgWin            float64
	avgLoss           float64
	profitFactor      float64
	gainLossRatio     float64
	turnover          float64
	positivePeriods   float64
	tradeList         []tradeEntry
}
