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

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// EqualWeight builds a PortfolioPlan from a DataFrame by assigning equal
// weights to all selected assets at each timestep. It reads the Selected
// metric column to determine which assets are chosen. Any asset with
// Selected > 0 at a given timestep receives equal weight; magnitude is
// ignored. Returns an error if the Selected column is absent.
func EqualWeight(df *data.DataFrame) (PortfolioPlan, error) {
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
		return nil, fmt.Errorf("EqualWeight: DataFrame missing %q column", Selected)
	}

	plan := make(PortfolioPlan, len(times))
	for i, t := range times {
		// Collect selected assets at this timestep.
		var chosen []asset.Asset
		for _, a := range assets {
			v := df.ValueAt(a, Selected, t)
			if v > 0 {
				chosen = append(chosen, a)
			}
		}

		members := make(map[asset.Asset]float64, len(chosen))
		if len(chosen) > 0 {
			w := 1.0 / float64(len(chosen))
			for _, a := range chosen {
				members[a] = w
			}
		}
		plan[i] = Allocation{Date: t, Members: members}
	}

	return plan, nil
}
