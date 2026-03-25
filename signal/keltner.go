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

package signal

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

const (
	// KeltnerUpperSignal is the metric name for the upper Keltner Channel.
	KeltnerUpperSignal data.Metric = "KeltnerUpper"
	// KeltnerMiddleSignal is the metric name for the middle Keltner Channel (EMA).
	KeltnerMiddleSignal data.Metric = "KeltnerMiddle"
	// KeltnerLowerSignal is the metric name for the lower Keltner Channel.
	KeltnerLowerSignal data.Metric = "KeltnerLower"
)

// KeltnerChannels computes the Keltner Channels (upper, middle, lower) for
// each asset in the universe over the given period. The center line is an EMA
// of the price metric (defaults to Close). Bands are placed at +/- atrMultiplier
// times the ATR. Returns a single-row DataFrame with KeltnerUpperSignal,
// KeltnerMiddleSignal, KeltnerLowerSignal.
func KeltnerChannels(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, atrMultiplier float64, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	// Always fetch High, Low, Close for ATR, plus the custom metric if different.
	fetchMetrics := []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}
	if metric != data.MetricClose {
		fetchMetrics = append(fetchMetrics, metric)
	}

	df, err := assetUniverse.Window(ctx, period, fetchMetrics...)
	if err != nil {
		return data.WithErr(fmt.Errorf("KeltnerChannels: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("KeltnerChannels: need at least 2 data points, got %d", numRows))
	}

	// Compute EMA of the price metric for the center line.
	// Filter to only the price metric before EMA to avoid extra High/Low columns.
	windowSize := numRows
	emaDF := df.Metrics(metric).Rolling(windowSize).EMA()
	centerLine := emaDF.Last().RenameMetric(metric, KeltnerMiddleSignal)

	// Compute ATR inline (same logic as signal.ATR to avoid redundant data fetch).
	atrPeriod := numRows - 1
	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	atrCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		trValues := make([]float64, numRows-1)
		for jj := 1; jj < numRows; jj++ {
			highLow := highs[jj] - lows[jj]
			highPrevClose := math.Abs(highs[jj] - closes[jj-1])
			lowPrevClose := math.Abs(lows[jj] - closes[jj-1])
			trValues[jj-1] = math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
		}

		avgTR := 0.0
		for kk := range atrPeriod {
			avgTR += trValues[kk]
		}

		avgTR /= float64(atrPeriod)

		atrCols[ii] = []float64{avgTR}
	}

	atrDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{KeltnerMiddleSignal}, df.Frequency(), atrCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("KeltnerChannels: %w", err))
	}

	upper := centerLine.Add(atrDF.MulScalar(atrMultiplier)).RenameMetric(KeltnerMiddleSignal, KeltnerUpperSignal)
	lower := centerLine.Sub(atrDF.MulScalar(atrMultiplier)).RenameMetric(KeltnerMiddleSignal, KeltnerLowerSignal)

	result, err := data.MergeColumns(centerLine, upper, lower)
	if err != nil {
		return data.WithErr(fmt.Errorf("KeltnerChannels: %w", err))
	}

	return result
}
