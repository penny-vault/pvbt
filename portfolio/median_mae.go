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

import "sort"

type medianMAE struct{}

func (medianMAE) Name() string { return "MedianMAE" }

func (medianMAE) Description() string {
	return "Median Maximum Adverse Excursion (MAE) across all round-trip trades, " +
		"expressed as a fraction of entry price. Values are typically negative; " +
		"the median is more robust to outliers than the mean."
}

func (medianMAE) Compute(acct *Account, _ *Period) (float64, error) {
	trades := acct.TradeDetails()
	if len(trades) == 0 {
		return 0, nil
	}

	values := make([]float64, len(trades))
	for idx, trade := range trades {
		values[idx] = trade.MAE
	}

	sort.Float64s(values)

	count := len(values)
	if count%2 == 0 {
		return (values[count/2-1] + values[count/2]) / 2, nil
	}

	return values[count/2], nil
}

func (medianMAE) ComputeSeries(acct *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// MedianMAE is the median Maximum Adverse Excursion across all round-trip trades.
var MedianMAE PerformanceMetric = medianMAE{}
