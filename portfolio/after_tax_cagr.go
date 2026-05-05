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
	"math"

	"github.com/penny-vault/pvbt/data"
)

type afterTaxCAGR struct{}

func (afterTaxCAGR) Name() string { return "AfterTaxCAGR" }

func (afterTaxCAGR) Description() string {
	return "Compound annual growth rate computed on an after-tax equity curve. Realized capital gains within the window are taxed via FIFO lot matching at 15% (long-term) or 25% (short-term), then subtracted from the equity curve before annualizing the start-to-end return. Dividend taxation is excluded, matching TaxDrag's accounting."
}

func (afterTaxCAGR) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, ErrInsufficientData
	}

	equity := df.Column(portfolioAsset, data.PortfolioEquity)
	times := df.Times()

	if len(equity) < 2 || len(times) < 2 {
		return 0, ErrInsufficientData
	}

	start, end := windowBounds(ctx, stats, window)
	txns := stats.TransactionsView(ctx)
	adjusted := afterTaxEquity(equity, times, txns, start, end, defaultTaxRates())

	if adjusted == nil || adjusted[0] <= 0 {
		return 0, ErrInsufficientData
	}

	years := times[len(times)-1].Sub(times[0]).Hours() / 24 / 365.25
	if years <= 0 {
		return 0, ErrInsufficientData
	}

	flows := buildFlowMap(txns, times[0], times[len(times)-1])
	returns := subPeriodReturns(adjusted, times, flows)

	growth := 1.0
	for _, ri := range returns {
		growth *= 1 + ri
	}

	if growth <= 0 {
		return 0, ErrInsufficientData
	}

	return math.Pow(growth, 1.0/years) - 1, nil
}

func (afterTaxCAGR) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

func (afterTaxCAGR) HigherIsBetter() bool { return true }

// AfterTaxCAGR is the annualized compound growth rate on the portfolio's
// after-tax equity curve. Use BenchmarkAfterTaxCAGR for the equivalent
// benchmark figure.
var AfterTaxCAGR PerformanceMetric = afterTaxCAGR{}
