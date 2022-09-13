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
 * Keller's Protective Asset Allocation v1.0
 * https://indexswingtrader.blogspot.com/2016/04/introducing-protective-asset-allocation.html
 * https://papers.ssrn.com/sol3/papers.cfm?abstract_id=2759734
 */

package paa

import (
	"context"
	"errors"
	"fmt"
	"sort"
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

func (paa *KellersProtectiveAssetAllocation) downloadPriceData(ctx context.Context, manager *data.ManagerV0) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

	securities := []*data.Security{}
	securities = append(securities, paa.protectiveUniverse...)
	securities = append(securities, paa.riskUniverse...)

	prices, errs := manager.GetDataFrame(ctx, data.MetricAdjustedClose, securities...)
	if errs != nil {
		return ErrDataRetrievalFailed
	}

	prices, err := dataframe.DropNA(ctx, prices)
	if err != nil {
		return err
	}
	paa.prices = prices
	if err != nil {
		return err
	}

	// include last day if it is a non-trade day
	log.Debug().Msg("getting last day eod prices of requested range")
	manager.Frequency = data.FrequencyDaily
	begin := manager.Begin
	manager.Begin = manager.End.AddDate(0, 0, -10)
	final, err := manager.GetDataFrame(ctx, data.MetricAdjustedClose, securities...)
	if err != nil {
		log.Error().Err(err).Msg("error getting final")
		return err
	}
	manager.Begin = begin
	manager.Frequency = data.FrequencyMonthly

	final, err = dataframe.DropNA(ctx, final)
	if err != nil {
		return err
	}

	nrows := final.NRows()
	row := final.Row(nrows-1, false, dataframe.SeriesName)
	dt := row[common.DateIdx].(time.Time)
	lastRow := paa.prices.Row(paa.prices.NRows()-1, false, dataframe.SeriesName)
	lastDt := lastRow[common.DateIdx].(time.Time)
	if !dt.Equal(lastDt) {
		paa.prices.Append(nil, row)
	}

	return nil
}

// validateTimeRange
func (paa *KellersProtectiveAssetAllocation) validateTimeRange(manager *data.ManagerV0) {
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
}

// mom calculates the momentum based on the sma: MOM(L) = p0/SMA(L) - 1
func (paa *KellersProtectiveAssetAllocation) mom(ctx context.Context) error {
	dontLock := dataframe.Options{DontLock: true}

	sma, err := dataframe.SMA(paa.lookback+1, paa.prices)
	if err != nil {
		return err
	}

	/*
		// calculate momentum 13, mom13 = (p0 / SMA13) - 1
		for _, security := range paa.allTickers {
			name := fmt.Sprintf("%s_MOM", security.CompositeFigi)
			if err := sma.AddSeries(dataframe.NewSeriesFloat64(name, &dataframe.SeriesInit{
				Size: sma.NRows(dontLock),
			}), nil); err != nil {
				log.Error().Stack().Err(err).Msg("could not add series")
				return err
			}
			expr := fmt.Sprintf("(%s/%s_SMA-1)*100", security.CompositeFigi, security.CompositeFigi)
			fn := funcs.RegFunc(expr)
			err := funcs.Evaluate(ctx, sma, fn, name)
			if err != nil {
				return err
			}
		}
	*/

	series := make([]dataframe.Series, 0, len(paa.allTickers)+1)
	dateIdx := sma.MustNameToColumn(common.DateIdx)
	series = append(series, sma.Series[dateIdx].Copy())
	for _, security := range paa.allTickers {
		name := fmt.Sprintf("%s_MOM", security.CompositeFigi)
		idx := sma.MustNameToColumn(name)
		s := sma.Series[idx].Copy()
		s.Rename(security.CompositeFigi)
		series = append(series, s)
	}
	paa.momentum = dataframe.NewDataFrame(series...)

	return nil
}

// rank securities based on their momentum scores
func (paa *KellersProtectiveAssetAllocation) rank() ([]common.PairList, []string) {
	df := paa.momentum
	iterator := df.ValuesIterator(dataframe.ValuesOptions{
		InitialRow:   0,
		Step:         1,
		DontReadLock: true,
	})

	riskRanked := make([]common.PairList, df.NRows())
	protectiveSelection := make([]string, df.NRows())

	df.Lock()
	for {
		row, vals, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		// rank each risky asset if it's momentum is greater than 0
		sortable := make(common.PairList, 0, len(paa.riskUniverse))
		for _, security := range paa.riskUniverse {
			floatVal := vals[security.CompositeFigi].(float64)
			if floatVal > 0 {
				sortable = append(sortable, common.Pair{
					Key:   security.CompositeFigi,
					Value: floatVal,
				})
			}
		}

		sort.Sort(sort.Reverse(sortable)) // sort by momentum score

		// limit to topN assest
		riskRanked[*row] = sortable[0:min(len(sortable), paa.topN)]

		// rank each protective asset and select max
		sortable = make(common.PairList, 0, len(paa.protectiveUniverse))
		for _, security := range paa.protectiveUniverse {
			sortable = append(sortable, common.Pair{
				Key:   security.CompositeFigi,
				Value: vals[security.CompositeFigi].(float64),
			})
		}

		sort.Sort(sort.Reverse(sortable)) // sort by momentum score
		protectiveSelection[*row] = sortable[0].Key
	}
	df.Unlock()

	return riskRanked, protectiveSelection
}

// buildPortfolio computes the bond fraction at each period and creates a listing of target holdings
func (paa *KellersProtectiveAssetAllocation) buildPortfolio(ctx context.Context, riskRanked []common.PairList, protectiveSelection []string) (*dataframe.DataFrame, error) {
	// N is the number of assets in the risky universe
	N := float64(len(paa.riskUniverse))

	// n1 scales the protective factor by the number of assets in the risky universe
	n1 := float64(paa.protectionFactor) * N / 4.0

	// n is the number of good assets in the risky universe, i.e. number of assets with a positive momentum
	// calculate for every period
	mom := paa.momentum
	name := "paa_n" // name must be lower-case so it won't conflict with potential tickers
	if err := mom.AddSeries(dataframe.NewSeriesFloat64(name, &dataframe.SeriesInit{
		Size: mom.NRows(),
	}), nil); err != nil {
		log.Error().Stack().Err(err).Msg("could not add series to momentum dataframe")
	}

	riskUniverseMomNames := make([]string, len(paa.riskUniverse))
	for idx, security := range paa.riskUniverse {
		riskUniverseMomNames[idx] = security.CompositeFigi
	}

	/*
		fn := funcs.RegFunc(fmt.Sprintf("countPositive(%s)", strings.Join(riskUniverseMomNames, ",")))
		err := funcs.Evaluate(ctx, mom, fn, name,
			funcs.EvaluateOptions{
				CustomFns: map[string]func(args ...float64) float64{
					"countPositive": func(args ...float64) float64 {
						var result float64
						for _, x := range args {
							if x > 0 {
								result += 1.0
							}
						}
						return result
					},
				},
			},
		)
		if err != nil {
			return nil, err
		}
	*/

	// bf is the bond fraction that should be used in portfolio construction
	// bf = (N-n) / (N-n1)
	bfCol := "paa_bf" // name must be lower-case so it won't conflict with potential tickers
	if err := mom.AddSeries(dataframe.NewSeriesFloat64(bfCol, &dataframe.SeriesInit{
		Size: mom.NRows(),
	}), nil); err != nil {
		log.Error().Stack().Err(err).Msg("could not add series to dataframe")
	}
	/*
		fn = funcs.RegFunc(fmt.Sprintf("min(1.0, (%f - paa_n) / %f)", N, N-n1))
		err = funcs.Evaluate(ctx, mom, fn, bfCol)
		if err != nil {
			return nil, err
		}
	*/

	// initialize the target portfolio
	targetAssets := make([]interface{}, mom.NRows())

	// now actually build the target portfolio which is a dataframe
	iterator := mom.ValuesIterator(dataframe.ValuesOptions{
		InitialRow:   0,
		Step:         1,
		DontReadLock: true,
	})

	mom.Lock()
	for {
		row, vals, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		bf := vals[bfCol].(float64)
		sf := 1.0 - bf

		riskAssets := riskRanked[*row]
		protectiveAsset := protectiveSelection[*row]

		// equal weight risk assets
		numRiskAssetsToHold := min(paa.topN, len(riskAssets))
		riskAssetsEqualWeightPercentage := sf / float64(numRiskAssetsToHold)

		targetMap := make(map[data.Security]float64)

		if riskAssetsEqualWeightPercentage > 1.0e-5 {
			for _, asset := range riskAssets {
				security, err := data.SecurityFromFigi(asset.Key)
				if err != nil {
					log.Error().Err(err).Str("CompositeFigi", asset.Key).Msg("security not found")
					return nil, err
				}
				targetMap[*security] = riskAssetsEqualWeightPercentage
			}
		}

		// allocate 100% of bond fraction to protective asset with highest momentum score
		if bf > 0 {
			security, err := data.SecurityFromFigi(protectiveAsset)
			if err != nil {
				log.Error().Err(err).Str("CompositeFigi", protectiveAsset).Msg("security not found")
				return nil, err
			}
			targetMap[*security] = bf
		}

		targetAssets[*row] = targetMap
	}
	mom.Unlock()

	timeIdx, err := mom.NameToColumn(common.DateIdx)
	if err != nil {
		log.Error().Stack().Err(err).Msg("time series not set on momentum series")
	}
	timeSeries := mom.Series[timeIdx].Copy()
	targetSeries := dataframe.NewSeriesMixed(common.TickerName, &dataframe.SeriesInit{Size: len(targetAssets)}, targetAssets...)

	series := make([]dataframe.Series, 0, len(paa.riskUniverse)+len(paa.protectiveUniverse))
	series = append(series, timeSeries)
	series = append(series, targetSeries)

	for _, security := range paa.allTickers {
		colIdx := mom.MustNameToColumn(security.CompositeFigi)
		col := mom.Series[colIdx]
		col.Lock()
		newCol := col.Copy()
		col.Unlock()
		newCol.Rename(security.Ticker)
		series = append(series, newCol)
	}

	// add # good assets
	colIdx, err := mom.NameToColumn("paa_n")
	if err != nil {
		return nil, err
	}
	col := mom.Series[colIdx]
	col.Lock()
	newCol := col.Copy()
	col.Unlock()
	newCol.Rename("# Good")
	series = append(series, newCol)

	// add bond fraction
	colIdx, err = mom.NameToColumn("paa_bf")
	if err != nil {
		return nil, err
	}
	col = mom.Series[colIdx]
	col.Lock()
	newCol = col.Copy()
	col.Unlock()
	newCol.Rename("Bond Fraction")
	series = append(series, newCol)

	targetPortfolio := dataframe.NewDataFrame(series...)

	return targetPortfolio, nil
}

func (paa *KellersProtectiveAssetAllocation) calculatePredictedPortfolio(targetPortfolio *dataframe.DataFrame) *strategy.Prediction {
	var predictedPortfolio *strategy.Prediction
	if targetPortfolio.NRows() >= 2 {
		lastRow := targetPortfolio.Row(targetPortfolio.NRows()-1, true, dataframe.SeriesName)
		predictedJustification := make(map[string]float64, len(lastRow)-1)
		for k, v := range lastRow {
			if k != common.TickerName && k != common.DateIdx {
				predictedJustification[k.(string)] = v.(float64)
			}
		}

		lastTradeDate := lastRow[common.DateIdx].(time.Time)
		isTradeDay := paa.schedule.IsTradeDay(lastTradeDate)
		if !isTradeDay {
			targetPortfolio.Remove(targetPortfolio.NRows() - 1)
		}

		nextTradeDate := paa.schedule.Next(lastTradeDate)
		predictedPortfolio = &strategy.Prediction{
			TradeDate:     nextTradeDate,
			Target:        lastRow[common.TickerName].(map[data.Security]float64),
			Justification: predictedJustification,
		}
	}

	return predictedPortfolio
}

// Compute signal
func (paa *KellersProtectiveAssetAllocation) Compute(ctx context.Context, manager *data.ManagerV0) (*dataframe.DataFrame, *strategy.Prediction, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "paa.Compute")
	defer span.End()

	paa.validateTimeRange(manager)

	err := paa.downloadPriceData(ctx, manager)
	if err != nil {
		return nil, nil, err
	}

	if err := paa.mom(ctx); err != nil {
		return nil, nil, err
	}

	riskRanked, protectiveSelection := paa.rank()

	targetPortfolio, err := paa.buildPortfolio(ctx, riskRanked, protectiveSelection)
	if err != nil {
		return nil, nil, err
	}

	predictedPortfolio := paa.calculatePredictedPortfolio(targetPortfolio)

	return targetPortfolio, predictedPortfolio, nil
}
