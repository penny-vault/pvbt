// Copyright 2021-2025
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
	"math"
	"time"

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

// riskAdjustedMomentum calculates riskAdjustedMomentum(period) = (df / lag(df, period) - 1) * 100 - riskFreeRate
// where riskFreeRate is the monthly yield of a risk free investment
func riskAdjustedMomentum(period int, df, riskFreeRate *dataframe.DataFrame) *dataframe.DataFrame {
	riskFreeRate = riskFreeRate.RollingSumScaled(period, -1.0/12.0)
	return df.Div(df.Lag(period)).AddScalar(-1).MulScalar(100).AddVec(riskFreeRate.Vals[0])
}

// momentum calculates momentum(period) = (df / lag(df, period) - 1) * 100
// where riskFreeRate is the monthly yield of a risk free investment
func momentum(period int, df *dataframe.DataFrame) *dataframe.DataFrame {
	return df.Div(df.Lag(period)).AddScalar(-1).MulScalar(100)
}

// Momentum631 computes the 6-3-1 momentum of each column in df
// momentum(period) = (df / lag(df, period) - 1) * 100 - riskFreeRate
func Momentum631(prices *dataframe.DataFrame, riskFreeRate *dataframe.DataFrame) *dataframe.DataFrame {
	momentum6 := riskAdjustedMomentum(6, prices, riskFreeRate)
	momentum3 := riskAdjustedMomentum(3, prices, riskFreeRate)
	momentum1 := riskAdjustedMomentum(1, prices, riskFreeRate)

	avgMomentum := dataframe.Mean(momentum6, momentum3, momentum1) // take the average of the same security column across all dataframes
	return avgMomentum
}

// Momentum12631 computes the equal weighted average momentum score over 1-, 3-, 6-, and 12-month periods
func Momentum12631(prices *dataframe.DataFrame) *dataframe.DataFrame {
	momentum12 := momentum(12, prices).MulScalar(1.0 / 100.0)
	momentum6 := momentum(6, prices).MulScalar(2.0 / 100.0)
	momentum3 := momentum(3, prices).MulScalar(4.0 / 100.0)
	momentum1 := momentum(1, prices).MulScalar(12.0 / 100.0)

	avgMomentum := dataframe.Mean(momentum12, momentum6, momentum3, momentum1) // take the average of the same security column across all dataframes
	return avgMomentum
}

func (m *Momentum) IndicatorForPeriod(_ context.Context, start time.Time, end time.Time) (*dataframe.DataFrame, error) {
	begin := start.AddDate(0, -6, 0)

	// Add the risk free asset
	securities := make([]*data.Security, 0, len(m.Securities)+1)
	securities = append(securities, &data.Security{
		CompositeFigi: "PVGG06TNP6J8",
		Ticker:        "DGS3MO",
	})
	securities = append(securities, m.Securities...)

	// fetch prices
	priceMap, err := data.NewDataRequest(securities...).Metrics(data.MetricAdjustedClose).Between(begin, end)
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

	riskFreeRate, prices := prices.Split(data.SecurityMetric{
		SecurityObject: *dgs3mo,
		MetricObject:   data.MetricAdjustedClose,
	}.String())

	// run calculations
	// for each security and period compute
	// mom(security, period) = ((security / lag(security, period) - 1) * 100) - (sum(risk free rate, period) / 12)
	// score(security) = average(mom(security, 1), mom(security, 3), mom(security, 6))
	// indicator = max(score)
	indicatorDF := Momentum631(prices, riskFreeRate).Max()
	indicatorDF.ColNames[0] = SeriesName
	indicatorDF = indicatorDF.Drop(math.NaN())

	return indicatorDF, nil
}
