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
	"time"

	"github.com/goccy/go-json"
	"github.com/jackc/pgx/v4"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/indicators"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
)

var (
	ErrInvalidPeriod = errors.New("invalid period")
	ErrHoldings      = errors.New("not enough holdings in portfolio")
	ErrInvalidRisk   = errors.New("risk i dicator must be one of 'None' or 'Momentum'")
)

type MomentumDrivenEarningsPrediction struct {
	NumHoldings   int
	RiskIndicator string
	OutTicker     *data.Security
	Period        dataframe.Frequency
}

type Period struct {
	Security *data.Security
	Begin    time.Time
	End      time.Time
}

// New Construct a new Momentum Driven Earnings Prediction (MDEP) strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	numHoldings := 100
	if err := json.Unmarshal(args["numHoldings"], &numHoldings); err != nil {
		return nil, err
	}
	if numHoldings < 1 {
		return nil, ErrHoldings
	}

	riskIndicator := "None"
	if err := json.Unmarshal(args["indicator"], &riskIndicator); err != nil {
		log.Error().Err(err).Msg("unmarshal indicator failed")
		return nil, err
	}

	if riskIndicator != "None" && riskIndicator != "Momentum" {
		log.Error().Str("RiskIndicatorValue", riskIndicator).Msg("Unknown risk indicator type")
		return nil, ErrInvalidRisk
	}

	var outTicker data.Security
	if err := json.Unmarshal(args["outTicker"], &outTicker); err != nil {
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
		log.Error().Str("PeriodArg", string(period)).Msg("could not create MDEP strategy, period must be dataframe.Weekly (WeekEnd) or dataframe.Monthly (MonthEnd)")
		return nil, ErrInvalidPeriod
	}

	var mdep strategy.Strategy = &MomentumDrivenEarningsPrediction{
		NumHoldings:   numHoldings,
		OutTicker:     &outTicker,
		RiskIndicator: riskIndicator,
		Period:        period,
	}

	return mdep, nil
}

// Compute signal
func (mdep *MomentumDrivenEarningsPrediction) Compute(ctx context.Context, begin, end time.Time) (data.PortfolioPlan, *data.SecurityAllocation, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "mdep.Compute")
	defer span.End()

	subLog := log.With().Str("Strategy", "MDEP").Logger()
	subLog.Info().Time("Start", begin).Time("End", end).Msg("computing MDEP strategy")

	// Ensure time range is valid
	nullTime := time.Time{}
	if end.Equal(nullTime) {
		end = time.Now()
	}
	if begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		begin = end.AddDate(-50, 0, 0)
	}

	// Get database transaction
	db, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		log.Warn().Stack().Err(err).Msg("could not get database transaction")
		return nil, nil, err
	}

	// get the starting date for ratings
	var startDate time.Time
	sql := "SELECT min(event_date) FROM zacks_financials"
	err = db.QueryRow(ctx, sql).Scan(&startDate)
	if err != nil {
		log.Warn().Stack().Err(err).Str("SQL", sql).Msg("could not query database")
		if err := db.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, nil, err
	}

	if startDate.After(begin) {
		begin = startDate
	}

	subLog.Debug().Time("Start", begin).Time("End", end).Msg("updated time period")

	// get a list of dates to invest in
	manager := data.GetManagerInstance()
	tradeDaysDf := manager.TradingDays(begin, end)
	tradeDaysDf = tradeDaysDf.Frequency(mdep.Period)
	tradeDays := tradeDaysDf.Dates

	indicator, err := mdep.getRiskOnOffIndicator(ctx, begin, end)
	if err != nil {
		if err := db.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, nil, err
	}

	// build target portfolio
	targetPortfolio, err := mdep.buildTargetPortfolio(ctx, tradeDays, indicator, db)
	if err != nil {
		if err := db.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, nil, err
	}

	// Get predicted portfolio
	predictedPortfolio, err := mdep.buildPredictedPortfolio(ctx, tradeDays, db)
	if err != nil {
		if err := db.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, nil, err
	}

	subLog.Info().Msg("MDEP computed")

	if err := db.Commit(ctx); err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not commit transaction")
	}

	return targetPortfolio, predictedPortfolio, nil
}

func (mdep *MomentumDrivenEarningsPrediction) getRiskOnOffIndicator(ctx context.Context, begin, end time.Time) (*dataframe.DataFrame, error) {
	var indicator *dataframe.DataFrame

	subLog := log.With().Str("Strategy", "mdep").Logger()

	switch mdep.RiskIndicator {
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

func getMDEPAssets(ctx context.Context, day time.Time, numAssets int, db pgx.Tx) (map[data.Security]float64, error) {
	subLog := log.With().Str("Strategy", "seek").Logger()
	targetMap := make(map[data.Security]float64)
	cnt := 0
	rows, err := db.Query(ctx, "SELECT composite_figi FROM zacks_financials WHERE zacks_rank=1 AND event_date=$1 ORDER BY market_cap_mil DESC LIMIT $2", day, numAssets)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("could not query database for mdep portfolio")
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
			log.Error().Err(err).Str("CompositeFigi", compositeFigi).Msg("could not load security")
		}
		targetMap[*security] = 0.0
	}

	qty := 1.0 / float64(cnt)
	for k := range targetMap {
		targetMap[k] = qty
	}
	return targetMap, nil
}

func (mdep *MomentumDrivenEarningsPrediction) buildTargetPortfolio(ctx context.Context, tradeDays []time.Time, riskOn *dataframe.DataFrame, db pgx.Tx) (data.PortfolioPlan, error) {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "mdep.buildTargetPortfolio")
	defer span.End()

	subLog := log.With().Str("Strategy", "MDEP").Logger()
	subLog.Debug().Msg("build target portfolio")

	riskIndicator := false
	riskIdx := 0
	NRisk := riskOn.Len()

	targetPortfolio := make(data.PortfolioPlan, 0, len(tradeDays))

	for _, day := range tradeDays {
		var err error
		var riskDate time.Time

		pie := &data.SecurityAllocation{
			Date:           day,
			Justifications: make(map[string]float64),
		}

		// check if risk indicator should be updated
		riskDate = riskOn.Dates[riskIdx]
		if !day.Before(riskDate) {
			riskIndicator = riskOn.Vals[0][riskIdx] > 0
			riskIdx++
			if riskIdx >= NRisk {
				riskIdx--
			}
		}

		if riskIndicator {
			pie.Members, err = getMDEPAssets(ctx, day, mdep.NumHoldings, db)
			if err != nil {
				return nil, err
			}
		} else {
			pie.Members = make(map[data.Security]float64)
		}

		if len(pie.Members) == 0 {
			// nothing to invest in - use cash like asset
			pie.Members[*mdep.OutTicker] = 1.0
		}

		targetPortfolio = append(targetPortfolio, pie)
	}

	return targetPortfolio, nil
}

func (mdep *MomentumDrivenEarningsPrediction) buildPredictedPortfolio(ctx context.Context, tradeDays []time.Time, db pgx.Tx) (*data.SecurityAllocation, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "mdep.buildPredictedPortfolio")
	defer span.End()

	subLog := log.With().Str("Strategy", "MDEP").Logger()
	subLog.Debug().Msg("calculating predicted portfolio")

	var compositeFigi string
	predictedTarget := make(map[data.Security]float64)
	lastDateIdx := len(tradeDays) - 1
	rows, err := db.Query(ctx, "SELECT composite_figi FROM zacks_financials WHERE zacks_rank=1 AND event_date=$1 ORDER BY market_cap_mil DESC LIMIT $2", tradeDays[lastDateIdx], mdep.NumHoldings)
	if err != nil {
		log.Error().Stack().Err(err).Msg("could not query database for MDEP predicted portfolio")
		return nil, err
	}
	for rows.Next() {
		if err := rows.Scan(&compositeFigi); err != nil {
			log.Error().Stack().Err(err).Msg("could not scan rows")
			return nil, err
		}
		security, err := data.SecurityFromFigi(compositeFigi)
		if err != nil {
			log.Error().Err(err).Str("CompositeFigi", compositeFigi).Msg("security not found")
			return nil, err
		}
		predictedTarget[*security] = 1.0 / float64(mdep.NumHoldings)
	}

	predictedPortfolio := &data.SecurityAllocation{
		Date:           tradeDays[lastDateIdx],
		Members:        predictedTarget,
		Justifications: make(map[string]float64),
	}

	return predictedPortfolio, nil
}
