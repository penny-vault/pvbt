package strategies

// AcceleratingDualMomentumInfo information describing this strategy
func AcceleratingDualMomentumInfo() StrategyInfo {
	return StrategyInfo{
		Name:        "Accelerating Dual Momentum",
		Shortcode:   "adm",
		Description: "A market timing strategy that uses a 1-, 3-, and 6-month momentum score to select assets.",
		Source:      "https://engineeredportfolio.com/2018/05/02/accelerating-dual-momentum-investing/",
		Version:     "1.0.0",
		YTDGain:     1.84,
		Arguments: map[string]Argument{
			"tickers": Argument{
				Name:        "Tickers",
				Description: "List of ETF, Mutual Fund, or Stock tickers to invest in",
				Typecode:    "[]string",
				DefaultVal:  "[\"VFINX\", \"PRIDX\"]",
			},
			"outOfMarketTicker": Argument{
				Name:        "Out-of-Market Ticker",
				Description: "Ticker to use when model scores are all below 0",
				Typecode:    "string",
				DefaultVal:  "VUSTX",
			},
		},
	}
}

type acceleratingDualMomentum struct {
	info StrategyInfo
}

// New Construct a new Accelerating Dual Momentum strategy
func New() acceleratingDualMomentum {
	adm := acceleratingDualMomentum{
		info: AcceleratingDualMomentumInfo(),
	}
	return adm
}

func (adm acceleratingDualMomentum) GetInfo() StrategyInfo {
	return adm.info
}

func (adm acceleratingDualMomentum) Compute() StrategyPerformance {
	return StrategyPerformance{
		StrategyInformation: adm.info,
		PeriodStart:         0,
		PeriodEnd:           1,
	}
}
