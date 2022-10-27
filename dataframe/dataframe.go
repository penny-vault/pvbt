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
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"gonum.org/v1/gonum/floats"
)

// AddScalar adds the scalar value to all columns in dataframe df and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) AddScalar(scalar float64) *DataFrame {
	df = df.Copy()

	for colIdx := range df.ColNames {
		for rowIdx := range df.Vals[colIdx] {
			df.Vals[colIdx][rowIdx] += scalar
		}
	}
	return df
}

// AddVec adds the vector to all columns in dataframe and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) AddVec(vec []float64) *DataFrame {
	df = df.Copy()
	for idx := range df.ColNames {
		floats.Add(df.Vals[idx], vec)
	}
	return df
}

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

// Copy creates a copy of the dataframe
func (df *DataFrame) Copy() *DataFrame {
	df2 := &DataFrame{
		ColNames: make([]string, len(df.ColNames)),
		Dates:    make([]time.Time, len(df.Dates)),
		Vals:     make([][]float64, len(df.Vals)),
	}

	copy(df2.ColNames, df.ColNames)
	copy(df2.Dates, df.Dates)

	for idx := range df2.Vals {
		df2.Vals[idx] = make([]float64, len(df.Vals[idx]))
		copy(df2.Vals[idx], df.Vals[idx])
	}

	return df2
}

// Div divides all columns in dataframe df by the corresponding column in dataframe other and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) Div(other *DataFrame) *DataFrame {
	df = df.Copy()

	otherMap := make(map[string]int, len(other.ColNames))
	for idx, val := range other.ColNames {
		otherMap[val] = idx
	}

	for idx, colName := range df.ColNames {
		if otherIdx, ok := otherMap[colName]; ok {
			floats.Div(df.Vals[idx], other.Vals[otherIdx])
		}
	}
	return df
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

// Lag shifts the dataframe by the specified number of rows, replacing shifted values by math.NaN() and returns a new dataframe
func (df *DataFrame) Lag(n int) *DataFrame {
	df = df.Copy()
	prepend := make([]float64, n)
	for idx := range prepend {
		prepend[idx] = math.NaN()
	}

	for idx := range df.Vals {
		l := len(df.Vals[idx])
		df.Vals[idx] = append(prepend, df.Vals[idx]...)[:l]
	}
	return df
}

// Len returns the number of rows in the dataframe
func (df *DataFrame) Len() int {
	return len(df.Dates)
}

// Max selects the max value for each row and returns a new dataframe
func (df *DataFrame) Max() *DataFrame {
	maxDf := &DataFrame{
		ColNames: []string{"max"},
		Dates:    df.Dates,
		Vals:     [][]float64{make([]float64, len(df.Dates))},
	}

	for rowIdx := range df.Dates {
		row := make([]float64, 0, len(df.ColNames))
		for colIdx := range df.ColNames {
			row = append(row, df.Vals[colIdx][rowIdx])
		}
		maxDf.Vals[0][rowIdx] = floats.Max(row)
	}

	return maxDf
}

// Mean calculates the mean of all like columns in the dataframes and returns a new dataframe
// panics if rows are not equal.
func Mean(dfs ...*DataFrame) *DataFrame {
	resDf := dfs[0].Copy()

	otherMaps := make([]map[string]int, len(dfs))
	for dfIdx, resDf := range dfs {
		otherMaps[dfIdx] = make(map[string]int, len(resDf.ColNames))
		for idx, val := range resDf.ColNames {
			otherMaps[dfIdx][val] = idx
		}
	}

	for resColIdx, colName := range resDf.ColNames {
		for rowIdx := range resDf.Vals[0] {
			row := 0.0
			cnt := 0.0
			for dfIdx := range dfs {
				df := dfs[dfIdx]
				colIdx := otherMaps[dfIdx][colName]
				row += df.Vals[colIdx][rowIdx]
				cnt += 1
			}
			resDf.Vals[resColIdx][rowIdx] = row / cnt
		}
	}

	return resDf
}

// Mul multiplies all columns in dataframe df by the corresponding column in dataframe other and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) Mul(other *DataFrame) *DataFrame {
	df = df.Copy()

	otherMap := make(map[string]int, len(other.ColNames))
	for idx, val := range other.ColNames {
		otherMap[val] = idx
	}

	for idx, colName := range df.ColNames {
		if otherIdx, ok := otherMap[colName]; ok {
			floats.Mul(df.Vals[idx], other.Vals[otherIdx])
		}
	}
	return df
}

// MulScalar multiplies all columns in dataframe df by the scalar and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) MulScalar(scalar float64) *DataFrame {
	df = df.Copy()

	for colIdx := range df.ColNames {
		for rowIdx := range df.Vals[colIdx] {
			df.Vals[colIdx][rowIdx] *= scalar
		}
	}
	return df
}

// RollingSumScaled computes ∑ df[ii] * scalar and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) RollingSumScaled(ii int, scalar float64) *DataFrame {
	df2 := df.Copy()
	for colIdx := range df.ColNames {
		roll := 0.0
		dropIdx := 0
		for rowIdx := range df.Vals[colIdx] {
			if rowIdx >= ii {
				roll += df.Vals[colIdx][rowIdx]
				roll -= df.Vals[colIdx][dropIdx]
				df2.Vals[colIdx][rowIdx] = roll * scalar
				dropIdx += 1
			} else if rowIdx == (ii - 1) {
				roll += df.Vals[colIdx][rowIdx]
				df2.Vals[colIdx][rowIdx] = roll * scalar
			} else {
				df2.Vals[colIdx][rowIdx] = math.NaN()
				roll += df.Vals[colIdx][rowIdx]
			}
		}
	}
	return df2
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

// Table prints an ASCII formated table to stdout
func (df *DataFrame) Table() string {
	if len(df.Dates) == 0 {
		return "" // nothing to do as there is no data available in the dataframe
	}

	// construct table header
	tableCols := append([]string{"Date"}, df.ColNames...)

	// initialize table
	s := &strings.Builder{}
	table := tablewriter.NewWriter(s)
	table.SetHeader(tableCols)
	table.SetBorder(false) // Set Border to false

	for rowIdx, date := range df.Dates {
		row := []string{date.Format("2006-01-02")}
		for _, col := range df.Vals {
			row = append(row, fmt.Sprintf("%.4f", col[rowIdx]))
		}
		table.Append(row)
	}

	table.Render()
	return s.String()
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
