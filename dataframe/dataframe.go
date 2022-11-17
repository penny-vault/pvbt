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

// Append takes the date and values from other and appends them to df. If cols do not align, cols in df that are not in other are filled
// with NaN. If the start date of other is not greater than df then do nothing
func (df *DataFrame) Append(other *DataFrame) *DataFrame {
	// if there is no data in other then do nothing
	if len(other.Dates) == 0 {
		return df
	}

	// if the first date in other is not after the last date of df then do nothing
	otherFirstDate := other.Dates[0]
	if len(df.Dates) != 0 && (otherFirstDate.Before(df.Dates[len(df.Dates)-1]) || otherFirstDate.Equal(df.Dates[len(df.Dates)-1])) {
		return df
	}

	df.Dates = append(df.Dates, other.Dates...)
	colMap := make(map[string]int, len(other.ColNames))

	for colIdx, colName := range other.ColNames {
		colMap[colName] = colIdx
	}

	for colIdx, colName := range df.ColNames {
		if otherColIdx, ok := colMap[colName]; ok {
			// fill with vals from other
			df.Vals[colIdx] = append(df.Vals[colIdx], other.Vals[otherColIdx]...)
		} else {
			// fill with NaN
			for ii := 0; ii < len(other.Dates); ii++ {
				df.Vals[colIdx] = append(df.Vals[colIdx], math.NaN())
			}
		}
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

// Drop removes rows that contain the value `val` from the dataframe
func (df *DataFrame) Drop(val float64) *DataFrame {
	isNA := math.IsNaN(val)
	newVals := make([][]float64, len(df.Vals))
	newDates := make([]time.Time, 0, len(df.Vals))

	for rowIdx, rowDate := range df.Dates {
		keep := true
		for _, col := range df.Vals {
			rowVal := col[rowIdx]
			keep = keep && !(rowVal == val || (isNA && math.IsNaN(rowVal)))
			if !keep {
				break
			}
		}

		if keep {
			newDates = append(newDates, rowDate)
			for colIdx, col := range df.Vals {
				rowVal := col[rowIdx]
				newVals[colIdx] = append(newVals[colIdx], rowVal)
			}
		}
	}

	df.Vals = newVals
	df.Dates = newDates
	return df
}

// ForEach takes a lambda function of prototype func(rowIdx int, rowDate time.Time, colNames []string, vals []float64) []float64
// and updates the row with the returned value
func (df *DataFrame) ForEach(update bool, lambda func(int, time.Time, []string, []float64) []float64) {
	res := make([]float64, len(df.ColNames))
	for rowIdx, dt := range df.Dates {
		for colIdx := range df.ColNames {
			res[colIdx] = df.Vals[colIdx][rowIdx]
		}
		ret := lambda(rowIdx, dt, df.ColNames, res)
		if update {
			for colIdx, val := range ret {
				df.Vals[colIdx][rowIdx] = val
			}
		}
	}
}

// ForEachMap takes a lambda function of prototype func(rowIdx int, rowDate time.Time, vals map[string]float64) map[string]float64
// and updates the row with the returned value; this is a convenience function and is slower than the `ForEach` sister function
func (df *DataFrame) ForEachMap(update bool, lambda func(int, time.Time, map[string]float64) map[string]float64) {
	res := make(map[string]float64, len(df.ColNames))
	colMap := make(map[string]int, len(df.ColNames))
	for rowIdx, dt := range df.Dates {
		for colIdx, colName := range df.ColNames {
			res[colName] = df.Vals[colIdx][rowIdx]
		}
		ret := lambda(rowIdx, dt, res)
		if update {
			for colName, val := range ret {
				if colIdx, ok := colMap[colName]; ok {
					df.Vals[colIdx][rowIdx] = val
				}
			}
		}
	}
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

// IdxMax finds the column with the largest value for each row and stores it in a new dataframe with the column name 'idxmax'
func (df *DataFrame) IdxMax() *DataFrame {
	maxVals := make([]float64, 0, len(df.Dates))

	for rowIdx := range df.Dates {
		max := math.NaN()
		var ind int
		for colIdx := range df.ColNames {
			v := df.Vals[colIdx][rowIdx]
			if math.IsNaN(v) {
				max = math.NaN()
				break
			}
			if v > max || math.IsNaN(max) {
				max = v
				ind = colIdx
			}
		}

		if !math.IsNaN(max) {
			maxVals = append(maxVals, float64(ind))
		} else {
			maxVals = append(maxVals, math.NaN())
		}
	}

	return &DataFrame{
		Dates:    df.Dates,
		Vals:     [][]float64{maxVals},
		ColNames: []string{"idxmax"},
	}
}

// Insert a new column to the end of the dataframe
func (df *DataFrame) Insert(name string, col []float64) *DataFrame {
	df.ColNames = append(df.ColNames, name)
	df.Vals = append(df.Vals, col)
	return df
}

// InsertRow adds a new row to the dataframe. Date must be after the last date in the dataframe and vals must equal the number
// of columns. If either of these conditions are not met then panic
func (df *DataFrame) InsertRow(date time.Time, vals ...float64) *DataFrame {
	// Check that the last date in the dataframe is prior to the new date
	if len(df.Dates) != 0 && !df.Dates[len(df.Dates)-1].Before(date) {
		log.Panic().Time("lastDate", df.Dates[len(df.Dates)-1]).Time("newDate", date).Msg("newDate must be after lastDate")
	}

	// Check that hte number of columns equals the number of vals passed
	if len(vals) != len(df.ColNames) {
		log.Panic().Int("NumValsPassed", len(vals)).Int("NumColumns", len(df.ColNames)).Msg("number of vals passed must equal number of columns")
	}

	df.Dates = append(df.Dates, date)
	for colIdx := range df.ColNames {
		df.Vals[colIdx] = append(df.Vals[colIdx], vals[colIdx])
	}

	return df
}

// InsertMap adds a new row to the dataframe. Date must be after the last date in the dataframe otherwise panic.
// all columns must already exist in the dataframe, any additional columns in vals is ignored
func (df *DataFrame) InsertMap(date time.Time, vals map[string]float64) *DataFrame {
	// Check that the last date in the dataframe is prior to the new date
	if len(df.Dates) != 0 && !df.Dates[len(df.Dates)-1].Before(date) {
		log.Panic().Time("lastDate", df.Dates[len(df.Dates)-1]).Time("newDate", date).Msg("newDate must be after lastDate")
	}

	df.Dates = append(df.Dates, date)
	for colIdx, colName := range df.ColNames {
		if val, ok := vals[colName]; ok {
			df.Vals[colIdx] = append(df.Vals[colIdx], val)
		} else {
			df.Vals[colIdx] = append(df.Vals[colIdx], math.NaN())
		}
	}

	return df
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

// Last returns a new dataframe with only the last item of the current dataframe
func (df *DataFrame) Last() *DataFrame {
	if df.Len() == 0 {
		return df
	}

	lastVals := make([][]float64, len(df.ColNames))
	lastRow := len(df.Dates) - 1
	for idx, col := range df.Vals {
		lastVals[idx] = []float64{col[lastRow]}
	}

	newDf := &DataFrame{
		ColNames: df.ColNames,
		Dates:    []time.Time{df.Dates[len(df.Dates)-1]},
		Vals:     lastVals,
	}

	return newDf
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

// Table prints an ASCII formatted table to stdout
func (df *DataFrame) Table() string {
	if len(df.Dates) == 0 {
		return "<NO DATA>" // nothing to do as there is no data available in the dataframe
	}

	// construct table header
	tableCols := append([]string{"Date"}, df.ColNames...)

	// initialize table
	s := &strings.Builder{}
	table := tablewriter.NewWriter(s)
	table.SetHeader(tableCols)
	footer := make([]string, len(tableCols))
	footer[0] = "Num Rows"
	footer[1] = fmt.Sprintf("%d", df.Len())
	table.SetFooter(footer)
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
