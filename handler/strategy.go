package handler

import (
	"encoding/json"
	"main/data"
	"main/strategies"
	"runtime/debug"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
)

// ListStrategies get a list of all strategies
func ListStrategies(c *fiber.Ctx) error {
	return c.JSON(strategies.StrategyList)
}

// GetStrategy get configuration for a specific strategy
func GetStrategy(c *fiber.Ctx) error {
	shortcode := c.Params("id")
	if strategy, ok := strategies.StrategyMap[shortcode]; ok {
		return c.JSON(strategy)
	}
	return fiber.ErrNotFound
}

// RunStrategy execute strategy
func RunStrategy(c *fiber.Ctx) (resp error) {
	shortcode := c.Params("id")

	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
			debug.PrintStack()
			resp = fiber.ErrInternalServerError
		}
	}()

	if strat, ok := strategies.StrategyMap[shortcode]; ok {
		credentials := make(map[string]string)

		// get tiingo token from jwt claims
		user := c.Locals("user").(*jwt.Token)
		claims := user.Claims.(jwt.MapClaims)
		tiingoToken := claims["https://pennyvault.com/tiingo_token"].(string)

		credentials["tiingo"] = tiingoToken
		manager := data.NewManager(credentials)

		params := map[string]json.RawMessage{}
		if err := json.Unmarshal(c.Body(), &params); err != nil {
			log.Println(err)
			return fiber.ErrBadRequest
		}

		stratObject, err := strat.Factory(params)
		if err != nil {
			log.Println(err)
			return fiber.ErrBadRequest
		}

		p, err := stratObject.Compute(&manager)
		if err != nil {
			log.Println(err)
			return fiber.ErrBadRequest
		}

		// calculate the portfolio's performance
		performance, err := p.Performance(manager.End)
		if err != nil {
			log.Println(err)
			return fiber.ErrBadRequest
		}

		return c.JSON(performance)
	}

	return fiber.ErrNotFound
}
