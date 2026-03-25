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

// CMFSignal is the output metric for [CMF]. Values range from -1 to +1;
// positive values indicate buying pressure, negative values indicate selling pressure.
const CMFSignal data.Metric = "CMF"

// CMF computes the Chaikin Money Flow for each asset in the universe over the
// given period. CMF is the sum of Money Flow Volume divided by the sum of Volume.
// When total volume is zero, CMF is NaN. Returns a single-row DataFrame with [CMFSignal].
func CMF(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("CMF: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("CMF: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	cmfCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		mfvs := moneyFlowVolumeSeries(highs, lows, closes, volumes)

		sumMFV := 0.0
		sumVolume := 0.0

		for jj := range mfvs {
			sumMFV += mfvs[jj]
			sumVolume += volumes[jj]
		}

		cmfVal := math.NaN()
		if sumVolume != 0 {
			cmfVal = sumMFV / sumVolume
		}

		cmfCols[ii] = []float64{cmfVal}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{CMFSignal}, df.Frequency(), cmfCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("CMF: %w", err))
	}

	return result
}
