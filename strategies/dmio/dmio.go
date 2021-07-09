/*
 * Dual Momentum In-Out
 * https://www.quantconnect.com/forum/discussion/9597/the-in-amp-out-strategy-continued-from-quantopian/p3/comment-28146
 *
 * Dual momentum strategy with daily trades
 */

package dmio

import (
	"context"
	"errors"
	"main/data"
	"main/dfextras"
	"main/portfolio"
	"main/strategies/strategy"
	"main/util"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/rocketlaunchr/dataframe-go"
)

// DualMomentumInOutInfo information describing this strategy
func DualMomentumInOutInfo() strategy.StrategyInfo {
	return strategy.StrategyInfo{
		Name:        "Dual Momentum In-Out",
		Shortcode:   "dmio",
		Description: `Dual Momentum In-Outt is a combination of the In-Out strategy with Dual Momentum`,
		Source:      "https://www.quantconnect.com/forum/discussion/9597/the-in-amp-out-strategy-continued-from-quantopian/p3/comment-28146",
		Version:     "2.5.0",
		Arguments: map[string]strategy.Argument{
			"stocks": {
				Name:        "Stocks",
				Description: "List of stocks, etf's or mutual funds to invest in",
				Typecode:    "[]string",
				Default:     `["QQQ", "FDN"]`,
			},
			"bonds": {
				Name:        "Bonds",
				Description: "List of bond etf's or mutual funds to invest in",
				Typecode:    "[]string",
				Default:     `["TLT", "TLH"]`,
			},
			"market": {
				Name:        "Market",
				Description: "Overall market",
				Typecode:    "string",
				Default:     "SPY",
			},
			"canary": {
				Name:        "Canary Universe",
				Description: "List of ETF, Mutual Fund, or Stock tickers to use as canaries",
				Typecode:    "[]string",
				Default:     `["SLV", "GLD", "XLI", "XLU", "DBB", "UUP"]`,
			},
			"volatilityDays": {
				Name:        "Volatility Days",
				Description: "Number of days to include in volatility measurements",
				Typecode:    "number",
				Default:     "126",
			},
			"baseLookBack": {
				Name:        "Base Look Back Period",
				Description: "Base look back period",
				Typecode:    "number",
				Default:     "83",
			},
			"dualLookBack": {
				Name:        "Dual Look Back Period",
				Description: "Dual look back period",
				Typecode:    "number",
				Default:     "252",
			},
			"exclusionWindow": {
				Name:        "Exclusion Window",
				Description: "# of days to exclude",
				Typecode:    "number",
				Default:     "21",
			},
		},
		Suggested: map[string]map[string]string{
			"Default": {
				"stocks":          `["QQQ", "FDN"]`,
				"bonds":           `["TLT", "TLH"]`,
				"market":          "SPY",
				"canary":          `["SLV", "GLD", "XLI", "XLU", "DBB", "UUP", "SPY"]`,
				"volatilityDays":  "126",
				"baseLookBack":    "83",
				"dualLookBack":    "252",
				"exclusionWindow": "21",
			},
		},
		Factory: NewDualMomentumInOut,
	}
}

// DualMomentumInOut strategy type
type DualMomentumInOut struct {
	stocks          []string
	bonds           []string
	market          string
	canary          []string
	volatilityDays  int64
	baseLookBack    int64
	dualLookBack    int64
	exclusionWindow int64

	targetPortfolio *dataframe.DataFrame
	prices          *dataframe.DataFrame
	momentum        *dataframe.DataFrame

	isBull bool

	// Public
	CurrentSymbol string
}

// NewDualMomentumInOut Construct a new Dual Momentum In/Out strategy
func NewDualMomentumInOut(args map[string]json.RawMessage) (strategy.Strategy, error) {
	stocks := []string{}
	if err := json.Unmarshal(args["stocks"], &stocks); err != nil {
		return nil, err
	}
	util.ArrToUpper(stocks)

	bonds := []string{}
	if err := json.Unmarshal(args["bonds"], &bonds); err != nil {
		return nil, err
	}
	util.ArrToUpper(bonds)

	canary := []string{}
	if err := json.Unmarshal(args["canary"], &canary); err != nil {
		return nil, err
	}
	util.ArrToUpper(canary)

	var market string
	if err := json.Unmarshal(args["market"], &market); err != nil {
		return nil, err
	}
	market = strings.ToUpper(market)

	var volatilityDays int64
	if err := json.Unmarshal(args["volatilityDays"], &volatilityDays); err != nil {
		return nil, err
	}

	var baseLookBack int64
	if err := json.Unmarshal(args["baseLookBack"], &baseLookBack); err != nil {
		return nil, err
	}

	var dualLookBack int64
	if err := json.Unmarshal(args["dualLookBack"], &dualLookBack); err != nil {
		return nil, err
	}

	var exclusionWindow int64
	if err := json.Unmarshal(args["exclusionWindow"], &exclusionWindow); err != nil {
		return nil, err
	}

	var dmio strategy.Strategy
	dmio = &DualMomentumInOut{
		stocks:          stocks,
		bonds:           bonds,
		canary:          canary,
		volatilityDays:  volatilityDays,
		baseLookBack:    baseLookBack,
		dualLookBack:    dualLookBack,
		exclusionWindow: exclusionWindow,
	}

	return dmio, nil
}

func (dmio *DualMomentumInOut) downloadPriceData(manager *data.Manager) error {
	// Load EOD quotes for in tickers
	manager.Frequency = data.FrequencyMonthly

	tickers := []string{
		dmio.market,
	}
	tickers = append(tickers, dmio.stocks...)
	tickers = append(tickers, dmio.bonds...)
	tickers = append(tickers, dmio.canary...)

	prices, errs := manager.GetMultipleData(tickers...)

	if len(errs) > 0 {
		return errors.New("Failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for _, v := range prices {
		eod = append(eod, v)
	}

	mergedEod, err := dfextras.MergeAndTimeAlign(context.TODO(), data.DateIdx, eod...)
	dmio.prices = mergedEod
	if err != nil {
		return err
	}

	return nil
}

func (dmio *DualMomentumInOut) calculateSignal() error {
	/*
		// calculate daily volatility of market asset
		pct, err := dfextras.PercentChange(dmio.prices, []string{dmio.market}, 1)
		if err != nil {
			log.WithFields(log.Fields{
				"Market": dmio.market,
				"Error":  err,
			}).Error("DMIO could not calculate percent change")
			return err
		}

		aggFn := dfextras.AggregateSeriesFn(func(vals []interface{}, firstRow int, finalRow int) (float64, error) {
			var sum float64
			for _, val := range vals {
				if v, ok := val.(float64); ok {
					sum += v
				}
			}

			return sum, nil
		})

		vola := stat.StdDev(pct, nil) * math.Sqrt(252.0) // annualize std deviation
		period := int((1.0 - vola) * float64(dmio.baseLookBack))

		momentum, err := dfextras.PercentChange(dmio.prices)
	*/
	return nil
}

// Compute signal
func (dmio *DualMomentumInOut) Compute(manager *data.Manager, myPortfolio *portfolio.Portfolio) error {
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

	manager.Frequency = data.FrequencyDaily

	err := dmio.downloadPriceData(manager)
	if err != nil {
		return err
	}

	dmio.calculateSignal()

	err = myPortfolio.TargetPortfolio(dmio.targetPortfolio)
	if err != nil {
		return err
	}

	return nil
}
