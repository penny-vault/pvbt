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

// ATRSignal is the metric name for ATR signal output.
const ATRSignal data.Metric = "ATR"

// ATR computes the Average True Range for each asset in the universe using
// Wilder's smoothing method. It always uses High, Low, and Close metrics.
// Returns a single-row DataFrame with ATRSignal.
func ATR(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("ATR: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("ATR: need at least 2 data points, got %d", numRows))
	}

	atrPeriod := numRows - 1
	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	atrCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		// Compute True Range for each row starting from index 1.
		trValues := make([]float64, numRows-1)
		for jj := 1; jj < numRows; jj++ {
			highLow := highs[jj] - lows[jj]
			highPrevClose := math.Abs(highs[jj] - closes[jj-1])
			lowPrevClose := math.Abs(lows[jj] - closes[jj-1])
			trValues[jj-1] = math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
		}

		// Initial average TR: SMA of first atrPeriod values.
		avgTR := 0.0
		for kk := range atrPeriod {
			avgTR += trValues[kk]
		}

		avgTR /= float64(atrPeriod)

		// Apply Wilder's smoothing for additional rows.
		for kk := atrPeriod; kk < len(trValues); kk++ {
			avgTR = (avgTR*float64(atrPeriod-1) + trValues[kk]) / float64(atrPeriod)
		}

		atrCols[ii] = []float64{avgTR}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{ATRSignal}, df.Frequency(), atrCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("ATR: %w", err))
	}

	return result
}
