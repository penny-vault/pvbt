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
 * Keller's Protective Asset Allocation v1.0
 * https://indexswingtrader.blogspot.com/2016/04/introducing-protective-asset-allocation.html
 * https://papers.ssrn.com/sol3/papers.cfm?abstract_id=2759734
 */

package paa

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	dataframe "github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
)

var (
	ErrDataRetrievalFailed = errors.New("failed to retrieve data for tickers")
)

func min(x int, y int) int {
	if x < y {
		return x
	}

	return y
}

// KellersProtectiveAssetAllocation strategy type
type KellersProtectiveAssetAllocation struct {
	protectiveUniverse []*data.Security
	riskUniverse       []*data.Security
	allTickers         []*data.Security
	protectionFactor   int
	topN               int
	lookback           int
	prices             *dataframe.DataFrame
	momentum           *dataframe.DataFrame
	schedule           *tradecron.TradeCron
}

// NewKellersProtectiveAssetAllocation Construct a new Kellers PAA strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
	protectiveUniverse := []*data.Security{}
	if err := json.Unmarshal(args["protectiveUniverse"], &protectiveUniverse); err != nil {
		return nil, err
	}

	riskUniverse := []*data.Security{}
	if err := json.Unmarshal(args["riskUniverse"], &riskUniverse); err != nil {
		return nil, err
	}

	var protectionFactor int
	if err := json.Unmarshal(args["protectionFactor"], &protectionFactor); err != nil {
		return nil, err
	}

	var lookback int
	if err := json.Unmarshal(args["lookback"], &lookback); err != nil {
		return nil, err
	}

	var topN int
	if err := json.Unmarshal(args["topN"], &topN); err != nil {
		return nil, err
	}

	allTickers := make([]*data.Security, 0, len(riskUniverse)+len(protectiveUniverse))
	allTickers = append(allTickers, riskUniverse...)
	allTickers = append(allTickers, protectiveUniverse...)

	schedule, err := tradecron.New("@monthend 0 16 * *", tradecron.RegularHours)
	if err != nil {
		return nil, err
	}

	var paa strategy.Strategy = &KellersProtectiveAssetAllocation{
		protectiveUniverse: protectiveUniverse,
		riskUniverse:       riskUniverse,
		allTickers:         allTickers,
		protectionFactor:   protectionFactor,
		lookback:           lookback,
		topN:               topN,
		schedule:           schedule,
	}

	return paa, nil
}

func (paa *KellersProtectiveAssetAllocation) downloadPriceData(ctx context.Context, begin, end time.Time) error {
	// Load EOD quotes for in tickers

	securities := []*data.Security{}
	securities = append(securities, paa.protectiveUniverse...)
	securities = append(securities, paa.riskUniverse...)

	priceMap, err := data.NewDataRequest(securities...).Metrics(data.MetricAdjustedClose).Between(ctx, begin, end)
	if err != nil {
		return ErrDataRetrievalFailed
	}

	priceMap = priceMap.Frequency(dataframe.Monthly)
	priceMap = priceMap.Drop(math.NaN())
	paa.prices = priceMap.DataFrame()

	// include last day if it is a non-trade day
	log.Debug().Msg("getting last day eod prices of requested range")
	finalPriceMap, err := data.NewDataRequest(securities...).Metrics(data.MetricAdjustedClose).Between(ctx, end.AddDate(0, 0, -10), end)
	if err != nil {
		log.Error().Err(err).Msg("error getting final eod prices")
		return err
	}

	finalPriceMap = finalPriceMap.Drop(math.NaN())
	finalPrices := finalPriceMap.DataFrame()
	paa.prices.Append(finalPrices.Last())

	// Rename columns to composite figi only -- this is to promote readability when debugging
	for ii := range paa.prices.ColNames {
		paa.prices.ColNames[ii] = strings.Split(paa.prices.ColNames[ii], ":")[0]
	}

	return nil
}

// validateTimeRange
func (paa *KellersProtectiveAssetAllocation) validateTimeRange(begin, end time.Time) (time.Time, time.Time) {
	// Ensure time range is valid (need at least 12 months)
	nullTime := time.Time{}
	if end.Equal(nullTime) {
		end = time.Now()
	}
	if begin.Equal(nullTime) {
		// Default computes things 50 years into the past
		begin = end.AddDate(-50, 0, 0)
	} else {
		// Set Begin 12 months in the past so we actually get the requested time range
		begin = begin.AddDate(0, -12, 0)
	}

	return begin, end
}

// mom calculates the momentum based on the sma: MOM(L) = p0/SMA(L) - 1, where L = 13
func (paa *KellersProtectiveAssetAllocation) mom() {
	sma := paa.prices.SMA(paa.lookback + 1)
	paa.momentum = paa.prices.Div(sma).AddScalar(-1).MulScalar(100).Drop(math.NaN())
}

// rank securities based on their momentum scores
func (paa *KellersProtectiveAssetAllocation) rank() ([]common.PairList, []string) {
	df := paa.momentum

	riskRanked := make([]common.PairList, df.Len())
	protectiveSelection := make([]string, df.Len())

	df.ForEach(func(rowIdx int, date time.Time, vals map[string]float64) map[string]float64 {
		// rank each risky asset if it's momentum is greater than 0
		sortable := make(common.PairList, 0, len(paa.riskUniverse))
		for _, security := range paa.riskUniverse {
			floatVal := vals[security.CompositeFigi]
			if floatVal > 0 {
				sortable = append(sortable, common.Pair{
					Key:   security.CompositeFigi,
					Value: floatVal,
				})
			}
		}

		sort.Sort(sort.Reverse(sortable)) // sort by momentum score

		// limit to topN assest
		riskRanked[rowIdx] = sortable[0:min(len(sortable), paa.topN)]

		// rank each protective asset and select max
		sortable = make(common.PairList, 0, len(paa.protectiveUniverse))
		for _, security := range paa.protectiveUniverse {
			sortable = append(sortable, common.Pair{
				Key:   security.CompositeFigi,
				Value: vals[security.CompositeFigi],
			})
		}

		sort.Sort(sort.Reverse(sortable)) // sort by momentum score
		protectiveSelection[rowIdx] = sortable[0].Key
		return nil
	})

	return riskRanked, protectiveSelection
}

// buildPortfolio computes the bond fraction at each period and creates a listing of target holdings
func (paa *KellersProtectiveAssetAllocation) buildPortfolio(riskRanked []common.PairList, protectiveSelection []string) data.PortfolioPlan {
	plan := make(data.PortfolioPlan, len(protectiveSelection))

	// N is the number of assets in the risky universe
	N := float64(len(paa.riskUniverse))

	// n1 scales the protective factor by the number of assets in the risky universe
	n1 := float64(paa.protectionFactor) * N / 4.0

	// n is the number of good assets in the risky universe, i.e. number of assets with a positive momentum
	// calculate for every period
	riskUniverseFigis := make([]string, len(paa.riskUniverse))
	for idx, security := range paa.riskUniverse {
		riskUniverseFigis[idx] = security.CompositeFigi
	}
	riskUniverseMom, _ := paa.momentum.Split(riskUniverseFigis...)

	n := riskUniverseMom.Count(func(x float64) bool { return x > 0 })
	n.ColNames[0] = "n"

	// bf is the bond fraction that should be used in portfolio construction
	// bf = (N-n) / (N-n1)
	bondFraction := make([]float64, n.Len())
	n.ForEach(func(rowIdx int, date time.Time, vals map[string]float64) map[string]float64 {
		bondFraction[rowIdx] = math.Min(1.0, (N-vals["n"])/(N-n1))
		return nil
	})

	// now actually build the target portfolio which is a dataframe
	paa.momentum.ForEach(func(rowIdx int, date time.Time, vals map[string]float64) map[string]float64 {
		bf := bondFraction[rowIdx]
		sf := 1.0 - bf

		allocation := &data.SecurityAllocation{
			Date:           date,
			Members:        make(map[data.Security]float64),
			Justifications: make(map[string]float64, len(vals)),
		}

		for k, v := range vals {
			if security, err := data.SecurityFromFigi(k); err == nil {
				allocation.Justifications[security.Ticker] = v
			} else {
				log.Error().Err(err).Str("compositeFigi", k).Msg("could not look up security")
			}
		}

		allocation.Justifications["# Good"] = n.Vals[0][rowIdx]
		allocation.Justifications["Bond Fraction"] = bf

		riskAssets := riskRanked[rowIdx]
		protectiveAsset := protectiveSelection[rowIdx]

		// equal weight risk assets
		numRiskAssetsToHold := min(paa.topN, len(riskAssets))
		riskAssetsEqualWeightPercentage := sf / float64(numRiskAssetsToHold)

		if riskAssetsEqualWeightPercentage > 1.0e-3 {
			for _, asset := range riskAssets {
				security, err := data.SecurityFromFigi(asset.Key)
				if err != nil {
					log.Error().Err(err).Str("CompositeFigi", asset.Key).Msg("security not found")
					return nil
				}
				allocation.Members[*security] = riskAssetsEqualWeightPercentage
			}
		}

		// allocate 100% of bond fraction to protective asset with highest momentum score
		if bf > 0 {
			security, err := data.SecurityFromFigi(protectiveAsset)
			if err != nil {
				log.Error().Err(err).Str("CompositeFigi", protectiveAsset).Msg("security not found")
				return nil
			}
			allocation.Members[*security] = bf
		}

		plan[rowIdx] = allocation
		return nil
	})

	return plan
}

func (paa *KellersProtectiveAssetAllocation) calculatePredictedPortfolio(plan data.PortfolioPlan) (data.PortfolioPlan, *data.SecurityAllocation) {
	var predictedPortfolio *data.SecurityAllocation
	if len(plan) >= 2 {
		lastRow := plan.Last()

		lastTradeDate := lastRow.Date
		isTradeDay := paa.schedule.IsTradeDay(lastTradeDate)
		if !isTradeDay {
			plan = plan[:len(plan)-1]
		}

		nextTradeDate := paa.schedule.Next(lastTradeDate)
		predictedPortfolio = &data.SecurityAllocation{
			Date:           nextTradeDate,
			Members:        lastRow.Members,
			Justifications: lastRow.Justifications,
		}
	}

	return plan, predictedPortfolio
}

// Compute signal
func (paa *KellersProtectiveAssetAllocation) Compute(ctx context.Context, begin, end time.Time) (data.PortfolioPlan, *data.SecurityAllocation, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "paa.Compute")
	defer span.End()

	begin, end = paa.validateTimeRange(begin, end)

	if err := paa.downloadPriceData(ctx, begin, end); err != nil {
		return nil, nil, err
	}

	// calculate momentum
	paa.mom()

	riskRanked, protectiveSelection := paa.rank()

	targetPortfolio := paa.buildPortfolio(riskRanked, protectiveSelection)
	targetPortfolio, predictedPortfolio := paa.calculatePredictedPortfolio(targetPortfolio)

	return targetPortfolio, predictedPortfolio, nil
}
