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

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// WeightedBySignal builds a PortfolioPlan from a DataFrame by weighting
// each selected asset proportionally to the values in the named metric
// column. It reads the Selected metric column to determine which assets
// are chosen at each timestep. Any asset with Selected > 0 is included;
// magnitude is ignored. Weights are normalized to sum to 1.0.
//
// Zero, NaN, and negative metric values are discarded. If all selected
// assets have non-positive metric values at a timestep, equal weight is
// assigned among the selected assets. Returns an error if the Selected
// column is absent.
func WeightedBySignal(df *data.DataFrame, metric data.Metric) (PortfolioPlan, error) {
	times := df.Times()
	assets := df.AssetList()

	// Verify Selected column exists.
	hasSelected := false
	for _, m := range df.MetricList() {
		if m == Selected {
			hasSelected = true
			break
		}
	}
	if !hasSelected {
		return nil, fmt.Errorf("WeightedBySignal: DataFrame missing %q column", Selected)
	}

	plan := make(PortfolioPlan, len(times))

	for i, t := range times {
		// Collect selected assets and their signal values.
		var chosen []asset.Asset
		var values []float64
		sum := 0.0

		for _, a := range assets {
			sel := df.ValueAt(a, Selected, t)
			if sel <= 0 || math.IsNaN(sel) {
				continue
			}
			chosen = append(chosen, a)

			v := df.ValueAt(a, metric, t)
			if math.IsNaN(v) || v <= 0 {
				values = append(values, 0)
			} else {
				values = append(values, v)
				sum += v
			}
		}

		members := make(map[asset.Asset]float64, len(chosen))

		if sum == 0 && len(chosen) > 0 {
			// Fall back to equal weight among selected assets.
			w := 1.0 / float64(len(chosen))
			for _, a := range chosen {
				members[a] = w
			}
		} else {
			for j, a := range chosen {
				w := values[j] / sum
				if w > 0 {
					members[a] = w
				}
			}
		}

		plan[i] = Allocation{Date: t, Members: members}
	}

	return plan, nil
}
