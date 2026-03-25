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

// OBVSignal is the output metric for [OBV]. Values are unbounded cumulative
// volume; positive indicates net buying pressure, negative indicates net selling.
const OBVSignal data.Metric = "OBV"

// OBV computes On-Balance Volume for each asset in the universe over the
// given period. Starting from zero, volume is added when the close rises
// and subtracted when it falls. Returns a single-row DataFrame with [OBVSignal].
func OBV(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("OBV: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("OBV: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	obvCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		obv := 0.0

		for jj := 1; jj < numRows; jj++ {
			if closes[jj] > closes[jj-1] {
				obv += volumes[jj]
			} else if closes[jj] < closes[jj-1] {
				obv -= volumes[jj]
			}
		}

		obvCols[ii] = []float64{obv}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{OBVSignal}, df.Frequency(), obvCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("OBV: %w", err))
	}

	return result
}
