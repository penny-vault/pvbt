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

// WilliamsRSignal is the output metric for [WilliamsR]. Values range from
// -100 (close at the lowest low) to 0 (close at the highest high).
const WilliamsRSignal data.Metric = "WilliamsR"

// WilliamsR computes Williams %R for each asset in the universe over the
// given period. It measures where the current close sits relative to the
// high-low range, scaled to -100..0. Returns a single-row DataFrame with
// [WilliamsRSignal].
func WilliamsR(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("WilliamsR: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("WilliamsR: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	wrCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		highestHigh := math.Inf(-1)
		lowestLow := math.Inf(1)

		for jj := range numRows {
			if highs[jj] > highestHigh {
				highestHigh = highs[jj]
			}

			if lows[jj] < lowestLow {
				lowestLow = lows[jj]
			}
		}

		rangeHL := highestHigh - lowestLow

		var wr float64
		if rangeHL == 0 {
			wr = math.NaN()
		} else {
			wr = (highestHigh - closes[numRows-1]) / rangeHL * -100
		}

		wrCols[ii] = []float64{wr}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{WilliamsRSignal}, df.Frequency(), wrCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("WilliamsR: %w", err))
	}

	return result
}
