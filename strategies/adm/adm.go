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
	"main/common"
	"main/data"
	"main/dfextras"
	"main/strategies/strategy"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/rocketlaunchr/dataframe-go"
	"github.com/rocketlaunchr/dataframe-go/math/funcs"

	log "github.com/sirupsen/logrus"
)

type AcceleratingDualMomentum struct {
	inTickers    []string
	prices       *dataframe.DataFrame
	outTicker    string
	riskFreeRate *dataframe.DataFrame
	momentum     *dataframe.DataFrame

	// Public
	CurrentSymbol string
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

	var adm strategy.Strategy = &AcceleratingDualMomentum{
		inTickers: inTickers,
		outTicker: outTicker,
	}

	return adm, nil
}

func (adm *AcceleratingDualMomentum) downloadPriceData(manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

	tickers := []string{}
	tickers = append(tickers, adm.inTickers...)
	riskFreeSymbol := "$RATE.TB3MS"
	tickers = append(tickers, adm.outTicker, riskFreeSymbol)
	prices, errs := manager.GetMultipleData(tickers...)

	if len(errs) > 0 {
		return errors.New("failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for ii := range adm.inTickers {
		ticker := adm.inTickers[ii]
		eod = append(eod, prices[ticker])
	}

	eod = append(eod, prices[adm.outTicker])

	mergedEod, err := dfextras.MergeAndTimeAlign(context.TODO(), data.DateIdx, eod...)
	adm.prices = mergedEod
	if err != nil {
		return err
	}

	// Get aligned start and end times
	timeColumn, err := mergedEod.NameToColumn(data.DateIdx, dataframe.Options{})
	if err != nil {
		return err
	}

	timeSeries := mergedEod.Series[timeColumn]
	nrows := timeSeries.NRows(dataframe.Options{})
	startTime := timeSeries.Value(0, dataframe.Options{}).(time.Time)
	endTime := timeSeries.Value(nrows-1, dataframe.Options{}).(time.Time)

	// Get risk free rate (3-mo T-bill secondary rate)
	riskFreeRate := prices[riskFreeSymbol]

	// duplicate last row if it doesn't match endTime
	valueIdx, _ := riskFreeRate.NameToColumn("TB3MS")
	timeSeriesIdx, _ := riskFreeRate.NameToColumn(data.DateIdx)
	rr := riskFreeRate.Series[valueIdx]
	nrows = rr.NRows(dataframe.Options{})
	val := rr.Value(nrows-1, dataframe.Options{}).(float64)
	timeSeries = riskFreeRate.Series[timeSeriesIdx]
	timeVal := timeSeries.Value(nrows-1, dataframe.Options{}).(time.Time)
	if (endTime.Month() != timeVal.Month()) || (endTime.Year() != timeVal.Year()) {
		riskFreeRate.Append(&dataframe.Options{}, endTime, val)
	}

	// Align the risk-free rate to match the mergedEod
	_, err = dfextras.TimeTrim(context.TODO(), riskFreeRate, timeSeriesIdx, startTime, endTime, true)
	if err != nil {
		return err
	}

	timeVal = timeSeries.Value(0, dataframe.Options{}).(time.Time)
	val = rr.Value(0, dataframe.Options{}).(float64)
	if startTime.Before(timeVal) {
		riskFreeRate.Insert(0, &dataframe.Options{}, startTime, val)
	}

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

	dateSeriesIdx, err := adm.prices.NameToColumn(data.DateIdx)
	if err != nil {
		return err
	}

	series = append(series, adm.prices.Series[dateSeriesIdx].Copy())

	for ii := range adm.prices.Series {
		name := adm.prices.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, data.DateIdx) != 0 {
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

// Compute signal
func (adm *AcceleratingDualMomentum) Compute(manager *data.Manager) (*dataframe.DataFrame, error) {
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

	t1 := time.Now()
	err := adm.downloadPriceData(manager)
	if err != nil {
		return nil, err
	}
	t2 := time.Now()

	// Compute momentum scores
	t3 := time.Now()
	adm.computeScores()
	t4 := time.Now()

	t5 := time.Now()
	scores := []dataframe.Series{}
	timeIdx, _ := adm.momentum.NameToColumn(data.DateIdx)

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

	tmp, _ := dfextras.DropNA(context.TODO(), scoresDf)
	scoresDf = tmp.(*dataframe.DataFrame)

	argmax, err := dfextras.ArgMax(context.TODO(), scoresDf)
	argmax.Rename(common.TickerName)
	if err != nil {
		return nil, err
	}

	dateIdx, err := scoresDf.NameToColumn(data.DateIdx)
	if err != nil {
		return nil, err
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
	t6 := time.Now()
	adm.CurrentSymbol = targetPortfolio.Series[1].Value(targetPortfolio.NRows() - 1).(string)

	log.WithFields(log.Fields{
		"QuoteDownload":      t2.Sub(t1).Round(time.Millisecond),
		"ScoreCalculation":   t4.Sub(t3).Round(time.Millisecond),
		"PortfolioSelection": t6.Sub(t5).Round(time.Millisecond),
	}).Info("ADM calculation runtimes (s)")

	return targetPortfolio, nil
}
