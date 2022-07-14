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
	"github.com/jdfergason/dataframe-go"
	"github.com/jdfergason/dataframe-go/exports"
	"github.com/jdfergason/dataframe-go/math/funcs"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/dfextras"
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
	inTickers    []string
	prices       *dataframe.DataFrame
	outTicker    string
	riskFreeRate *dataframe.DataFrame
	momentum     *dataframe.DataFrame
	schedule     *tradecron.TradeCron
}

// New Construct a new Accelerating Dual Momentum strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	inTickers := []string{}
	if err := json.Unmarshal(args["inTickers"], &inTickers); err != nil {
		return nil, err
	}

	common.ArrToUpper(inTickers)

	var outTicker string
	if err := json.Unmarshal(args["outTicker"], &outTicker); err != nil {
		return nil, err
	}

	outTicker = strings.ToUpper(outTicker)

	schedule, err := tradecron.New("@monthend", tradecron.RegularHours)
	if err != nil {
		return nil, err
	}

	var adm strategy.Strategy = &AcceleratingDualMomentum{
		inTickers: inTickers,
		outTicker: outTicker,
		schedule:  schedule,
	}

	return adm, nil
}

func (adm *AcceleratingDualMomentum) downloadPriceData(ctx context.Context, manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

	tickers := []string{}
	tickers = append(tickers, adm.inTickers...)
	riskFreeSymbol := "DGS3MO"
	tickers = append(tickers, adm.outTicker, riskFreeSymbol)
	prices, errs := manager.GetDataFrame(ctx, data.MetricAdjustedClose, tickers...)

	if errs != nil {
		return ErrCouldNotRetrieveData
	}

	colNames := make([]string, len(adm.inTickers)+1)
	colNames[0] = adm.outTicker
	for ii, t := range adm.inTickers {
		colNames[ii+1] = t
	}
	prices, err := dfextras.DropNA(ctx, prices)
	if err != nil {
		return err
	}

	eod, riskFreeRate, err := dfextras.Split(ctx, prices, colNames...)
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

	aggFn := dfextras.AggregateSeriesFn(func(vals []interface{}, firstRow int, finalRow int) (float64, error) {
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
		lag := dfextras.Lag(ii, adm.prices)
		roll, err := dfextras.Rolling(context.TODO(), ii, rfr.Copy(), aggFn)

		if err != nil {
			return err
		}
		roll.Rename(fmt.Sprintf("RISKFREE%d", ii))
		series = append(series, roll)
		for _, ticker := range adm.inTickers {
			jj, err := lag.NameToColumn(ticker)
			if err != nil {
				return err
			}
			s := lag.Series[jj]
			name := fmt.Sprintf("%sLAG%d", ticker, ii)
			s.Rename(name)

			mom := dataframe.NewSeriesFloat64(fmt.Sprintf("%sMOM%d", ticker, ii), &dataframe.SeriesInit{Size: nrows})
			series = append(series, s, mom)
		}
	}

	adm.momentum = dataframe.NewDataFrame(series...)

	for ii := range adm.inTickers {
		ticker := adm.inTickers[ii]
		for _, jj := range periods {
			fn := funcs.RegFunc(fmt.Sprintf("(((%s/%sLAG%d)-1)*100)-(RISKFREE%d/12)", ticker, ticker, jj, jj))
			if err := funcs.Evaluate(ctx, adm.momentum, fn, fmt.Sprintf("%sMOM%d", ticker, jj)); err != nil {
				log.Error().Err(err).Msg("could not calculate momentum")
			}
		}
	}

	for ii := range adm.inTickers {
		ticker := adm.inTickers[ii]
		fn := funcs.RegFunc(fmt.Sprintf("(%sMOM1+%sMOM3+%sMOM6)/3", ticker, ticker, ticker))
		if err := funcs.Evaluate(ctx, adm.momentum, fn, fmt.Sprintf("%sSCORE", ticker)); err != nil {
			log.Error().Err(err).Msg("could not calculate score")
		}
	}

	// compute average scores
	if viper.GetBool("debug.dump_csv") {
		adm.writeDataFramesToCSV()
	}

	return nil
}

// Compute signal for strategy and return list of positions along with the next predicted
// set of assets to hold
func (adm *AcceleratingDualMomentum) Compute(ctx context.Context, manager *data.Manager) (*dataframe.DataFrame, *strategy.Prediction, error) {
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
	outOfMarketSeries := dataframe.NewSeriesFloat64(adm.outTicker, &dataframe.SeriesInit{
		Capacity: dfSize,
	}, zeroes...)

	scores = append(scores, adm.momentum.Series[timeIdx], outOfMarketSeries)
	for _, ticker := range adm.inTickers {
		ii, _ := adm.momentum.NameToColumn(fmt.Sprintf("%sSCORE", ticker))
		series := adm.momentum.Series[ii].Copy()
		series.Rename(ticker)
		scores = append(scores, series)
	}
	scoresDf := dataframe.NewDataFrame(scores...)
	scoresDf, _ = dfextras.DropNA(context.TODO(), scoresDf)

	argmax, err := dfextras.ArgMax(context.TODO(), scoresDf)
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
			xx.Rename(fmt.Sprintf("%s Score", xx.Name()))
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
		nextTradeDate, err := adm.schedule.Next(lastTradeDate)
		if err != nil {
			log.Error().Err(err).Msg("could not get next trade date")
			return nil, nil, err
		}
		if !lastTradeDate.Equal(nextTradeDate) {
			targetPortfolio.Remove(targetPortfolio.NRows() - 1)
		}

		predictedPortfolio = &strategy.Prediction{
			TradeDate: nextTradeDate,
			Target: map[string]float64{
				lastRow[common.TickerName].(string): 1.0,
			},
			Justification: predictedJustification,
		}
	}

	return targetPortfolio, predictedPortfolio, nil
}

func (adm *AcceleratingDualMomentum) writeDataFramesToCSV() {
	ctx := context.Background()

	// momentum
	fh, err := os.OpenFile("adm_momentum.csv", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Error().Err(err).Str("FileName", "adm_momentum.csv").Msg("error opening file")
		return
	}
	if err := exports.ExportToCSV(ctx, fh, adm.momentum); err != nil {
		log.Error().Err(err).Str("FileName", "adm_momentum.csv").Msg("error writing file")
		return
	}

	// prices
	fh, err = os.OpenFile("adm_prices.csv", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Error().Err(err).Str("FileName", "adm_prices.csv").Msg("error opening file")
		return
	}
	if err := exports.ExportToCSV(ctx, fh, adm.prices); err != nil {
		log.Error().Err(err).Str("FileName", "adm_prices.csv").Msg("error writing file")
		return
	}

	// riskfree
	fh, err = os.OpenFile("adm_riskfreerate.csv", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Error().Err(err).Str("FileName", "adm_riskfreerate.csv").Msg("error opening file")
		return
	}
	if err := exports.ExportToCSV(ctx, fh, adm.riskFreeRate); err != nil {
		log.Error().Err(err).Str("FileName", "adm_riskfreerate.csv").Msg("error writing file")
		return
	}
}
