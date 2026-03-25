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

const (
	// MACDLineSignal is the metric name for the MACD line output.
	MACDLineSignal data.Metric = "MACDLine"
	// MACDSignalLineSignal is the metric name for the MACD signal line output.
	MACDSignalLineSignal data.Metric = "MACDSignalLine"
	// MACDHistogramSignal is the metric name for the MACD histogram output.
	MACDHistogramSignal data.Metric = "MACDHistogram"
)

// MACD computes the Moving Average Convergence/Divergence indicator for each
// asset in the universe. It returns a single-row DataFrame with three metrics:
// MACDLineSignal (fast EMA - slow EMA), MACDSignalLineSignal (EMA of MACD
// line), and MACDHistogramSignal (MACD line - signal line).
func MACD(ctx context.Context, assetUniverse universe.Universe, fast, slow, signalPeriod portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, slow, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("MACD: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("MACD: need at least 2 data points, got %d", df.Len()))
	}

	fastWindow := fast.N
	slowWindow := slow.N
	sigWindow := signalPeriod.N

	fastEMA := df.Rolling(fastWindow).EMA()
	slowEMA := df.Rolling(slowWindow).EMA()

	// Drop NaN rows so the signal line EMA gets a clean seed.
	macdLine := fastEMA.Sub(slowEMA).Drop(math.NaN())
	signalLine := macdLine.Rolling(sigWindow).EMA()
	histogram := macdLine.Sub(signalLine)

	macdLast := macdLine.Last().RenameMetric(metric, MACDLineSignal)
	signalLast := signalLine.Last().RenameMetric(metric, MACDSignalLineSignal)
	histLast := histogram.Last().RenameMetric(metric, MACDHistogramSignal)

	result, err := data.MergeColumns(macdLast, signalLast, histLast)
	if err != nil {
		return data.WithErr(fmt.Errorf("MACD: %w", err))
	}

	return result
}
