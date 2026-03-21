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

type downsideCaptureRatio struct{}

func (downsideCaptureRatio) Name() string { return "DownsideCaptureRatio" }

func (downsideCaptureRatio) Description() string {
	return "Measures how much of the benchmark's downside the portfolio experiences. A value of 0.8 means the portfolio loses only 80% of the benchmark's loss during down periods. Lower is better."
}

func (downsideCaptureRatio) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	rdf := stats.Returns(ctx, window)
	bdf := stats.BenchmarkReturns(ctx, window)

	if rdf == nil || bdf == nil {
		return 0, nil
	}

	pCol := removeNaN(rdf.Column(portfolioAsset, data.PortfolioReturns))
	bCol := removeNaN(bdf.Column(portfolioAsset, data.PortfolioBenchReturns))

	count := len(pCol)
	if len(bCol) < count {
		count = len(bCol)
	}

	// Filter periods where benchmark return < 0.
	var downP, downB []float64

	for ii := 0; ii < count; ii++ {
		if bCol[ii] < 0 {
			downP = append(downP, pCol[ii])
			downB = append(downB, bCol[ii])
		}
	}

	if len(downP) == 0 {
		return 0, nil
	}

	geoP := geometricMean(downP)
	geoB := geometricMean(downB)

	if geoB == 0 {
		return 0, nil
	}

	return geoP / geoB, nil
}

func (downsideCaptureRatio) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// DownsideCaptureRatio measures how much of the benchmark's negative
// returns the portfolio captures. Computed as portfolio geometric mean
// return / benchmark geometric mean return during down periods. A ratio
// below 1.0 means the portfolio loses less than the benchmark in
// falling markets.
var DownsideCaptureRatio PerformanceMetric = downsideCaptureRatio{}
