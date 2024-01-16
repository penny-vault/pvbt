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

package ncave

import (
	"context"
	"errors"
	"time"

	"github.com/goccy/go-json"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
)

var (
	ErrCouldNotRetrieveData = errors.New("could not retrieve data")
)

type NetCurrentAssetValue struct {
	schedule  *tradecron.TradeCron
	threshold float32
}

// New Construct a new Accelerating Dual Momentum strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	schedule, err := tradecron.New("@monthbegin 0 10 * 6", tradecron.RegularHours)
	if err != nil {
		return nil, err
	}

	var threshold float32
	if err := json.Unmarshal(args["threshold"], &threshold); err != nil {
		return nil, err
	}

	var ncave strategy.Strategy = &NetCurrentAssetValue{
		schedule:  schedule,
		threshold: threshold,
	}

	return ncave, nil
}

func equitiesThatMeetNCAVCriteria(ctx context.Context, dt time.Time) (*data.SecurityAllocation, error) {
	// get the 3000 most liquid securities
	securities, err := data.NewUSTradeableEquities().Securities(ctx, dt)
	if err != nil {
		log.Error().Err(err).Msg("could not get tradeable equities")
		return nil, err
	}

	tickers := make([]string, len(securities))
	for idx, sec := range securities {
		tickers[idx] = sec.Ticker
	}

	log.Info().Strs("Tickers", tickers).Time("ForDate", dt).Msg("getting ncav for specified date")

	// get fundamental data
	//dfMap, err := data.NewDataRequest(securities...).Metrics(data.FundamentalMarketCap, data.FundamentalWorkingCapital).On(dt)

	// df.divide(Fundamentals.WorkingCapital, Fundamentals.MarketCap, "NCAV")
	// df.Threshold(ncavThreshold)

	// Other options include:
	// -> SP500Universe
	// -> Russell1000Universe

	return nil, nil
}

// Compute signal for strategy and return list of positions along with the next predicted
// set of assets to hold
func (ncave *NetCurrentAssetValue) Compute(ctx context.Context, begin, end time.Time) (data.PortfolioPlan, *data.SecurityAllocation, error) {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "adm.Compute")
	defer span.End()

	targetPortfolio := data.PortfolioPlan{}

	// Iterate over every investment period
	currDate := ncave.schedule.Next(begin)
	for currDate.Before(end) || currDate.Equal(end) {
		log.Debug().Time("InvestmentDate", currDate).Msg("calculating holdings")
		securityAllocation, err := equitiesThatMeetNCAVCriteria(ctx, currDate)
		if err != nil {
			log.Error().Err(err).Time("forDate", currDate).Msg("could not calculate NCAV/mv criteria for specified date")
		}
		targetPortfolio = append(targetPortfolio, securityAllocation)
		currDate = ncave.schedule.Next(currDate.Add(time.Second))
	}

	// compute the predicted asset
	predictedPortfolio, err := equitiesThatMeetNCAVCriteria(ctx, end)
	if err != nil {
		log.Error().Err(err).Time("forDate", currDate).Msg("could not calculate NCAV/mv criteria for specified date")
	}

	return targetPortfolio, predictedPortfolio, nil
}
