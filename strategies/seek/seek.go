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
	"time"

	"github.com/goccy/go-json"
	"github.com/jackc/pgx/v4"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/indicators"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
)

var (
	ErrHoldings      = errors.New("portfolio must contain at least 2 holdings")
	ErrInvalidPeriod = errors.New("period must be one of 'Weekly' or 'Monthly'")
	ErrInvalidRisk   = errors.New("risk i dicator must be one of 'None' or 'Momentum'")
)

type SeekingAlphaQuant struct {
	NumHoldings   int
	OutSecurity   *data.Security
	RiskIndicator string
	Period        dataframe.Frequency
	schedule      *tradecron.TradeCron
}

type Period struct {
	Security *data.Security
	Begin    time.Time
	End      time.Time
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
	numHoldings := 50
	if err := json.Unmarshal(args["numHoldings"], &numHoldings); err != nil {
		return nil, err
	}
	if numHoldings < 1 {
		return nil, ErrHoldings
	}

	var outSecurity *data.Security
	if err := json.Unmarshal(args["outTicker"], &outSecurity); err != nil {
		return nil, err
	}

	periodStr := string(dataframe.Weekly)
	if err := json.Unmarshal(args["period"], &periodStr); err != nil {
		return nil, err
	}
	switch periodStr {
	case "Weekly":
		periodStr = string(dataframe.Weekly)
	case "Monthly":
		periodStr = string(dataframe.Monthly)
	}
	period := dataframe.Frequency(periodStr)
	if (period != dataframe.Weekly) && (period != dataframe.Monthly) {
		log.Error().Str("PeriodArg", string(period)).Msg("could not create SEEK strategy, period must be dataframe.Weekly (WeekEnd) or dataframe.Monthly (MonthEnd)")
		return nil, ErrInvalidPeriod
	}

	var cronspec *tradecron.TradeCron
	var err error
	switch dataframe.Frequency(period) {
	case dataframe.Monthly:
		cronspec, err = tradecron.New("@monthend", tradecron.RegularHours)
		if err != nil {
			return nil, err
		}
	case dataframe.Weekly:
		cronspec, err = tradecron.New("@weekend", tradecron.RegularHours)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidPeriod
	}

	riskIndicator := "None"
	if err := json.Unmarshal(args["indicator"], &riskIndicator); err != nil {
		return nil, err
	}

	if riskIndicator != "None" && riskIndicator != "Momentum" {
		log.Error().Str("RiskIndicatorValue", riskIndicator).Msg("Unknown risk indicator type")
		return nil, ErrInvalidRisk
	}

	var seek strategy.Strategy = &SeekingAlphaQuant{
		NumHoldings:   numHoldings,
		OutSecurity:   outSecurity,
		Period:        dataframe.Frequency(period),
		RiskIndicator: riskIndicator,
		schedule:      cronspec,
	}

	return seek, nil
}

// Compute signal
func (seek *SeekingAlphaQuant) Compute(ctx context.Context, begin, end time.Time) (data.PortfolioPlan, *data.SecurityAllocation, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "seek.Compute")
	defer span.End()

	subLog := log.With().Str("Strategy", "seek").Logger()
	subLog.Info().Msg("computing strategy portfolio")

	nyc := common.GetTimezone()

	// Ensure time range is valid
	nullTime := time.Time{}
	if end.Equal(nullTime) {
		end = time.Now()
	}
	if begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		begin = end.AddDate(-50, 0, 0)
	}

	database.LogOpenTransactions()

	// Get database transaction
	db, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		log.Error().Stack().Err(err).Msg("could not start database transaction")
		return nil, nil, err
	}

	// get the starting date for ratings
	var startDate time.Time
	sql := "SELECT min(event_date) FROM seeking_alpha"
	err = db.QueryRow(ctx, sql).Scan(&startDate)
	if err != nil {
		log.Error().Stack().Err(err).Str("SQL", sql).Msg("could not get starting event_date database table")
		if err := db.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, nil, err
	}

	subLog.Debug().Time("Start", begin).Time("End", end).Msg("updated time period")

	startDate = startDate.In(nyc)
	if startDate.After(begin) {
		begin = startDate
	}

	// get a list of dates to invest in
	// NOTE: trading days always appends the last day, even if it doesn't match
	// the frequency specification, need to make sure you use tradecron
	// to check the last date and ensure that it's a tradeable day.
	manager := data.GetManagerInstance()
	tradeDaysDf := manager.TradingDays(begin, end)
	tradeDaysDf = tradeDaysDf.Frequency(seek.Period)
	tradeDays := tradeDaysDf.Dates

	if len(tradeDays) == 0 {
		subLog.Info().Msg("no available trading days")
	} else {
		subLog.Debug().Msg("checking trading days against schedule")
		endIdx := len(tradeDays) - 1
		lastDate := tradeDays[endIdx]
		isTradeDay := seek.schedule.IsTradeDay(lastDate)
		if !isTradeDay {
			tradeDays = tradeDays[:endIdx]
		}
	}

	// Calculate risk on/off indicator
	indicator, err := seek.getRiskOnOffIndicator(ctx, begin, end)
	if err != nil {
		if err := db.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, nil, err
	}

	// build target portfolio
	targetPortfolio, err := seek.buildTargetPortfolio(ctx, tradeDays, indicator, db)
	if err != nil {
		if err := db.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, nil, err
	}

	// Get predicted portfolio
	predictedPortfolio, err := seek.buildPredictedPortfolio(ctx, tradeDays, db)
	if err != nil {
		if err := db.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, nil, err
	}

	log.Info().Msg("SEEK computed")

	if err := db.Commit(ctx); err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not commit transaction")
	}
	return targetPortfolio, predictedPortfolio, nil
}

func (seek *SeekingAlphaQuant) getRiskOnOffIndicator(ctx context.Context, begin, end time.Time) (*dataframe.DataFrame, error) {
	var indicator *dataframe.DataFrame

	subLog := log.With().Str("Strategy", "seek").Logger()

	switch seek.RiskIndicator {
	case "Momentum":
		subLog.Debug().Msg("get risk on/off indicator")
		securities, err := data.SecurityFromTickerList([]string{"VFINX", "PRIDX"})
		if err != nil {
			log.Error().Err(err).Strs("Securities", []string{"VFINX", "PRIDX"}).Msg("securities not found")
			return nil, err
		}
		momentum := &indicators.Momentum{
			Securities: securities,
			Periods:    []int{1, 3, 6},
		}
		indicator, err = momentum.IndicatorForPeriod(ctx, begin, end)
		if err != nil {
			subLog.Error().Err(err).Msg("could not get risk on/off indicator")
			return nil, err
		}
	default:
		manager := data.GetManagerInstance()
		indicator = manager.TradingDays(begin, end)
		for idx := range indicator.Vals[0] {
			indicator.Vals[0][idx] = 1.0
		}
	}
	return indicator, nil
}

func (seek *SeekingAlphaQuant) buildPredictedPortfolio(ctx context.Context, tradeDays []time.Time, db pgx.Tx) (*data.SecurityAllocation, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "seek.buildPredictedPortfolio")
	defer span.End()

	subLog := log.With().Str("Strategy", "seek").Logger()
	subLog.Debug().Msg("calculating predicted portfolio")

	var compositeFigi string
	predictedTarget := make(map[data.Security]float64)
	lastDateIdx := len(tradeDays) - 1
	rows, err := db.Query(ctx, "SELECT composite_figi FROM seeking_alpha WHERE quant_rating=1 AND event_date=$1 AND market_cap_mil >= 500 ORDER BY quant_rating DESC, market_cap_mil DESC LIMIT $2", tradeDays[lastDateIdx], seek.NumHoldings)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("could not query database for SEEK predicted portfolio")
		return nil, err
	}
	for rows.Next() {
		if err := rows.Scan(&compositeFigi); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not scan rows")
			return nil, err
		}
		security, err := data.SecurityFromFigi(compositeFigi)
		if err != nil {
			log.Error().Err(err).Str("CompositeFigi", compositeFigi).Msg("security not found")
			return nil, err
		}
		predictedTarget[*security] = 1.0 / float64(seek.NumHoldings)
	}

	predictedPortfolio := &data.SecurityAllocation{
		Date:           tradeDays[lastDateIdx],
		Members:        predictedTarget,
		Justifications: make(map[string]float64),
	}

	return predictedPortfolio, nil
}

func getSeekAssets(ctx context.Context, day time.Time, numAssets int, db pgx.Tx) (map[data.Security]float64, error) {
	subLog := log.With().Str("Strategy", "seek").Logger()
	targetMap := make(map[data.Security]float64)
	cnt := 0
	rows, err := db.Query(ctx, "SELECT composite_figi FROM seeking_alpha WHERE quant_rating>=4.5 AND event_date=$1 ORDER BY quant_rating DESC, market_cap_mil DESC LIMIT $2", day, numAssets)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("could not query database for portfolio")
		return nil, err
	}
	for rows.Next() {
		cnt++
		var compositeFigi string
		err := rows.Scan(&compositeFigi)
		if err != nil {
			subLog.Error().Stack().Err(err).Msg("could not scan result")
			return nil, err
		}
		security, err := data.SecurityFromFigi(compositeFigi)
		if err != nil {
			log.Error().Err(err).Str("CompositeFigi", compositeFigi).Msg("security not found")
			return nil, err
		}

		targetMap[*security] = 0.0
	}

	qty := 1.0 / float64(cnt)
	for k := range targetMap {
		targetMap[k] = qty
	}
	return targetMap, nil
}

func (seek *SeekingAlphaQuant) buildTargetPortfolio(ctx context.Context, tradeDays []time.Time, riskOn *dataframe.DataFrame, db pgx.Tx) (data.PortfolioPlan, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "seek.buildTargetPortfolio")
	defer span.End()

	subLog := log.With().Str("Strategy", "seek").Logger()
	subLog.Debug().Msg("build target portfolio")

	targetPortfolio := make(data.PortfolioPlan, 0, len(tradeDays))

	riskIndicator := false
	riskIdx := 0
	NRisk := riskOn.Len()

	for _, day := range tradeDays {
		var err error
		var targetMap map[data.Security]float64

		var riskDate time.Time = riskOn.Dates[riskIdx]
		if !day.Before(riskDate) {
			riskValue := riskOn.Vals[0][riskIdx]
			riskIndicator = riskValue > 0

			riskIdx++
			if riskIdx >= NRisk {
				riskIdx--
			}
		}

		if riskIndicator {
			targetMap, err = getSeekAssets(ctx, day, seek.NumHoldings, db)
			if err != nil {
				return nil, err
			}
		} else {
			targetMap = make(map[data.Security]float64)
		}

		if len(targetMap) == 0 {
			// nothing to invest in - use cash like asset
			targetMap[*seek.OutSecurity] = 1.0
		}

		pie := &data.SecurityAllocation{
			Date:           day,
			Members:        targetMap,
			Justifications: make(map[string]float64),
		}

		targetPortfolio = append(targetPortfolio, pie)
	}

	return targetPortfolio, nil
}
