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

type excessKurtosis struct{}

func (excessKurtosis) Name() string { return "ExcessKurtosis" }

func (excessKurtosis) Compute(a *Account, window *Period) float64 {
	prices := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(prices)

	n := len(r)
	if n < 4 {
		return 0
	}

	s := stddev(r)
	if s == 0 {
		return 0
	}

	m := mean(r)
	sum := 0.0
	for _, v := range r {
		d := v - m
		sum += d * d * d * d
	}

	return sum/float64(n)/(s*s*s*s) - 3
}

func (excessKurtosis) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// ExcessKurtosis measures tail risk -- how much fatter the tails of
// the return distribution are compared to a normal distribution.
// Positive values indicate heavier tails (more extreme outcomes than
// a normal distribution would predict).
var ExcessKurtosis = excessKurtosis{}
