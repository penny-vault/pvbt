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

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// RSISignal is the metric name for RSI signal output.
const RSISignal data.Metric = "RSI"

// RSI computes the Relative Strength Index for each asset in the universe
// using Wilder's smoothing method. The period controls the lookback window;
// rsiPeriod = period.N - 1 price changes are used. Returns a single-row
// DataFrame with RSISignal metric.
func RSI(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, period, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("RSI: %w", err))
	}

	if df.Len() < 3 {
		return data.WithErr(fmt.Errorf("RSI: need at least 3 data points, got %d", df.Len()))
	}

	rsiPeriod := df.Len() - 1

	result := df.Diff().Apply(func(changes []float64) []float64 {
		out := make([]float64, len(changes))

		for ii := range out {
			out[ii] = math.NaN()
		}

		// Separate gains and losses, skipping index 0 (NaN from Diff).
		gains := make([]float64, 0, len(changes)-1)
		losses := make([]float64, 0, len(changes)-1)

		for ii := 1; ii < len(changes); ii++ {
			if changes[ii] > 0 {
				gains = append(gains, changes[ii])
				losses = append(losses, 0)
			} else {
				gains = append(gains, 0)
				losses = append(losses, -changes[ii])
			}
		}

		if len(gains) < rsiPeriod {
			return out
		}

		// Initial average gain/loss: SMA of first rsiPeriod values.
		avgGain := 0.0
		avgLoss := 0.0

		for ii := range rsiPeriod {
			avgGain += gains[ii]
			avgLoss += losses[ii]
		}

		avgGain /= float64(rsiPeriod)
		avgLoss /= float64(rsiPeriod)

		// Apply Wilder's smoothing for additional rows.
		for ii := rsiPeriod; ii < len(gains); ii++ {
			avgGain = (avgGain*float64(rsiPeriod-1) + gains[ii]) / float64(rsiPeriod)
			avgLoss = (avgLoss*float64(rsiPeriod-1) + losses[ii]) / float64(rsiPeriod)
		}

		// Compute RSI at last row.
		var rsiVal float64
		if avgLoss == 0 {
			rsiVal = 100
		} else {
			rs := avgGain / avgLoss
			rsiVal = 100 - 100/(1+rs)
		}

		out[len(out)-1] = rsiVal

		return out
	})

	return result.Last().RenameMetric(metric, RSISignal)
}
