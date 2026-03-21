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

import "github.com/penny-vault/pvbt/data"

type taxDrag struct{}

func (taxDrag) Name() string { return "TaxDrag" }

func (taxDrag) Description() string {
	return "Percentage of pre-tax return consumed by taxes from trading activity, excluding dividend taxation. Uses 25% for short-term gains and 15% for long-term gains."
}

func (taxDrag) Compute(acct *Account, _ *Period) (float64, error) {
	perfData := acct.PerfData()
	if perfData == nil {
		return 0, nil
	}

	ec := perfData.Column(portfolioAsset, data.PortfolioEquity)
	if len(ec) < 2 {
		return 0, nil
	}

	preTaxReturn := ec[len(ec)-1] - ec[0]
	if preTaxReturn <= 0 {
		return 0, nil
	}

	ltcg, stcg, _, _ := realizedGains(acct.Transactions())

	estimatedTax := 0.0
	if stcg > 0 {
		estimatedTax += 0.25 * stcg
	}

	if ltcg > 0 {
		estimatedTax += 0.15 * ltcg
	}

	return estimatedTax / preTaxReturn, nil
}

func (taxDrag) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// TaxDragMetric is the percentage of pre-tax return consumed by taxes from trading activity.
var TaxDragMetric PerformanceMetric = taxDrag{}
