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
 * Accelerating Dual Momentum v1.0
 * https://engineeredportfolio.com/2018/05/02/accelerating-dual-momentum-investing/
 *
 * A momentum strategy by Chris Ludlow and Steve Hanly that tries to avoid the
 * S&P500's worst drawdowns while capturing most of the upside. It is an evolution
 * of Gary Antonacci's Dual Momentum strategy.
 */

package adm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
)

var (
	ErrCouldNotRetrieveData = errors.New("could not retrieve data")
)

type AcceleratingDualMomentum struct {
	inTickers    []*data.Security
	prices       *dataframe.DataFrame
	outTicker    *data.Security
	riskFreeRate *dataframe.DataFrame
	momentum     *dataframe.DataFrame
	schedule     *tradecron.TradeCron
}

// New Construct a new Accelerating Dual Momentum strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	inSecurities := []*data.Security{}
	if err := json.Unmarshal(args["inTickers"], &inSecurities); err != nil {
		log.Error().Err(err).Msg("could not un-marshal inTickers argument")
		return nil, err
	}

	var outSecurity data.Security
	if err := json.Unmarshal(args["outTicker"], &outSecurity); err != nil {
		return nil, err
	}

	schedule, err := tradecron.New("@monthend 0 16 * *", tradecron.RegularHours)
	if err != nil {
		return nil, err
	}

	var adm strategy.Strategy = &AcceleratingDualMomentum{
		inTickers: inSecurities,
		outTicker: &outSecurity,
		schedule:  schedule,
	}

	return adm, nil
}

// donloadPriceData loads EOD quotes for in tickers
func (adm *AcceleratingDualMomentum) downloadPriceData(ctx context.Context, manager *data.ManagerV0) error {
	manager.Frequency = data.FrequencyMonthly

	tickers := []*data.Security{}
	tickers = append(tickers, adm.inTickers...)
	riskFreeSymbol, err := data.SecurityFromTicker("DGS3MO")
	if err != nil {
		log.Error().Err(err).Msg("could not find the DGS3MO security")
		return err
	}
	tickers = append(tickers, adm.outTicker, riskFreeSymbol)
	prices, errs := manager.GetDataFrame(ctx, data.MetricAdjustedClose, tickers...)

	if errs != nil {
		return ErrCouldNotRetrieveData
	}

	prices, err = dataframe.DropNA(ctx, prices)
	if err != nil {
		return err
	}

	// include last day if it is a non-trade day
	log.Debug().Msg("getting last day eod prices of requested range")
	manager.Frequency = data.FrequencyDaily
	begin := manager.Begin
	manager.Begin = manager.End.AddDate(0, 0, -10)
	final, err := manager.GetDataFrame(ctx, data.MetricAdjustedClose, tickers...)
	if err != nil {
		log.Error().Err(err).Msg("error getting final")
		return err
	}
	manager.Begin = begin
	manager.Frequency = data.FrequencyMonthly

	final, err = dataframe.DropNA(ctx, final)
	if err != nil {
		return err
	}

	nrows := final.NRows()
	row := final.Row(nrows-1, false, dataframe.SeriesName)
	dt := row[common.DateIdx].(time.Time)
	lastRow := prices.Row(prices.NRows()-1, false, dataframe.SeriesName)
	lastDt := lastRow[common.DateIdx].(time.Time)
	if !dt.Equal(lastDt) {
		prices.Append(nil, row)
	}

	colNames := make([]string, len(adm.inTickers)+1)
	colNames[0] = adm.outTicker.CompositeFigi
	for ii, t := range adm.inTickers {
		colNames[ii+1] = t.CompositeFigi
	}

	eod, riskFreeRate, err := dataframe.Split(ctx, prices, colNames...)
	if err != nil {
		return err
	}

	adm.prices = eod
	adm.riskFreeRate = riskFreeRate

	return nil
}

func (adm *AcceleratingDualMomentum) computeScores(ctx context.Context) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "adm.computeScores")
	defer span.End()

	nrows := adm.prices.NRows(dataframe.Options{})
	periods := []int{1, 3, 6}
	series := []dataframe.Series{}

	rfr := adm.riskFreeRate.Series[1]

	aggFn := dataframe.AggregateSeriesFn(func(vals []interface{}, firstRow int, finalRow int) (float64, error) {
		var sum float64
		for _, val := range vals {
			if v, ok := val.(float64); ok {
				sum += v
			}
		}

		return sum, nil
	})

	dateSeriesIdx, err := adm.prices.NameToColumn(common.DateIdx)
	if err != nil {
		return err
	}

	series = append(series, adm.prices.Series[dateSeriesIdx].Copy())

	for ii := range adm.prices.Series {
		name := adm.prices.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, common.DateIdx) != 0 {
			score := dataframe.NewSeriesFloat64(fmt.Sprintf("%sSCORE", name), &dataframe.SeriesInit{Size: nrows})
			series = append(series, adm.prices.Series[ii].Copy(), score)
		}
	}

	for _, ii := range periods {
		lag := dataframe.Lag(ii, adm.prices)
		roll, err := dataframe.Rolling(ctx, ii, rfr.Copy(), aggFn)

		if err != nil {
			return err
		}
		roll.Rename(fmt.Sprintf("RISKFREE%d", ii))
		series = append(series, roll)
		for _, security := range adm.inTickers {
			jj, err := lag.NameToColumn(security.CompositeFigi)
			if err != nil {
				return err
			}
			s := lag.Series[jj]
			name := fmt.Sprintf("%sLAG%d", security.CompositeFigi, ii)
			s.Rename(name)

			mom := dataframe.NewSeriesFloat64(fmt.Sprintf("%sMOM%d", security.CompositeFigi, ii), &dataframe.SeriesInit{Size: nrows})
			series = append(series, s, mom)
		}
	}

	adm.momentum = dataframe.NewDataFrame(series...)

	/*
		for ii := range adm.inTickers {
			security := adm.inTickers[ii]
			for _, jj := range periods {
				fn := funcs.RegFunc(fmt.Sprintf("(((%s/%sLAG%d)-1)*100)-(RISKFREE%d/12)", security.CompositeFigi, security.CompositeFigi, jj, jj))
				if err := funcs.Evaluate(ctx, adm.momentum, fn, fmt.Sprintf("%sMOM%d", security.CompositeFigi, jj)); err != nil {
					log.Error().Stack().Err(err).Msg("could not calculate momentum")
				}
			}
		}

		for ii := range adm.inTickers {
			security := adm.inTickers[ii]
			fn := funcs.RegFunc(fmt.Sprintf("(%sMOM1+%sMOM3+%sMOM6)/3", security.CompositeFigi, security.CompositeFigi, security.CompositeFigi))
			if err := funcs.Evaluate(ctx, adm.momentum, fn, fmt.Sprintf("%sSCORE", security.CompositeFigi)); err != nil {
				log.Error().Stack().Err(err).Msg("could not calculate score")
			}
		}
	*/

	// write to csv
	if viper.GetBool("debug.dump_csv") {
		adm.writeDataFramesToCSV(ctx)
	}

	return nil
}

// Compute signal for strategy and return list of positions along with the next predicted
// set of assets to hold
func (adm *AcceleratingDualMomentum) Compute(ctx context.Context, manager *data.ManagerV0) (*dataframe.DataFrame, *strategy.Prediction, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "adm.Compute")
	defer span.End()

	// Ensure time range is valid (need at least 6 months)
	nullTime := time.Time{}
	if manager.End.Equal(nullTime) {
		manager.End = time.Now()
	}
	if manager.Begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		manager.Begin = manager.End.AddDate(-50, 0, 0)
	} else {
		// Set Begin 6 months in the past so we actually get the requested time range
		manager.Begin = manager.Begin.AddDate(0, -6, 0)
	}

	err := adm.downloadPriceData(ctx, manager)
	if err != nil {
		return nil, nil, err
	}

	// Compute momentum scores
	if err := adm.computeScores(ctx); err != nil {
		return nil, nil, err
	}

	scores := []dataframe.Series{}
	timeIdx, _ := adm.momentum.NameToColumn(common.DateIdx)

	// create out-of-market series
	dfSize := adm.momentum.Series[timeIdx].NRows()
	zeroes := make([]interface{}, dfSize)
	for ii := 0; ii < dfSize; ii++ {
		zeroes[ii] = 0.0
	}
	outOfMarketSeries := dataframe.NewSeriesFloat64(adm.outTicker.CompositeFigi, &dataframe.SeriesInit{
		Capacity: dfSize,
	}, zeroes...)

	scores = append(scores, adm.momentum.Series[timeIdx], outOfMarketSeries)
	for _, security := range adm.inTickers {
		ii, _ := adm.momentum.NameToColumn(fmt.Sprintf("%sSCORE", security.CompositeFigi))
		series := adm.momentum.Series[ii].Copy()
		series.Rename(security.CompositeFigi)
		scores = append(scores, series)
	}
	scoresDf := dataframe.NewDataFrame(scores...)
	scoresDf, _ = dataframe.DropNA(ctx, scoresDf)

	argmax, err := dataframe.ArgMax(ctx, scoresDf)
	argmax.Rename(common.TickerName)
	if err != nil {
		return nil, nil, err
	}

	dateIdx, err := scoresDf.NameToColumn(common.DateIdx)
	if err != nil {
		return nil, nil, err
	}
	timeSeries := scoresDf.Series[dateIdx].Copy()
	targetPortfolioSeries := make([]dataframe.Series, 0, len(scores))
	targetPortfolioSeries = append(targetPortfolioSeries, timeSeries)
	targetPortfolioSeries = append(targetPortfolioSeries, argmax)
	for ii, xx := range scoresDf.Series {
		if ii >= 2 {
			security, err := data.SecurityFromFigi(xx.Name())
			if err != nil {
				log.Warn().Str("Name", xx.Name()).Err(err).Msg("could not find security from figi")
				xx.Rename(fmt.Sprintf("%s Score", xx.Name()))
			} else {
				xx.Rename(fmt.Sprintf("%s Score", security.Ticker))
			}
			targetPortfolioSeries = append(targetPortfolioSeries, xx)
		}
	}
	targetPortfolio := dataframe.NewDataFrame(targetPortfolioSeries...)

	// compute the predicted asset
	var predictedPortfolio *strategy.Prediction
	if targetPortfolio.NRows() >= 1 {
		lastRow := targetPortfolio.Row(targetPortfolio.NRows()-1, true, dataframe.SeriesName)
		predictedJustification := make(map[string]float64, len(lastRow)-1)
		for k, v := range lastRow {
			if k != common.TickerName && k != common.DateIdx {
				predictedJustification[k.(string)] = v.(float64)
			}
		}

		lastTradeDate := lastRow[common.DateIdx].(time.Time)
		isTradeDay := adm.schedule.IsTradeDay(lastTradeDate)
		if !isTradeDay {
			targetPortfolio.Remove(targetPortfolio.NRows() - 1)
		}

		nextTradeDate := adm.schedule.Next(lastTradeDate)
		compositeFigi := lastRow[common.TickerName].(string)
		security, err := data.SecurityFromFigi(compositeFigi)
		if err != nil {
			log.Error().Err(err).Str("CompositeFigi", compositeFigi).Msg("could not find security")
			return nil, nil, err
		}
		predictedPortfolio = &strategy.Prediction{
			TradeDate: nextTradeDate,
			Target: map[data.Security]float64{
				*security: 1.0,
			},
			Justification: predictedJustification,
		}
	}

	return targetPortfolio, predictedPortfolio, nil
}

func (adm *AcceleratingDualMomentum) writeDataFramesToCSV(ctx context.Context) {
	// momentum
	fh, err := os.OpenFile("adm_momentum.csv", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Error().Stack().Err(err).Str("FileName", "adm_momentum.csv").Msg("error opening file")
		return
	}
	/*
		if err := exports.ExportToCSV(ctx, fh, adm.momentum); err != nil {
			log.Error().Stack().Err(err).Str("FileName", "adm_momentum.csv").Msg("error writing file")
			return
		}
	*/

	// prices
	fh, err = os.OpenFile("adm_prices.csv", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Error().Stack().Err(err).Str("FileName", "adm_prices.csv").Msg("error opening file")
		return
	}
	/*
		if err := exports.ExportToCSV(ctx, fh, adm.prices); err != nil {
			log.Error().Stack().Err(err).Str("FileName", "adm_prices.csv").Msg("error writing file")
			return
		}
	*/

	// riskfree
	fh, err = os.OpenFile("adm_riskfreerate.csv", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Error().Stack().Err(err).Str("FileName", "adm_riskfreerate.csv").Msg("error opening file")
		return
	}
	/*
		if err := exports.ExportToCSV(ctx, fh, adm.riskFreeRate); err != nil {
			log.Error().Stack().Err(err).Str("FileName", "adm_riskfreerate.csv").Msg("error writing file")
			return
		}
	*/
}
