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
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/rs/zerolog/log"
)

type maxAboveZero struct {
	metric   data.Metric
	fallback *data.DataFrame
}

// Select marks the asset with the highest positive value in the
// configured metric as selected at each timestep. If no asset has a
// positive value, fallback assets are inserted into the DataFrame and
// marked as selected. The DataFrame is mutated in place.
//
// Insert errors are logged but not returned because the Selector
// interface does not support error returns. A mismatched fallback
// DataFrame (e.g., different timestamps) will produce log warnings.
func (m maxAboveZero) Select(df *data.DataFrame) *data.DataFrame {
	times := df.Times()
	assets := df.AssetList()
	T := len(times)

	// Build Selected column per asset.
	selCols := make(map[string][]float64)
	for _, a := range assets {
		selCols[a.CompositeFigi] = make([]float64, T)
	}

	// Track which fallback assets need to be inserted.
	needsFallback := false
	var fbAssets []asset.Asset
	fbSelCols := make(map[string][]float64)
	fbSet := make(map[string]bool)
	if m.fallback != nil {
		fbAssets = m.fallback.AssetList()
		for _, a := range fbAssets {
			fbSelCols[a.CompositeFigi] = make([]float64, T)
			fbSet[a.CompositeFigi] = true
		}
	}

	for ti, t := range times {
		bestVal := math.Inf(-1)
		var bestFigi string

		for _, a := range assets {
			// Skip assets that are in the fallback set so they
			// do not prevent fallback from triggering.
			if fbSet[a.CompositeFigi] {
				continue
			}
			v := df.ValueAt(a, m.metric, t)
			if math.IsNaN(v) {
				continue
			}
			if v > 0 && v > bestVal {
				bestVal = v
				bestFigi = a.CompositeFigi
			}
		}

		if bestFigi != "" {
			selCols[bestFigi][ti] = 1.0
		} else if m.fallback != nil {
			needsFallback = true
			for _, a := range fbAssets {
				fbSelCols[a.CompositeFigi][ti] = 1.0
			}
		}
	}

	// Insert fallback asset data into the DataFrame.
	if needsFallback {
		fbMetrics := m.fallback.MetricList()
		for _, a := range fbAssets {
			for _, met := range fbMetrics {
				vals := m.fallback.Column(a, met)
				if err := df.Insert(a, met, vals); err != nil {
					log.Warn().Err(err).
						Str("asset", a.CompositeFigi).
						Str("metric", string(met)).
						Msg("MaxAboveZero: failed to insert fallback data")
				}
			}
		}
	}

	// Write Selected columns for original assets.
	for _, a := range assets {
		if err := df.Insert(a, Selected, selCols[a.CompositeFigi]); err != nil {
			log.Warn().Err(err).
				Str("asset", a.CompositeFigi).
				Msg("MaxAboveZero: failed to insert Selected column")
		}
	}

	// Write Selected columns for fallback assets.
	for _, a := range fbAssets {
		if err := df.Insert(a, Selected, fbSelCols[a.CompositeFigi]); err != nil {
			log.Warn().Err(err).
				Str("asset", a.CompositeFigi).
				Msg("MaxAboveZero: failed to insert fallback Selected column")
		}
	}

	return df
}

// MaxAboveZero returns a Selector that picks the asset with the highest
// value above zero in the given metric column. If no assets qualify at
// a timestep, the fallback DataFrame's assets are inserted and marked
// as selected. Pass nil for fallback if no fallback is needed.
func MaxAboveZero(metric data.Metric, fallback *data.DataFrame) Selector {
	return maxAboveZero{metric: metric, fallback: fallback}
}
