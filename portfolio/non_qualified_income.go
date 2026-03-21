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

import (
	"context"

	"github.com/penny-vault/pvbt/data"
)

type nonQualifiedIncome struct{}

func (nonQualifiedIncome) Name() string { return "NonQualifiedIncome" }

func (nonQualifiedIncome) Description() string {
	return "Non-qualified dividend income, taxed as ordinary income. Dividends are classified " +
		"as non-qualified when the position was held for 60 days or fewer before the dividend " +
		"date. Common sources include REIT distributions, short-term holdings, and bond fund " +
		"interest distributions."
}

func (nonQualifiedIncome) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	var total float64

	for _, tx := range stats.TransactionsView(ctx) {
		if tx.Type == DividendTransaction && !tx.Qualified {
			total += tx.Amount
		}
	}

	return total, nil
}

func (nonQualifiedIncome) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// NonQualifiedIncomeMetric is non-qualified dividend and interest income.
// Currently always returns 0 as no transaction type populates this.
var NonQualifiedIncomeMetric PerformanceMetric = nonQualifiedIncome{}
