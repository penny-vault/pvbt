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
	"github.com/penny-vault/pvbt/asset"
)

type twrr struct{}

func (twrr) Name() string { return "TWRR" }

func (twrr) Description() string {
	return "Time-weighted rate of return. Measures portfolio performance independent of the timing and size of cash flows. Computed by geometrically linking sub-period returns. The standard measure for comparing investment manager performance."
}

// Compute returns the total time-weighted return over the window (or full
// history when window is nil). It compounds sub-period returns derived
// from the equity curve: product(1 + r_i) - 1.
func (twrr) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return 0, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioReturns))
	if len(col) == 0 {
		return 0, nil
	}

	product := 1.0
	for _, ri := range col {
		product *= (1 + ri)
	}

	return product - 1, nil
}

// ComputeSeries returns the cumulative return at each point: the running
// product of (1 + r_i) minus 1. The result has length len(equity)-1.
func (twrr) ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return nil, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioReturns))
	if len(col) == 0 {
		return nil, nil
	}

	// The returns DataFrame has NaN at index 0; the clean col starts at index 1.
	// Use times[1:] to match.
	times := df.Times()
	seriesTimes := times[1:]

	cum := make([]float64, len(col))

	product := 1.0
	for idx, ri := range col {
		product *= (1 + ri)
		cum[idx] = product - 1
	}

	return data.NewDataFrame(
		seriesTimes,
		[]asset.Asset{portfolioAsset},
		[]data.Metric{data.PortfolioReturns},
		df.Frequency(),
		[][]float64{cum},
	)
}

func (twrr) BenchmarkTargetable() {}

// TWRR is the time-weighted rate of return, which eliminates the effect
// of cash flows (deposits/withdrawals) on portfolio returns.
var TWRR PerformanceMetric = twrr{}
