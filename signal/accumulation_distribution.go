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

// AccumulationDistributionSignal is the output metric for [AccumulationDistribution].
// Values are unbounded cumulative money flow volume; positive indicates accumulation
// (buying pressure), negative indicates distribution (selling pressure).
const AccumulationDistributionSignal data.Metric = "AccumulationDistribution"

// AccumulationDistribution computes the Accumulation/Distribution line for each
// asset in the universe over the given period. It sums money flow volume across
// all bars, where each bar's contribution is weighted by the Money Flow Multiplier:
// ((Close-Low) - (High-Close)) / (High-Low). When High==Low, the multiplier is
// zero. Returns a single-row DataFrame with [AccumulationDistributionSignal].
func AccumulationDistribution(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("AccumulationDistribution: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("AccumulationDistribution: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	adCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		mfvs := moneyFlowVolumeSeries(highs, lows, closes, volumes)

		adVal := 0.0

		for _, mfv := range mfvs {
			adVal += mfv
		}

		adCols[ii] = []float64{adVal}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{AccumulationDistributionSignal}, df.Frequency(), adCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("AccumulationDistribution: %w", err))
	}

	return result
}

// moneyFlowVolumeSeries computes the per-bar Money Flow Volume for a single asset.
// MFM = ((Close-Low) - (High-Close)) / (High-Low). When High==Low, MFM=0.
// MFV = MFM * Volume. The returned slice has the same length as the inputs.
func moneyFlowVolumeSeries(highs, lows, closes, volumes []float64) []float64 {
	mfvs := make([]float64, len(highs))

	for ii := range highs {
		hl := highs[ii] - lows[ii]
		if hl == 0 {
			mfvs[ii] = 0
			continue
		}

		mfm := ((closes[ii] - lows[ii]) - (highs[ii] - closes[ii])) / hl
		mfvs[ii] = mfm * volumes[ii]
	}

	return mfvs
}
