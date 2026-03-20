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

package portfolio

import (
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// CollectSelected returns the assets whose Selected value is > 0 at the given
// timestamp. NaN and non-positive values are excluded.
func CollectSelected(df *data.DataFrame, timestamp time.Time) []asset.Asset {
	var chosen []asset.Asset

	for _, currentAsset := range df.AssetList() {
		sel := df.ValueAt(currentAsset, Selected, timestamp)
		if sel > 0 && !math.IsNaN(sel) {
			chosen = append(chosen, currentAsset)
		}
	}

	return chosen
}

// HasSelectedColumn reports whether the DataFrame contains the Selected metric.
func HasSelectedColumn(df *data.DataFrame) bool {
	for _, m := range df.MetricList() {
		if m == Selected {
			return true
		}
	}

	return false
}

// ErrMissingSelected returns the standard error for a missing Selected column.
func ErrMissingSelected(funcName string) error {
	return fmt.Errorf("%s: DataFrame missing %q column", funcName, Selected)
}

// ErrNoDataSource returns the standard error for a nil DataSource.
func ErrNoDataSource(funcName, dataDesc string) error {
	return fmt.Errorf("%s: DataFrame has no DataSource; cannot fetch %s", funcName, dataDesc)
}

// equalWeightMembers assigns 1/N weight to each asset in the slice.
func equalWeightMembers(chosen []asset.Asset) map[asset.Asset]float64 {
	members := make(map[asset.Asset]float64, len(chosen))
	if len(chosen) > 0 {
		weight := 1.0 / float64(len(chosen))
		for _, currentAsset := range chosen {
			members[currentAsset] = weight
		}
	}

	return members
}

// defaultLookback returns the given period if non-zero, otherwise 60 days.
func defaultLookback(lookback data.Period) data.Period {
	if lookback == (data.Period{}) {
		return data.Days(60)
	}

	return lookback
}

// nanSafeStd computes the sample standard deviation of a metric's values for
// a given asset across all timestamps, skipping NaN values. This is needed
// because Pct() produces NaN in the first row, and the built-in Std() method
// propagates that NaN through stat.Mean.
func nanSafeStd(df *data.DataFrame, currentAsset asset.Asset, metric data.Metric) float64 {
	times := df.Times()

	var values []float64

	for _, timestamp := range times {
		val := df.ValueAt(currentAsset, metric, timestamp)
		if !math.IsNaN(val) {
			values = append(values, val)
		}
	}

	if len(values) < 2 {
		return 0
	}

	mean := 0.0
	for _, val := range values {
		mean += val
	}

	mean /= float64(len(values))

	sumSq := 0.0

	for _, val := range values {
		diff := val - mean
		sumSq += diff * diff
	}

	return math.Sqrt(sumSq / float64(len(values)-1))
}
