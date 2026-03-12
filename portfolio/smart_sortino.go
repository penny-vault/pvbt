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

type smartSortino struct{}

func (smartSortino) Name() string { return "SmartSortino" }

func (smartSortino) Description() string {
	return "Sortino ratio penalized for autocorrelation in returns. Applies the same Lo (2002) correction as SmartSharpe but to the Sortino ratio. Lower than Sortino when returns exhibit positive autocorrelation."
}

func (smartSortino) Compute(a *Account, window *Period) (float64, error) {
	if len(a.RiskFreePrices()) == 0 {
		return 0, ErrNoRiskFreeRate
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(eq)
	rf := returns(windowSlice(a.RiskFreePrices(), a.EquityTimes(), window))
	er := excessReturns(r, rf)

	n := len(er)
	if n == 0 {
		return 0, nil
	}

	sumSq := 0.0
	for _, v := range er {
		if v < 0 {
			sumSq += v * v
		}
	}

	dd := math.Sqrt(sumSq / float64(n))
	if dd == 0 {
		return 0, nil
	}

	af := annualizationFactor(a.EquityTimes())
	rawSortino := mean(er) / dd * math.Sqrt(af)

	penalty := autocorrelationPenalty(er)
	if penalty == 0 {
		return 0, nil
	}

	return rawSortino / penalty, nil
}

func (smartSortino) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// SmartSortino is the Sortino ratio corrected for autocorrelation in
// returns using the Lo (2002) method. It divides the standard Sortino
// by the same penalty factor used by SmartSharpe.
var SmartSortino PerformanceMetric = smartSortino{}
