/*
 * Keller's Protective Asset Allocation v1.0
 * https://indexswingtrader.blogspot.com/2016/04/introducing-protective-asset-allocation.html
 * https://papers.ssrn.com/sol3/papers.cfm?abstract_id=2759734
 */

package strategies

import (
	"context"
	"errors"
	"fmt"
	"main/data"
	"main/dfextras"
	"main/portfolio"
	"main/util"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-json"
	log "github.com/sirupsen/logrus"

	"github.com/rocketlaunchr/dataframe-go"
	"github.com/rocketlaunchr/dataframe-go/math/funcs"
)

func max(x int, y int) int {
	if x < y {
		return y
	}

	return x
}

// KellersProtectiveAssetAllocationInfo information describing this strategy
func KellersProtectiveAssetAllocationInfo() StrategyInfo {
	return StrategyInfo{
		Name:        "Kellers Protective Asset Allocation",
		Shortcode:   "paa",
		Description: `A simple dual-momentum model (called Protective Asset Allocation or PAA) with a vigorous “crash protection” which might fit this bill. It is a tactical variation on the traditional 60/40 stock/bond portfolio where the optimal stock/bond mix is determined by multi-market breadth using dual momentum.`,
		Source:      "https://indexswingtrader.blogspot.com/2016/04/introducing-protective-asset-allocation.html",
		Version:     "1.0.0",
		Arguments: map[string]Argument{
			"riskUniverse": {
				Name:        "Risk Universe",
				Description: "List of ETF, Mutual Fund, or Stock tickers in the 'risk' universe",
				Typecode:    "[]string",
				DefaultVal:  `["SPY", "QQQ", "IWM", "VGK", "EWJ", "EEM", "IYR", "GSG", "GLD", "HYG", "LQD", "TLT"]`,
			},
			"protectiveUniverse": {
				Name:        "Protective Universe",
				Description: "List of ETF, Mutual Fund, or Stock tickers in the 'protective' universe to use as canary assets, signaling when to invest in risk vs cash",
				Typecode:    "[]string",
				DefaultVal:  `["IEF"]`,
			},
			"protectionFactor": {
				Name:        "Protection Factor",
				Description: "Factor describing how protective the crash protection should be; higher numbers are more protective.",
				Typecode:    "number",
				Advanced:    true,
				DefaultVal:  "2",
			},
			"lookback": {
				Name:        "Lookback",
				Description: "Number of months to lookback in momentum filter.",
				Typecode:    "number",
				Advanced:    true,
				DefaultVal:  "12",
			},
			"topN": {
				Name:        "Top N",
				Description: "Number of top risk assets to invest in at a time",
				Typecode:    "number",
				Advanced:    true,
				DefaultVal:  "6",
			},
		},
		SuggestedParameters: map[string]map[string]string{
			"PAA-Conservative": {
				"riskUniverse":       `["SPY", "QQQ", "IWM", "VGK", "EWJ", "EEM", "IYR", "GSG", "GLD", "HYG", "LQD", "TLT"]`,
				"protectiveUniverse": `["$CASH"]`,
				"protectiveFactor":   "2",
				"lookback":           "12",
				"topN":               "6",
			},
			"PAA0": {
				"riskUniverse":       `["SPY", "QQQ", "IWM", "VGK", "EWJ", "EEM", "IYR", "GSG", "GLD", "HYG", "LQD", "TLT"]`,
				"protectiveUniverse": `["IEF"]`,
				"lookback":           "12",
				"topN":               "6",
			},
		},
		Factory: NewKellersProtectiveAssetAllocation,
	}
}

// KellersProtectiveAssetAllocation strategy type
type KellersProtectiveAssetAllocation struct {
	protectiveUniverse []string
	riskUniverse       []string
	protectionFactor   int
	topN               int
	lookback           int
	prices             *dataframe.DataFrame
	dataStartTime      time.Time
	dataEndTime        time.Time

	// Public
	CurrentSymbol string
}

// NewKellersProtectiveAssetAllocation Construct a new Kellers PAA strategy
func NewKellersProtectiveAssetAllocation(args map[string]json.RawMessage) (Strategy, error) {
	protectiveUniverse := []string{}
	if err := json.Unmarshal(args["protectiveUniverse"], &protectiveUniverse); err != nil {
		return nil, err
	}
	util.ArrToUpper(protectiveUniverse)

	riskUniverse := []string{}
	if err := json.Unmarshal(args["riskUniverse"], &riskUniverse); err != nil {
		return nil, err
	}
	util.ArrToUpper(riskUniverse)

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

	var paa Strategy = &KellersProtectiveAssetAllocation{
		protectiveUniverse: protectiveUniverse,
		riskUniverse:       riskUniverse,
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

	// Get aligned start and end times
	timeColumn, err := mergedEod.NameToColumn(data.DateIdx, dataframe.Options{})
	if err != nil {
		return err
	}

	timeSeries := mergedEod.Series[timeColumn]
	nrows := timeSeries.NRows(dataframe.Options{})
	startTime := timeSeries.Value(0, dataframe.Options{}).(time.Time)
	endTime := timeSeries.Value(nrows-1, dataframe.Options{}).(time.Time)
	paa.dataStartTime = startTime
	paa.dataEndTime = endTime

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

	allTickers := make([]string, 0, len(paa.riskUniverse)+len(paa.protectiveUniverse))
	allTickers = append(allTickers, paa.riskUniverse...)
	allTickers = append(allTickers, paa.protectiveUniverse...)

	for _, ticker := range allTickers {
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
func (paa *KellersProtectiveAssetAllocation) rank(df *dataframe.DataFrame) ([]util.PairList, []string) {
	iterator := df.ValuesIterator(dataframe.ValuesOptions{
		InitialRow:   0,
		Step:         1,
		DontReadLock: true,
	})

	riskRanked := make([]util.PairList, df.NRows())
	protectiveSelection := make([]string, df.NRows())

	df.Lock()
	for {
		row, vals, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		// rank each risky asset if it's momentum is greater than 0
		sortable := make(util.PairList, 0, len(paa.riskUniverse))
		for _, ticker := range paa.riskUniverse {
			momCol := fmt.Sprintf("%s_MOM", ticker)
			floatVal := vals[momCol].(float64)
			if floatVal > 0 {
				sortable = append(sortable, util.Pair{
					Key:   ticker,
					Value: floatVal,
				})
			}
		}

		sort.Sort(sortable) // sort by momentum score
		riskRanked[*row] = sortable

		// rank each protective asset and select max
		sortable = make(util.PairList, 0, len(paa.protectiveUniverse))
		for _, ticker := range paa.protectiveUniverse {
			momCol := fmt.Sprintf("%s_MOM", ticker)
			sortable = append(sortable, util.Pair{
				Key:   ticker,
				Value: vals[momCol].(float64),
			})
		}

		sort.Sort(sortable) // sort by momentum score
		protectiveSelection[*row] = sortable[0].Key
	}
	df.Unlock()

	return riskRanked, protectiveSelection
}

// buildPortfolio computes the bond fraction at each period and creates a listing of target holdings
func (paa *KellersProtectiveAssetAllocation) buildPortfolio(riskRanked []util.PairList, protectiveSelection []string, mom *dataframe.DataFrame) (*dataframe.DataFrame, error) {
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
		numRiskAssetsToHold := max(paa.topN, len(riskAssets))
		riskAssetsEqualWeightPercentage := sf / float64(numRiskAssetsToHold)

		targetMap := make(map[string]float64)

		for _, asset := range riskAssets {
			targetMap[asset.Key] = riskAssetsEqualWeightPercentage
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
	targetSeries := dataframe.NewSeriesMixed(portfolio.TickerName, &dataframe.SeriesInit{Size: len(targetAssets)}, targetAssets...)
	targetPortfolio := dataframe.NewDataFrame(timeSeries, targetSeries)

	return targetPortfolio, nil
}

// Compute signal
func (paa *KellersProtectiveAssetAllocation) Compute(manager *data.Manager) (*portfolio.Portfolio, error) {
	paa.validateTimeRange(manager)

	err := paa.downloadPriceData(manager)
	if err != nil {
		return nil, err
	}

	df, err := dfextras.SMA(paa.lookback, paa.prices)
	if err != nil {
		return nil, err
	}

	if err := paa.mom(df); err != nil {
		return nil, err
	}

	riskRanked, protectiveSelection := paa.rank(df)

	targetPortfolio, err := paa.buildPortfolio(riskRanked, protectiveSelection, df)
	if err != nil {
		return nil, err
	}

	p := portfolio.NewPortfolio("Protective Asset Allocation Portfolio", manager)
	err = p.TargetPortfolio(10000, targetPortfolio)
	if err != nil {
		return nil, err
	}

	row := targetPortfolio.Row(targetPortfolio.NRows()-1, true, dataframe.SeriesName)
	rowMap := row[portfolio.TickerName].(map[string]float64)
	assets := make([]string, 0, len(rowMap))
	for k := range rowMap {
		assets = append(assets, k)
	}
	paa.CurrentSymbol = strings.Join(assets, " ")

	return &p, nil
}
