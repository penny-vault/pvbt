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

type nPositivePeriods struct{}

func (nPositivePeriods) Name() string { return "NPositivePeriods" }

func (nPositivePeriods) Description() string {
	return "Fraction of periods with positive equity curve returns. A value of 0.55 means 55% of periods had positive returns. Combined with GainLossRatio, gives a complete picture of the return distribution's win/loss profile."
}

func (nPositivePeriods) Compute(a *Account, window *Period) (float64, error) {
	prices := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(prices)

	if len(r) == 0 {
		return 0, nil
	}

	count := 0
	for _, v := range r {
		if v > 0 {
			count++
		}
	}

	return float64(count) / float64(len(r)), nil
}

func (nPositivePeriods) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// NPositivePeriods is the percentage of periods with positive returns.
// A higher value indicates the portfolio gains more often than it
// loses, though it says nothing about the magnitude of gains vs losses.
var NPositivePeriods PerformanceMetric = nPositivePeriods{}
