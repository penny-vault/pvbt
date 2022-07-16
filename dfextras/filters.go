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
	"strings"

	"github.com/penny-vault/pv-api/common"
	"github.com/rs/zerolog/log"

	"github.com/jdfergason/dataframe-go"
	"github.com/jdfergason/dataframe-go/math/funcs"
	"gonum.org/v1/gonum/stat"
)

var (
	ErrInvalidLookback = errors.New("invalid lookback")
)

// SMA computes the simple moving average of all the columns in df for the specified
// lookback period. For each column in the input dataframe a new column is added with
// the suffix SMA
// NOTE: lookback is in terms of months
func SMA(lookback int, df *dataframe.DataFrame, colSuffix ...string) (*dataframe.DataFrame, error) {

	suffix := "_SMA"
	if len(colSuffix) > 0 {
		suffix = colSuffix[0]
	}

	if (lookback > df.NRows()) || (lookback <= 0) {
		log.Error().Stack().Int("Lookback", lookback).Int("NRows", df.NRows()).Msg("lookback must be: 0 < lookback <= df.NRows()")
		return nil, ErrInvalidLookback
	}

	seriesMap := make(map[string]*dataframe.SeriesFloat64)
	filterMap := make(map[string][]float64)

	dateSeries := dataframe.NewSeriesTime(common.DateIdx, nil)

	// Get list of columns and allocate 1 filter array each
	for ii := range df.Series {
		name := df.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, common.DateIdx) != 0 {
			filterMap[name] = make([]float64, lookback)
			smaName := fmt.Sprintf("%s%s", name, suffix)
			seriesMap[smaName] = dataframe.NewSeriesFloat64(smaName, nil)
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
			if strings.Compare(k.(string), common.DateIdx) != 0 {
				filterMap[k.(string)][idx] = v.(float64)
				if !warmup {
					// out of warmup period; save average to a new row
					name := k.(string)
					smaName := fmt.Sprintf("%s%s", name, suffix)
					seriesMap[smaName].Append(stat.Mean(filterMap[name], nil))
					seriesMap[name].Append(v.(float64))
				}
			} else if !warmup {
				dateSeries.Append(v)
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

	dateSeriesIdx, err := eod.NameToColumn(common.DateIdx)
	if err != nil {
		return nil, err
	}

	series = append(series, eod.Series[dateSeriesIdx].Copy())

	// Get list of tickers and pre-allocate result series
	for ii := range eod.Series {
		name := eod.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, common.DateIdx) != 0 {
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
			if err := funcs.Evaluate(context.TODO(), mom, fn, fmt.Sprintf("%sMOM%d", ticker, jj)); err != nil {
				log.Error().Stack().Err(err).Msg("could not evaluate equation against dataframe")
			}
		}
	}

	// Compute the equal weighted average of the 1-, 3-, 6-, and 12-month momentums
	for _, ticker := range tickers {
		fn := funcs.RegFunc(fmt.Sprintf("((12.0*%sMOM1)+(4.0*%sMOM3)+(2.0*%sMOM6)+%sMOM12)*0.25", ticker, ticker, ticker, ticker))
		if err := funcs.Evaluate(context.TODO(), mom, fn, fmt.Sprintf("%sSCORE", ticker)); err != nil {
			log.Error().Stack().Err(err).Msg("could not evalute expression")
		}
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
	if _, err := DropNA(context.TODO(), df, dataframe.FilterOptions{InPlace: true}); err != nil {
		log.Error().Stack().Err(err).Msg("could not drop na")
	}

	return df, nil
}
