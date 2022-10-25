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

package indicators

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/rs/zerolog/log"
)

// Momentum calculates the average momentum for each security listed in `Securities` for each
// lookback period in `Periods` and calculates an indicator based on the max momentum of all securities
type Momentum struct {
	Securities []*data.Security
	Periods    []int
}

func (m *Momentum) buildDataFrame(ctx context.Context, dateSeriesIdx int, prices *dataframe.DataFrame, rfr dataframe.Series) (*dataframe.DataFrame, error) {
	nrows := prices.NRows(dataframe.Options{})
	series := []dataframe.Series{}
	series = append(series, prices.Series[dateSeriesIdx].Copy())

	// aggFn computes the sum of a series
	aggFn := dataframe.AggregateSeriesFn(func(vals []interface{}, firstRow int, finalRow int) (float64, error) {
		var sum float64
		for _, val := range vals {
			if v, ok := val.(float64); ok {
				sum += v
			}
		}

		return sum, nil
	})

	// for each column in the prices dataframe create a new column with the postfix of SCORE
	for ii := range prices.Series {
		name := prices.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, common.DateIdx) != 0 {
			score := dataframe.NewSeriesFloat64(fmt.Sprintf("%sSCORE", name), &dataframe.SeriesInit{Size: nrows})
			series = append(series, prices.Series[ii].Copy(), score)
		}
	}

	// for each period
	//   1) create a new dataframe that is lagged by the period (lagging prepends N nil values and removes the last N values)
	//   2) compute the rolling sum of the risk free rate (where the period length is used as the number of values in the sum)
	//   3) rename the roll series to RISKFREE<period length>
	//
	//   for each securirty in the matrix
	//     4) rename the lagged series to <security composite figi>LAG<period length>
	//     5) create a new series named <security composite figi>MOM<period length>
	// 6) add a new series named indicator to the dataframe (initialized with zero-values)
	for _, ii := range m.Periods {
		// compute a lag for each series
		lag := dataframe.Lag(ii, prices)
		roll, err := dataframe.Rolling(ctx, ii, rfr.Copy(), aggFn)
		if err != nil {
			log.Error().Err(err).Msg("error computing rolling sum of risk free rate")
			return nil, err
		}

		roll.Rename(fmt.Sprintf("RISKFREE%d", ii))
		series = append(series, roll)
		for _, security := range m.Securities {
			jj, err := lag.NameToColumn(security.CompositeFigi)
			if err != nil {
				return nil, err
			}
			s := lag.Series[jj]
			name := fmt.Sprintf("%sLAG%d", security.CompositeFigi, ii)
			s.Rename(name)

			mom := dataframe.NewSeriesFloat64(fmt.Sprintf("%sMOM%d", security.CompositeFigi, ii), &dataframe.SeriesInit{Size: nrows})
			series = append(series, s, mom)
		}
	}

	series = append(series, dataframe.NewSeriesFloat64(SeriesName, &dataframe.SeriesInit{Size: nrows}))
	momentum := dataframe.NewDataFrame(series...)
	return momentum, nil
}

func (m *Momentum) IndicatorForPeriod(ctx context.Context, start time.Time, end time.Time) (*dataframe.DataFrame, error) {
	begin := start.AddDate(0, -6, 0)

	// Add the risk free asset
	securities := make([]*data.Security, 0, len(m.Securities)+1)
	securities = append(securities, &data.Security{
		CompositeFigi: "PVGG06TNP6J8",
		Ticker:        "DGS3MO",
	})
	securities = append(securities, m.Securities...)

	// fetch prices
	priceMap, err := data.NewDataRequest(securities...).Metrics(data.MetricAdjustedClose).Between(ctx, begin, end)
	if err != nil {
		log.Error().Err(err).Msg("could not load data for momentum indicator")
		return nil, err
	}

	prices := priceMap.DataFrame().Frequency(dataframe.Monthly)

	// create a new series with date column
	dgs3mo, err := data.SecurityFromFigi("PVGG06TNP6J8")
	if err != nil {
		log.Error().Err(err).Msg("could not get DGS3MO security")
		return nil, err
	}

	riskFreeRate, prices = prices.Split(data.SecurityMetric{
		SecurityObject: *dgs3mo,
		MetricObject:   data.MetricAdjustedClose,
	}.String())

	// build copy of dataframe
	momentum, err := m.buildDataFrame(ctx, dateSeriesIdx, prices, rfr)
	if err != nil {
		// already logged
		return nil, err
	}

	// run calculations
	/*
		for _, security := range m.Securities {
			for _, jj := range m.Periods {
				fn := funcs.RegFunc(fmt.Sprintf("(((%s/%sLAG%d)-1)*100)-(RISKFREE%d/12)", security.CompositeFigi, security.CompositeFigi, jj, jj))
				if err := funcs.Evaluate(ctx, momentum, fn, fmt.Sprintf("%sMOM%d", security.CompositeFigi, jj)); err != nil {
					log.Error().Stack().Err(err).Msg("could not calculate momentum")
					return nil, err
				}
			}
		}

		cols := make([]string, 0, len(m.Securities))
		for _, security := range m.Securities {
			scoreName := fmt.Sprintf("%sSCORE", security.CompositeFigi)
			cols = append(cols, scoreName)
			fn := funcs.RegFunc(fmt.Sprintf("(%sMOM1+%sMOM3+%sMOM6)/3", security.CompositeFigi, security.CompositeFigi, security.CompositeFigi))
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
	*/

	var seriesIdx int
	if seriesIdx, err = momentum.NameToColumn(SeriesName); err != nil {
		log.Error().Stack().Err(err).Msg("indicator column does not exist")
		return nil, err

	}
	indicatorSeries := momentum.Series[seriesIdx]
	dateSeries := prices.Series[dateSeriesIdx]
	indicatorDF := dataframe.NewDataFrame(dateSeries, indicatorSeries)

	if indicatorDF, err = dataframe.DropNA(ctx, indicatorDF); err != nil {
		log.Error().Err(err).Msg("could not drop indicator NA rows")
		return nil, err
	}

	return indicatorDF, nil
}
