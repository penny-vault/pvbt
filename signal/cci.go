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

// CCISignal is the output metric for [CCI]. Values are unbounded; readings
// above +100 suggest overbought conditions, below -100 suggest oversold.
const CCISignal data.Metric = "CCI"

// cciConstant is Lambert's constant, chosen so that roughly 70-80% of CCI
// values fall between -100 and +100 under normal market conditions.
const cciConstant = 0.015

// CCI computes the Commodity Channel Index for each asset in the universe
// over the given period. It measures deviation of the typical price from
// its simple moving average, scaled by the mean absolute deviation. Returns
// a single-row DataFrame with [CCISignal].
func CCI(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("CCI: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("CCI: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	cciCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		// Compute typical prices.
		tp := make([]float64, numRows)
		for jj := range numRows {
			tp[jj] = (highs[jj] + lows[jj] + closes[jj]) / 3.0
		}

		// SMA of typical prices.
		sma := 0.0
		for jj := range numRows {
			sma += tp[jj]
		}

		sma /= float64(numRows)

		// Mean absolute deviation from SMA.
		meanDev := 0.0
		for jj := range numRows {
			meanDev += math.Abs(tp[jj] - sma)
		}

		meanDev /= float64(numRows)

		var cciVal float64
		if meanDev == 0 {
			cciVal = math.NaN()
		} else {
			cciVal = (tp[numRows-1] - sma) / (cciConstant * meanDev)
		}

		cciCols[ii] = []float64{cciVal}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{CCISignal}, df.Frequency(), cciCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("CCI: %w", err))
	}

	return result
}
