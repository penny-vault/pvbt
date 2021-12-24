// Copyright 2021 JD Fergason
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

/*
 * Momentum Driven Earnings Prediction v1.0
 *
 * Invest in a diversified portfolio of stocks ranked by their likelihood to
 * out-perform based on earnings estimates and earnings estimate revisions.
 * Includes a built-in crash protection that exits to a "safe" asset when
 * market sentiment goes negative.
 *
 * StormGuard Armour is used as the sentiment metric.
 * Earnings estimates and revisions are provided by Zack's Investment Research.
 */

package mdep

import (
	"context"
	"errors"
	"main/common"
	"main/data"
	"main/database"
	"main/strategies/strategy"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/rocketlaunchr/dataframe-go"
)

type MomentumDrivenEarningsPrediction struct {
	NumHoldings int
	OutTicker   string
	Period      string
}

type sentiment struct {
	EventDate       time.Time
	StormGuardArmor float64
}

const (
	WEEKLY  = "weekly"
	MONTHLY = "monthly"
)

// New Construct a new Momentum Driven Earnings Prediction (MDEP) strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	numHoldings := 100
	if err := json.Unmarshal(args["numHoldings"], &numHoldings); err != nil {
		return nil, err
	}
	if numHoldings < 1 {
		return nil, errors.New("numHoldings must be > 1")
	}

	var outTicker string
	if err := json.Unmarshal(args["outTicker"], &outTicker); err != nil {
		return nil, err
	}
	outTicker = strings.ToUpper(outTicker)

	period := "weekly"
	if err := json.Unmarshal(args["period"], &period); err != nil {
		return nil, err
	}
	if (period != WEEKLY) && (period != MONTHLY) {
		return nil, errors.New("period must be one of 'weekly' or 'monthly'")
	}

	var mdep strategy.Strategy = &MomentumDrivenEarningsPrediction{
		NumHoldings: numHoldings,
		OutTicker:   outTicker,
		Period:      period,
	}

	return mdep, nil
}

// Compute signal
func (mdep *MomentumDrivenEarningsPrediction) Compute(manager *data.Manager) (*dataframe.DataFrame, error) {
	// Ensure time range is valid
	nullTime := time.Time{}
	if manager.End.Equal(nullTime) {
		manager.End = time.Now()
	}
	if manager.Begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		manager.Begin = manager.End.AddDate(-50, 0, 0)
	}

	db, err := database.TrxForUser("pvuser")
	if err != nil {
		return nil, err
	}

	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
	var startDate time.Time
	err = db.QueryRow(context.Background(), "SELECT min(event_date) FROM zacks_financials_v1").Scan(&startDate)
	if err != nil {
		return nil, err
	}

	if startDate.After(manager.Begin) {
		manager.Begin = startDate
	}

	manager.Frequency = data.FrequencyDaily

	// get a list of dates to invest in
	dates := make([]time.Time, 0, 600)
	tradeDays, err := manager.GetDataFrame(data.MetricAdjustedClose, "VFINX")
	if err != nil {
		return nil, err
	}

	switch mdep.Period {
	case WEEKLY:
		iterator := tradeDays.ValuesIterator()
		wk := -1
		for {
			row, val, _ := iterator(dataframe.SeriesName)
			if row == nil {
				break
			}

			curr := val[common.DateIdx].(time.Time)
			if wk == -1 {
				_, wk = curr.ISOWeek()
				dates = append(dates, curr)
			} else {
				_, newWk := curr.ISOWeek()
				if newWk != wk {
					dates = append(dates, curr)
				}
				wk = newWk
			}
		}
	case MONTHLY:
		iterator := tradeDays.ValuesIterator()
		month := -1
		for {
			row, val, _ := iterator(dataframe.SeriesName)
			if row == nil {
				break
			}

			curr := val[common.DateIdx].(time.Time)
			if month == -1 {
				month = int(curr.Month())
				dates = append(dates, curr)
			} else {
				newMonth := int(curr.Month())
				if newMonth != month {
					dates = append(dates, curr)
				}
				month = newMonth
			}
		}
	}

	// load market sentiment scores
	crashDetection := make([]*sentiment, 0, 600)
	rows, err := db.Query(context.Background(), "SELECT event_date, COALESCE(sg_armor, 0.1) FROM risk_indicators_v1 WHERE event_date >= $1 ORDER BY event_date", manager.Begin)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var newSentiment sentiment
		err := rows.Scan(&newSentiment.EventDate, &newSentiment.StormGuardArmor)
		if err != nil {
			return nil, err
		}
		newSentiment.EventDate = time.Date(newSentiment.EventDate.Year(), newSentiment.EventDate.Month(), newSentiment.EventDate.Day(), 16, 0, 0, 0, tz)
		crashDetection = append(crashDetection, &newSentiment)
	}

	// build target portfolio
	targetAssets := make([]interface{}, 0, 600)
	targetDates := make([]interface{}, 0, 600)
	nextDateIdx := 0
	for _, s := range crashDetection {
		if s.StormGuardArmor <= 0 {
			targetMap := map[string]float64{
				mdep.OutTicker: 1.0,
			}
			targetDates = append(targetDates, s.EventDate)
			targetAssets = append(targetAssets, targetMap)
			continue
		}

		for nextDateIdx < len(dates) && dates[nextDateIdx].Before(s.EventDate) {
			nextDateIdx++
		}

		if nextDateIdx < len(dates) &&
			s.EventDate.Year() == dates[nextDateIdx].Year() &&
			s.EventDate.Month() == dates[nextDateIdx].Month() &&
			s.EventDate.Day() == dates[nextDateIdx].Day() {
			targetMap := make(map[string]float64)
			cnt := 0
			//rows, err := db.Query(context.Background(), "SELECT ticker FROM zacks_financials_v1 WHERE zacks_rank=1 AND market_cap_mil>=100 AND percent_rating_change_4wk >= 0 AND percent_change_q1_est >= 0 AND event_date=$1 ORDER BY percent_rating_change_4wk desc, percent_change_q1_est desc, market_cap_mil desc LIMIT $2", dates[nextDateIdx], mdep.NumHoldings)
			rows, err := db.Query(context.Background(), "SELECT ticker FROM zacks_financials_v1 WHERE zacks_rank=1 AND event_date=$1 ORDER BY market_cap_mil DESC LIMIT $2", dates[nextDateIdx], mdep.NumHoldings)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				cnt++
				var ticker string
				err := rows.Scan(&ticker)
				if err != nil {
					return nil, err
				}
				targetMap[ticker] = 0.0
			}
			qty := 1.0 / float64(cnt)
			for k := range targetMap {
				targetMap[k] = qty
			}

			targetDates = append(targetDates, s.EventDate)
			targetAssets = append(targetAssets, targetMap)

			nextDateIdx++
		}
	}

	timeSeries := dataframe.NewSeriesTime(common.DateIdx, &dataframe.SeriesInit{Size: len(targetDates)}, targetDates...)
	targetSeries := dataframe.NewSeriesMixed(common.TickerName, &dataframe.SeriesInit{Size: len(targetAssets)}, targetAssets...)
	targetPortfolio := dataframe.NewDataFrame(timeSeries, targetSeries)

	return targetPortfolio, nil
}
