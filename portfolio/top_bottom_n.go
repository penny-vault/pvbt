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
	"sort"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/rs/zerolog/log"
)

type topBottomN struct {
	count     int
	metric    data.Metric
	ascending bool
}

type assetValue struct {
	asset asset.Asset
	value float64
}

// Select marks the top or bottom N assets by metric value as selected
// at each timestep. The DataFrame is mutated in place.
func (tb topBottomN) Select(df *data.DataFrame) *data.DataFrame {
	times := df.Times()
	assets := df.AssetList()
	nTimes := len(times)

	selCols := make(map[string][]float64, len(assets))
	for _, a := range assets {
		selCols[a.CompositeFigi] = make([]float64, nTimes)
	}

	for timeIdx, timestamp := range times {
		ranked := make([]assetValue, 0, len(assets))

		for _, a := range assets {
			v := df.ValueAt(a, tb.metric, timestamp)
			if !math.IsNaN(v) {
				ranked = append(ranked, assetValue{asset: a, value: v})
			}
		}

		sort.Slice(ranked, func(i, j int) bool {
			if tb.ascending {
				return ranked[i].value < ranked[j].value
			}

			return ranked[i].value > ranked[j].value
		})

		limit := tb.count
		if limit > len(ranked) {
			limit = len(ranked)
		}

		for idx := 0; idx < limit; idx++ {
			selCols[ranked[idx].asset.CompositeFigi][timeIdx] = 1.0
		}
	}

	for _, a := range assets {
		if err := df.Insert(a, Selected, selCols[a.CompositeFigi]); err != nil {
			log.Warn().Err(err).
				Str("asset", a.CompositeFigi).
				Msg("TopBottomN: failed to insert Selected column")
		}
	}

	return df
}

// TopN returns a Selector that picks the N assets with the highest
// values in the given metric column at each timestep. Panics if n < 1.
func TopN(count int, metric data.Metric) Selector {
	if count < 1 {
		panic("TopN: count must be >= 1")
	}

	return topBottomN{count: count, metric: metric, ascending: false}
}

// BottomN returns a Selector that picks the N assets with the lowest
// values in the given metric column at each timestep. Panics if n < 1.
func BottomN(count int, metric data.Metric) Selector {
	if count < 1 {
		panic("BottomN: count must be >= 1")
	}

	return topBottomN{count: count, metric: metric, ascending: true}
}
