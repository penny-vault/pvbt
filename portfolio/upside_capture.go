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

import "math"

type upsideCaptureRatio struct{}

func (upsideCaptureRatio) Name() string { return "UpsideCaptureRatio" }

func (upsideCaptureRatio) Compute(a *Account, window *Period) float64 {
	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)

	pRet := returns(eq)
	bRet := returns(bm)

	n := len(pRet)
	if len(bRet) < n {
		n = len(bRet)
	}

	// Filter periods where benchmark return > 0.
	var upP, upB []float64
	for i := 0; i < n; i++ {
		if bRet[i] > 0 {
			upP = append(upP, pRet[i])
			upB = append(upB, bRet[i])
		}
	}

	if len(upP) == 0 {
		return 0
	}

	geoP := geometricMean(upP)
	geoB := geometricMean(upB)

	if geoB == 0 {
		return 0
	}

	return (geoP / geoB) * 100
}

func (upsideCaptureRatio) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// geometricMean computes the geometric mean of returns:
// (product(1 + r_i))^(1/n) - 1
func geometricMean(r []float64) float64 {
	if len(r) == 0 {
		return 0
	}

	product := 1.0
	for _, v := range r {
		product *= (1 + v)
	}

	return math.Pow(product, 1.0/float64(len(r))) - 1
}

// UpsideCaptureRatio measures how much of the benchmark's positive
// returns the portfolio captures. Computed as portfolio return /
// benchmark return during periods when the benchmark is up. A ratio
// above 100% means the portfolio outperforms in rising markets.
var UpsideCaptureRatio PerformanceMetric = upsideCaptureRatio{}
