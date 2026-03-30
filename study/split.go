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

package study

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DateRange represents a closed interval of time [Start, End].
type DateRange struct {
	Start time.Time
	End   time.Time
}

// Split describes a single train/test partition of a date range, optionally
// excluding sub-ranges from the training period.
type Split struct {
	Name      string
	FullRange DateRange
	Train     DateRange
	Test      DateRange
	Exclude   []DateRange
}

// overlaps reports whether two DateRanges share any time.
func overlaps(aa, bb DateRange) bool {
	return aa.Start.Before(bb.End) && bb.Start.Before(aa.End)
}

// SubtractRanges returns the portions of window not covered by any range in
// exclude. Exclude ranges are assumed non-overlapping. The returned slices
// share boundary timestamps with the exclusion ranges; this is acceptable
// because metric computations are insensitive to a single shared data point.
func SubtractRanges(window DateRange, exclude []DateRange) []DateRange {
	if len(exclude) == 0 {
		return []DateRange{window}
	}

	// Sort exclusions by start time.
	sorted := make([]DateRange, len(exclude))
	copy(sorted, exclude)
	sort.Slice(sorted, func(ii, jj int) bool {
		return sorted[ii].Start.Before(sorted[jj].Start)
	})

	var result []DateRange

	cursor := window.Start

	for _, ex := range sorted {
		// Clamp exclusion to the window.
		exStart := ex.Start
		if exStart.Before(window.Start) {
			exStart = window.Start
		}

		exEnd := ex.End
		if exEnd.After(window.End) {
			exEnd = window.End
		}

		// Emit the segment before this exclusion.
		if cursor.Before(exStart) {
			result = append(result, DateRange{Start: cursor, End: exStart})
		}

		// Advance cursor past the exclusion.
		if exEnd.After(cursor) {
			cursor = exEnd
		}
	}

	// Emit the segment after the last exclusion.
	if cursor.Before(window.End) {
		result = append(result, DateRange{Start: cursor, End: window.End})
	}

	return result
}

// TrainTest produces a single split where training covers [start, cutoff] and
// testing covers [cutoff, end]. It returns an error if start >= end or if
// cutoff falls outside [start, end].
func TrainTest(start, cutoff, end time.Time) ([]Split, error) {
	if !start.Before(end) {
		return nil, fmt.Errorf("train/test split: start must be before end")
	}

	if cutoff.Before(start) || cutoff.After(end) {
		return nil, fmt.Errorf("train/test split: cutoff %v is outside [%v, %v]", cutoff, start, end)
	}

	sp := Split{
		Name:      "train/test",
		FullRange: DateRange{Start: start, End: end},
		Train:     DateRange{Start: start, End: cutoff},
		Test:      DateRange{Start: cutoff, End: end},
	}

	return []Split{sp}, nil
}

// KFold partitions [start, end] into the given number of equal folds. Each
// split holds one fold out as the test set and trains on the full range, with
// the test fold listed in Exclude. It returns an error if folds < 2.
func KFold(start, end time.Time, folds int) ([]Split, error) {
	if folds < 2 {
		return nil, fmt.Errorf("k-fold: folds must be at least 2, got %d", folds)
	}

	totalDur := end.Sub(start)
	foldDur := totalDur / time.Duration(folds)

	splits := make([]Split, 0, folds)

	for ii := range folds {
		foldStart := start.Add(time.Duration(ii) * foldDur)

		var foldEnd time.Time
		if ii == folds-1 {
			foldEnd = end
		} else {
			foldEnd = start.Add(time.Duration(ii+1) * foldDur)
		}

		testRange := DateRange{Start: foldStart, End: foldEnd}

		sp := Split{
			Name:      fmt.Sprintf("fold %d/%d", ii+1, folds),
			FullRange: DateRange{Start: start, End: end},
			Train:     DateRange{Start: start, End: end},
			Test:      testRange,
			Exclude:   []DateRange{testRange},
		}

		splits = append(splits, sp)
	}

	return splits, nil
}

// WalkForward produces an expanding-window walk-forward validation. The first
// split trains on [start, start+minTrain) and tests on [start+minTrain,
// start+minTrain+testLen). Each subsequent split advances the test window by
// step and expands the training window accordingly. It returns an error if
// minTrain+testLen exceeds end-start.
func WalkForward(start, end time.Time, minTrain, testLen, step time.Duration) ([]Split, error) {
	totalDur := end.Sub(start)
	if minTrain+testLen > totalDur {
		return nil, fmt.Errorf(
			"walk-forward: minTrain (%v) + testLen (%v) exceeds total range (%v)",
			minTrain, testLen, totalDur,
		)
	}

	var (
		splits []Split
		idx    int
	)

	for {
		trainEnd := start.Add(minTrain + time.Duration(idx)*step)
		testEnd := trainEnd.Add(testLen)

		if testEnd.After(end) {
			break
		}

		sp := Split{
			Name:      fmt.Sprintf("walk-forward %d", idx+1),
			FullRange: DateRange{Start: start, End: testEnd},
			Train:     DateRange{Start: start, End: trainEnd},
			Test:      DateRange{Start: trainEnd, End: testEnd},
		}

		splits = append(splits, sp)
		idx++
	}

	return splits, nil
}

// ScenarioLeaveNOut produces C(len(scenarios), holdOut) splits. Each split
// holds out holdOut scenarios as the test set. Any remaining scenario that
// overlaps a held-out scenario is added to Exclude. The FullRange spans the
// earliest Start to the latest End across all scenarios. It returns an error
// if holdOut < 1 or holdOut > len(scenarios).
func ScenarioLeaveNOut(scenarios []Scenario, holdOut int) ([]Split, error) {
	if holdOut < 1 {
		return nil, fmt.Errorf("scenario leave-n-out: holdOut must be at least 1, got %d", holdOut)
	}

	if holdOut > len(scenarios) {
		return nil, fmt.Errorf(
			"scenario leave-n-out: holdOut (%d) exceeds number of scenarios (%d)",
			holdOut, len(scenarios),
		)
	}

	// Compute the overall full range across all scenarios.
	fullStart := scenarios[0].Start
	fullEnd := scenarios[0].End

	for _, sc := range scenarios[1:] {
		if sc.Start.Before(fullStart) {
			fullStart = sc.Start
		}

		if sc.End.After(fullEnd) {
			fullEnd = sc.End
		}
	}

	fullRange := DateRange{Start: fullStart, End: fullEnd}

	var splits []Split

	// Generate all C(n, holdOut) combinations via index combinations.
	indices := make([]int, holdOut)
	for ii := range holdOut {
		indices[ii] = ii
	}

	for {
		// Build the held-out set.
		heldOut := make([]Scenario, holdOut)
		for ii, idx := range indices {
			heldOut[ii] = scenarios[idx]
		}

		// Determine test range: earliest start to latest end among held-out.
		testStart := heldOut[0].Start
		testEnd := heldOut[0].End

		for _, sc := range heldOut[1:] {
			if sc.Start.Before(testStart) {
				testStart = sc.Start
			}

			if sc.End.After(testEnd) {
				testEnd = sc.End
			}
		}

		// Build the held-out date ranges for overlap checking.
		heldRanges := make([]DateRange, holdOut)
		for ii, sc := range heldOut {
			heldRanges[ii] = DateRange{Start: sc.Start, End: sc.End}
		}

		// Find scenarios not held out that overlap with any held-out range.
		heldSet := make(map[int]bool, holdOut)
		for _, idx := range indices {
			heldSet[idx] = true
		}

		var exclude []DateRange

		for jj, sc := range scenarios {
			if heldSet[jj] {
				continue
			}

			scRange := DateRange{Start: sc.Start, End: sc.End}

			for _, hr := range heldRanges {
				if overlaps(scRange, hr) {
					exclude = append(exclude, scRange)
					break
				}
			}
		}

		// Build held-out names for the split name.
		names := make([]string, holdOut)
		for ii, sc := range heldOut {
			names[ii] = sc.Name
		}

		sp := Split{
			Name:      fmt.Sprintf("leave-out: %s", strings.Join(names, ", ")),
			FullRange: fullRange,
			Train:     fullRange,
			Test:      DateRange{Start: testStart, End: testEnd},
			Exclude:   append(heldRanges, exclude...),
		}

		splits = append(splits, sp)

		// Advance to the next combination.
		if !nextCombination(indices, len(scenarios)) {
			break
		}
	}

	return splits, nil
}

// nextCombination advances the index slice to the next combination in
// lexicographic order. It returns false when no more combinations exist.
func nextCombination(indices []int, nn int) bool {
	kk := len(indices)

	ii := kk - 1
	for ii >= 0 && indices[ii] == nn-kk+ii {
		ii--
	}

	if ii < 0 {
		return false
	}

	indices[ii]++

	for jj := ii + 1; jj < kk; jj++ {
		indices[jj] = indices[jj-1] + 1
	}

	return true
}
