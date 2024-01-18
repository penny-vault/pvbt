// Copyright 2021-2023
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
func (df *DataFrame[T]) Append(other *DataFrame[T]) *DataFrame[T] {
	// if there is no data in other then do nothing
	if len(other.Index) == 0 {
		return df
	}

	// if the first date in other is not after the last date of df then do nothing
	otherFirstDate := other.Index[0]
	if otherFirstDate, ok := any(otherFirstDate).(time.Time); ok {
		if len(df.Index) != 0 {
			lastDate := any(df.Index[len(df.Index)-1]).(time.Time)
			if otherFirstDate.Before(lastDate) || otherFirstDate.Equal(lastDate) {
				return df
			}
		}
	}

	df.Index = append(df.Index, other.Index...)
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
			for ii := 0; ii < len(other.Index); ii++ {
				df.Vals[colIdx] = append(df.Vals[colIdx], math.NaN())
			}
		}
	}

	return df
}

// AsMap creates a map with the index as the key and the specified column as the value
func (df *DataFrame[T]) AsMap(colName string) map[T]float64 {
	res := make(map[T]float64, df.Len())
	colIdx := df.ColIndex(colName)
	if colIdx == -1 {
		// column does exist, return empty list
		return res
	}

	for idx, rowKey := range df.Index {
		res[rowKey] = df.Vals[colIdx][idx]
	}

	return res
}

// Breakout takes a dataframe with multiple columns and returns a map of dataframes, one per column
func (df *DataFrame[T]) Breakout() Map[T] {
	dfMap := Map[T]{}
	for idx, col := range df.ColNames {
		dfMap[col] = &DataFrame[T]{
			Index:    df.Index,
			ColNames: []string{col},
			Vals:     [][]float64{df.Vals[idx]},
		}
	}
	return dfMap
}

// Get index of specified column; returns -1 if column doesn't exist
func (df *DataFrame[T]) ColIndex(colName string) int {
	for idx, val := range df.ColNames {
		if colName == val {
			return idx
		}
	}

	return -1
}

// ColCount returns the number of columns in the dataframe
func (df *DataFrame[T]) ColCount() int {
	return len(df.ColNames)
}

// Copy creates a copy of the dataframe
func (df *DataFrame[T]) Copy() *DataFrame[T] {
	df2 := &DataFrame[T]{
		ColNames: make([]string, len(df.ColNames)),
		Index:    make([]T, len(df.Index)),
		Vals:     make([][]float64, len(df.Vals)),
	}

	copy(df2.ColNames, df.ColNames)
	copy(df2.Index, df.Index)

	for idx := range df2.Vals {
		df2.Vals[idx] = make([]float64, len(df.Vals[idx]))
		copy(df2.Vals[idx], df.Vals[idx])
	}

	return df2
}

// Drop removes rows that contain the value `val` from the dataframe
func (df *DataFrame[T]) Drop(val float64) *DataFrame[T] {
	isNA := math.IsNaN(val)
	newVals := make([][]float64, len(df.Vals))
	newIndex := make([]T, 0, len(df.Vals))

	for idx, rowIdx := range df.Index {
		keep := true
		for _, col := range df.Vals {
			rowVal := col[idx]
			keep = keep && !(rowVal == val || (isNA && math.IsNaN(rowVal)))
			if !keep {
				break
			}
		}

		if keep {
			newIndex = append(newIndex, rowIdx)
			for colIdx, col := range df.Vals {
				rowVal := col[idx]
				newVals[colIdx] = append(newVals[colIdx], rowVal)
			}
		}
	}

	df.Vals = newVals
	df.Index = newIndex
	return df
}

// End returns the last time in the DataFrame
func (df *DataFrame[T]) End() time.Time {
	if len(df.Index) == 0 {
		return time.Time{}
	}

	if lastDate, ok := any(df.Index[len(df.Index)-1]).(time.Time); ok {
		return lastDate
	}

	return time.Time{}
}

// ForEachMap takes a lambda function of prototype func(rowIdx int, rowDate time.Time, vals map[string]float64) map[string]float64
// and updates the row with the returned value; if nil is returned then don't update the row, otherwise update row with returned values
func (df *DataFrame[T]) ForEach(lambda func(int, T, map[string]float64) map[string]float64) {
	res := make(map[string]float64, len(df.ColNames))
	colMap := make(map[string]int, len(df.ColNames))
	for idx, rowIdx := range df.Index {
		for colIdx, colName := range df.ColNames {
			res[colName] = df.Vals[colIdx][idx]
		}
		ret := lambda(idx, rowIdx, res)
		for colName, val := range ret {
			if colIdx, ok := colMap[colName]; ok {
				df.Vals[colIdx][idx] = val
			}
		}
	}
}

// Frequency returns a data frame filtered to the requested frequency; note this is not
// an in-place function but creates a copy of the data
//
// NOTE: If the dataframe's index is not time.Time then the function will throw an exception
func (df *DataFrame[T]) Frequency(frequency Frequency) *DataFrame[T] {
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

	newIndex := make([]T, 0, len(df.Index))
	newVals := make([][]float64, len(df.ColNames))
	for idx, rowIdx := range df.Index {
		dt := any(rowIdx).(time.Time)
		if schedule.IsTradeDay(dt) {
			newIndex = append(newIndex, rowIdx)
			for colIdx := range newVals {
				newVals[colIdx] = append(newVals[colIdx], df.Vals[colIdx][idx])
			}
		}
	}

	newDf := &DataFrame[T]{
		Index:    newIndex,
		ColNames: df.ColNames,
		Vals:     newVals,
	}

	return newDf
}

// IdxMax finds the column with the largest value for each row and stores it in a new dataframe with the column name 'idxmax'
func (df *DataFrame[T]) IdxMax() *DataFrame[T] {
	maxVals := make([]float64, 0, len(df.Index))

	for rowIdx := range df.Index {
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

	return &DataFrame[T]{
		Index:    df.Index,
		Vals:     [][]float64{maxVals},
		ColNames: []string{"idxmax"},
	}
}

// Insert a new column to the end of the dataframe
func (df *DataFrame[T]) Insert(name string, col []float64) *DataFrame[T] {
	df.ColNames = append(df.ColNames, name)
	df.Vals = append(df.Vals, col)
	return df
}

// InsertRow adds a new row to the dataframe. Date must be after the last date in the dataframe and vals must equal the number
// of columns. If either of these conditions are not met then panic
func (df *DataFrame[T]) InsertRow(idx T, vals ...float64) *DataFrame[T] {
	// Check that the last date in the dataframe is prior to the new date
	if len(df.Index) != 0 {
		if last, ok := any(df.Index[len(df.Index)-1]).(time.Time); ok {
			newDate := any(idx).(time.Time)
			if !last.Before(newDate) {
				log.Panic().Time("lastDate", last).Time("newDate", newDate).Msg("newDate must be after lastDate")
			}
		}
	}

	// Check that the number of columns equals the number of vals passed
	if len(vals) != len(df.ColNames) {
		log.Panic().Int("NumValsPassed", len(vals)).Int("NumColumns", len(df.ColNames)).Msg("number of vals passed must equal number of columns")
	}

	df.Index = append(df.Index, idx)
	for colIdx := range df.ColNames {
		df.Vals[colIdx] = append(df.Vals[colIdx], vals[colIdx])
	}

	return df
}

// InsertMap adds a new row to the dataframe. Date must be after the last date in the dataframe otherwise panic.
// all columns must already exist in the dataframe, any additional columns in vals is ignored
func (df *DataFrame[T]) InsertMap(idx T, vals map[string]float64) *DataFrame[T] {
	// Check that the last date in the dataframe is prior to the new date
	if len(df.Index) != 0 {
		if last, ok := any(df.Index[len(df.Index)-1]).(time.Time); ok {
			newDate := any(idx).(time.Time) // safe because if last is time.Time then idx must be time.Time
			if !last.Before(newDate) {
				log.Panic().Time("lastDate", last).Time("newDate", newDate).Msg("newDate must be after lastDate")
			}
		}
	}

	df.Index = append(df.Index, idx)
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
func (df *DataFrame[T]) Lag(n int) *DataFrame[T] {
	df = df.Copy()
	prepend := make([]float64, n)
	for idx := range prepend {
		prepend[idx] = math.NaN()
	}

	for idx := range df.Vals {
		l := len(df.Vals[idx])
		df.Vals[idx] = append(prepend, df.Vals[idx]...)[:l] //nolint:makezero
	}
	return df
}

// Last returns a new dataframe with only the last item of the current dataframe
func (df *DataFrame[T]) Last() *DataFrame[T] {
	if df.Len() == 0 {
		return df
	}

	lastVals := make([][]float64, len(df.ColNames))
	lastRow := len(df.Index) - 1
	for idx, col := range df.Vals {
		lastVals[idx] = []float64{col[lastRow]}
	}

	newDf := &DataFrame[T]{
		ColNames: df.ColNames,
		Index:    []T{df.Index[len(df.Index)-1]},
		Vals:     lastVals,
	}

	return newDf
}

// Len returns the number of rows in the dataframe
func (df *DataFrame[T]) Len() int {
	return len(df.Index)
}

// Max selects the max value for each row and returns a new dataframe
func (df *DataFrame[T]) Max() *DataFrame[T] {
	maxDf := &DataFrame[T]{
		ColNames: []string{"max"},
		Index:    df.Index,
		Vals:     [][]float64{make([]float64, len(df.Index))},
	}

	for rowIdx := range df.Index {
		row := make([]float64, 0, len(df.ColNames))
		for colIdx := range df.ColNames {
			row = append(row, df.Vals[colIdx][rowIdx])
		}
		maxDf.Vals[0][rowIdx] = floats.Max(row)
	}

	return maxDf
}

// Min selects the min value for each row and returns a new dataframe
func (df *DataFrame[T]) Min() *DataFrame[T] {
	minDf := &DataFrame[T]{
		ColNames: []string{"min"},
		Index:    df.Index,
		Vals:     [][]float64{make([]float64, len(df.Index))},
	}

	for rowIdx := range df.Index {
		row := make([]float64, 0, len(df.ColNames))
		for colIdx := range df.ColNames {
			row = append(row, df.Vals[colIdx][rowIdx])
		}
		minDf.Vals[0][rowIdx] = floats.Min(row)
	}

	return minDf
}

// Split the dataframe into 2, with columns being in the first dataframe and
// all remaining columns in the second
func (df *DataFrame[T]) Split(columns ...string) (*DataFrame[T], *DataFrame[T]) {
	one := &DataFrame[T]{
		Index:    df.Index,
		ColNames: []string{},
		Vals:     [][]float64{},
	}

	two := &DataFrame[T]{
		Index:    df.Index,
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

// Start returns the first date of the dataframe
func (df *DataFrame[T]) Start() time.Time {
	if len(df.Index) == 0 {
		return time.Time{}
	}

	if firstDate, ok := any(df.Index[0]).(time.Time); ok {
		return firstDate
	}

	return time.Time{}
}

// Table prints an ASCII formatted table to stdout
func (df *DataFrame[T]) Table() string {
	if len(df.Index) == 0 {
		return "<NO DATA>" // nothing to do as there is no data available in the dataframe
	}

	// construct table header
	tableCols := append([]string{"Index"}, df.ColNames...)

	// initialize table
	s := &strings.Builder{}
	table := tablewriter.NewWriter(s)
	table.SetHeader(tableCols)
	footer := make([]string, len(tableCols))
	footer[0] = "Num Rows"
	footer[1] = fmt.Sprintf("%d", df.Len())
	table.SetFooter(footer)
	table.SetBorder(false) // Set Border to false

	for idx, rowIdx := range df.Index {
		row := make([]string, 0, len(df.Vals)+1)

		if date, ok := any(rowIdx).(time.Time); ok {
			row = append(row, date.Format("2006-01-02"))
		} else {
			row = append(row, any(rowIdx).(string))
		}

		for _, col := range df.Vals {
			row = append(row, fmt.Sprintf("%.4f", col[idx]))
		}

		table.Append(row)
	}

	table.Render()
	return s.String()
}

// Trim the dataframe to the specified date range (inclusive)
// NOTE: If T is not time.Time then an empty dataframe is returned
func (df *DataFrame[T]) Trim(begin, end time.Time) *DataFrame[T] {
	df2 := &DataFrame[T]{
		ColNames: df.ColNames,
		Index:    df.Index,
		Vals:     df.Vals,
	}

	var (
		first time.Time
		last  time.Time
		ok    bool
	)

	// special case 0: requested range is invalid
	if end.Before(begin) {
		df2.Index = make([]T, 0)
		df2.Vals = make([][]float64, 0)
		return df2
	}

	// special case 1: data frame is empty
	if df.Len() == 0 {
		return df2
	}

	// ensure that index is a date index
	if first, ok = any(df.Index[0]).(time.Time); !ok {
		return df2
	}

	if last, ok = any(df.Index[len(df.Index)-1]).(time.Time); !ok {
		return df2
	}

	// special case 2: end time is before data frame start
	if end.Before(first) {
		df2.Index = []T{}
		df2.Vals = [][]float64{}
		return df2
	}

	// special case 3: start time is after data frame end
	if begin.After(last) {
		df2.Index = []T{}
		df2.Vals = [][]float64{}
		return df2
	}

	// Use binary search to find the index corresponding to the start and end times
	beginIdx := sort.Search(len(df.Index), func(i int) bool {
		idxVal := any(df.Index[i]).(time.Time)
		return (idxVal.After(begin) || idxVal.Equal(begin))
	})

	endIdx := sort.Search(len(df.Index), func(i int) bool {
		idxVal := any(df.Index[i]).(time.Time)
		return (idxVal.After(end) || idxVal.Equal(end))
	})

	if endIdx != len(df.Index) {
		endIdx++
	}

	df2.Index = df.Index[beginIdx:endIdx]
	for colIdx, col := range df.Vals {
		df2.Vals[colIdx] = col[beginIdx:endIdx]
	}

	return df2
}
