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

type cagrMetric struct{}

func (cagrMetric) Name() string { return "CAGR" }

func (cagrMetric) Description() string {
	return "Compound Annual Growth Rate. The annualized rate of return that would produce the same total return if compounded each year. The standard metric for comparing returns across strategies with different time horizons."
}

func (cagrMetric) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, ErrInsufficientData
	}

	eqCol := df.Column(portfolioAsset, data.PortfolioEquity)
	eqTimes := df.Times()

	if len(eqCol) < 2 || len(eqTimes) < 2 {
		return 0, ErrInsufficientData
	}

	years := eqTimes[len(eqTimes)-1].Sub(eqTimes[0]).Hours() / 24 / 365.25
	if years <= 0 || eqCol[0] <= 0 || eqCol[len(eqCol)-1] <= 0 {
		return 0, ErrInsufficientData
	}

	// Compound flow-adjusted sub-period returns the same way TWRR does, then
	// annualize. This isolates market-driven growth from external deposits and
	// withdrawals -- a naive end/start ratio would treat a deposit as a return.
	flows := buildFlowMap(stats.TransactionsView(ctx), eqTimes[0], eqTimes[len(eqTimes)-1])
	returns := subPeriodReturns(eqCol, eqTimes, flows)

	growth := 1.0
	for _, ri := range returns {
		growth *= 1 + ri
	}

	if growth <= 0 {
		return 0, ErrInsufficientData
	}

	return math.Pow(growth, 1.0/years) - 1, nil
}

func (cagrMetric) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// CAGR is the Compound Annual Growth Rate -- the annualized return
// that accounts for compounding. It is the standard way to compare
// returns across different time horizons.
func (cagrMetric) BenchmarkTargetable() {}

func (cagrMetric) HigherIsBetter() bool { return true }

var CAGR PerformanceMetric = cagrMetric{}
