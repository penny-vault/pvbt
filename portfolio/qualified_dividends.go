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

type qualifiedDividends struct{}

func (qualifiedDividends) Name() string { return "QualifiedDividends" }

func (qualifiedDividends) Description() string {
	return "Total qualified dividend income received. Qualified dividends are taxed at preferential capital gains rates rather than ordinary income rates."
}

func (qualifiedDividends) Compute(ctx context.Context, stats PortfolioStats, _ *Period) (float64, error) {
	var total float64

	for _, tx := range stats.TransactionsView(ctx) {
		if tx.Type == DividendTransaction && tx.Qualified {
			total += tx.Amount
		}
	}

	return total, nil
}

func (qualifiedDividends) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// QualifiedDividendsMetric is the total qualified dividend income received.
var QualifiedDividendsMetric PerformanceMetric = qualifiedDividends{}
