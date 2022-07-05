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

	"github.com/goccy/go-json"
	"github.com/jackc/pgx/v4"
	"github.com/jdfergason/dataframe-go"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
)

var (
	ErrHoldings      = errors.New("portfolio must contain at least 2 holdings")
	ErrInvalidPeriod = errors.New("period must be one of 'Weekly' or 'Monthly'")
)

type SeekingAlphaQuant struct {
	NumHoldings int
	OutTicker   string
	Period      data.Frequency
}

type Period struct {
	Asset string
	Begin time.Time
	End   time.Time
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

// New Construct a new Momentum Driven Earnings Prediction (seek) strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	numHoldings := 100
	if err := json.Unmarshal(args["numHoldings"], &numHoldings); err != nil {
		return nil, err
	}
	if numHoldings < 1 {
		return nil, ErrHoldings
	}

	var outTicker string
	if err := json.Unmarshal(args["outTicker"], &outTicker); err != nil {
		return nil, err
	}
	outTicker = strings.ToUpper(outTicker)

	period := "Weekly"
	if err := json.Unmarshal(args["period"], &period); err != nil {
		return nil, err
	}
	if (period != string(data.FrequencyWeekly)) && (period != string(data.FrequencyMonthly)) {
		return nil, ErrInvalidPeriod
	}

	var seek strategy.Strategy = &SeekingAlphaQuant{
		NumHoldings: numHoldings,
		OutTicker:   outTicker,
		Period:      data.Frequency(period),
	}

	return seek, nil
}

// Compute signal
func (seek *SeekingAlphaQuant) Compute(ctx context.Context, manager *data.Manager) (*dataframe.DataFrame, *strategy.Prediction, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "seek.Compute")
	defer span.End()

	nyc, err := time.LoadLocation("America/New_York") // New York is the reference time
	if err != nil {
		log.Panic().Err(err).Msg("cannot load nyc timezone")
	}

	// Ensure time range is valid
	nullTime := time.Time{}
	if manager.End.Equal(nullTime) {
		manager.End = time.Now()
	}
	if manager.Begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		manager.Begin = manager.End.AddDate(-50, 0, 0)
	}

	// Get database transaction
	db, err := database.TrxForUser("pvuser")
	if err != nil {
		log.Error().Err(err).Msg("could not start database transaction")
		return nil, nil, err
	}

	// get the starting date for ratings
	var startDate time.Time
	sql := "SELECT min(event_date) FROM seeking_alpha"
	err = db.QueryRow(ctx, sql).Scan(&startDate)
	if err != nil {
		log.Error().Err(err).Str("SQL", sql).Msg("could not get starting event_date database table")
		return nil, nil, err
	}

	startDate = startDate.In(nyc)
	if startDate.After(manager.Begin) {
		manager.Begin = startDate
	}

	manager.Frequency = data.FrequencyDaily

	// get a list of dates to invest in
	tradeDays := manager.TradingDays(ctx, manager.Begin, manager.End, seek.Period)
	if err != nil {
		return nil, nil, err
	}

	// build target portfolio
	targetPortfolio, err := seek.buildTargetPortfolio(ctx, tradeDays, db)
	if err != nil {
		return nil, nil, err
	}

	// Get predicted portfolio
	predictedPortfolio, err := seek.buildPredictedPortfolio(ctx, tradeDays, db)
	if err != nil {
		return nil, nil, err
	}

	coveredPeriods := findCoveredPeriods(ctx, targetPortfolio)
	prepopulateDataCache(ctx, coveredPeriods, manager)

	log.Info().Msg("SEEK computed")

	return targetPortfolio, predictedPortfolio, nil
}

func (seek *SeekingAlphaQuant) buildPredictedPortfolio(ctx context.Context, tradeDays []time.Time, db pgx.Tx) (*strategy.Prediction, error) {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "seek.buildPredictedPortfolio")
	defer span.End()
	log.Info().Msg("calculating predicted portfolio")

	var ticker string
	predictedTarget := make(map[string]float64)
	lastDateIdx := len(tradeDays) - 1
	rows, err := db.Query(context.Background(), "SELECT ticker FROM seeking_alpha WHERE quant_rating=1 AND event_date=$1 AND market_cap_mil >= 500 ORDER BY quant_rating DESC, market_cap_mil DESC LIMIT $2", tradeDays[lastDateIdx], seek.NumHoldings)
	if err != nil {
		log.Error().Err(err).Msg("could not query database for SEEK predicted portfolio")
		return nil, err
	}
	for rows.Next() {
		if err := rows.Scan(&ticker); err != nil {
			log.Error().Err(err).Msg("could not scan rows")
			return nil, err
		}
		predictedTarget[ticker] = 1.0 / float64(seek.NumHoldings)
	}

	predictedPortfolio := &strategy.Prediction{
		TradeDate:     tradeDays[lastDateIdx],
		Target:        predictedTarget,
		Justification: make(map[string]float64),
	}

	return predictedPortfolio, nil
}

func (seek *SeekingAlphaQuant) buildTargetPortfolio(ctx context.Context, tradeDays []time.Time, db pgx.Tx) (*dataframe.DataFrame, error) {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "seek.buildTargetPortfolio")
	defer span.End()

	log.Info().Msg("build SEEK target portfolio")

	// build target portfolio
	targetAssets := make([]interface{}, 0, 600)
	targetDates := make([]interface{}, 0, 600)
	for _, day := range tradeDays {
		targetMap := make(map[string]float64)
		cnt := 0
		rows, err := db.Query(context.Background(), "SELECT ticker FROM seeking_alpha WHERE quant_rating>=4.5 AND event_date=$1 ORDER BY quant_rating DESC, market_cap_mil DESC LIMIT $2", day, seek.NumHoldings)
		if err != nil {
			log.Error().Err(err).Msg("could not query database for portfolio")
			return nil, err
		}
		for rows.Next() {
			cnt++
			var ticker string
			err := rows.Scan(&ticker)
			if err != nil {
				log.Error().Err(err).Msg("could not scan result")
				return nil, err
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

	return targetPortfolio, nil
}

// prepopulateDataCache loads asset eod prices into the in-memory cache
func prepopulateDataCache(ctx context.Context, covered []*Period, manager *data.Manager) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "prepopulateDataCache")
	defer span.End()

	log.Info().Msg("pre-populate data cache")
	tickerSet := make(map[string]bool, len(covered))

	begin := time.Now()
	end := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, v := range covered {
		tickerSet[v.Asset] = true
		if begin.After(v.Begin) {
			begin = v.Begin
		}
		if end.Before(v.End) {
			end = v.End
		}
	}

	tickerList := make([]string, len(tickerSet))
	ii := 0
	for k := range tickerSet {
		tickerList[ii] = k
		ii++
	}

	manager.Begin = begin
	manager.End = end

	log.Debug().Time("Begin", begin).Time("End", end).Int("NumAssets", len(tickerList)).Msg("querying database for eod")
	if _, err := manager.GetDataFrame(ctx, data.MetricAdjustedClose, tickerList...); err != nil {
		log.Error().Err(err).Strs("Assets", tickerList).Msg("could not get adjusted close dataframe")
	}
	fmt.Println("finished database query")
}

// findCoveredPeriods creates periods that each assets stock prices should be downloaded
func findCoveredPeriods(ctx context.Context, target *dataframe.DataFrame) []*Period {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "buildQueryPlan")
	defer span.End()

	log.Info().Msg("find covered periods in portfolio plan")

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
			for ticker := range assetMap {
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
