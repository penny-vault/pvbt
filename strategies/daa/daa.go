// Copyright 2021-2023
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
	"strings"
	"time"

	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/indicators"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"
	"go.opentelemetry.io/otel"

	"github.com/goccy/go-json"

	"github.com/penny-vault/pv-api/dataframe"
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
	predictedPortfolio *data.SecurityAllocation
	prices             *dataframe.DataFrame
	schedule           *tradecron.TradeCron
	targetPortfolio    data.PortfolioPlan
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

func (daa *KellersDefensiveAssetAllocation) downloadPriceData(begin, end time.Time) error {
	// Load EOD quotes for in tickers
	tickers := []*data.Security{}
	tickers = append(tickers, daa.cashUniverse...)
	tickers = append(tickers, daa.protectiveUniverse...)
	tickers = append(tickers, daa.riskUniverse...)

	prices, err := data.NewDataRequest(tickers...).Metrics(data.MetricAdjustedClose).Between(begin, end)
	if err != nil {
		return ErrCouldNotRetrieveData
	}

	prices = prices.Frequency(dataframe.Monthly)
	prices = prices.Drop(math.NaN())
	daa.prices = prices.DataFrame()

	// include last day if it is a non-trade day
	log.Debug().Msg("getting last day eod prices of requested range")
	finalPricesMap, err := data.NewDataRequest(tickers...).Metrics(data.MetricAdjustedClose).Between(end.AddDate(0, 0, -10), end)
	if err != nil {
		log.Error().Err(err).Msg("error getting final prices in daa")
		return err
	}

	finalPrices := finalPricesMap.DataFrame()
	finalPrices = finalPrices.Drop(math.NaN())
	daa.prices.Append(finalPrices.Last())

	// Rename columns to composite figi only -- this is to promote readability when debugging
	for ii := range daa.prices.ColNames {
		daa.prices.ColNames[ii] = strings.Split(daa.prices.ColNames[ii], ":")[0]
	}

	return nil
}

func (daa *KellersDefensiveAssetAllocation) calculatePortfolio() {
	pies := make(data.PortfolioPlan, daa.momentum.Len())
	securityMap := make(map[string]*data.Security) // create a local lookup table for securities for performance reasons
	daa.momentum.ForEach(func(rowIdx int, rowDt time.Time, vals map[string]float64) map[string]float64 {
		pie := &data.SecurityAllocation{
			Date:           rowDt,
			Members:        make(map[data.Security]float64),
			Justifications: make(map[string]float64),
		}
		pies[rowIdx] = pie

		// copy momentum's over to justification using the securities ticker as the column name
		for k, v := range vals {
			var sec *data.Security
			var ok bool
			var err error

			if sec, ok = securityMap[k]; !ok {
				if sec, err = data.SecurityFromFigi(k); err == nil {
					securityMap[k] = sec
				}
			}

			pie.Justifications[sec.Ticker] = v
		}

		// compute the number of bad assets in canary (protective) universe
		var b float64
		for _, security := range daa.protectiveUniverse {
			v := vals[security.CompositeFigi]
			if v < 0 {
				b++
			}
		}
		pie.Justifications["B"] = b

		// compute the cash fraction
		cf := math.Min(1.0, 1.0/float64(daa.topT)*math.Floor(b*float64(daa.topT)/daa.breadth))
		pie.Justifications["CF"] = cf

		// compute the t parameter for daa
		t := int(math.Round((1.0 - cf) * float64(daa.topT)))
		pie.Justifications["T"] = float64(t)

		// select the top-T risk assets
		riskyScores := make([]momScore, len(daa.riskUniverse))
		for ii, security := range daa.riskUniverse {
			riskyScores[ii] = momScore{
				Security: security,
				Score:    vals[security.CompositeFigi],
			}
		}
		sort.Sort(byTicker(riskyScores))

		// select highest scored cash instrument
		cashScores := make([]momScore, len(daa.cashUniverse))
		for ii, security := range daa.cashUniverse {
			cashScores[ii] = momScore{
				Security: security,
				Score:    vals[security.CompositeFigi],
			}
		}
		sort.Sort(byTicker(cashScores))
		cashSecurity := cashScores[0].Security

		// build portfolio
		if cf > 1.0e-5 {
			pie.Members[*cashSecurity] = cf
		}

		w := (1.0 - cf) / float64(t)
		if t == 0 {
			pie.Justifications["W"] = 0
		} else {
			pie.Justifications["W"] = w
		}

		// get t risk assets
		for ii := 0; ii < t; ii++ {
			security := riskyScores[ii].Security
			if alloc, ok := pie.Members[*security]; ok {
				alloc = w + alloc
				if alloc > 1.0e-5 {
					pie.Members[*security] = alloc
				}
			} else if w > 1.0e-5 {
				pie.Members[*security] = w
			}
		}

		return nil
	})

	daa.targetPortfolio = pies
}

func (daa *KellersDefensiveAssetAllocation) setPredictedPortfolio() {
	numPeriods := len(daa.targetPortfolio)
	if numPeriods >= 2 {
		lastPie := daa.targetPortfolio[numPeriods-1]
		if !daa.schedule.IsTradeDay(lastPie.Date) {
			daa.targetPortfolio = daa.targetPortfolio[:numPeriods-1]
		}

		nextTradeDate := daa.schedule.Next(lastPie.Date)
		daa.predictedPortfolio = &data.SecurityAllocation{
			Date:           nextTradeDate,
			Members:        lastPie.Members,
			Justifications: lastPie.Justifications,
		}
	}
}

// Compute signal
func (daa *KellersDefensiveAssetAllocation) Compute(ctx context.Context, begin, end time.Time) (data.PortfolioPlan, *data.SecurityAllocation, error) {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "daa.Compute")
	defer span.End()

	// Ensure time range is valid (need at least 12 months)
	nullTime := time.Time{}
	if end.Equal(nullTime) {
		end = time.Now()
	}
	if begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		begin = end.AddDate(-50, 0, 0)
	} else {
		// Set begin 12 months in the past so we actually get the requested time range
		begin = begin.AddDate(0, -12, 0)
	}

	err := daa.downloadPriceData(begin, end)
	if err != nil {
		return nil, nil, err
	}

	// Compute momentum scores
	daa.momentum = indicators.Momentum12631(daa.prices)
	daa.momentum = daa.momentum.Drop(math.NaN())

	// Calculate the portfolio
	daa.calculatePortfolio()

	// compute the predicted asset
	daa.setPredictedPortfolio()

	return daa.targetPortfolio, daa.predictedPortfolio, nil
}
