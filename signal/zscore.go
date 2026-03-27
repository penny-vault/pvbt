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

// ZScoreSignal is the metric name for z-score signal output.
const ZScoreSignal data.Metric = "ZScore"

// ZScore computes the z-score of each asset's current value relative to its
// rolling mean and standard deviation over the lookback period. Positive values
// indicate the asset is above its rolling mean; negative values indicate below.
// Values beyond +/-2 indicate significant deviation. Returns a single-row
// DataFrame with [ZScoreSignal].
func ZScore(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, period, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("ZScore: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("ZScore: need at least 2 data points, got %d", df.Len()))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	zCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		col := df.Column(aa, metric)

		zz, zErr := zScore(col)
		if zErr != nil {
			return data.WithErr(fmt.Errorf("ZScore [%s]: %w", aa.Ticker, zErr))
		}

		zCols[ii] = []float64{zz}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{ZScoreSignal}, df.Frequency(), zCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("ZScore: %w", err))
	}

	return result
}
