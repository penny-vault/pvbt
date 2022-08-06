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

package indicators

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jdfergason/dataframe-go"
	"github.com/jdfergason/dataframe-go/math/funcs"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/dfextras"
	"github.com/rs/zerolog/log"
)

// Momentum calculates the average momentum for each asset listed in `Assets` for each
// lookback period in `Periods` and calculates an indicator based on the max momentum of all assets
type Momentum struct {
	Assets  []string
	Periods []int
	Manager *data.Manager
}

func (m *Momentum) buildDataFrame(ctx context.Context, dateSeriesIdx int, prices *dataframe.DataFrame, rfr dataframe.Series) (*dataframe.DataFrame, error) {
	nrows := prices.NRows(dataframe.Options{})
	series := []dataframe.Series{}
	series = append(series, prices.Series[dateSeriesIdx].Copy())

	aggFn := dfextras.AggregateSeriesFn(func(vals []interface{}, firstRow int, finalRow int) (float64, error) {
		var sum float64
		for _, val := range vals {
			if v, ok := val.(float64); ok {
				sum += v
			}
		}

		return sum, nil
	})

	for ii := range prices.Series {
		name := prices.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, common.DateIdx) != 0 {
			score := dataframe.NewSeriesFloat64(fmt.Sprintf("%sSCORE", name), &dataframe.SeriesInit{Size: nrows})
			series = append(series, prices.Series[ii].Copy(), score)
		}
	}

	for _, ii := range m.Periods {
		// compute a lag for each series
		lag := dfextras.Lag(ii, prices)
		roll, err := dfextras.Rolling(ctx, ii, rfr.Copy(), aggFn)
		if err != nil {
			log.Error().Err(err).Msg("error computing rolling sum of risk free rate")
			return nil, err
		}

		roll.Rename(fmt.Sprintf("RISKFREE%d", ii))
		series = append(series, roll)
		for _, ticker := range m.Assets {
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

	series = append(series, dataframe.NewSeriesFloat64(SeriesName, &dataframe.SeriesInit{Size: nrows}))
	momentum := dataframe.NewDataFrame(series...)
	return momentum, nil
}

func (m *Momentum) IndicatorForPeriod(ctx context.Context, start time.Time, end time.Time) (*dataframe.DataFrame, error) {
	origStart := m.Manager.Begin
	origEnd := m.Manager.End
	origFrequency := m.Manager.Frequency

	defer func() {
		m.Manager.Begin = origStart
		m.Manager.End = origEnd
		m.Manager.Frequency = origFrequency
	}()

	m.Manager.Begin = start
	m.Manager.End = end
	m.Manager.Frequency = data.FrequencyMonthly

	// Add the risk free asset
	assets := make([]string, 0, len(m.Assets)+1)
	assets = append(assets, "DGS3MO")
	assets = append(assets, m.Assets...)

	// fetch prices
	prices, err := m.Manager.GetDataFrame(ctx, data.MetricAdjustedClose, assets...)
	if err != nil {
		log.Error().Err(err).Msg("could not load data for momentum indicator")
		return nil, err
	}

	// create a new series with date column
	dateSeriesIdx, err := prices.NameToColumn(common.DateIdx)
	if err != nil {
		log.Error().Err(err).Msg("could not get date index")
		return nil, err
	}
	dgsIdx, err := prices.NameToColumn("DGS3MO")
	if err != nil {
		log.Error().Err(err).Msg("could not get DGS3MO index")
		return nil, err
	}

	rfr := prices.Series[dgsIdx]

	if err := prices.RemoveSeries("DGS3MO"); err != nil {
		log.Error().Err(err).Msg("could not remove DGS3MO series")
		return nil, err
	}

	// build copy of dataframe
	momentum, err := m.buildDataFrame(ctx, dateSeriesIdx, prices, rfr)
	if err != nil {
		// already logged
		return nil, err
	}

	// run calculations
	for _, ticker := range m.Assets {
		for _, jj := range m.Periods {
			fn := funcs.RegFunc(fmt.Sprintf("(((%s/%sLAG%d)-1)*100)-(RISKFREE%d/12)", ticker, ticker, jj, jj))
			if err := funcs.Evaluate(ctx, momentum, fn, fmt.Sprintf("%sMOM%d", ticker, jj)); err != nil {
				log.Error().Stack().Err(err).Msg("could not calculate momentum")
				return nil, err
			}
		}
	}

	cols := make([]string, 0, len(m.Assets))
	for _, ticker := range m.Assets {
		scoreName := fmt.Sprintf("%sSCORE", ticker)
		cols = append(cols, scoreName)
		fn := funcs.RegFunc(fmt.Sprintf("(%sMOM1+%sMOM3+%sMOM6)/3", ticker, ticker, ticker))
		if err := funcs.Evaluate(ctx, momentum, fn, scoreName); err != nil {
			log.Error().Stack().Err(err).Msg("could not calculate score")
			return nil, err
		}
	}

	// compute indicator
	fn := funcs.RegFunc(fmt.Sprintf("max(%s)", strings.Join(cols, ",")))
	if err := funcs.Evaluate(ctx, momentum, fn, SeriesName); err != nil {
		log.Error().Stack().Err(err).Msg("could not calculate indicator column")
		return nil, err
	}

	var seriesIdx int
	if seriesIdx, err = momentum.NameToColumn(SeriesName); err != nil {
		log.Error().Stack().Err(err).Msg("indicator column does not exist")
		return nil, err

	}
	indicatorSeries := momentum.Series[seriesIdx]
	dateSeries := prices.Series[dateSeriesIdx]
	indicatorDF := dataframe.NewDataFrame(dateSeries, indicatorSeries)

	if indicatorDF, err = dfextras.DropNA(ctx, indicatorDF); err != nil {
		log.Error().Err(err).Msg("could not drop indicator NA rows")
		return nil, err
	}

	return indicatorDF, nil
}
