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
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// EqualWeight builds a PortfolioPlan from a DataFrame by assigning equal
// weights to all selected assets at each timestep. For example, if three
// assets are selected at a given date, each receives a weight of 1/3.
// The DataFrame should have been filtered by a Selector first.
func EqualWeight(df *data.DataFrame) PortfolioPlan {
	times := df.Times()
	assets := df.AssetList()
	weight := 1.0 / float64(len(assets))

	plan := make(PortfolioPlan, len(times))
	for i, t := range times {
		members := make(map[asset.Asset]float64, len(assets))
		for _, a := range assets {
			members[a] = weight
		}
		plan[i] = Allocation{Date: t, Members: members}
	}

	return plan
}
