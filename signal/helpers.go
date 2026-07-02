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

package signal

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// barsToCalendarDays converts a required trading-bar count into a calendar-day
// lookback long enough to contain at least that many trading bars: the 7/5
// factor covers weekends, bars/20 covers regular market holidays, and the
// constant absorbs holiday clusters and rounding.
func barsToCalendarDays(bars int) int {
	return bars*7/5 + bars/20 + 10
}

// extendedWindow fetches the caller's period plus extraBars additional trading
// bars of warm-up history from the universe.
//
// Day-unit periods are interpreted as trading-bar counts: the fetch is
// inflated to enough calendar days to cover weekends and market holidays and
// the result is trimmed to the last period.N + extraBars bars. Calendar-unit
// periods (months, years, to-date) keep their span and gain extraBars bars of
// history before the window start. Intraday periods extend N directly because
// their bars already count trading time.
//
// It returns the trimmed frame together with the number of bars that
// correspond to the caller's original period, clamped to the rows actually
// available.
func extendedWindow(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, extraBars int, metrics ...data.Metric) (*data.DataFrame, int, error) {
	ref := assetUniverse.CurrentDate()

	var fetchPeriod portfolio.Period

	switch period.Unit {
	case portfolio.UnitDay:
		fetchPeriod = portfolio.Days(barsToCalendarDays(period.N + extraBars))
	case portfolio.UnitMinuteBar, portfolio.UnitDailyAtTime:
		fetchPeriod = portfolio.Period{N: period.N + extraBars, Unit: period.Unit, TimeOfDay: period.TimeOfDay}
	default:
		spanDays := int(math.Ceil(ref.Sub(period.Before(ref)).Hours() / 24))
		fetchPeriod = portfolio.Days(spanDays + barsToCalendarDays(extraBars))
	}

	df, err := assetUniverse.Window(ctx, fetchPeriod, metrics...)
	if err != nil {
		return nil, 0, err
	}

	switch period.Unit {
	case portfolio.UnitDay:
		df = tailBars(df, period.N+extraBars)
		return df, min(period.N, df.Len()), nil
	case portfolio.UnitMinuteBar, portfolio.UnitDailyAtTime:
		return df, min(period.N, df.Len()), nil
	default:
		start := period.Before(ref)
		startDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
		times := df.Times()

		idx := sort.Search(len(times), func(ii int) bool {
			return !times[ii].Before(startDate)
		})

		baseBars := len(times) - idx

		trimIdx := idx - extraBars
		if trimIdx < 0 {
			trimIdx = 0
		}

		if trimIdx > 0 {
			df = df.Between(times[trimIdx], times[len(times)-1])
		}

		return df, baseBars, nil
	}
}

// tailBars returns df trimmed to at most the last barCount rows.
func tailBars(df *data.DataFrame, barCount int) *data.DataFrame {
	if barCount <= 0 || df.Len() <= barCount {
		return df
	}

	times := df.Times()

	return df.Between(times[len(times)-barCount], times[len(times)-1])
}

// alignFrames trims two DataFrames to their common timestamps so that row ii
// of each frame refers to the same bar. Daily-or-coarser frames are matched
// on calendar date, finer frames on the exact timestamp. Returns an error
// when the frames share no timestamps or contain duplicates that prevent a
// one-to-one alignment.
func alignFrames(dfA, dfB *data.DataFrame) (*data.DataFrame, *data.DataFrame, error) {
	daily := dfA.Frequency() >= data.Daily && dfB.Frequency() >= data.Daily

	keyOf := func(tt time.Time) int64 {
		if daily {
			return int64(tt.Year())*10000 + int64(tt.Month())*100 + int64(tt.Day())
		}

		return tt.UnixNano()
	}

	timesA := dfA.Times()
	timesB := dfB.Times()

	if len(timesA) == len(timesB) {
		same := true

		for ii := range timesA {
			if keyOf(timesA[ii]) != keyOf(timesB[ii]) {
				same = false
				break
			}
		}

		if same {
			return dfA, dfB, nil
		}
	}

	keysA := make(map[int64]struct{}, len(timesA))
	for _, tt := range timesA {
		keysA[keyOf(tt)] = struct{}{}
	}

	keysB := make(map[int64]struct{}, len(timesB))
	for _, tt := range timesB {
		keysB[keyOf(tt)] = struct{}{}
	}

	alignedA := dfA.Filter(func(tt time.Time, _ *data.DataFrame) bool {
		_, ok := keysB[keyOf(tt)]
		return ok
	})

	alignedB := dfB.Filter(func(tt time.Time, _ *data.DataFrame) bool {
		_, ok := keysA[keyOf(tt)]
		return ok
	})

	if alignedA.Len() == 0 || alignedB.Len() == 0 {
		return nil, nil, fmt.Errorf("no overlapping timestamps between the two data sets")
	}

	if alignedA.Len() != alignedB.Len() {
		return nil, nil, fmt.Errorf("cannot align data sets: duplicate timestamps (%d vs %d overlapping rows)", alignedA.Len(), alignedB.Len())
	}

	return alignedA, alignedB, nil
}

// populationStd computes the population (N denominator) standard deviation of
// values. Returns NaN for an empty slice.
func populationStd(values []float64) float64 {
	nn := len(values)
	if nn == 0 {
		return math.NaN()
	}

	sum := 0.0
	for _, vv := range values {
		sum += vv
	}

	mean := sum / float64(nn)

	sumSq := 0.0

	for _, vv := range values {
		diff := vv - mean
		sumSq += diff * diff
	}

	return math.Sqrt(sumSq / float64(nn))
}

// zScore computes the z-score of the last element in values relative to the
// mean and standard deviation of the full slice. Returns an error if the
// standard deviation is zero (constant series) or the slice has fewer than
// 2 elements.
func zScore(values []float64) (float64, error) {
	nn := len(values)
	if nn < 2 {
		return 0, fmt.Errorf("zScore: need at least 2 values, got %d", nn)
	}

	sum := 0.0
	for _, vv := range values {
		sum += vv
	}

	mean := sum / float64(nn)

	sumSq := 0.0

	for _, vv := range values {
		diff := vv - mean
		sumSq += diff * diff
	}

	stddev := math.Sqrt(sumSq / float64(nn))
	if stddev == 0 {
		return 0, fmt.Errorf("zScore: standard deviation is zero (constant series)")
	}

	return (values[nn-1] - mean) / stddev, nil
}

// linRegress performs simple linear regression of yy on xx, returning the
// slope and intercept. Both slices must have the same length (>= 2).
func linRegress(xx, yy []float64) (slope, intercept float64, err error) {
	nn := len(xx)
	if nn < 2 {
		return 0, 0, fmt.Errorf("linRegress: need at least 2 points, got %d", nn)
	}

	if len(yy) != nn {
		return 0, 0, fmt.Errorf("linRegress: x and y lengths differ (%d vs %d)", nn, len(yy))
	}

	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0

	for ii := range nn {
		sumX += xx[ii]
		sumY += yy[ii]
		sumXY += xx[ii] * yy[ii]
		sumX2 += xx[ii] * xx[ii]
	}

	nf := float64(nn)
	denom := nf*sumX2 - sumX*sumX

	if denom == 0 {
		return 0, 0, fmt.Errorf("linRegress: all x values are identical")
	}

	slope = (nf*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / nf

	return slope, intercept, nil
}
