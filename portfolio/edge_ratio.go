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

type edgeRatio struct{}

func (edgeRatio) Name() string { return "EdgeRatio" }

func (edgeRatio) Description() string {
	return "Ratio of average MFE to the absolute value of average MAE across all " +
		"round-trip trades. Values above 1.0 indicate that favorable excursions " +
		"typically exceed adverse excursions, suggesting a positive trading edge."
}

func (edgeRatio) Compute(acct *Account, _ *Period) (float64, error) {
	trades := acct.TradeDetails()
	if len(trades) == 0 {
		return math.NaN(), nil
	}

	var sumMFE, sumMAE float64
	for _, trade := range trades {
		sumMFE += trade.MFE
		sumMAE += trade.MAE
	}

	tradeCount := float64(len(trades))
	avgMFE := sumMFE / tradeCount
	avgMAE := sumMAE / tradeCount

	if avgMAE == 0 {
		return math.NaN(), nil
	}

	return avgMFE / math.Abs(avgMAE), nil
}

func (edgeRatio) ComputeSeries(acct *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// EdgeRatio is the ratio of average MFE to the absolute value of average MAE.
var EdgeRatio PerformanceMetric = edgeRatio{}
