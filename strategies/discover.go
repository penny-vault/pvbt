package strategies

// StrategyList List of all strategies
var StrategyList = []StrategyInfo{
	AcceleratingDualMomentumInfo(),
	KellersDefensiveAssetAllocationInfo(),
}

// StrategyMap Map of strategies
var StrategyMap = make(map[string]StrategyInfo)

// IntializeStrategyMap configure the strategy map
func IntializeStrategyMap() {
	for ii := range StrategyList {
		strat := StrategyList[ii]
		StrategyMap[strat.Shortcode] = strat
	}
}
