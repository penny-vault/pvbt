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
	"gonum.org/v1/gonum/stat"
)

type excessKurtosis struct{}

func (excessKurtosis) Name() string { return "ExcessKurtosis" }

func (excessKurtosis) Description() string {
	return "Measures the heaviness of return distribution tails relative to a normal distribution. Positive values indicate fat tails (more extreme returns than expected). Negative values indicate thin tails."
}

func (excessKurtosis) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return 0, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioEquity))

	numValues := len(col)
	if numValues < 4 {
		return 0, nil
	}

	stdDev := stat.StdDev(col, nil)
	if stdDev == 0 {
		return 0, nil
	}

	m := stat.Mean(col, nil)
	sum := 0.0

	for _, val := range col {
		d := val - m
		sum += d * d * d * d
	}

	return sum/float64(numValues)/(stdDev*stdDev*stdDev*stdDev) - 3, nil
}

func (excessKurtosis) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// ExcessKurtosis measures tail risk -- how much fatter the tails of
// the return distribution are compared to a normal distribution.
// Positive values indicate heavier tails (more extreme outcomes than
// a normal distribution would predict).
func (excessKurtosis) BenchmarkTargetable() {}

var ExcessKurtosis PerformanceMetric = excessKurtosis{}
