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
	// DonchianUpperSignal is the metric name for the upper Donchian Channel.
	DonchianUpperSignal data.Metric = "DonchianUpper"
	// DonchianMiddleSignal is the metric name for the middle Donchian Channel.
	DonchianMiddleSignal data.Metric = "DonchianMiddle"
	// DonchianLowerSignal is the metric name for the lower Donchian Channel.
	DonchianLowerSignal data.Metric = "DonchianLower"
)

// DonchianChannels computes the Donchian Channels (upper, middle, lower) for
// each asset in the universe over the given period. Upper is the rolling max
// of High, lower is the rolling min of Low, middle is the midpoint. Returns a
// single-row DataFrame with DonchianUpperSignal, DonchianMiddleSignal,
// DonchianLowerSignal.
func DonchianChannels(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow)
	if err != nil {
		return data.WithErr(fmt.Errorf("DonchianChannels: %w", err))
	}

	if df.Len() < 1 {
		return data.WithErr(fmt.Errorf("DonchianChannels: need at least 1 data point, got %d", df.Len()))
	}

	windowSize := df.Len()

	upper := df.Rolling(windowSize).Max().Last().
		RenameMetric(data.MetricHigh, DonchianUpperSignal).
		Metrics(DonchianUpperSignal)
	lower := df.Rolling(windowSize).Min().Last().
		RenameMetric(data.MetricLow, DonchianLowerSignal).
		Metrics(DonchianLowerSignal)

	middle := upper.Add(lower.RenameMetric(DonchianLowerSignal, DonchianUpperSignal)).
		DivScalar(2).
		RenameMetric(DonchianUpperSignal, DonchianMiddleSignal)

	result, err := data.MergeColumns(upper, middle, lower)
	if err != nil {
		return data.WithErr(fmt.Errorf("DonchianChannels: %w", err))
	}

	return result
}
