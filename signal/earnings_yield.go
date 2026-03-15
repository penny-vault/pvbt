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
	"github.com/penny-vault/pvbt/universe"
)

// EarningsYield computes earnings per share divided by price for each
// asset in the universe. Returns a single-row DataFrame with one column
// per asset containing the earnings yield.
func EarningsYield(ctx context.Context, assetUniverse universe.Universe, t ...time.Time) *data.DataFrame {
	fetchTime := assetUniverse.CurrentDate()
	if len(t) > 0 {
		fetchTime = t[0]
	}

	df, err := assetUniverse.At(ctx, fetchTime, data.EarningsPerShare, data.Price)
	if err != nil {
		return data.WithErr(fmt.Errorf("EarningsYield: %w", err))
	}

	// Validate required metrics are present.
	epsDF := df.Metrics(data.EarningsPerShare)
	if epsDF.ColCount() == 0 {
		return data.WithErr(fmt.Errorf("EarningsYield: missing EarningsPerShare metric"))
	}

	priceDF := df.Metrics(data.Price)
	if priceDF.ColCount() == 0 {
		return data.WithErr(fmt.Errorf("EarningsYield: missing Price metric"))
	}

	// Build result manually to avoid the Div metric-intersection problem.
	// (Two DataFrames with different metrics would have an empty intersection.)
	assets := df.AssetList()
	times := df.Times()
	resultData := make([]float64, len(assets))

	for i, a := range assets {
		eps := df.ValueAt(a, data.EarningsPerShare, times[0])
		price := df.ValueAt(a, data.Price, times[0])
		resultData[i] = eps / price
	}

	result, buildErr := data.NewDataFrame(times, assets, []data.Metric{EarningsYieldSignal}, data.Daily, resultData)
	if buildErr != nil {
		return data.WithErr(fmt.Errorf("EarningsYield: %w", buildErr))
	}

	return result
}
