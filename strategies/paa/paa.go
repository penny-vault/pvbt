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
 * Keller's Protective Asset Allocation v1.0
 * https://indexswingtrader.blogspot.com/2016/04/introducing-protective-asset-allocation.html
 * https://papers.ssrn.com/sol3/papers.cfm?abstract_id=2759734
 */

package paa

import (
	"context"
	"errors"
	"fmt"
	"main/common"
	"main/data"
	"main/dfextras"
	"main/strategies/strategy"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-json"
	log "github.com/sirupsen/logrus"

	"github.com/rocketlaunchr/dataframe-go"
	"github.com/rocketlaunchr/dataframe-go/math/funcs"
)

func min(x int, y int) int {
	if x < y {
		return x
	}

	return y
}

// KellersProtectiveAssetAllocation strategy type
type KellersProtectiveAssetAllocation struct {
	protectiveUniverse []string
	riskUniverse       []string
	allTickers         []string
	protectionFactor   int
	topN               int
	lookback           int
	prices             *dataframe.DataFrame
}

// NewKellersProtectiveAssetAllocation Construct a new Kellers PAA strategy
func New(args map[string]json.RawMessage) (strategy.Strategy, error) {
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

	allTickers := make([]string, 0, len(riskUniverse)+len(protectiveUniverse))
	allTickers = append(allTickers, riskUniverse...)
	allTickers = append(allTickers, protectiveUniverse...)

	var paa strategy.Strategy = &KellersProtectiveAssetAllocation{
		protectiveUniverse: protectiveUniverse,
		riskUniverse:       riskUniverse,
		allTickers:         allTickers,
		protectionFactor:   protectionFactor,
		lookback:           lookback,
		topN:               topN,
	}

	return paa, nil
}

func (paa *KellersProtectiveAssetAllocation) downloadPriceData(manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

	tickers := []string{}
	tickers = append(tickers, paa.protectiveUniverse...)
	tickers = append(tickers, paa.riskUniverse...)

	prices, errs := manager.GetMultipleData(tickers...)

	if len(errs) > 0 {
		return errors.New("failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for _, v := range prices {
		eod = append(eod, v)
	}

	mergedEod, err := dfextras.MergeAndTimeAlign(context.TODO(), data.DateIdx, eod...)
	paa.prices = mergedEod
	if err != nil {
		return err
	}

	return nil
}

// validateTimeRange
func (paa *KellersProtectiveAssetAllocation) validateTimeRange(manager *data.Manager) {
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
}

// mom calculates the momentum based on the sma: MOM(L) = p0/SMA(L) - 1
func (paa *KellersProtectiveAssetAllocation) mom(sma *dataframe.DataFrame) error {
	dontLock := dataframe.Options{DontLock: true}

	for _, ticker := range paa.allTickers {
		name := fmt.Sprintf("%s_MOM", ticker)
		sma.AddSeries(dataframe.NewSeriesFloat64(name, &dataframe.SeriesInit{
			Size: sma.NRows(dontLock),
		}), nil)
		expr := fmt.Sprintf("%s/%s_SMA-1", ticker, ticker)
		fn := funcs.RegFunc(expr)
		err := funcs.Evaluate(context.TODO(), sma, fn, name)
		if err != nil {
			return err
		}
	}
	return nil
}

// rank securities based on their momentum scores
func (paa *KellersProtectiveAssetAllocation) rank(df *dataframe.DataFrame) ([]common.PairList, []string) {
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
		for _, ticker := range paa.riskUniverse {
			momCol := fmt.Sprintf("%s_MOM", ticker)
			floatVal := vals[momCol].(float64)
			if floatVal > 0 {
				sortable = append(sortable, common.Pair{
					Key:   ticker,
					Value: floatVal,
				})
			}
		}

		sort.Sort(sort.Reverse(sortable)) // sort by momentum score

		// limit to topN assest
		riskRanked[*row] = sortable[0:min(len(sortable), paa.topN)]

		// rank each protective asset and select max
		sortable = make(common.PairList, 0, len(paa.protectiveUniverse))
		for _, ticker := range paa.protectiveUniverse {
			momCol := fmt.Sprintf("%s_MOM", ticker)
			sortable = append(sortable, common.Pair{
				Key:   ticker,
				Value: vals[momCol].(float64),
			})
		}

		sort.Sort(sort.Reverse(sortable)) // sort by momentum score
		protectiveSelection[*row] = sortable[0].Key
	}
	df.Unlock()

	return riskRanked, protectiveSelection
}

// buildPortfolio computes the bond fraction at each period and creates a listing of target holdings
func (paa *KellersProtectiveAssetAllocation) buildPortfolio(riskRanked []common.PairList, protectiveSelection []string, mom *dataframe.DataFrame) (*dataframe.DataFrame, error) {
	// N is the number of assets in the risky universe
	N := float64(len(paa.riskUniverse))

	// n1 scales the protective factor by the number of assets in the risky universe
	n1 := float64(paa.protectionFactor) * N / 4.0

	// n is the number of good assets in the risky universe, i.e. number of assets with a positive momentum
	// calculate for every period
	name := "paa_n" // name must be lower-case so it won't conflict with potential tickers
	mom.AddSeries(dataframe.NewSeriesFloat64(name, &dataframe.SeriesInit{
		Size: mom.NRows(),
	}), nil)
	riskUniverseMomNames := make([]string, len(paa.riskUniverse))
	for idx, x := range paa.riskUniverse {
		riskUniverseMomNames[idx] = fmt.Sprintf("%s_MOM", x)
	}
	fn := funcs.RegFunc(fmt.Sprintf("countPositive(%s)", strings.Join(riskUniverseMomNames, ",")))
	err := funcs.Evaluate(context.TODO(), mom, fn, name,
		funcs.EvaluateOptions{
			CustomFns: map[string]func(args ...float64) float64{
				"countPositive": func(args ...float64) float64 {
					var result float64 = 0.0
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

	// bf is the bond fraction that should be used in portfolio construction
	// bf = (N-n) / (N-n1)
	bfCol := "paa_bf" // name must be lower-case so it won't conflict with potential tickers
	mom.AddSeries(dataframe.NewSeriesFloat64(bfCol, &dataframe.SeriesInit{
		Size: mom.NRows(),
	}), nil)
	fn = funcs.RegFunc(fmt.Sprintf("min(1.0, (%f - paa_n) / %f)", N, N-n1))
	err = funcs.Evaluate(context.TODO(), mom, fn, bfCol)
	if err != nil {
		return nil, err
	}

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

		targetMap := make(map[string]float64)

		if riskAssetsEqualWeightPercentage > 1.0e-5 {
			for _, asset := range riskAssets {
				targetMap[asset.Key] = riskAssetsEqualWeightPercentage
			}
		}

		// allocate 100% of bond fraction to protective asset with highest momentum score
		if bf > 0 {
			targetMap[protectiveAsset] = bf
		}

		targetAssets[*row] = targetMap
	}
	mom.Unlock()

	timeIdx, err := mom.NameToColumn(data.DateIdx)
	if err != nil {
		log.Error("Time series not set on momentum series")
	}
	timeSeries := mom.Series[timeIdx].Copy()
	targetSeries := dataframe.NewSeriesMixed(common.TickerName, &dataframe.SeriesInit{Size: len(targetAssets)}, targetAssets...)

	series := make([]dataframe.Series, 0, len(paa.riskUniverse)+len(paa.protectiveUniverse))
	series = append(series, timeSeries)
	series = append(series, targetSeries)

	for _, ticker := range paa.allTickers {
		colIdx, err := mom.NameToColumn(fmt.Sprintf("%s_MOM", ticker))
		if err != nil {
			return nil, err
		}

		col := mom.Series[colIdx]
		col.Lock()
		newCol := col.Copy()
		col.Unlock()
		newCol.Rename(ticker)
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

// Compute signal
func (paa *KellersProtectiveAssetAllocation) Compute(manager *data.Manager) (*dataframe.DataFrame, error) {
	paa.validateTimeRange(manager)

	err := paa.downloadPriceData(manager)
	if err != nil {
		return nil, err
	}

	df, err := dfextras.SMA(paa.lookback-1, paa.prices)
	if err != nil {
		return nil, err
	}

	// offset the *_SMA columns by 1-month
	smaCols := make([]string, 0, len(paa.allTickers))
	for _, ticker := range paa.allTickers {
		smaCols = append(smaCols, fmt.Sprintf("%s_SMA", ticker))
	}

	df = dfextras.Lag(1, df, smaCols...)
	dfextras.DropNA(context.TODO(), df, dataframe.FilterOptions{
		InPlace: true,
	})

	if err := paa.mom(df); err != nil {
		return nil, err
	}

	riskRanked, protectiveSelection := paa.rank(df)

	targetPortfolio, err := paa.buildPortfolio(riskRanked, protectiveSelection, df)
	if err != nil {
		return nil, err
	}

	return targetPortfolio, nil
}
