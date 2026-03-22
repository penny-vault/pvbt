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
	"gonum.org/v1/gonum/stat"
)

type informationRatio struct{}

func (informationRatio) Name() string { return "InformationRatio" }

func (informationRatio) Description() string {
	return "Active return divided by tracking error. Measures how consistently the portfolio outperforms its benchmark per unit of active risk taken. Higher values indicate more consistent outperformance."
}

func (informationRatio) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	// Check if benchmark is configured by examining the raw equity-level
	// benchmark column. A zero first value means no benchmark was set.
	pd := stats.PerfDataView(ctx)
	if pd == nil {
		return 0, nil
	}

	bmRaw := pd.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmRaw) == 0 || bmRaw[0] == 0 {
		return 0, ErrNoBenchmark
	}

	rdf := stats.Returns(ctx, window)
	bdf := stats.BenchmarkReturns(ctx, window)

	if rdf == nil || bdf == nil {
		return 0, nil
	}

	pCol, bCol := alignedRemoveNaN(
		rdf.Column(portfolioAsset, data.PortfolioEquity),
		bdf.Column(portfolioAsset, data.PortfolioBenchmark),
	)

	if len(pCol) < 2 {
		return 0, nil
	}

	arCol := make([]float64, len(pCol))
	for idx := range pCol {
		arCol[idx] = pCol[idx] - bCol[idx]
	}

	trackingErr := stat.StdDev(arCol, nil)
	if trackingErr == 0 || math.IsNaN(trackingErr) {
		return 0, nil
	}

	af := annualizationFactor(rdf.Times())

	return stat.Mean(arCol, nil) / trackingErr * math.Sqrt(af), nil
}

func (informationRatio) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// InformationRatio measures the portfolio's active return per unit of
// tracking error relative to the benchmark.
var InformationRatio PerformanceMetric = informationRatio{}
