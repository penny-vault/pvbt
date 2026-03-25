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

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

const (
	// CrossoverFastSignal is the metric name for the fast moving average.
	CrossoverFastSignal data.Metric = "CrossoverFast"
	// CrossoverSlowSignal is the metric name for the slow moving average.
	CrossoverSlowSignal data.Metric = "CrossoverSlow"
	// CrossoverSignal is the metric name for the crossover signal.
	CrossoverSignal data.Metric = "Crossover"
)

// Crossover computes a moving-average crossover signal for each asset in the
// universe. It fetches data over slowPeriod, computes a fast SMA (fastPeriod.N
// bars) and a slow SMA (slowPeriod.N bars), and returns +1 when fast > slow and
// -1 when fast <= slow. Returns a single-row DataFrame with three metrics:
// CrossoverFastSignal, CrossoverSlowSignal, CrossoverSignal.
func Crossover(ctx context.Context, assetUniverse universe.Universe, fastPeriod, slowPeriod portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, slowPeriod, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("Crossover: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("Crossover: need at least 2 data points, got %d", df.Len()))
	}

	fastWindow := min(fastPeriod.N, df.Len())
	slowWindow := min(slowPeriod.N, df.Len())

	fastSMA := df.Rolling(fastWindow).Mean().Last().RenameMetric(metric, CrossoverFastSignal)
	slowSMA := df.Rolling(slowWindow).Mean().Last().RenameMetric(metric, CrossoverSlowSignal)

	assets := fastSMA.AssetList()

	crossoverCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		fastVal := fastSMA.Value(aa, CrossoverFastSignal)
		slowVal := slowSMA.Value(aa, CrossoverSlowSignal)

		crossoverVal := -1.0
		if fastVal > slowVal {
			crossoverVal = 1.0
		}

		crossoverCols[ii] = []float64{crossoverVal}
	}

	crossoverDF, err := data.NewDataFrame(fastSMA.Times(), assets, []data.Metric{CrossoverSignal}, fastSMA.Frequency(), crossoverCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("Crossover: %w", err))
	}

	result, err := data.MergeColumns(fastSMA, slowSMA, crossoverDF)
	if err != nil {
		return data.WithErr(fmt.Errorf("Crossover: %w", err))
	}

	return result
}
