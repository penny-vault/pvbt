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

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

type maxDrawdown struct{}

func (maxDrawdown) Name() string { return "MaxDrawdown" }

func (maxDrawdown) Description() string {
	return "Largest peak-to-trough decline in the equity curve as a decimal fraction. A value of -0.20 means the portfolio fell 20% from its peak. More negative values indicate larger drawdowns."
}

func (maxDrawdown) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Drawdown(ctx, window)
	if df == nil {
		return 0, nil
	}

	ddCol := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(ddCol) == 0 {
		return 0, nil
	}

	minDD := 0.0
	for _, v := range ddCol {
		if v < minDD {
			minDD = v
		}
	}

	return minDD, nil
}

func (maxDrawdown) ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error) {
	df := stats.Drawdown(ctx, window)
	if df == nil {
		return nil, nil
	}

	ddCol := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(ddCol) == 0 {
		return nil, nil
	}

	return data.NewDataFrame(
		df.Times(),
		[]asset.Asset{portfolioAsset},
		[]data.Metric{data.PortfolioEquity},
		df.Frequency(),
		[][]float64{ddCol},
	)
}

// MaxDrawdown is the largest peak-to-trough decline in portfolio value.
func (maxDrawdown) BenchmarkTargetable() {}

func (maxDrawdown) HigherIsBetter() bool { return true }

var MaxDrawdown PerformanceMetric = maxDrawdown{}
