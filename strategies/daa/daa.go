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
 * Keller's Defensive Asset Allocation v1.0
 * https://indexswingtrader.blogspot.com/2018/07/announcing-defensive-asset-allocation.html
 * https://papers.ssrn.com/sol3/papers.cfm?abstract_id=3212862
 *
 * Keller's Defensive Asset Allocation (DAA) builds on the framework designed for
 * Keller's Vigilant Asset Allocation (VAA). For DAA the need for crash protection
 * is quantified using a separate “canary” universe instead of the full investment
 * universe as with VAA. DAA leads to lower out-of-market allocations and hence
 * improves the tracking error due to higher in-the-market-rates
 */

package daa

import (
	"context"
	"errors"
	"main/common"
	"main/data"
	"main/dfextras"
	"main/strategies/strategy"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

// KellersDefensiveAssetAllocation strategy type
type KellersDefensiveAssetAllocation struct {
	cashUniverse       []string
	protectiveUniverse []string
	riskUniverse       []string
	breadth            float64
	topT               int64
	targetPortfolio    *dataframe.DataFrame
	prices             *dataframe.DataFrame
	momentum           *dataframe.DataFrame

	// Public
	CurrentSymbol string
}

type momScore struct {
	Ticker string
	Score  float64
}

type byTicker []momScore

func (a byTicker) Len() int           { return len(a) }
func (a byTicker) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byTicker) Less(i, j int) bool { return a[i].Score > a[j].Score }

// New constructs a new Kellers DAA strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	cashUniverse := []string{}
	if err := json.Unmarshal(args["cashUniverse"], &cashUniverse); err != nil {
		return nil, err
	}
	common.ArrToUpper(cashUniverse)

	protectiveUniverse := []string{}
	if err := json.Unmarshal(args["protectiveUniverse"], &protectiveUniverse); err != nil {
		return nil, err
	}
	common.ArrToUpper(protectiveUniverse)

	riskUniverse := []string{}
	if err := json.Unmarshal(args["riskUniverse"], &riskUniverse); err != nil {
		return nil, err
	}
	common.ArrToUpper(riskUniverse)

	var breadth float64
	if err := json.Unmarshal(args["breadth"], &breadth); err != nil {
		return nil, err
	}

	var topT int64
	if err := json.Unmarshal(args["topT"], &topT); err != nil {
		return nil, err
	}

	var daa strategy.Strategy = &KellersDefensiveAssetAllocation{
		cashUniverse:       cashUniverse,
		protectiveUniverse: protectiveUniverse,
		riskUniverse:       riskUniverse,
		breadth:            breadth,
		topT:               topT,
	}

	return daa, nil
}

func (daa *KellersDefensiveAssetAllocation) findTopTRiskAssets() {
	targetAssets := make([]interface{}, daa.momentum.NRows())
	iterator := daa.momentum.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: true})

	for {
		row, val, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		// compute the number of bad assets in canary (protective) universe
		var b float64
		for _, ticker := range daa.protectiveUniverse {
			v := val[ticker].(float64)
			if v < 0 {
				b++
			}
		}

		// compute the cash fraction
		cf := math.Min(1.0, 1.0/float64(daa.topT)*math.Floor(b*float64(daa.topT)/daa.breadth))

		// compute the t parameter for daa
		t := int(math.Round((1.0 - cf) * float64(daa.topT)))
		riskyScores := make([]momScore, len(daa.riskUniverse))
		for ii, ticker := range daa.riskUniverse {
			riskyScores[ii] = momScore{
				Ticker: ticker,
				Score:  val[ticker].(float64),
			}
		}
		sort.Sort(byTicker(riskyScores))

		// get t risk assets
		riskAssets := make([]string, t)
		for ii := 0; ii < t; ii++ {
			riskAssets[ii] = riskyScores[ii].Ticker
		}

		// select highest scored cash instrument
		cashScores := make([]momScore, len(daa.cashUniverse))
		for ii, ticker := range daa.cashUniverse {
			cashScores[ii] = momScore{
				Ticker: ticker,
				Score:  val[ticker].(float64),
			}
		}
		sort.Sort(byTicker(cashScores))
		cashAsset := cashScores[0].Ticker

		// build investment map
		targetMap := make(map[string]float64)
		if cf > 1.0e-5 {
			targetMap[cashAsset] = cf
		}
		w := (1.0 - cf) / float64(t)

		for _, asset := range riskAssets {
			if alloc, ok := targetMap[asset]; ok {
				shares := w + alloc
				if shares > 1.0e-5 {
					targetMap[asset] = shares
				}
			} else {
				// skip 0 allocations
				if w > 1.0e-5 {
					targetMap[asset] = w
				}
			}
		}

		targetAssets[*row] = targetMap
	}

	timeIdx, err := daa.momentum.NameToColumn(data.DateIdx)
	if err != nil {
		log.Error("Time series not set on momentum series")
	}
	timeSeries := daa.momentum.Series[timeIdx]

	targetSeries := dataframe.NewSeriesMixed(common.TickerName, &dataframe.SeriesInit{Size: len(targetAssets)}, targetAssets...)
	daa.targetPortfolio = dataframe.NewDataFrame(timeSeries, targetSeries)
}

func (daa *KellersDefensiveAssetAllocation) downloadPriceData(manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly
	manager.Metric = data.MetricAdjustedClose

	tickers := []string{}
	tickers = append(tickers, daa.cashUniverse...)
	tickers = append(tickers, daa.protectiveUniverse...)
	tickers = append(tickers, daa.riskUniverse...)

	prices, errs := manager.GetMultipleData(tickers...)

	if len(errs) > 0 {
		return errors.New("failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for _, v := range prices {
		eod = append(eod, v)
	}

	mergedEod, err := dfextras.MergeAndTimeAlign(context.TODO(), data.DateIdx, eod...)
	daa.prices = mergedEod
	if err != nil {
		return err
	}

	return nil
}

// Compute signal
func (daa *KellersDefensiveAssetAllocation) Compute(manager *data.Manager) (*dataframe.DataFrame, error) {
	// Ensure time range is valid (need at least 12 months)
	nullTime := time.Time{}
	if manager.End == nullTime {
		manager.End = time.Now()
	}
	if manager.Begin == nullTime {
		// Default computes things 50 years into the past
		manager.Begin = manager.End.AddDate(-50, 0, 0)
	} else {
		// Set Begin 12 months in the past so we actually get the requested time range
		manager.Begin = manager.Begin.AddDate(0, -12, 0)
	}

	err := daa.downloadPriceData(manager)
	if err != nil {
		return nil, err
	}

	// Compute momentum scores
	momentum, err := dfextras.Momentum13612(daa.prices)
	if err != nil {
		return nil, err
	}

	daa.momentum = momentum
	daa.findTopTRiskAssets()

	symbols := []string{}
	tickerIdx, _ := daa.targetPortfolio.NameToColumn(common.TickerName)
	lastTarget := daa.targetPortfolio.Series[tickerIdx].Value(daa.targetPortfolio.NRows() - 1).(map[string]float64)
	for kk := range lastTarget {
		symbols = append(symbols, kk)
	}
	sort.Strings(symbols)
	daa.CurrentSymbol = strings.Join(symbols, " ")

	return daa.targetPortfolio, nil
}
