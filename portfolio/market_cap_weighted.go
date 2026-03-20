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
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// MarketCapWeighted builds a PortfolioPlan by weighting each selected asset
// proportionally to its MarketCap value. If MarketCap is not in the DataFrame,
// it fetches via the DataFrame's DataSource. Falls back to equal weight when
// all selected assets have zero or NaN market caps.
func MarketCapWeighted(ctx context.Context, df *data.DataFrame) (PortfolioPlan, error) {
	if !HasSelectedColumn(df) {
		return nil, ErrMissingSelected("MarketCapWeighted")
	}

	times := df.Times()
	assets := df.AssetList()

	// Ensure MarketCap data is available.
	mcapDF, err := ensureMarketCap(ctx, df, assets)
	if err != nil {
		return nil, fmt.Errorf("MarketCapWeighted: %w", err)
	}

	plan := make(PortfolioPlan, len(times))

	for timeIdx, timestamp := range times {
		chosen := CollectSelected(df, timestamp)

		var (
			values []float64
			sum    float64
		)

		for _, currentAsset := range chosen {
			mcap := mcapDF.ValueAt(currentAsset, data.MarketCap, timestamp)
			if math.IsNaN(mcap) || mcap <= 0 {
				values = append(values, 0)
			} else {
				values = append(values, mcap)
				sum += mcap
			}
		}

		members := make(map[asset.Asset]float64, len(chosen))

		if sum == 0 && len(chosen) > 0 {
			members = equalWeightMembers(chosen)
		} else {
			for idx, currentAsset := range chosen {
				weight := values[idx] / sum
				if weight > 0 {
					members[currentAsset] = weight
				}
			}
		}

		plan[timeIdx] = Allocation{Date: timestamp, Members: members}
	}

	return plan, nil
}

// ensureMarketCap checks whether the DataFrame contains MarketCap. If not,
// it fetches via FetchAt using the source's current date.
func ensureMarketCap(ctx context.Context, df *data.DataFrame, assets []asset.Asset) (*data.DataFrame, error) {
	for _, metric := range df.MetricList() {
		if metric == data.MarketCap {
			return df, nil
		}
	}

	source := df.Source()
	if source == nil {
		return nil, ErrNoDataSource("MarketCapWeighted", "MarketCap")
	}

	return source.FetchAt(ctx, assets, source.CurrentDate(), []data.Metric{data.MarketCap})
}
