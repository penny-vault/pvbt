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

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// InverseVolatility builds a PortfolioPlan by weighting each selected asset
// inversely proportional to its trailing volatility. A zero-value lookback
// defaults to 60 calendar days. Falls back to equal weight when all selected
// assets have zero or NaN volatility.
func InverseVolatility(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error) {
	if !HasSelectedColumn(df) {
		return nil, ErrMissingSelected("InverseVolatility")
	}

	lookback = defaultLookback(lookback)
	times := df.Times()
	assets := df.AssetList()

	// Ensure AdjClose data is available.
	priceDF, err := ensureMetric(ctx, df, assets, lookback, data.AdjClose)
	if err != nil {
		return nil, fmt.Errorf("InverseVolatility: %w", err)
	}

	plan := make(PortfolioPlan, len(times))

	for timeIdx, timestamp := range times {
		chosen := CollectSelected(df, timestamp)

		if len(chosen) <= 1 {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		// Compute trailing volatility for each chosen asset.
		window := priceDF.Between(lookback.Before(timestamp), timestamp)
		returns := window.Pct()

		invVols := make([]float64, len(chosen))
		sumInvVol := 0.0

		for idx, currentAsset := range chosen {
			vol := nanSafeStd(returns, currentAsset, data.AdjClose)

			if vol <= 0 {
				invVols[idx] = 0
			} else {
				invVols[idx] = 1.0 / vol
				sumInvVol += invVols[idx]
			}
		}

		members := make(map[asset.Asset]float64, len(chosen))

		if sumInvVol == 0 {
			members = equalWeightMembers(chosen)
		} else {
			for idx, currentAsset := range chosen {
				weight := invVols[idx] / sumInvVol
				if weight > 0 {
					members[currentAsset] = weight
				}
			}
		}

		plan[timeIdx] = Allocation{Date: timestamp, Members: members}
	}

	return plan, nil
}

// ensureMetric checks whether the DataFrame contains the given metric.
// If not, it fetches data via the DataFrame's DataSource. Returns a DataFrame
// that contains the metric for the given assets.
func ensureMetric(ctx context.Context, df *data.DataFrame, assets []asset.Asset, lookback data.Period, metric data.Metric) (*data.DataFrame, error) {
	// Check if metric is already present.
	for _, existingMetric := range df.MetricList() {
		if existingMetric == metric {
			return df, nil
		}
	}

	// Metric not present; fetch via source.
	source := df.Source()
	if source == nil {
		return nil, ErrNoDataSource("ensureMetric", string(metric))
	}

	return source.Fetch(ctx, assets, lookback, []data.Metric{metric})
}
