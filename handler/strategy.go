package handler

import (
	"main/strategies"

	"github.com/gofiber/fiber/v2"
)

var strategyList = [1]strategies.StrategyInfo{
	strategies.AcceleratingDualMomentumInfo(),
}

var strategyMap = make(map[string]strategies.StrategyInfo)

// IntializeStrategyMap configure the strategy map
func IntializeStrategyMap() {
	for ii := range strategyList {
		strat := strategyList[ii]
		strategyMap[strat.Shortcode] = strat
	}
}

// ListStrategies get a list of all strategies
func ListStrategies(c *fiber.Ctx) error {
	return c.JSON(strategyList)
}

// GetStrategy get configuration for a specific strategy
func GetStrategy(c *fiber.Ctx) error {
	shortcode := c.Params("id")
	if strategy, ok := strategyMap[shortcode]; ok {
		return c.JSON(strategy)
	}
	return fiber.ErrNotFound
}
