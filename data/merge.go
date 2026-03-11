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

package data

import (
	"fmt"
	"sort"
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// MergeColumns combines DataFrames with the same timestamps but different
// metrics or assets. Used for multi-provider routing.
func MergeColumns(frames ...*DataFrame) (*DataFrame, error) {
	if len(frames) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil), nil
	}
	if len(frames) == 1 {
		return frames[0], nil
	}

	// Verify all frames have the same timestamps.
	base := frames[0]
	for i := 1; i < len(frames); i++ {
		if len(frames[i].times) != len(base.times) {
			return nil, fmt.Errorf("MergeColumns: timestamp count mismatch: %d vs %d",
				len(base.times), len(frames[i].times))
		}
		for j := range base.times {
			if !base.times[j].Equal(frames[i].times[j]) {
				return nil, fmt.Errorf("MergeColumns: timestamp mismatch at index %d", j)
			}
		}
	}

	// Start with a copy of the base, then insert columns from other frames.
	result := base.Copy()
	for i := 1; i < len(frames); i++ {
		f := frames[i]
		for _, a := range f.assets {
			for _, m := range f.metrics {
				col := f.Column(a, m)
				if col != nil {
					colCopy := make([]float64, len(col))
					copy(colCopy, col)
					if err := result.Insert(a, m, colCopy); err != nil {
						return nil, fmt.Errorf("MergeColumns: insert: %w", err)
					}
				}
			}
		}
	}
	return result, nil
}

// MergeTimes combines DataFrames with the same assets and metrics but
// different, non-overlapping time ranges. Timestamps must not overlap.
func MergeTimes(frames ...*DataFrame) (*DataFrame, error) {
	if len(frames) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil), nil
	}
	if len(frames) == 1 {
		return frames[0], nil
	}

	// Sort frames by start time.
	sorted := make([]*DataFrame, len(frames))
	copy(sorted, frames)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start().Before(sorted[j].Start())
	})

	// Verify no overlap.
	for i := 1; i < len(sorted); i++ {
		if !sorted[i].Start().After(sorted[i-1].End()) {
			return nil, fmt.Errorf("MergeTimes: overlapping time ranges")
		}
	}

	// Collect all times.
	var allTimes []time.Time
	for _, f := range sorted {
		allTimes = append(allTimes, f.times...)
	}

	// Use the first frame's assets and metrics as the schema.
	assets := make([]asset.Asset, len(sorted[0].assets))
	copy(assets, sorted[0].assets)
	metrics := make([]Metric, len(sorted[0].metrics))
	copy(metrics, sorted[0].metrics)

	totalLen := len(allTimes)
	newData := make([]float64, len(assets)*len(metrics)*totalLen)

	tOffset := 0
	for _, f := range sorted {
		fTimeLen := len(f.times)
		for aIdx, a := range assets {
			for mIdx, m := range metrics {
				col := f.Column(a, m)
				if col == nil {
					continue
				}
				dstOff := (aIdx*len(metrics)+mIdx)*totalLen + tOffset
				copy(newData[dstOff:dstOff+fTimeLen], col)
			}
		}
		tOffset += fTimeLen
	}

	return NewDataFrame(allTimes, assets, metrics, newData)
}
