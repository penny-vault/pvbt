package strategies

import (
	"encoding/json"
	"log"
	"main/data"

	"github.com/rocketlaunchr/dataframe-go"
)

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
			"inTickers": Argument{
				Name:        "Tickers",
				Description: "List of ETF, Mutual Fund, or Stock tickers to invest in",
				Typecode:    "[]string",
				DefaultVal:  "[\"VFINX\", \"PRIDX\"]",
			},
			"outTicker": Argument{
				Name:        "Out-of-Market Ticker",
				Description: "Ticker to use when model scores are all below 0",
				Typecode:    "string",
				DefaultVal:  "VUSTX",
			},
		},
		Factory: New,
	}
}

type acceleratingDualMomentum struct {
	info      StrategyInfo
	inTickers []string
	outTicker string
}

// New Construct a new Accelerating Dual Momentum strategy
func New(args map[string]json.RawMessage) (Strategy, error) {
	inTickers := []string{}
	if err := json.Unmarshal(args["inTickers"], &inTickers); err != nil {
		return acceleratingDualMomentum{}, err
	}

	var outTicker string
	if err := json.Unmarshal(args["outTicker"], &outTicker); err != nil {
		return acceleratingDualMomentum{}, err
	}

	adm := acceleratingDualMomentum{
		info:      AcceleratingDualMomentumInfo(),
		inTickers: inTickers,
		outTicker: outTicker,
	}

	return adm, nil
}

func (adm acceleratingDualMomentum) GetInfo() StrategyInfo {
	return adm.info
}

func (adm acceleratingDualMomentum) Compute(manager data.Manager) (StrategyPerformance, error) {
	// Load EOD quotes for in tickers
	manager.Frequency = "monthly"
	var eod = make(map[string]*dataframe.DataFrame)
	for ii := range adm.inTickers {
		ticker := adm.inTickers[ii]
		tickerEod, err := manager.GetData(ticker)
		if err != nil {
			log.Printf("failed to load ticker: %s\n", ticker)
		}
		eod[ticker] = tickerEod
	}

	// Get out-of-market EOD
	outOfMarketEod, err := manager.GetData(adm.outTicker)
	if err != nil {
		log.Printf("failed to load out of market ticker: %s\n", adm.outTicker)
		return StrategyPerformance{}, err
	}

	log.Println(outOfMarketEod)

	// Get risk free rate (3-mo T-bill secondary rate)
	riskFreeSymbol := "$RATE.MTB3"
	riskFreeRate, err := manager.GetData(riskFreeSymbol)
	if err != nil {
		log.Printf("failed to load risk free rate: %s\n", riskFreeSymbol)
		return StrategyPerformance{}, err
	}

	log.Println(riskFreeRate)

	/*

		// Compute 1-month momentum
		mom1 := inEod/inEod.lag(1) - riskFreeRate
		// Compute 3-month momentum
		mom3 := inEod/inEod.lag(3) - riskFreeRate.rolling(3).sum()
		// Compute 6-month momentum
		mom6 := inEod/inEod.lag(6) - riskFreeRate.rolling(6).sum()
		// Calculate adm score (mom1 + mom3 + mom6) / 3
		score := (mom1 + mom3 + mom6).average()
		score = score.dropna()

		// If all scores > 0 then invest in max(score) else outOfMarketAsset
		score.argmax(ROWS)
		holdings := score.gt(0) || score.lte(0, adm.outTicker)

		portfolio := portfolio.New()
		portfolio.SetTargetHoldings(holdings)

		performance := portfolio.evaluatePerformance()
		performance.StrategyInformation = adm.info
		return performance
	*/
	return StrategyPerformance{}, nil
}
