// Copyright 2021-2022
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

package dataframe

import (
	"math"
	"sort"
	"time"

	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
)

// Breakout takes a dataframe with multiple columns and returns a map of dataframes, one per column
func (df *DataFrame) Breakout() DataFrameMap {
	dfMap := DataFrameMap{}
	for idx, col := range df.ColNames {
		dfMap[col] = &DataFrame{
			Dates:    df.Dates,
			ColNames: []string{col},
			Vals:     [][]float64{df.Vals[idx]},
		}
	}
	return dfMap
}

// ColCount returns the number of columns in the dataframe
func (df *DataFrame) ColCount() int {
	return len(df.ColNames)
}

// Drop removes rows that contain the value `val` from the dataframe
func (df *DataFrame) Drop(val float64) *DataFrame {
	isNA := math.IsNaN(val)
	newVals := make([][]float64, len(df.Vals))
	newDates := make([]time.Time, 0, len(df.Vals))
	for colIdx, col := range df.Vals {
		for rowIdx, rowVal := range col {
			if !(rowVal == val || (isNA && math.IsNaN(rowVal))) {
				newVals[colIdx] = append(newVals[colIdx], rowVal)
				newDates = append(newDates, df.Dates[rowIdx])
			}
		}
	}
	df.Vals = newVals
	df.Dates = newDates
	return df
}

// Frequency returns a data frame filtered to the requested frequency; note this is not
// an in-place function but creates a copy of the data
func (df *DataFrame) Frequency(frequency Frequency) *DataFrame {
	var schedule *tradecron.TradeCron
	var err error

	switch frequency {
	case Daily:
		schedule, err = tradecron.New("@close * * *", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close * * *").Msg("could not build tradecron schedule")
		}
	case WeekBegin:
		schedule, err = tradecron.New("@close @weekbegin", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @weekbegin").Msg("could not build tradecron schedule")
		}
	case WeekEnd:
		schedule, err = tradecron.New("@close @weekend", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @weekend").Msg("could not build tradecron schedule")
		}
	case MonthBegin:
		schedule, err = tradecron.New("@close @monthbegin", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthbegin").Msg("could not build tradecron schedule")
		}
	case MonthEnd:
		schedule, err = tradecron.New("@close @monthend", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthend").Msg("could not build tradecron schedule")
		}
	case YearBegin:
		schedule, err = tradecron.New("@close @monthbegin 0 * * 1", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthbegin 0 * * 1").Msg("could not build tradecron schedule")
		}
	case YearEnd:
		schedule, err = tradecron.New("@close @monthend 0 * * 12", tradecron.RegularHours)
		log.Debug().Str("ScheduleString", schedule.ScheduleString).Msg("trick")
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthend 0 * * 12").Msg("could not build tradecron schedule")
		}
	default:
		log.Panic().Str("Frequency", string(frequency)).Msg("Unknown frequncy provided to dataframe frequency function")
	}

	newDates := make([]time.Time, 0, len(df.Dates))
	newVals := make([][]float64, len(df.ColNames))
	for rowIdx, xx := range df.Dates {
		if schedule.IsTradeDay(xx) {
			newDates = append(newDates, xx)
			for colIdx := range newVals {
				newVals[colIdx] = append(newVals[colIdx], df.Vals[colIdx][rowIdx])
			}
		}
	}

	newDf := &DataFrame{
		Dates:    newDates,
		ColNames: df.ColNames,
		Vals:     newVals,
	}

	return newDf
}

// Len returns the number of rows in the dataframe
func (df *DataFrame) Len() int {
	return len(df.Dates)
}

// Split the dataframe into 2, with columns being in the first dataframe and
// all remaining columns in the second
func (df *DataFrame) Split(columns ...string) (*DataFrame, *DataFrame) {
	one := &DataFrame{
		Dates:    df.Dates,
		ColNames: []string{},
		Vals:     [][]float64{},
	}
	two := &DataFrame{
		Dates:    df.Dates,
		ColNames: []string{},
		Vals:     [][]float64{},
	}

	// convert requested columns to a map for easy lookup
	colMap := make(map[string]bool, len(columns))
	for _, col := range columns {
		colMap[col] = true
	}

	for idx, col := range df.ColNames {
		if _, ok := colMap[col]; ok {
			one.ColNames = append(one.ColNames, col)
			one.Vals = append(one.Vals, df.Vals[idx])
		} else {
			two.ColNames = append(two.ColNames, col)
			two.Vals = append(two.Vals, df.Vals[idx])
		}
	}

	return one, two
}

// Trim the dataframe to the specified date range (inclusive)
func (df *DataFrame) Trim(begin, end time.Time) *DataFrame {
	// special case 0: requested range is invalid
	if end.Before(begin) {
		df.Dates = []time.Time{}
		df.Vals = [][]float64{}
		return df
	}

	// special case 1: data frame is empty
	if df.Len() == 0 {
		return df
	}

	// special case 2: end time is before data frame start
	if end.Before(df.Dates[0]) {
		df.Dates = []time.Time{}
		df.Vals = [][]float64{}
		return df
	}

	// special case 3: start time is after data frame end
	if begin.After(df.Dates[len(df.Dates)-1]) {
		df.Dates = []time.Time{}
		df.Vals = [][]float64{}
		return df
	}

	// Use binary search to find the index corresponding to the start and end times
	beginIdx := sort.Search(len(df.Dates), func(i int) bool {
		idxVal := df.Dates[i]
		return (idxVal.After(begin) || idxVal.Equal(begin))
	})

	endIdx := sort.Search(len(df.Dates), func(i int) bool {
		idxVal := df.Dates[i]
		return (idxVal.After(end) || idxVal.Equal(end))
	})

	if endIdx != len(df.Dates) {
		endIdx += 1
	}
	df.Dates = df.Dates[beginIdx:endIdx]
	for colIdx, col := range df.Vals {
		df.Vals[colIdx] = col[beginIdx:endIdx]
	}

	return df
}
