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

type kRatio struct{}

func (kRatio) Name() string { return "KRatio" }

func (kRatio) Description() string {
	return "Measures the consistency of equity curve growth by fitting a linear regression to the log equity curve and dividing the slope by the standard error. Higher values indicate smoother, more consistent growth. Negative values indicate a declining equity curve."
}

func (kRatio) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return 0, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioEquity))

	count := len(col)
	if count < 3 {
		return 0, nil
	}

	// Compute log(VAMI) where VAMI = 1000 * cumulative product of (1 + r_i).
	logVAMI := make([]float64, count)

	cumProd := 1000.0
	for idx, ri := range col {
		cumProd *= (1 + ri)
		logVAMI[idx] = math.Log(cumProd)
	}

	// OLS regression: y = logVAMI, x = [0, 1, ..., count-1].
	numPeriods := float64(count)
	meanX := (numPeriods - 1) / 2.0
	meanY := stat.Mean(logVAMI, nil)

	sumXXdev := 0.0
	sumXYdev := 0.0

	for idx := 0; idx < count; idx++ {
		dx := float64(idx) - meanX
		sumXXdev += dx * dx
		sumXYdev += dx * (logVAMI[idx] - meanY)
	}

	if sumXXdev == 0 {
		return 0, nil
	}

	slope := sumXYdev / sumXXdev
	intercept := meanY - slope*meanX

	// Compute residuals and standard error of slope.
	sumResidSq := 0.0

	for idx := 0; idx < count; idx++ {
		predicted := slope*float64(idx) + intercept
		resid := logVAMI[idx] - predicted
		sumResidSq += resid * resid
	}

	stderr := math.Sqrt(sumResidSq/(numPeriods-2)) / math.Sqrt(sumXXdev)
	if stderr == 0 {
		return 0, nil
	}

	return slope / stderr, nil
}

func (kRatio) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// KRatio measures the consistency of returns over time: the slope of
// the log-VAMI regression line divided by the standard error of the
// slope (2003 Kestner revision). Higher values indicate more consistent growth.
var KRatio PerformanceMetric = kRatio{}
