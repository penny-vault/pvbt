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

package strategies

import (
	"context"
	"errors"
	"main/data"
	"main/dfextras"
	"main/portfolio"
	"main/util"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

// KellersDefensiveAssetAllocationInfo information describing this strategy
func KellersDefensiveAssetAllocationInfo() StrategyInfo {
	return StrategyInfo{
		Name:        "Kellers Defensive Asset Allocation",
		Shortcode:   "daa",
		Description: `A strategy that has a built-in crash protection that looks at the "breadth-momentum" of a canary universe.`,
		Source:      "https://indexswingtrader.blogspot.com/2018/07/announcing-defensive-asset-allocation.html",
		Version:     "1.0.0",
		Arguments: map[string]Argument{
			"riskUniverse": {
				Name:        "Risk Universe",
				Description: "List of ETF, Mutual Fund, or Stock tickers in the 'risk' universe",
				Typecode:    "[]string",
				DefaultVal:  `["SPY", "IWM", "QQQ", "VGK", "EWJ", "VWO", "VNQ", "GSG", "GLD", "TLT", "HYG", "LQD"]`,
			},
			"protectiveUniverse": {
				Name:        "Protective Universe",
				Description: "List of ETF, Mutual Fund, or Stock tickers in the 'protective' universe to use as canary assets, signaling when to invest in risk vs cash",
				Typecode:    "[]string",
				DefaultVal:  `["VWO", "AGG"]`,
			},
			"cashUniverse": {
				Name:        "Cash Universe",
				Description: "List of ETF, Mutual Fund, or Stock tickers in the 'cash' universe",
				Typecode:    "[]string",
				DefaultVal:  `["SHY", "IEF", "LQD"]`,
			},
			"breadth": {
				Name:        "Breadth",
				Description: "Breadth (B) parameter that determines the cash fraction given the canary breadth",
				Typecode:    "number",
				DefaultVal:  "2",
			},
			"topT": {
				Name:        "Top T",
				Description: "Number of top risk assets to invest in at a time",
				Typecode:    "number",
				DefaultVal:  "6",
			},
		},
		SuggestedParameters: map[string]map[string]string{
			"DAA-G12": {
				"riskUniverse":       `["SPY", "IWM", "QQQ", "VGK", "EWJ", "VWO", "VNQ", "GSG", "GLD", "TLT", "HYG", "LQD"]`,
				"protectiveUniverse": `["VWO", "AGG"]`,
				"cashUniverse":       `["SHY", "IEF", "LQD"]`,
				"breadth":            "2",
				"topT":               "6",
			},
			"DAA-G6": {
				"riskUniverse":       `["SPY", "VEA", "VWO", "LQD", "TLT", "HYG"]`,
				"protectiveUniverse": `["VWO", "AGG"]`,
				"cashUniverse":       `["SHY", "IEF", "LQD"]`,
				"breadth":            "2",
				"topT":               "6",
			},
			"DAA1-G4 - Aggressive G4": {
				"riskUniverse":       `["SPY", "VEA", "VWO", "AGG"]`,
				"protectiveUniverse": `["VWO", "AGG"]`,
				"cashUniverse":       `["SHV", "IEF", "UST"]`,
				"breadth":            "1",
				"topT":               "4",
			},
			"DAA1-G12 - Aggressive G12": {
				"riskUniverse":       `["SPY", "IWM", "QQQ", "VGK", "EWJ", "VWO", "VNQ", "GSG", "GLD", "TLT", "HYG", "LQD"]`,
				"protectiveUniverse": `["VWO", "AGG"]`,
				"cashUniverse":       `["SHV", "IEF", "UST"]`,
				"breadth":            "1",
				"topT":               "2",
			},
			"DAA1-U1 - Minimalistic": {
				"riskUniverse":       `["SPY"]`,
				"protectiveUniverse": `["VWO", "AGG"]`,
				"cashUniverse":       `["SHV", "IEF", "UST"]`,
				"breadth":            "1",
				"topT":               "1",
			},
		},
		Factory: NewKellersDefensiveAssetAllocation,
	}
}

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
	dataStartTime      time.Time
	dataEndTime        time.Time

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

// NewKellersDefensiveAssetAllocation Construct a new Kellers DAA strategy
func NewKellersDefensiveAssetAllocation(args map[string]json.RawMessage) (Strategy, error) {
	cashUniverse := []string{}
	if err := json.Unmarshal(args["cashUniverse"], &cashUniverse); err != nil {
		return nil, err
	}
	util.ArrToUpper(cashUniverse)

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

	var breadth float64
	if err := json.Unmarshal(args["breadth"], &breadth); err != nil {
		return nil, err
	}

	var topT int64
	if err := json.Unmarshal(args["topT"], &topT); err != nil {
		return nil, err
	}

	var daa Strategy = &KellersDefensiveAssetAllocation{
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
		targetMap[cashAsset] = cf
		w := (1.0 - cf) / float64(t)

		for _, asset := range riskAssets {
			if alloc, ok := targetMap[asset]; ok {
				targetMap[asset] = w + alloc
			} else {
				targetMap[asset] = w
			}
		}

		targetAssets[*row] = targetMap
	}

	timeIdx, err := daa.momentum.NameToColumn(data.DateIdx)
	if err != nil {
		log.Error("Time series not set on momentum series")
	}
	timeSeries := daa.momentum.Series[timeIdx]

	targetSeries := dataframe.NewSeriesMixed(portfolio.TickerName, &dataframe.SeriesInit{Size: len(targetAssets)}, targetAssets...)
	daa.targetPortfolio = dataframe.NewDataFrame(timeSeries, targetSeries)
}

func (daa *KellersDefensiveAssetAllocation) downloadPriceData(manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

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

	// Get aligned start and end times
	timeColumn, err := mergedEod.NameToColumn(data.DateIdx, dataframe.Options{})
	if err != nil {
		return err
	}

	timeSeries := mergedEod.Series[timeColumn]
	nrows := timeSeries.NRows(dataframe.Options{})
	startTime := timeSeries.Value(0, dataframe.Options{}).(time.Time)
	endTime := timeSeries.Value(nrows-1, dataframe.Options{}).(time.Time)
	daa.dataStartTime = startTime
	daa.dataEndTime = endTime

	return nil
}

// Compute signal
func (daa *KellersDefensiveAssetAllocation) Compute(manager *data.Manager) (*portfolio.Portfolio, error) {
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
	tickerIdx, _ := daa.targetPortfolio.NameToColumn(portfolio.TickerName)
	lastTarget := daa.targetPortfolio.Series[tickerIdx].Value(daa.targetPortfolio.NRows() - 1).(map[string]float64)
	for kk := range lastTarget {
		symbols = append(symbols, kk)
	}
	sort.Strings(symbols)
	daa.CurrentSymbol = strings.Join(symbols, " ")
	p := portfolio.NewPortfolio("Defensive Asset Allocation Portfolio", manager)
	err = p.TargetPortfolio(10000, daa.targetPortfolio)
	if err != nil {
		return nil, err
	}

	return &p, nil
}
