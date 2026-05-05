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

type afterTaxTWRR struct{}

func (afterTaxTWRR) Name() string { return "AfterTaxTWRR" }

func (afterTaxTWRR) Description() string {
	return "Cumulative time-weighted rate of return computed on an after-tax equity curve. Realized capital gains within the window are taxed via FIFO lot matching at 15% (long-term, held > 365 days) or 25% (short-term), then subtracted from the equity curve before geometrically linking sub-period returns. Dividend taxation is excluded, matching TaxDrag's accounting."
}

func (afterTaxTWRR) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, ErrInsufficientData
	}

	equity := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(equity) < 2 {
		return 0, ErrInsufficientData
	}

	times := df.Times()
	start, end := windowBounds(ctx, stats, window)
	txns := stats.TransactionsView(ctx)

	adjusted := afterTaxEquity(equity, times, txns, start, end, defaultTaxRates())
	if adjusted == nil {
		return 0, ErrInsufficientData
	}

	flows := buildFlowMap(txns, times[0], times[len(times)-1])
	returns := subPeriodReturns(adjusted, times, flows)

	product := 1.0
	for _, ri := range returns {
		product *= (1 + ri)
	}

	return product - 1, nil
}

func (afterTaxTWRR) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// AfterTaxTWRR is the cumulative time-weighted rate of return on the
// portfolio's after-tax equity curve. Use BenchmarkAfterTaxTWRR for the
// equivalent benchmark figure.
var AfterTaxTWRR PerformanceMetric = afterTaxTWRR{}
