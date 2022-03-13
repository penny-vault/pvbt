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
	"fmt"
	"main/common"
	"main/data"
	"main/dfextras"
	"main/strategies/strategy"
	"main/tradecron"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/rocketlaunchr/dataframe-go"
	"github.com/rocketlaunchr/dataframe-go/math/funcs"
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

func (adm *AcceleratingDualMomentum) downloadPriceData(manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

	tickers := []string{}
	tickers = append(tickers, adm.inTickers...)
	riskFreeSymbol := "DGS3MO"
	tickers = append(tickers, adm.outTicker, riskFreeSymbol)
	prices, errs := manager.GetDataFrame(data.MetricAdjustedClose, tickers...)

	if errs != nil {
		return fmt.Errorf("failed to download data in adm for tickers: %s", errs)
	}

	colNames := make([]string, len(adm.inTickers)+1)
	colNames[0] = adm.outTicker
	for ii, t := range adm.inTickers {
		colNames[ii+1] = t
	}
	prices, err := dfextras.DropNA(context.Background(), prices)
	if err != nil {
		return err
	}

	eod, riskFreeRate, err := dfextras.Split(context.Background(), prices, colNames...)
	if err != nil {
		return err
	}

	adm.prices = eod
	adm.riskFreeRate = riskFreeRate

	return nil
}

func (adm *AcceleratingDualMomentum) computeScores() error {
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
			funcs.Evaluate(context.TODO(), adm.momentum, fn, fmt.Sprintf("%sMOM%d", ticker, jj))
		}
	}

	// compute average scores
	for ii := range adm.inTickers {
		ticker := adm.inTickers[ii]
		fn := funcs.RegFunc(fmt.Sprintf("(%sMOM1+%sMOM3+%sMOM6)/3", ticker, ticker, ticker))
		funcs.Evaluate(context.TODO(), adm.momentum, fn, fmt.Sprintf("%sSCORE", ticker))
	}

	return nil
}

// Compute signal for strategy and return list of positions along with the next predicted
// set of assets to hold
func (adm *AcceleratingDualMomentum) Compute(manager *data.Manager) (*dataframe.DataFrame, *strategy.Prediction, error) {
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

	// t1 := time.Now()
	err := adm.downloadPriceData(manager)
	if err != nil {
		return nil, nil, err
	}
	// t2 := time.Now()

	// Compute momentum scores
	// t3 := time.Now()
	adm.computeScores()
	// t4 := time.Now()

	// t5 := time.Now()
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
		nextTradeDate := adm.schedule.Next(lastTradeDate)
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

	// t6 := time.Now()

	/*
		log.WithFields(log.Fields{
			"QuoteDownload":      t2.Sub(t1).Round(time.Millisecond),
			"ScoreCalculation":   t4.Sub(t3).Round(time.Millisecond),
			"PortfolioSelection": t6.Sub(t5).Round(time.Millisecond),
		}).Info("ADM calculation runtimes")
	*/

	return targetPortfolio, predictedPortfolio, nil
}
