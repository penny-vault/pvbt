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
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// MFISignal is the output metric for [MFI]. Values range from 0 to 100;
// readings above 80 suggest overbought conditions, below 20 suggest oversold.
const MFISignal data.Metric = "MFI"

// MFI computes the Money Flow Index for each asset in the universe over the
// given period. It compares positive and negative money flow (typical price
// times volume) to produce a momentum oscillator. One extra bar is fetched to
// establish the initial typical price for comparison. Returns a single-row
// DataFrame with [MFISignal].
func MFI(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	// Fetch one extra bar so bar 0 serves as the TP baseline for comparison.
	adjustedPeriod := portfolio.Days(period.N + 1)

	df, err := assetUniverse.Window(ctx, adjustedPeriod, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("MFI: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("MFI: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	mfiCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		// Compute typical prices for all bars.
		tp := make([]float64, numRows)

		for jj := range numRows {
			tp[jj] = (highs[jj] + lows[jj] + closes[jj]) / 3.0
		}

		// Accumulate positive and negative money flow starting from bar 1.
		posFlow := 0.0
		negFlow := 0.0

		for jj := 1; jj < numRows; jj++ {
			mf := tp[jj] * volumes[jj]

			if tp[jj] > tp[jj-1] {
				posFlow += mf
			} else if tp[jj] < tp[jj-1] {
				negFlow += mf
			}
		}

		var mfiVal float64

		if negFlow == 0 {
			mfiVal = 100.0
		} else if posFlow == 0 {
			mfiVal = 0.0
		} else {
			ratio := posFlow / negFlow
			mfiVal = 100.0 - 100.0/(1.0+ratio)
		}

		mfiCols[ii] = []float64{mfiVal}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{MFISignal}, df.Frequency(), mfiCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("MFI: %w", err))
	}

	return result
}
