package dfextras

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/rocketlaunchr/dataframe-go"
)

// Collection of helpers make it easier to work on dataframes

// DropNA remove rows in the series or dataframe that have NA's
func DropNA(ctx context.Context, sdf interface{}, opts ...dataframe.FilterOptions) (interface{}, error) {
	switch sdf.(type) {
	case dataframe.Series:
		filterFn := dataframe.FilterSeriesFn(func(val interface{}, row, nRows int) (dataframe.FilterAction, error) {
			if v, ok := val.(float64); ok {
				if math.IsNaN(v) {
					return dataframe.DROP, nil
				}
			}
			return dataframe.KEEP, nil
		})
		res, err := dataframe.Filter(ctx, sdf, filterFn, opts...)
		return res, err
	case *dataframe.DataFrame:
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
		return res, err
	default:
		return nil, errors.New("sdf must be a Series or DataFrame")
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

		if searchVal == val.(time.Time) {
			return *row
		}
	}

	return -1
}

// TimeTrim trim dataframe to rows within the startTime and endTime range
func TimeTrim(ctx context.Context, df *dataframe.DataFrame, timeAxisColumn int, startTime time.Time, endTime time.Time, inPlace bool) (*dataframe.DataFrame, error) {
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

// TimeAlign truncate df to match specified time range
func TimeAlign(ctx context.Context, df *dataframe.DataFrame, timeAxisColumn int, startTime time.Time, endTime time.Time) (*dataframe.DataFrame, error) {
	timeSeries := df.Series[timeAxisColumn]
	startIdx := IndexOf(ctx, startTime, timeSeries, false)
	endIdx := IndexOf(ctx, endTime, timeSeries, true)

	if startIdx == -1 || endIdx == -1 {
		return nil, errors.New("dataframes do not overlap")
	}

	r := dataframe.Range{
		Start: &startIdx,
		End:   &endIdx,
	}

	df.Lock()
	defer df.Unlock()

	return df.Copy(r), nil
}

// MergeAndTimeAlign merge multiple dataframes on their time axis
// Assumptions:
//     1) timeAxisName is in all dataframes and refers to a TimeSeries
//     2) All dataframes are sampled at the same rate; i.e., if values are taken on the first day of the month then that is the way it is done for all data frames
//     3) Time series does not begin or end with nil values
//     4) Time series is sorted ascending
//     5) Time series must overlap
func MergeAndTimeAlign(ctx context.Context, timeAxisName string, dfs ...*dataframe.DataFrame) (*dataframe.DataFrame, error) {
	startTime := time.Time{}
	unixToInternal := int64((1969*365 + 1969/4 - 1969/100 + 1969/400) * 24 * 60 * 60)
	endTime := time.Unix(1<<63-1-unixToInternal, 999999999)

	dontLock := dataframe.Options{DontLock: true}

	// find latest start and earliest end in all dataframes
	timeAxisMap := make(map[int]int)
	for ii := range dfs {
		jj, err := dfs[ii].NameToColumn(timeAxisName, dontLock)
		if err != nil {
			return nil, errors.New("All dataframes must contain the time axis")
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
		newDf, err := TimeAlign(ctx, dfs[ii], timeAxisMap[ii], startTime, endTime)
		if err != nil {
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

// AggregateSeriesFn function
type AggregateSeriesFn func(vals []interface{}, firstRow int, finalRow int) (float64, error)

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
		var maxV float64

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

// Lag return a copy of the dataframe offset by
func Lag(n int, df *dataframe.DataFrame) *dataframe.DataFrame {
	series := []dataframe.Series{}

	df.Lock()
	defer df.Unlock()

	dontLock := dataframe.Options{DontLock: true}

	for ii := range df.Series {
		s := df.Series[ii].Copy()
		for x := 0; x < n; x++ {
			s.Prepend(nil)
			s.Remove(s.NRows(dontLock)-1, dontLock)
		}
		series = append(series, s)
	}

	return dataframe.NewDataFrame(series...)
}
