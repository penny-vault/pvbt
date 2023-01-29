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
	"math"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/indicators"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
)

var (
	ErrCouldNotRetrieveData = errors.New("could not retrieve data")
)

type AcceleratingDualMomentum struct {
	inTickers    []*data.Security
	prices       *dataframe.DataFrame
	outTickers   []*data.Security
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

	outSecurities := []*data.Security{}
	if err := json.Unmarshal(args["outTickers"], &outSecurities); err != nil {
		return nil, err
	}

	schedule, err := tradecron.New("@monthend 0 16 * *", tradecron.RegularHours)
	if err != nil {
		return nil, err
	}

	var adm strategy.Strategy = &AcceleratingDualMomentum{
		inTickers:  inSecurities,
		outTickers: outSecurities,
		schedule:   schedule,
	}

	return adm, nil
}

// donloadPriceData loads EOD quotes for in tickers
func (adm *AcceleratingDualMomentum) downloadPriceData(ctx context.Context, begin, end time.Time) error {
	tickers := []*data.Security{}
	tickers = append(tickers, adm.inTickers...)
	riskFreeSymbol, err := data.SecurityFromTicker("DGS3MO")
	if err != nil {
		log.Error().Err(err).Msg("could not find the DGS3MO security")
		return err
	}
	tickers = append(tickers, adm.outTickers...)
	tickers = append(tickers, riskFreeSymbol)

	priceMap, err := data.NewDataRequest(tickers...).Metrics(data.MetricAdjustedClose).Between(ctx, begin, end)
	if err != nil {
		return ErrCouldNotRetrieveData
	}

	priceMap = priceMap.Frequency(dataframe.Monthly)
	prices := priceMap.DataFrame()
	prices = prices.Drop(math.NaN())

	// include last day if it is a non-trade day
	log.Debug().Msg("getting last day eod prices of requested range")
	finalPricesMap, err := data.NewDataRequest(tickers...).Metrics(data.MetricAdjustedClose).Between(ctx, end.AddDate(0, 0, -10), end)
	if err != nil {
		log.Error().Err(err).Msg("error getting final prices in adm")
		return err
	}

	finalPrices := finalPricesMap.DataFrame()
	prices.Append(finalPrices.Last())

	for ii := range prices.ColNames {
		prices.ColNames[ii] = strings.Split(prices.ColNames[ii], ":")[0]
	}

	riskFreeRate, eod := prices.Split(riskFreeSymbol.CompositeFigi)

	adm.prices = eod
	adm.riskFreeRate = riskFreeRate

	return nil
}

// Compute signal for strategy and return list of positions along with the next predicted
// set of assets to hold
func (adm *AcceleratingDualMomentum) Compute(ctx context.Context, begin, end time.Time) (data.PortfolioPlan, *data.SecurityAllocation, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "adm.Compute")
	defer span.End()

	// Ensure time range is valid (need at least 6 months)
	nullTime := time.Time{}
	if end.Equal(nullTime) {
		end = time.Now()
	}
	if begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		begin = end.AddDate(-50, 0, 0)
	} else {
		// Set Begin 6 months in the past so we actually get the requested time range
		begin = begin.AddDate(0, -6, 0)
	}

	err := adm.downloadPriceData(ctx, begin, end)
	if err != nil {
		return nil, nil, err
	}

	// Compute momentum scores
	adm.momentum = indicators.Momentum631(adm.prices, adm.riskFreeRate)
	adm.momentum = adm.momentum.Drop(math.NaN())

	// split momentum into inMarket and outOfMarket
	cols := make([]string, len(adm.inTickers))
	for ii := range adm.inTickers {
		cols[ii] = adm.inTickers[ii].CompositeFigi
	}
	inMarket, outOfMarket := adm.momentum.Split(cols...)

	inMarketIdxMax := inMarket.IdxMax()
	outOfMarketIdxMax := outOfMarket.IdxMax()

	// create security map
	inMarketSecurityMap := make([]data.Security, len(inMarket.ColNames))
	for idx, colName := range inMarket.ColNames {
		security, err := data.SecurityFromFigi(colName)
		if err != nil {
			log.Error().Err(err).Str("ColName", colName).Msg("could not lookup security")
			return nil, nil, err
		}
		inMarketSecurityMap[idx] = *security
	}

	outOfMarketSecurityMap := make([]data.Security, len(outOfMarket.ColNames))
	for idx, colName := range outOfMarket.ColNames {
		security, err := data.SecurityFromFigi(colName)
		if err != nil {
			log.Error().Err(err).Str("ColName", colName).Msg("could not lookup security")
			return nil, nil, err
		}
		outOfMarketSecurityMap[idx] = *security
	}

	securityMap := make([]*data.Security, adm.momentum.ColCount())
	for idx, colName := range adm.momentum.ColNames {
		security, err := data.SecurityFromFigi(colName)
		if err != nil {
			log.Error().Err(err).Str("ColName", colName).Msg("could not lookup security")
			return nil, nil, err
		}
		securityMap[idx] = security
	}

	// create investment plan
	targetPortfolio := data.PortfolioPlan{}
	for rowIdx, date := range inMarketIdxMax.Dates {
		inMarketIdx := int(inMarketIdxMax.Vals[0][rowIdx])
		var security data.Security
		if inMarket.Vals[inMarketIdx][rowIdx] < 0.0 {
			// use out-of-market security
			outOfMarketIdx := int(outOfMarketIdxMax.Vals[0][rowIdx])
			security = outOfMarketSecurityMap[outOfMarketIdx]
		} else {
			// use in-market security
			security = inMarketSecurityMap[inMarketIdx]
		}

		justifications := make(map[string]float64, adm.momentum.ColCount())
		for scoreIdx := range adm.momentum.ColNames {
			justifications[securityMap[scoreIdx].Ticker] = adm.momentum.Vals[scoreIdx][rowIdx]
		}

		pie := &data.SecurityAllocation{
			Date: date,
			Members: map[data.Security]float64{
				security: 1.0,
			},
			Justifications: justifications,
		}
		targetPortfolio = append(targetPortfolio, pie)
	}

	// compute the predicted asset
	var predictedPortfolio *data.SecurityAllocation
	if len(targetPortfolio) >= 1 {
		lastPie := targetPortfolio.Last()
		lastTradeDate := lastPie.Date

		isTradeDay := adm.schedule.IsTradeDay(lastTradeDate)
		if !isTradeDay {
			targetPortfolio = targetPortfolio[:len(targetPortfolio)-1]
		}

		nextTradeDate := adm.schedule.Next(lastTradeDate)
		predictedPortfolio = &data.SecurityAllocation{
			Date:           nextTradeDate,
			Members:        lastPie.Members,
			Justifications: lastPie.Justifications,
		}
	}

	return targetPortfolio, predictedPortfolio, nil
}
