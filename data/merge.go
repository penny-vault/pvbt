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
		return mustNewDataFrame(nil, nil, nil, 0, nil), nil
	}

	if len(frames) == 1 {
		return frames[0], nil
	}

	// Verify all frames have the same timestamps.
	base := frames[0]
	for frameIdx := 1; frameIdx < len(frames); frameIdx++ {
		if len(frames[frameIdx].times) != len(base.times) {
			return nil, fmt.Errorf("MergeColumns: timestamp count mismatch: %d vs %d",
				len(base.times), len(frames[frameIdx].times))
		}

		for j := range base.times {
			if !base.times[j].Equal(frames[frameIdx].times[j]) {
				return nil, fmt.Errorf("MergeColumns: timestamp mismatch at index %d", j)
			}
		}
	}

	// Start with a copy of the base, then insert columns from other frames.
	result := base.Copy()

	for frameIdx := 1; frameIdx < len(frames); frameIdx++ {
		f := frames[frameIdx]
		for _, mergeAsset := range f.assets {
			for _, metric := range f.metrics {
				col := f.Column(mergeAsset, metric)
				if col != nil {
					colCopy := make([]float64, len(col))
					copy(colCopy, col)

					if err := result.Insert(mergeAsset, metric, colCopy); err != nil {
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
		return mustNewDataFrame(nil, nil, nil, 0, nil), nil
	}

	if len(frames) == 1 {
		return frames[0], nil
	}

	// Filter out empty frames.
	nonEmpty := make([]*DataFrame, 0, len(frames))
	for _, f := range frames {
		if f.Len() > 0 {
			nonEmpty = append(nonEmpty, f)
		}
	}

	if len(nonEmpty) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil), nil
	}

	if len(nonEmpty) == 1 {
		return nonEmpty[0], nil
	}

	// Sort frames by start time.
	sorted := make([]*DataFrame, len(nonEmpty))
	copy(sorted, nonEmpty)
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

	result, err := NewDataFrame(allTimes, assets, metrics, sorted[0].freq, newData)
	if err != nil {
		return nil, err
	}

	// Concatenate risk-free rates if all frames have them.
	allHaveRF := true

	for _, f := range sorted {
		if f.RiskFreeRates() == nil {
			allHaveRF = false
			break
		}
	}

	if allHaveRF {
		rfConcat := make([]float64, 0, totalLen)
		for _, f := range sorted {
			rfConcat = append(rfConcat, f.RiskFreeRates()...)
		}

		if err := result.SetRiskFreeRates(rfConcat); err != nil {
			return nil, fmt.Errorf("concat: set risk-free rates: %w", err)
		}
	}

	return result, nil
}
