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

type upsideCaptureRatio struct{}

func (upsideCaptureRatio) Name() string { return "UpsideCaptureRatio" }

func (upsideCaptureRatio) Description() string {
	return "Measures how much of the benchmark's upside the portfolio captures. A value of 1.1 means the portfolio gains 110% of the benchmark's return during up periods. Higher is better."
}

func (upsideCaptureRatio) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
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

	// Filter periods where benchmark return > 0.
	var upP, upB []float64

	for idx := range pCol {
		if bCol[idx] > 0 {
			upP = append(upP, pCol[idx])
			upB = append(upB, bCol[idx])
		}
	}

	if len(upP) == 0 {
		return 0, nil
	}

	geoP := geometricMean(upP)
	geoB := geometricMean(upB)

	if geoB == 0 {
		return 0, nil
	}

	return geoP / geoB, nil
}

func (upsideCaptureRatio) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// geometricMean computes the geometric mean of returns:
// (product(1 + r_i))^(1/n) - 1
func geometricMean(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	product := 1.0
	for _, val := range returns {
		product *= (1 + val)
	}

	return math.Pow(product, 1.0/float64(len(returns))) - 1
}

// UpsideCaptureRatio measures how much of the benchmark's positive
// returns the portfolio captures. Computed as portfolio geometric mean
// return / benchmark geometric mean return during up periods. A ratio
// above 1.0 means the portfolio outperforms in rising markets.
var UpsideCaptureRatio PerformanceMetric = upsideCaptureRatio{}
