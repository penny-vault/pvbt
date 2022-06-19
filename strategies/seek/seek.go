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

/*
 * Seeking Alpha Quant Ratings v1.0
 *
 * Invest in a diversified portfolio of stocks ranked by their likelihood to
 * out-perform based on quantitative ratings from Seeking Alpha.
 * Includes a built-in crash protection that exits to a "safe" asset when
 * market sentiment goes negative.
 *
 */

package seek

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"
	"go.opentelemetry.io/otel"

	"github.com/goccy/go-json"

	"github.com/rocketlaunchr/dataframe-go"
)

type SeekingAlphaQuant struct {
	NumHoldings int
	OutTicker   string
	Period      string
	schedule    *tradecron.TradeCron
}

type Period struct {
	Asset string
	Begin time.Time
	End   time.Time
}

type Cluster struct {
	Begin   time.Time
	End     time.Time
	Members []*Period
}

type ByStartDur []*Period

func (a ByStartDur) Len() int      { return len(a) }
func (a ByStartDur) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByStartDur) Less(i, j int) bool {
	aDur := a[i].End.Sub(a[i].Begin)
	bDur := a[j].End.Sub(a[j].Begin)
	if a[i].Begin.Equal(a[j].Begin) {
		return aDur < bDur
	}

	return a[i].Begin.Before(a[j].Begin)
}

const (
	WEEKLY  = "weekly"
	MONTHLY = "monthly"
)

func absDur(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// New Construct a new Momentum Driven Earnings Prediction (seek) strategy
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

	var seek strategy.Strategy = &SeekingAlphaQuant{
		NumHoldings: numHoldings,
		OutTicker:   outTicker,
		Period:      period,
	}

	return seek, nil
}

// Compute signal
func (seek *SeekingAlphaQuant) Compute(ctx context.Context, manager *data.Manager) (*dataframe.DataFrame, *strategy.Prediction, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "seek.Compute")
	defer span.End()

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
		return nil, nil, err
	}

	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
	var startDate time.Time
	err = db.QueryRow(ctx, "SELECT min(event_date) FROM seeking_alpha").Scan(&startDate)
	if err != nil {
		return nil, nil, err
	}

	startDate = startDate.In(tz)
	if startDate.After(manager.Begin) {
		manager.Begin = startDate
	}

	manager.Frequency = data.FrequencyDaily

	// get a list of dates to invest in
	dates := make([]time.Time, 0, 600)
	tradeDays := manager.TradingDays(ctx, manager.Begin, manager.End)
	if err != nil {
		return nil, nil, err
	}

	switch seek.Period {
	case WEEKLY:
		wk := -1
		for _, curr := range tradeDays {
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
		month := -1
		for _, curr := range tradeDays {
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

	// build target portfolio
	targetAssets := make([]interface{}, 0, 600)
	targetDates := make([]interface{}, 0, 600)
	for _, day := range dates {
		targetMap := make(map[string]float64)
		cnt := 0
		rows, err := db.Query(context.Background(), "SELECT ticker FROM seeking_alpha WHERE quant_rating>=4.5 AND event_date=$1 ORDER BY quant_rating DESC, market_cap_mil DESC LIMIT $2", day, seek.NumHoldings)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			cnt++
			var ticker string
			err := rows.Scan(&ticker)
			if err != nil {
				return nil, nil, err
			}
			targetMap[ticker] = 0.0
		}

		qty := 1.0 / float64(cnt)
		for k := range targetMap {
			targetMap[k] = qty
		}

		if len(targetMap) == 0 {
			// nothing to invest in - use cash like asset
			targetMap["VUSTX"] = 1.0
		}

		targetDates = append(targetDates, day)
		targetAssets = append(targetAssets, targetMap)
	}

	timeSeries := dataframe.NewSeriesTime(common.DateIdx, &dataframe.SeriesInit{Size: len(targetDates)}, targetDates...)
	targetSeries := dataframe.NewSeriesMixed(common.TickerName, &dataframe.SeriesInit{Size: len(targetAssets)}, targetAssets...)
	targetPortfolio := dataframe.NewDataFrame(timeSeries, targetSeries)

	// Get predicted portfolio
	var ticker string
	predictedTarget := make(map[string]float64)
	lastDateIdx := len(dates) - 1
	rows, err := db.Query(context.Background(), "SELECT ticker FROM seeking_alpha WHERE quant_rating=1 AND event_date=$1 AND market_cap_mil >= 500 ORDER BY quant_rating DESC, market_cap_mil DESC LIMIT $2", dates[lastDateIdx], seek.NumHoldings)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		rows.Scan(&ticker)
		predictedTarget[ticker] = 1.0 / float64(seek.NumHoldings)
	}

	//nextTradeDate := seek.schedule.Next(time.Now())
	predictedPortfolio := &strategy.Prediction{
		TradeDate:     dates[lastDateIdx],
		Target:        predictedTarget,
		Justification: make(map[string]float64),
	}

	coveredPeriods := findCoveredPeriods(ctx, targetPortfolio)
	prepopulateDataCache(ctx, coveredPeriods, manager)

	return targetPortfolio, predictedPortfolio, nil
}

// prepopulateDataCache loads asset eod prices into the in-memory cache
func prepopulateDataCache(ctx context.Context, covered []*Period, manager *data.Manager) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "prepopulateDataCache")
	defer span.End()

	s := time.Now()
	for _, v := range covered {
		fmt.Printf("%s\t%s\t%s\n", v.Asset, v.Begin.Format("2006-01-02"), v.End.Format("2006-01-02"))
		manager.Begin = v.Begin
		manager.End = v.End
		manager.GetDataFrame(ctx, data.MetricAdjustedClose, v.Asset)
	}
	e := time.Now()

	fmt.Printf("Elapsed: %s", e.Sub(s))
}

// findCoveredPeriods creates periods that each assets stock prices should be downloaded
func findCoveredPeriods(ctx context.Context, target *dataframe.DataFrame) []*Period {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "buildQueryPlan")
	defer span.End()

	coveredPeriods := make([]*Period, 0, target.NRows())
	activeAssets := make(map[string]*Period)
	var pendingClose map[string]*Period

	tickerSeriesIdx := target.MustNameToColumn(common.TickerName)

	// check series type
	isSingleAsset := false
	series := target.Series[tickerSeriesIdx]
	if series.Type() == "string" {
		isSingleAsset = true
	}

	// Create a map of asset time periods
	iterator := target.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
	for {
		row, val, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		date := val[common.DateIdx].(time.Time)

		pendingClose = activeAssets
		activeAssets = make(map[string]*Period)

		if isSingleAsset {
			ticker := val[common.TickerName].(string)
			period, ok := pendingClose[ticker]
			if !ok {
				period = &Period{
					Asset: ticker,
					Begin: date,
				}
			} else {
				delete(pendingClose, ticker)
			}
			if period.End.Before(date) {
				period.End = date.AddDate(0, 0, 7)
			}
			activeAssets[ticker] = period
		} else {
			// it's multi-asset which means a map of tickers
			assetMap := val[common.TickerName].(map[string]float64)
			for ticker, _ := range assetMap {
				period, ok := pendingClose[ticker]
				if !ok {
					period = &Period{
						Asset: ticker,
						Begin: date,
					}
				} else {
					delete(pendingClose, ticker)
				}
				if period.End.Before(date) {
					period.End = date.AddDate(0, 0, 8)
				}
				activeAssets[ticker] = period
			}
		}

		// any assets that remain in pending close should be added to covered periods
		for _, v := range pendingClose {
			coveredPeriods = append(coveredPeriods, v)
		}
	}

	// any remaining assets should be added to coveredPeriods
	for _, v := range activeAssets {
		coveredPeriods = append(coveredPeriods, v)
	}

	return coveredPeriods
}
