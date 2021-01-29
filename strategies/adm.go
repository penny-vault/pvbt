package strategies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"main/data"
	"main/dfextras"
	"strings"
	"time"

	"github.com/rocketlaunchr/dataframe-go"
	"github.com/rocketlaunchr/dataframe-go/math/funcs"
)

// AcceleratingDualMomentumInfo information describing this strategy
func AcceleratingDualMomentumInfo() StrategyInfo {
	return StrategyInfo{
		Name:        "Accelerating Dual Momentum",
		Shortcode:   "adm",
		Description: "A market timing strategy that uses a 1-, 3-, and 6-month momentum score to select assets.",
		Source:      "https://engineeredportfolio.com/2018/05/02/accelerating-dual-momentum-investing/",
		Version:     "1.0.0",
		YTDGain:     1.84,
		Arguments: map[string]Argument{
			"inTickers": Argument{
				Name:        "Tickers",
				Description: "List of ETF, Mutual Fund, or Stock tickers to invest in",
				Typecode:    "[]string",
				DefaultVal:  "[\"VFINX\", \"PRIDX\"]",
			},
			"outTicker": Argument{
				Name:        "Out-of-Market Ticker",
				Description: "Ticker to use when model scores are all below 0",
				Typecode:    "string",
				DefaultVal:  "VUSTX",
			},
		},
		Factory: New,
	}
}

type acceleratingDualMomentum struct {
	info          StrategyInfo
	inTickers     []string
	inPrices      *dataframe.DataFrame
	outTicker     string
	outPrices     *dataframe.DataFrame
	riskFreeRate  *dataframe.DataFrame
	momentum      *dataframe.DataFrame
	dataStartTime time.Time
	dataEndTime   time.Time
}

// New Construct a new Accelerating Dual Momentum strategy
func New(args map[string]json.RawMessage) (Strategy, error) {
	inTickers := []string{}
	if err := json.Unmarshal(args["inTickers"], &inTickers); err != nil {
		return acceleratingDualMomentum{}, err
	}

	var outTicker string
	if err := json.Unmarshal(args["outTicker"], &outTicker); err != nil {
		return acceleratingDualMomentum{}, err
	}

	adm := acceleratingDualMomentum{
		info:      AcceleratingDualMomentumInfo(),
		inTickers: inTickers,
		outTicker: outTicker,
	}

	return adm, nil
}

func (adm acceleratingDualMomentum) GetInfo() StrategyInfo {
	return adm.info
}

func (adm *acceleratingDualMomentum) downloadPriceData(manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

	tickers := []string{}
	tickers = append(tickers, adm.inTickers...)
	riskFreeSymbol := "$RATE.TB3MS"
	tickers = append(tickers, adm.outTicker, riskFreeSymbol)
	data, errs := manager.GetMultipleData(tickers...)

	if len(errs) > 0 {
		return errors.New("Failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for ii := range adm.inTickers {
		ticker := adm.inTickers[ii]
		eod = append(eod, data[ticker])
	}

	mergedEod, err := dfextras.MergeAndTimeAlign(context.TODO(), "DATE", eod...)
	adm.inPrices = mergedEod

	if err != nil {
		return err
	}

	// Get aligned start and end times
	timeColumn, err := mergedEod.NameToColumn("DATE", dataframe.Options{})
	if err != nil {
		return err
	}

	timeSeries := mergedEod.Series[timeColumn]
	nrows := timeSeries.NRows(dataframe.Options{})
	startTime := timeSeries.Value(0, dataframe.Options{}).(time.Time)
	endTime := timeSeries.Value(nrows-1, dataframe.Options{}).(time.Time)
	adm.dataStartTime = startTime
	adm.dataEndTime = endTime

	// Get out-of-market EOD
	outOfMarketEod := data[adm.outTicker]
	timeSeriesIdx, err := outOfMarketEod.NameToColumn("DATE")
	if err != nil {
		return err
	}

	outOfMarketEod, err = dfextras.TimeAlign(context.TODO(), outOfMarketEod, timeSeriesIdx, startTime, endTime)
	if err != nil {
		return err
	}

	adm.outPrices = outOfMarketEod

	// Get risk free rate (3-mo T-bill secondary rate)
	riskFreeRate := data[riskFreeSymbol]

	// duplicate last row if it doesn't match endTime
	valueIdx, err := riskFreeRate.NameToColumn("TB3MS")
	timeSeriesIdx, err = riskFreeRate.NameToColumn("DATE")
	rr := riskFreeRate.Series[valueIdx]
	nrows = rr.NRows(dataframe.Options{})
	val := rr.Value(nrows-1, dataframe.Options{}).(float64)
	timeSeries = riskFreeRate.Series[timeSeriesIdx]
	timeVal := timeSeries.Value(nrows-1, dataframe.Options{}).(time.Time)
	if endTime.After(timeVal) {
		riskFreeRate.Append(&dataframe.Options{}, endTime, val)
	}

	if err != nil {
		return err
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

func (adm *acceleratingDualMomentum) computeScores() error {
	nrows := adm.inPrices.NRows(dataframe.Options{})
	periods := []int{1, 3, 6}
	series := []dataframe.Series{}

	rfr := adm.riskFreeRate.Series[1]

	aggFn := dfextras.AggregateSeriesFn(func(vals []interface{}, firstRow int, finalRow int) (interface{}, error) {
		var sum float64
		for _, val := range vals {
			if v, ok := val.(float64); ok {
				sum += v
			}
		}

		return sum, nil
	})

	dateSeriesIdx, err := adm.inPrices.NameToColumn("DATE")
	if err != nil {
		return err
	}

	series = append(series, adm.inPrices.Series[dateSeriesIdx].Copy())

	for ii := range adm.inPrices.Series {
		name := adm.inPrices.Series[ii].Name(dataframe.Options{})
		if strings.Compare(name, "DATE") != 0 {
			score := dataframe.NewSeriesFloat64(fmt.Sprintf("%sSCORE", name), &dataframe.SeriesInit{Size: nrows})
			series = append(series, adm.inPrices.Series[ii].Copy(), score)
		}
	}

	for _, ii := range periods {
		lag := dfextras.Lag(ii, adm.inPrices)
		roll, err := dfextras.Rolling(context.TODO(), ii, rfr.Copy(), aggFn)

		if err != nil {
			return err
		}
		roll.Rename(fmt.Sprintf("RISKFREE%d", ii))
		series = append(series, roll)
		for jj := range lag.Series {
			s := lag.Series[jj]
			symbol := s.Name(dataframe.Options{})
			if strings.Compare(symbol, "DATE") != 0 {
				name := fmt.Sprintf("%sLAG%d", symbol, ii)
				s.Rename(name)

				mom := dataframe.NewSeriesFloat64(fmt.Sprintf("%sMOM%d", symbol, ii), &dataframe.SeriesInit{Size: nrows})
				series = append(series, s, mom)
			}
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

func (adm acceleratingDualMomentum) Compute(manager *data.Manager) (StrategyPerformance, error) {
	// Ensure time range is valid (need at least 6 months)
	nullTime := time.Time{}
	if manager.End == nullTime {
		manager.End = time.Now()
	}
	if manager.Begin == nullTime {
		//dur, _ := time.ParseDuration("-8760h")
		//manager.Begin = manager.End.Add(dur)
		manager.Begin = manager.End.AddDate(-35, 0, 0)
	}

	err := adm.downloadPriceData(manager)
	if err != nil {
		return StrategyPerformance{}, err
	}

	// Compute momentum scores
	adm.computeScores()

	scores := []dataframe.Series{}
	timeIdx, _ := adm.momentum.NameToColumn("DATE")
	scores = append(scores, adm.momentum.Series[timeIdx])
	for _, ticker := range adm.inTickers {
		ii, _ := adm.momentum.NameToColumn(fmt.Sprintf("%sSCORE", ticker))
		series := adm.momentum.Series[ii]
		scores = append(scores, series)
	}

	scoresDf := dataframe.NewDataFrame(scores...)
	log.Println(scoresDf.Table())

	/*
		// Calculate adm score (mom1 + mom3 + mom6) / 3
		score := (mom1 + mom3 + mom6).average()
		score = score.dropna()

		// If all scores > 0 then invest in max(score) else outOfMarketAsset
		score.argmax(ROWS)
		holdings := score.gt(0) || score.lte(0, adm.outTicker)

		portfolio := portfolio.New()
		portfolio.SetTargetHoldings(holdings)

		performance := portfolio.evaluatePerformance()
		performance.StrategyInformation = adm.info
		return performance
	*/
	return StrategyPerformance{}, nil
}
