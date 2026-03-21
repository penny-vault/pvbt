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

type averageMFE struct{}

func (averageMFE) Name() string { return "AverageMFE" }

func (averageMFE) Description() string {
	return "Mean Maximum Favorable Excursion (MFE) across all round-trip trades, " +
		"expressed as a fraction of entry price. Higher values indicate the strategy " +
		"captures larger upside moves on average."
}

func (averageMFE) Compute(acct *Account, _ *Period) (float64, error) {
	trades := acct.TradeDetails()
	if len(trades) == 0 {
		return 0, nil
	}

	var sumMFE float64
	for _, trade := range trades {
		sumMFE += trade.MFE
	}

	return sumMFE / float64(len(trades)), nil
}

func (averageMFE) ComputeSeries(acct *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// AverageMFE is the mean Maximum Favorable Excursion across all round-trip trades.
var AverageMFE PerformanceMetric = averageMFE{}
