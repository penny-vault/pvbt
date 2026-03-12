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

type downsideCaptureRatio struct{}

func (downsideCaptureRatio) Name() string { return "DownsideCaptureRatio" }

func (downsideCaptureRatio) Description() string {
	return "Measures how much of the benchmark's downside the portfolio experiences. A value of 0.8 means the portfolio loses only 80% of the benchmark's loss during down periods. Lower is better."
}

func (downsideCaptureRatio) Compute(a *Account, window *Period) (float64, error) {
	if len(a.BenchmarkPrices()) == 0 {
		return 0, ErrNoBenchmark
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)

	pRet := returns(eq)
	bRet := returns(bm)

	n := len(pRet)
	if len(bRet) < n {
		n = len(bRet)
	}

	// Filter periods where benchmark return < 0.
	var downP, downB []float64
	for i := 0; i < n; i++ {
		if bRet[i] < 0 {
			downP = append(downP, pRet[i])
			downB = append(downB, bRet[i])
		}
	}

	if len(downP) == 0 {
		return 0, nil
	}

	geoP := geometricMean(downP)
	geoB := geometricMean(downB)

	if geoB == 0 {
		return 0, nil
	}

	return geoP / geoB, nil
}

func (downsideCaptureRatio) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// DownsideCaptureRatio measures how much of the benchmark's negative
// returns the portfolio captures. Computed as portfolio geometric mean
// return / benchmark geometric mean return during down periods. A ratio
// below 1.0 means the portfolio loses less than the benchmark in
// falling markets.
var DownsideCaptureRatio PerformanceMetric = downsideCaptureRatio{}
