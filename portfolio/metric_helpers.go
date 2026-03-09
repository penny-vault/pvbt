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
	"math"
	"time"
)

// returns computes period-over-period returns from a price series.
// Returns empty slice for single-element input.
func returns(prices []float64) []float64 {
	if len(prices) < 2 {
		return []float64{}
	}

	r := make([]float64, len(prices)-1)
	for i := 0; i < len(prices)-1; i++ {
		r[i] = (prices[i+1] - prices[i]) / prices[i]
	}

	return r
}

// excessReturns subtracts risk-free returns from portfolio returns element-wise.
func excessReturns(r, rf []float64) []float64 {
	n := len(r)
	if len(rf) < n {
		n = len(rf)
	}

	er := make([]float64, n)
	for i := 0; i < n; i++ {
		er[i] = r[i] - rf[i]
	}

	return er
}

// windowSlice trims a series to the trailing window based on timestamps.
// If window is nil, returns the full series.
func windowSlice(series []float64, times []time.Time, window *Period) []float64 {
	if window == nil {
		return series
	}

	if len(times) == 0 {
		return series
	}

	last := times[len(times)-1]

	var cutoff time.Time
	switch window.Unit {
	case UnitDay:
		cutoff = last.AddDate(0, 0, -window.N)
	case UnitMonth:
		cutoff = last.AddDate(0, -window.N, 0)
	case UnitYear:
		cutoff = last.AddDate(-window.N, 0, 0)
	}

	for i, t := range times {
		if !t.Before(cutoff) {
			return series[i:]
		}
	}

	return series
}

// mean computes the arithmetic mean.
func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range x {
		sum += v
	}

	return sum / float64(len(x))
}

// variance computes the sample variance (using N-1 denominator).
func variance(x []float64) float64 {
	n := len(x)
	if n < 2 {
		return 0
	}

	m := mean(x)
	sum := 0.0
	for _, v := range x {
		d := v - m
		sum += d * d
	}

	return sum / float64(n-1)
}

// stddev computes the sample standard deviation (using N-1 denominator).
func stddev(x []float64) float64 {
	return math.Sqrt(variance(x))
}

// covariance computes the sample covariance between x and y.
func covariance(x, y []float64) float64 {
	n := len(x)
	if len(y) < n {
		n = len(y)
	}

	if n < 2 {
		return 0
	}

	mx := mean(x[:n])
	my := mean(y[:n])

	sum := 0.0
	for i := 0; i < n; i++ {
		sum += (x[i] - mx) * (y[i] - my)
	}

	return sum / float64(n-1)
}

// cagr computes the compound annual growth rate.
func cagr(startValue, endValue, years float64) float64 {
	if startValue <= 0 || endValue <= 0 || years <= 0 {
		return 0
	}

	return math.Pow(endValue/startValue, 1.0/years) - 1
}

// drawdownSeries computes the drawdown at each point in the equity curve.
// Values are negative (or zero at peaks).
func drawdownSeries(equity []float64) []float64 {
	dd := make([]float64, len(equity))
	peak := math.Inf(-1)

	for i, v := range equity {
		if v > peak {
			peak = v
		}
		dd[i] = (v - peak) / peak
	}

	return dd
}
