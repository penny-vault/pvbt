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

// VWMASignal is the output metric for [VWMA]. Values represent the
// volume-weighted average price over the period; higher volume bars
// contribute more to the average than lower volume bars.
const VWMASignal data.Metric = "VWMA"

// VWMA computes the Volume-Weighted Moving Average for each asset in the
// universe over the given period. The result is Sum(Close*Volume)/Sum(Volume).
// If total volume is zero for an asset, the result is NaN. Returns a
// single-row DataFrame with [VWMASignal].
func VWMA(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("VWMA: %w", err))
	}

	numRows := df.Len()
	if numRows < 1 {
		return data.WithErr(fmt.Errorf("VWMA: need at least 1 data point, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	vwmaCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		sumPV := 0.0
		sumV := 0.0

		for jj := range numRows {
			sumPV += closes[jj] * volumes[jj]
			sumV += volumes[jj]
		}

		var vwmaVal float64
		if sumV == 0 {
			vwmaVal = math.NaN()
		} else {
			vwmaVal = sumPV / sumV
		}

		vwmaCols[ii] = []float64{vwmaVal}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{VWMASignal}, df.Frequency(), vwmaCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("VWMA: %w", err))
	}

	return result
}
