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
	"math"
	"time"
)

// SliceMean computes the arithmetic mean of a float64 slice.
// Returns 0 for empty or nil input.
func SliceMean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range x {
		sum += v
	}
	return sum / float64(len(x))
}

// Variance computes the sample variance (N-1 denominator).
// Returns 0 for fewer than 2 elements.
func Variance(x []float64) float64 {
	n := len(x)
	if n < 2 {
		return 0
	}
	m := SliceMean(x)
	sum := 0.0
	for _, v := range x {
		d := v - m
		sum += d * d
	}
	return sum / float64(n-1)
}

// Stddev computes the sample standard deviation (N-1 denominator).
func Stddev(x []float64) float64 {
	return math.Sqrt(Variance(x))
}

// Covariance computes the sample covariance between x and y.
// Trims to the shorter of the two slices. Returns 0 for fewer than 2 pairs.
func Covariance(x, y []float64) float64 {
	n := len(x)
	if len(y) < n {
		n = len(y)
	}
	if n < 2 {
		return 0
	}
	mx := SliceMean(x[:n])
	my := SliceMean(y[:n])
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += (x[i] - mx) * (y[i] - my)
	}
	return sum / float64(n-1)
}

// AnnualizationFactor estimates periods-per-year from timestamps.
// If the average gap exceeds 20 calendar days, returns 12 (monthly);
// otherwise returns 252 (daily). Defaults to 252 for fewer than 2 timestamps.
func AnnualizationFactor(times []time.Time) float64 {
	if len(times) < 2 {
		return 252
	}
	avgDays := times[len(times)-1].Sub(times[0]).Hours() / 24 / float64(len(times)-1)
	if avgDays > 20 {
		return 12
	}
	return 252
}

// PeriodsReturns computes period-over-period returns from a price series.
// Returns a slice of length len(prices)-1. Returns empty slice for fewer than 2 prices.
func PeriodsReturns(prices []float64) []float64 {
	if len(prices) < 2 {
		return []float64{}
	}
	r := make([]float64, len(prices)-1)
	for i := 0; i < len(prices)-1; i++ {
		r[i] = (prices[i+1] - prices[i]) / prices[i]
	}
	return r
}
