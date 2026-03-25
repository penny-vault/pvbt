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
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

type twrr struct{}

func (twrr) Name() string { return "TWRR" }

func (twrr) Description() string {
	return "Time-weighted rate of return. Measures portfolio performance independent of the timing and size of cash flows. Computed by geometrically linking sub-period returns. The standard measure for comparing investment manager performance."
}

// buildFlowMap returns a map from date to net external cash flow on that date.
// Only DepositTransaction and WithdrawalTransaction are considered external flows.
// Flows outside the [start, end] range are excluded.
func buildFlowMap(txns []Transaction, start, end time.Time) map[time.Time]float64 {
	flows := make(map[time.Time]float64)

	for _, tx := range txns {
		if tx.Date.Before(start) || tx.Date.After(end) {
			continue
		}

		switch tx.Type {
		case asset.DepositTransaction:
			flows[tx.Date] += tx.Amount
		case asset.WithdrawalTransaction:
			flows[tx.Date] += tx.Amount // Amount is already negative for withdrawals
		}
	}

	return flows
}

// subPeriodReturns computes flow-adjusted sub-period returns from an equity
// curve and a flow map. For each equity point after the first, the external
// flow is subtracted to isolate market-driven movement, and the return is
// measured relative to the previous sub-period starting equity.
func subPeriodReturns(equityCurve []float64, times []time.Time, flows map[time.Time]float64) []float64 {
	if len(equityCurve) < 2 {
		return nil
	}

	returns := make([]float64, 0, len(equityCurve)-1)
	subPeriodStart := equityCurve[0]

	for ii := 1; ii < len(equityCurve); ii++ {
		flow := flows[times[ii]]
		preFlowEquity := equityCurve[ii] - flow

		var ri float64
		if subPeriodStart != 0 {
			ri = preFlowEquity/subPeriodStart - 1
		}

		returns = append(returns, ri)
		subPeriodStart = equityCurve[ii]
	}

	return returns
}

// Compute returns the total time-weighted return over the window (or full
// history when window is nil). It compounds flow-adjusted sub-period returns:
// product(1 + r_i) - 1.
func (twrr) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, nil
	}

	equity := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(equity) < 2 {
		return 0, nil
	}

	times := df.Times()
	txns := stats.TransactionsView(ctx)
	flows := buildFlowMap(txns, times[0], times[len(times)-1])
	returns := subPeriodReturns(equity, times, flows)

	product := 1.0
	for _, ri := range returns {
		product *= (1 + ri)
	}

	return product - 1, nil
}

// ComputeSeries returns the cumulative return at each point: the running
// product of flow-adjusted (1 + r_i) minus 1.
func (twrr) ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return nil, nil
	}

	equity := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(equity) < 2 {
		return nil, nil
	}

	times := df.Times()
	txns := stats.TransactionsView(ctx)
	flows := buildFlowMap(txns, times[0], times[len(times)-1])
	returns := subPeriodReturns(equity, times, flows)

	cum := make([]float64, len(returns))
	product := 1.0

	for ii, ri := range returns {
		product *= (1 + ri)
		cum[ii] = product - 1
	}

	return data.NewDataFrame(
		times[1:],
		[]asset.Asset{portfolioAsset},
		[]data.Metric{data.PortfolioEquity},
		df.Frequency(),
		[][]float64{cum},
	)
}

func (twrr) BenchmarkTargetable() {}

// TWRR is the time-weighted rate of return, which eliminates the effect
// of cash flows (deposits/withdrawals) on portfolio returns.
var TWRR PerformanceMetric = twrr{}
