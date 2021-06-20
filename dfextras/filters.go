// Package dfextras provides convenience functions for operating on dataframes.
package dfextras

import (
	"context"
	"errors"
	"fmt"
	"main/data"
	"strings"

	"github.com/rocketlaunchr/dataframe-go"
	"github.com/rocketlaunchr/dataframe-go/math/funcs"
	"gonum.org/v1/gonum/stat"
)

// SMA computes the simple moving average of all the columns in eod for the specified lookback period.
// NOTE: lookback is in terms of months
func SMA(lookback int, df *dataframe.DataFrame) (*dataframe.DataFrame, error) {

	if (lookback > df.NRows()) ||
		(lookback <= 0) {
		return nil, errors.New("lookback must be: 0 < lookback <= df.NRows()")
	}

	seriesMap := make(map[string]*dataframe.SeriesFloat64)
	filterMap := make(map[string][]float64)

	dateSeries := dataframe.NewSeriesTime(data.DateIdx, nil)

	// Get list of columns and allocate 1 filter array each
	for ii := range df.Series {
		name := df.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, data.DateIdx) != 0 {
			filterMap[name] = make([]float64, lookback)
			seriesMap[name] = dataframe.NewSeriesFloat64(name, nil)
		}
	}

	warmup := true
	iterator := df.ValuesIterator(dataframe.ValuesOptions{
		InitialRow:   0,
		Step:         1,
		DontReadLock: true,
	})

	df.Lock()
	for {
		row, vals, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		// if we have seen at least lookback rows then we are out of the warmup period
		// NOTE: row is 0 based, lookback is 1 based; hence the test applied below
		if *row == (lookback - 1) {
			warmup = false
		}

		idx := *row % lookback

		for k, v := range vals {
			if strings.Compare(k.(string), data.DateIdx) != 0 {
				filterMap[k.(string)][idx] = v.(float64)
				if !warmup {
					// out of warmup period; save average to a new row
					seriesMap[k.(string)].Append(stat.Mean(filterMap[k.(string)], nil))
				}
			} else {
				if !warmup {
					dateSeries.Append(v)
				}
			}
		}

	}
	df.Unlock()

	series := []dataframe.Series{}
	series = append(series, dateSeries)
	for _, v := range seriesMap {
		series = append(series, v)
	}

	return dataframe.NewDataFrame(series...), nil
}

// Momentum13612 computes the average momentum score over 1-, 3-, 6-, and 12-month periods
func Momentum13612(eod *dataframe.DataFrame) (*dataframe.DataFrame, error) {
	nrows := eod.NRows(dataframe.Options{})
	periods := []int{1, 3, 6, 12}
	series := []dataframe.Series{}
	tickers := []string{}

	dateSeriesIdx, err := eod.NameToColumn(data.DateIdx)
	if err != nil {
		return nil, err
	}

	series = append(series, eod.Series[dateSeriesIdx].Copy())

	// Get list of tickers and pre-allocate result series
	for ii := range eod.Series {
		name := eod.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, data.DateIdx) != 0 {
			tickers = append(tickers, name)
			score := dataframe.NewSeriesFloat64(fmt.Sprintf("%sSCORE", name), &dataframe.SeriesInit{Size: nrows})
			series = append(series, eod.Series[ii].Copy(), score)
		}
	}

	// Compute lag series and pre-allocate momentum series for all periods
	for _, ii := range periods {
		lag := Lag(ii, eod)
		for _, ticker := range tickers {
			jj, err := lag.NameToColumn(ticker)
			if err != nil {
				return nil, err
			}
			s := lag.Series[jj]
			name := fmt.Sprintf("%sLAG%d", ticker, ii)
			s.Rename(name)

			mom := dataframe.NewSeriesFloat64(fmt.Sprintf("%sMOM%d", ticker, ii), &dataframe.SeriesInit{Size: nrows})
			series = append(series, s, mom)
		}
	}

	mom := dataframe.NewDataFrame(series...)

	// Calculate momentums for all periods
	for _, ticker := range tickers {
		for _, jj := range periods {
			fn := funcs.RegFunc(fmt.Sprintf("((%s/%sLAG%d)-1)", ticker, ticker, jj))
			funcs.Evaluate(context.TODO(), mom, fn, fmt.Sprintf("%sMOM%d", ticker, jj))
		}
	}

	// Compute the equal weighted average of the 1-, 3-, 6-, and 12-month momentums
	for _, ticker := range tickers {
		fn := funcs.RegFunc(fmt.Sprintf("((12.0*%sMOM1)+(4.0*%sMOM3)+(2.0*%sMOM6)+%sMOM12)*0.25", ticker, ticker, ticker, ticker))
		funcs.Evaluate(context.TODO(), mom, fn, fmt.Sprintf("%sSCORE", ticker))
	}

	// Build dataseries just from scores
	scoresArr := make([]dataframe.Series, 0, 16)
	scoresArr = append(scoresArr, eod.Series[dateSeriesIdx].Copy())
	for _, scoreSeries := range mom.Series {
		name := scoreSeries.Name()
		if strings.HasSuffix(name, "SCORE") {
			symbol := strings.TrimSuffix(name, "SCORE")
			scoreSeries = scoreSeries.Copy()
			scoreSeries.Rename(symbol)
			scoresArr = append(scoresArr, scoreSeries)
		}
	}

	df := dataframe.NewDataFrame(scoresArr...)
	DropNA(context.TODO(), df, dataframe.FilterOptions{InPlace: true})

	return df, nil
}
