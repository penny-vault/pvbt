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

type trackingError struct{}

func (trackingError) Name() string { return "TrackingError" }

func (trackingError) Description() string {
	return "Annualized standard deviation of the difference between portfolio and benchmark returns. Measures how closely the portfolio follows its benchmark. Lower values indicate tighter tracking."
}

func (trackingError) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
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

	pCol := removeNaN(rdf.Column(portfolioAsset, data.PortfolioEquity))
	bCol := removeNaN(bdf.Column(portfolioAsset, data.PortfolioBenchmark))

	// Compute active returns = portfolio returns - benchmark returns.
	minLen := len(pCol)
	if len(bCol) < minLen {
		minLen = len(bCol)
	}

	if minLen < 2 {
		return 0, nil
	}

	arCol := make([]float64, minLen)
	for idx := 0; idx < minLen; idx++ {
		arCol[idx] = pCol[idx] - bCol[idx]
	}

	stdDev := stat.StdDev(arCol, nil)
	if math.IsNaN(stdDev) {
		return 0, nil
	}

	af := annualizationFactor(rdf.Times())

	return stdDev * math.Sqrt(af), nil
}

func (trackingError) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// TrackingError is the standard deviation of the difference between
// portfolio returns and benchmark returns.
var TrackingError PerformanceMetric = trackingError{}
