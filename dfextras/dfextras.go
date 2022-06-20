// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dfextras

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"

	"github.com/rocketlaunchr/dataframe-go"
)

const MaxUint64 = ^uint64(0)
const MaxInt64 = int64(MaxUint64 >> 1)
const MinInt64 = -MaxInt64 - 1

func abs(n time.Duration) time.Duration {
	y := n >> 63
	return (n ^ y) - y
}

// Collection of helpers make it easier to work on dataframes

// AggregateSeriesFn function
type AggregateSeriesFn func(vals []interface{}, firstRow int, finalRow int) (float64, error)

// ArgMax select float64 series with largest value for each row
func ArgMax(ctx context.Context, df *dataframe.DataFrame) (dataframe.Series, error) {
	// only apply to float64 Series
	keepSeries := []dataframe.Series{}
	for ii := range df.Series {
		if df.Series[ii].Type() == "float64" {
			keepSeries = append(keepSeries, df.Series[ii])
		}
	}

	if len(keepSeries) < 2 {
		return nil, errors.New("DataFrame must contain at-least 2 float64 series")
	}

	df1 := dataframe.NewDataFrame(keepSeries...)
	series := dataframe.NewSeriesString("argmax", nil)

	df1.Lock()
	defer df1.Unlock()

	iterator := df1.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: true})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		row, val, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		maxK := ""
		maxV := math.MaxFloat64 * -1

		for k, v := range val {
			vf := v.(float64)
			if vf > maxV {
				maxK = k.(string)
				maxV = vf
			}
		}

		series.Append(maxK)
	}

	return series, nil
}

// DropNA remove rows in the dataframe that have NA's
func DropNA(ctx context.Context, sdf *dataframe.DataFrame, opts ...dataframe.FilterOptions) (*dataframe.DataFrame, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "dfextras.DropNA")
	defer span.End()

	filterFn := dataframe.FilterDataFrameFn(func(vals map[interface{}]interface{}, row, nRows int) (dataframe.FilterAction, error) {
		for _, val := range vals {
			if val == nil {
				return dataframe.DROP, nil
			}
			if v, ok := val.(float64); ok {
				if math.IsNaN(v) {
					return dataframe.DROP, nil
				}
			}
		}
		return dataframe.KEEP, nil
	})
	res, err := dataframe.Filter(ctx, sdf, filterFn, opts...)
	if res == nil {
		return nil, err
	}
	return res.(*dataframe.DataFrame), err
}

// Find row with value
func FindTime(df *dataframe.DataFrame, searchVal time.Time, col string) map[interface{}]interface{} {
	iterator := df.ValuesIterator()
	for {
		row, val, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		if val[col].(time.Time).Equal(searchVal) {
			return val
		}
	}
	return nil
}

// FindNearestTime locates the row with the time closest to timeVal, assuming that the
// input dataframe is sorted in ascending order. Returns nil if there is not a value
// within at least maxDistance
func FindNearestTime(df *dataframe.DataFrame, timeVal time.Time, maxDistance time.Duration, startHint ...int) (map[interface{}]interface{}, int) {
	start := 0
	if len(startHint) > 0 {
		start = startHint[0]
	}
	iterator := df.ValuesIterator(dataframe.ValuesOptions{
		InitialRow: start,
		Step:       1,
	})
	lastDistance := time.Duration(MaxInt64)
	var lastRow map[interface{}]interface{} = nil
	var hint = start
	for {
		row, val, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		rowTime := val[common.DateIdx].(time.Time)
		distance := abs(rowTime.Sub(timeVal))

		if lastDistance < distance {
			if lastDistance <= maxDistance {
				return lastRow, hint
			} else {
				return nil, 0
			}
		}

		lastRow = val
		hint = *row
		lastDistance = distance
	}

	if lastDistance <= maxDistance {
		return lastRow, hint
	} else {
		return nil, hint
	}
}

// IndexOf value v in series
func IndexOf(ctx context.Context, searchVal time.Time, series dataframe.Series, reverse bool) int {
	var opts dataframe.ValuesOptions
	if reverse {
		opts = dataframe.ValuesOptions{
			InitialRow:   -1,
			Step:         -1,
			DontReadLock: false,
		}
	} else {
		opts = dataframe.ValuesOptions{
			InitialRow:   0,
			Step:         1,
			DontReadLock: false,
		}
	}

	iterator := series.ValuesIterator(opts)
	for {
		if err := ctx.Err(); err != nil {
			return -1
		}

		row, val, _ := iterator()
		if row == nil {
			break
		}

		if searchVal.Equal(val.(time.Time)) {
			return *row
		}
	}

	return -1
}

// Lag return a copy of the dataframe offset by n
func Lag(n int, df *dataframe.DataFrame, cols ...string) *dataframe.DataFrame {
	seriesArr := []dataframe.Series{}

	df.Lock()
	defer df.Unlock()

	dontLock := dataframe.Options{DontLock: true}

	// convert cols to a map
	sz := len(cols)
	if sz == 0 {
		sz = len(df.Series)
	}

	colMap := make(map[string]struct{}, sz)
	if len(cols) != 0 {
		for _, col := range cols {
			colMap[col] = struct{}{}
		}
	} else {
		for _, series := range df.Series {
			colMap[series.Name()] = struct{}{}
		}
	}

	for _, series := range df.Series {
		if _, ok := colMap[series.Name()]; ok {
			s := series.Copy()
			for x := 0; x < n; x++ {
				s.Prepend(nil)
				s.Remove(s.NRows(dontLock)-1, dontLock)
			}
			seriesArr = append(seriesArr, s)
		} else {
			s := series.Copy()
			seriesArr = append(seriesArr, s)
		}
	}

	return dataframe.NewDataFrame(seriesArr...)
}

// Merge multiple dataframes according to the time axis; rows that don't have corresponding values in
// all dataframes are not included.
func Merge(ctx context.Context, dfs ...*dataframe.DataFrame) (*dataframe.DataFrame, error) {
	timeAxisName := common.DateIdx
	unixToInternal := int64((1969*365 + 1969/4 - 1969/100 + 1969/400) * 24 * 60 * 60)
	startTime := time.Unix(1<<63-1-unixToInternal, 999999999)
	endTime := time.Time{}

	dontLock := dataframe.Options{DontLock: true}
	var startTimeAxis dataframe.Series
	var endTimeAxis dataframe.Series

	// find earliest start and latest end in all dataframes
	timeAxisMap := make(map[int]int)
	for ii := range dfs {
		jj, err := dfs[ii].NameToColumn(timeAxisName, dontLock)
		if err != nil {
			return nil, errors.New("all dataframes must contain the time axis")
		}

		timeSeries := dfs[ii].Series[jj]
		timeAxisMap[ii] = jj
		// Check if this is a later startTime
		val := timeSeries.Value(0, dontLock)
		if v, ok := val.(time.Time); ok {
			if v.Before(startTime) {
				startTime = v
				startTimeAxis = timeSeries
			}
		} else {
			return nil, errors.New("timeAxis must refer to a time column")
		}

		// Check if this is an earlier endTime
		val = timeSeries.Value(timeSeries.NRows(dontLock)-1, dontLock)
		if v, ok := val.(time.Time); ok {
			if v.After(endTime) {
				endTime = v
				endTimeAxis = timeSeries
			}
		} else {
			return nil, errors.New("timeAxis must refer to a time column")
		}
	}

	// create time axis
	newTimeAxis := startTimeAxis.Copy()
	lastTimeInStart := startTimeAxis.Value(startTimeAxis.NRows() - 1).(time.Time)
	iterator := endTimeAxis.ValuesIterator()
	for {
		row, val, _ := iterator()
		if row == nil {
			break
		}

		if t, ok := val.(time.Time); ok {
			if lastTimeInStart.Before(t) {
				newTimeAxis.Append(t)
			}
		}
	}

	// build series, using math.NaN to fill non-value areas
	series := []dataframe.Series{newTimeAxis}
	for ii := range dfs {
		timeAxisColumn, _ := dfs[ii].NameToColumn(timeAxisName)
		timeSeries := dfs[ii].Series[timeAxisColumn]
		// calculate num to add to beginning and end of df
		iterator := newTimeAxis.ValuesIterator()
		nStartAdd := 0
		nEndAdd := 0

		dfStart := timeSeries.Value(0).(time.Time)
		dfEnd := timeSeries.Value(timeSeries.NRows() - 1).(time.Time)

		for {
			row, val, _ := iterator()
			if row == nil {
				break
			}
			t := val.(time.Time)
			if t.Before(dfStart) {
				nStartAdd++
			}
			if t.After(dfEnd) {
				nEndAdd++
			}
		}

		newDf := dfs[ii].Copy()
		blankRow := make([]interface{}, len(newDf.Series))

		for ii := 0; ii < nStartAdd; ii++ {
			newDf.Insert(0, &dataframe.Options{}, blankRow...)
		}

		for ii := 0; ii < nEndAdd; ii++ {
			newDf.Append(&dataframe.Options{}, blankRow...)
		}

		err := newDf.RemoveSeries(timeAxisName)
		if err != nil {
			return nil, err
		}

		series = append(series, newDf.Series...)
	}

	finalDf := dataframe.NewDataFrame(series...)
	return finalDf, nil
}

// MergeAndFill merge multiple dataframes on their time axis. Where there are missing values fill with specified fill value
// Assumptions:
//     1) All dataframes must be sampled at the same rate
//     2) All dataframes must have a time column
func MergeAndFill(ctx context.Context, dfs ...*dataframe.DataFrame) (*dataframe.DataFrame, error) {
	timeAxisName := common.DateIdx
	// build time axis
	timeMap := make(map[int64]time.Time)
	for _, df := range dfs {
		iterator := df.ValuesIterator(dataframe.ValuesOptions{
			InitialRow:   0,
			Step:         1,
			DontReadLock: true})
		for {
			row, vals, _ := iterator(dataframe.SeriesName)
			if row == nil {
				break
			}
			t := vals[timeAxisName].(time.Time)
			timeMap[t.Unix()] = t
		}
	}

	timeArr := make([]time.Time, 0, len(timeMap))
	for _, v := range timeMap {
		timeArr = append(timeArr, v)
	}
	sort.Slice(timeArr, func(i, j int) bool {
		return timeArr[i].Before(timeArr[j])
	})

	// build series
	timeAxis := dataframe.NewSeriesTime(timeAxisName, &dataframe.SeriesInit{Size: len(timeArr)}, timeArr)
	series := []dataframe.Series{timeAxis}
	for _, df := range dfs {
		iterator := df.ValuesIterator(dataframe.ValuesOptions{
			InitialRow:   0,
			Step:         1,
			DontReadLock: true})
		mainIdx := 0
		lastVal := make(map[string]float64, len(df.Names())-1)
		seriesVals := make(map[string][]float64, len(df.Names())-1)
		for _, v := range df.Names() {
			if v != timeAxisName {
				seriesVals[v] = make([]float64, len(timeArr))
				lastVal[v] = math.NaN()
			}
		}
		for {
			row, vals, _ := iterator(dataframe.SeriesName)
			if row == nil {
				break
			}

			t := vals[timeAxisName].(time.Time)
			mainT := timeArr[mainIdx]

			for _, v := range df.Names() {
				if v == timeAxisName {
					continue
				}
				if mainT.Equal(t) {
					seriesVals[v][mainIdx] = vals[v].(float64)
					lastVal[v] = vals[v].(float64)
					mainIdx++
				} else {
					for ; mainIdx < len(timeArr); mainIdx++ {
						mainT = timeArr[mainIdx]
						if mainT.Equal(t) {
							seriesVals[v][mainIdx] = vals[v].(float64)
							lastVal[v] = vals[v].(float64)
							break
						} else {
							seriesVals[v][mainIdx] = lastVal[v]
						}
					}
				}
			}
		}

		for _, v := range df.Names() {
			if v == timeAxisName {
				continue
			}
			for ; mainIdx < len(timeArr); mainIdx++ {
				seriesVals[v][mainIdx] = lastVal[v]
			}
			newSeries := dataframe.NewSeriesFloat64(v, &dataframe.SeriesInit{Size: len(timeArr)}, seriesVals[v])
			series = append(series, newSeries)

		}
	}

	return dataframe.NewDataFrame(series...), nil
}

// MergeAndTimeAlign merge multiple dataframes on their time axis
// Assumptions:
//     1) timeAxisName is in all dataframes and refers to a TimeSeries
//     2) All dataframes are sampled at the same rate; i.e., if values are taken on the first day of the month then that is the way it is done for all data frames
//     3) Time series does not begin or end with nil values
//     4) Time series is sorted ascending
//     5) Time series must overlap
func MergeAndTimeAlign(ctx context.Context, dfs ...*dataframe.DataFrame) (*dataframe.DataFrame, error) {
	timeAxisName := common.DateIdx
	startTime := time.Time{}
	unixToInternal := int64((1969*365 + 1969/4 - 1969/100 + 1969/400) * 24 * 60 * 60)
	endTime := time.Unix(1<<63-1-unixToInternal, 999999999)

	dontLock := dataframe.Options{DontLock: true}

	// find latest start and earliest end in all dataframes
	timeAxisMap := make(map[int]int)
	for ii := range dfs {
		jj, err := dfs[ii].NameToColumn(timeAxisName, dontLock)
		if err != nil {
			return nil, errors.New("all dataframes must contain the time axis")
		}

		timeSeries := dfs[ii].Series[jj]
		timeAxisMap[ii] = jj
		// Check if this is a later startTime
		val := timeSeries.Value(0, dontLock)
		if v, ok := val.(time.Time); ok {
			if v.After(startTime) {
				startTime = v
			}
		} else {
			return nil, errors.New("timeAxis must refer to a time column")
		}

		// Check if this is an earlier endTime
		val = timeSeries.Value(timeSeries.NRows(dontLock)-1, dontLock)
		if v, ok := val.(time.Time); ok {
			if v.Before(endTime) {
				endTime = v
			}
		} else {
			return nil, errors.New("timeAxis must refer to a time column")
		}
	}

	// Align series
	series := []dataframe.Series{}
	var alignedTimeSeries dataframe.Series
	for ii := range dfs {
		newDf, err := TimeAlign(ctx, dfs[ii], startTime, endTime)
		if err != nil {
			log.Error().Err(err).Msg("time align failed")
			return nil, err
		}

		alignedTimeSeries = newDf.Series[timeAxisMap[ii]]
		err = newDf.RemoveSeries(timeAxisName)
		if err != nil {
			return nil, err
		}

		series = append(series, newDf.Series...)
	}
	series = append(series, alignedTimeSeries)
	finalDf := dataframe.NewDataFrame(series...)
	return finalDf, nil
}

// Rolling aggregate function
func Rolling(ctx context.Context, n int, s dataframe.Series, fn AggregateSeriesFn) (dataframe.Series, error) {
	if fn == nil {
		return nil, errors.New("fn is required")
	}

	s.Lock()
	defer s.Unlock()

	dontLock := dataframe.Options{DontLock: true}

	ns := dataframe.NewSeriesFloat64(s.Name(dontLock), &dataframe.SeriesInit{Capacity: s.NRows(dontLock)})

	iterator := s.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: true})

	var groupedVals []interface{}
	nVals := 0
	firstRow := 0

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		row, val, _ := iterator()
		if row == nil {
			break
		}

		groupedVals = append(groupedVals, val)
		nVals++

		if nVals >= n {
			v, err := fn(groupedVals, firstRow, *row)
			if err != nil {
				return nil, err
			}
			ns.Append(v)
			groupedVals = groupedVals[1:]
			firstRow++
		} else {
			ns.Append(math.NaN())
		}
	}

	return ns, nil
}

// Split divides the dataframe into two, columns listed go into the first data frame, any remaining
// columns do into the second data frame. This modifies the original dataframe in place to hold the
// remaining columns
func Split(ctx context.Context, df *dataframe.DataFrame, cols ...string) (*dataframe.DataFrame, *dataframe.DataFrame, error) {
	timeSeries := df.Series[df.MustNameToColumn(common.DateIdx)]
	df1 := make([]dataframe.Series, 0, len(cols)+1)
	df1 = append(df1, timeSeries.Copy())

	for _, colName := range cols {
		colIdx, err := df.NameToColumn(colName)
		if err != nil {
			return nil, nil, err
		}
		df1 = append(df1, df.Series[colIdx])
		df.RemoveSeries(colName)
	}

	return dataframe.NewDataFrame(df1...), df, nil
}

// TimeAlign truncate df to match specified time range
func TimeAlign(ctx context.Context, df *dataframe.DataFrame, startTime time.Time, endTime time.Time) (*dataframe.DataFrame, error) {
	timeAxisColumn, err := df.NameToColumn(common.DateIdx)
	if err != nil {
		return nil, err
	}
	timeSeries := df.Series[timeAxisColumn]
	startIdx := IndexOf(ctx, startTime, timeSeries, false)
	endIdx := IndexOf(ctx, endTime, timeSeries, true)

	if startIdx == -1 || endIdx == -1 {
		return nil, fmt.Errorf("dataframes do not overlap. startIdx=%d  endIdx=%d", startIdx, endIdx)
	}

	r := dataframe.Range{
		Start: &startIdx,
		End:   &endIdx,
	}

	df.Lock()
	defer df.Unlock()

	return df.Copy(r), nil
}

// TimeTrim trim dataframe to rows within the startTime and endTime range
func TimeTrim(ctx context.Context, df *dataframe.DataFrame, startTime time.Time, endTime time.Time, inPlace bool) (*dataframe.DataFrame, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "dfextras.TimeTrim")
	defer span.End()

	filterFn := dataframe.FilterDataFrameFn(func(vals map[interface{}]interface{}, row, nRows int) (dataframe.FilterAction, error) {
		for _, val := range vals {
			if v, ok := val.(time.Time); ok {
				if (startTime.Before(v) || startTime.Equal(v)) && (endTime.After(v) || endTime.Equal(v)) {
					return dataframe.KEEP, nil
				}
			}
		}
		return dataframe.DROP, nil
	})
	opts := dataframe.FilterOptions{
		InPlace: inPlace,
	}
	res, err := dataframe.Filter(ctx, df, filterFn, opts)
	if res != nil {
		df2 := res.(*dataframe.DataFrame)
		return df2, err
	}

	return nil, err
}
