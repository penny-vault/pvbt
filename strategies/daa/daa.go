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
	"math"
	"sort"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/dfextras"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"
	"go.opentelemetry.io/otel"

	"github.com/goccy/go-json"

	"github.com/jdfergason/dataframe-go"
	"github.com/rs/zerolog/log"
)

var (
	ErrCouldNotRetrieveData = errors.New("could not retrieve eod data")
)

// KellersDefensiveAssetAllocation strategy type
type KellersDefensiveAssetAllocation struct {
	// arguments
	breadth            float64
	cashUniverse       []*data.Security
	protectiveUniverse []*data.Security
	riskUniverse       []*data.Security
	topT               int64

	// class variables
	momentum           *dataframe.DataFrame
	predictedPortfolio *strategy.Prediction
	prices             *dataframe.DataFrame
	schedule           *tradecron.TradeCron
	targetPortfolio    *dataframe.DataFrame
}

type momScore struct {
	Security *data.Security
	Score    float64
}

type byTicker []momScore

func (a byTicker) Len() int           { return len(a) }
func (a byTicker) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byTicker) Less(i, j int) bool { return a[i].Score > a[j].Score }

// New constructs a new Kellers DAA strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	cashUniverse := []*data.Security{}
	if err := json.Unmarshal(args["cashUniverse"], &cashUniverse); err != nil {
		return nil, err
	}

	protectiveUniverse := []*data.Security{}
	if err := json.Unmarshal(args["protectiveUniverse"], &protectiveUniverse); err != nil {
		return nil, err
	}

	riskUniverse := []*data.Security{}
	if err := json.Unmarshal(args["riskUniverse"], &riskUniverse); err != nil {
		return nil, err
	}

	var breadth float64
	if err := json.Unmarshal(args["breadth"], &breadth); err != nil {
		return nil, err
	}

	var topT int64
	if err := json.Unmarshal(args["topT"], &topT); err != nil {
		return nil, err
	}

	schedule, err := tradecron.New("@monthend 0 16 * *", tradecron.RegularHours)
	if err != nil {
		return nil, err
	}

	var daa strategy.Strategy = &KellersDefensiveAssetAllocation{
		cashUniverse:       cashUniverse,
		protectiveUniverse: protectiveUniverse,
		riskUniverse:       riskUniverse,
		breadth:            breadth,
		topT:               topT,
		schedule:           schedule,
	}

	return daa, nil
}

func (daa *KellersDefensiveAssetAllocation) downloadPriceData(ctx context.Context, manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

	tickers := []*data.Security{}
	tickers = append(tickers, daa.cashUniverse...)
	tickers = append(tickers, daa.protectiveUniverse...)
	tickers = append(tickers, daa.riskUniverse...)

	prices, errs := manager.GetDataFrame(ctx, data.MetricAdjustedClose, tickers...)

	if errs != nil {
		return ErrCouldNotRetrieveData
	}

	prices, err := dfextras.DropNA(ctx, prices)
	if err != nil {
		return err
	}
	daa.prices = prices

	// include last day if it is a non-trade day
	log.Debug().Msg("getting last day eod prices of requested range")
	manager.Frequency = data.FrequencyDaily
	begin := manager.Begin
	manager.Begin = manager.End.AddDate(0, 0, -10)
	final, err := manager.GetDataFrame(ctx, data.MetricAdjustedClose, tickers...)
	if err != nil {
		log.Error().Err(err).Msg("error getting final")
		return ErrCouldNotRetrieveData
	}
	manager.Begin = begin
	manager.Frequency = data.FrequencyMonthly

	final, err = dfextras.DropNA(ctx, final)
	if err != nil {
		return err
	}

	nrows := final.NRows()
	row := final.Row(nrows-1, false, dataframe.SeriesName)
	dt := row[common.DateIdx].(time.Time)
	lastRow := daa.prices.Row(daa.prices.NRows()-1, false, dataframe.SeriesName)
	lastDt := lastRow[common.DateIdx].(time.Time)
	if !dt.Equal(lastDt) {
		daa.prices.Append(nil, row)
	}

	return nil
}

func (daa *KellersDefensiveAssetAllocation) findTopTRiskAssets() {
	targetAssets := make([]interface{}, daa.momentum.NRows())
	tArray := make([]interface{}, daa.momentum.NRows())
	wArray := make([]interface{}, daa.momentum.NRows())
	iterator := daa.momentum.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: true})

	for {
		row, val, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		// compute the number of bad assets in canary (protective) universe
		var b float64
		for _, security := range daa.protectiveUniverse {
			v := val[security.CompositeFigi].(float64)
			if v < 0 {
				b++
			}
		}

		// compute the cash fraction
		cf := math.Min(1.0, 1.0/float64(daa.topT)*math.Floor(b*float64(daa.topT)/daa.breadth))

		// compute the t parameter for daa
		t := int(math.Round((1.0 - cf) * float64(daa.topT)))
		riskyScores := make([]momScore, len(daa.riskUniverse))
		for ii, security := range daa.riskUniverse {
			riskyScores[ii] = momScore{
				Security: security,
				Score:    val[security.CompositeFigi].(float64),
			}
		}
		tArray[*row] = float64(t)
		sort.Sort(byTicker(riskyScores))

		// get t risk assets
		riskAssets := make([]*data.Security, t)
		for ii := 0; ii < t; ii++ {
			riskAssets[ii] = riskyScores[ii].Security
		}

		// select highest scored cash instrument
		cashScores := make([]momScore, len(daa.cashUniverse))
		for ii, security := range daa.cashUniverse {
			cashScores[ii] = momScore{
				Security: security,
				Score:    val[security.CompositeFigi].(float64),
			}
		}
		sort.Sort(byTicker(cashScores))
		cashSecurity := cashScores[0].Security

		// build investment map
		targetMap := make(map[data.Security]float64)
		if cf > 1.0e-5 {
			targetMap[*cashSecurity] = cf
		}
		w := (1.0 - cf) / float64(t)
		if t == 0 {
			wArray[*row] = 0
		} else {
			wArray[*row] = w
		}

		for _, security := range riskAssets {
			if alloc, ok := targetMap[*security]; ok {
				shares := w + alloc
				if shares > 1.0e-5 {
					targetMap[*security] = shares
				}
			} else if w > 1.0e-5 {
				targetMap[*security] = w
			}
		}

		targetAssets[*row] = targetMap
	}

	timeIdx, err := daa.momentum.NameToColumn(common.DateIdx)
	if err != nil {
		log.Error().Stack().Err(err).Msg("time series not set on momentum series")
	}
	timeSeries := daa.momentum.Series[timeIdx]
	targetSeries := dataframe.NewSeriesMixed(common.TickerName, &dataframe.SeriesInit{Size: len(targetAssets)}, targetAssets...)
	tSeries := dataframe.NewSeriesFloat64("T", &dataframe.SeriesInit{Size: len(tArray)}, tArray...)
	wSeries := dataframe.NewSeriesFloat64("W", &dataframe.SeriesInit{Size: len(wArray)}, wArray...)

	series := make([]dataframe.Series, 0, 4+len(daa.riskUniverse)+len(daa.cashUniverse)+len(daa.protectiveUniverse))
	series = append(series, timeSeries)
	series = append(series, targetSeries)
	series = append(series, tSeries)
	series = append(series, wSeries)

	assetMap := make(map[data.Security]bool)

	universe := make([]*data.Security, 0, len(daa.cashUniverse)+len(daa.riskUniverse)+len(daa.protectiveUniverse))
	universe = append(universe, daa.cashUniverse...)
	universe = append(universe, daa.protectiveUniverse...)
	universe = append(universe, daa.riskUniverse...)

	for _, security := range universe {
		if _, ok := assetMap[*security]; ok {
			continue
		} else {
			assetMap[*security] = true
		}
		idx, err := daa.momentum.NameToColumn(security.CompositeFigi)
		if err != nil {
			log.Warn().Str("Asset", security.CompositeFigi).Msg("could not transalate asset name to series")
		}
		series = append(series, daa.momentum.Series[idx].Copy())
	}

	daa.targetPortfolio = dataframe.NewDataFrame(series...)
}

func (daa *KellersDefensiveAssetAllocation) setPredictedPortfolio() {
	if daa.targetPortfolio.NRows() >= 2 {
		lastRow := daa.targetPortfolio.Row(daa.targetPortfolio.NRows()-1, true, dataframe.SeriesName)
		predictedJustification := make(map[string]float64, len(lastRow)-1)
		for k, v := range lastRow {
			if k != common.TickerName && k != common.DateIdx {
				if val, ok := v.(float64); ok {
					predictedJustification[k.(string)] = val
				}
			}
		}

		lastTradeDate := lastRow[common.DateIdx].(time.Time)
		if !daa.schedule.IsTradeDay(lastTradeDate) {
			daa.targetPortfolio.Remove(daa.targetPortfolio.NRows() - 1)
		}

		nextTradeDate := daa.schedule.Next(lastTradeDate)
		daa.predictedPortfolio = &strategy.Prediction{
			TradeDate:     nextTradeDate,
			Target:        lastRow[common.TickerName].(map[data.Security]float64),
			Justification: predictedJustification,
		}
	}
}

// Compute signal
func (daa *KellersDefensiveAssetAllocation) Compute(ctx context.Context, manager *data.Manager) (*dataframe.DataFrame, *strategy.Prediction, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "daa.Compute")
	defer span.End()

	// Ensure time range is valid (need at least 12 months)
	nullTime := time.Time{}
	if manager.End.Equal(nullTime) {
		manager.End = time.Now()
	}
	if manager.Begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		manager.Begin = manager.End.AddDate(-50, 0, 0)
	} else {
		// Set Begin 12 months in the past so we actually get the requested time range
		manager.Begin = manager.Begin.AddDate(0, -12, 0)
	}

	err := daa.downloadPriceData(ctx, manager)
	if err != nil {
		return nil, nil, err
	}

	// Compute momentum scores
	momentum, err := dfextras.Momentum13612(ctx, daa.prices)
	if err != nil {
		return nil, nil, err
	}

	daa.momentum = momentum
	daa.findTopTRiskAssets()

	// compute the predicted asset
	daa.setPredictedPortfolio()

	return daa.targetPortfolio, daa.predictedPortfolio, nil
}
