package handler

import (
	"main/data"
	"main/strategies"
	"runtime/debug"
	"time"

	"github.com/goccy/go-json"

	"github.com/gofiber/fiber/v2"
	"github.com/lestrrat-go/jwx/jwt"
	log "github.com/sirupsen/logrus"
)

// ListStrategies get a list of all strategies
func ListStrategies(c *fiber.Ctx) error {
	return c.JSON(strategies.StrategyList)
}

// GetStrategy get configuration for a specific strategy
func GetStrategy(c *fiber.Ctx) error {
	shortcode := c.Params("shortcode")
	if strategy, ok := strategies.StrategyMap[shortcode]; ok {
		return c.JSON(strategy)
	}
	return fiber.ErrNotFound
}

// RunStrategy execute strategy
func RunStrategy(c *fiber.Ctx) (resp error) {
	shortcode := c.Params("shortcode")
	startDateStr := c.Query("startDate", "1980-01-01")
	endDateStr := c.Query("endDate", "now")

	var startDate time.Time
	var endDate time.Time

	tz, err := time.LoadLocation("America/New_York") // New York is the reference time
	if err != nil {
		log.WithFields(log.Fields{
			"Timezone": "America/New_York",
			"Error":    err,
		}).Warn("Could not load timezone")
		return fiber.ErrInternalServerError
	}

	startDate, err = time.ParseInLocation("2006-01-02", startDateStr, tz)
	if err != nil {
		log.WithFields(log.Fields{
			"Function":     "handler/strategy.go:RunStrategy",
			"Strategy":     shortcode,
			"StartDateStr": startDateStr,
			"EndDateStr":   endDateStr,
			"Error":        err,
		}).Error("Cannoy parse start date query parameter")
		return fiber.ErrNotAcceptable
	}

	if endDateStr == "now" {
		endDate = time.Now()
		year, month, day := endDate.Date()
		endDate = time.Date(year, month, day, 0, 0, 0, 0, tz)
	} else {
		var err error
		endDate, err = time.ParseInLocation("2006-01-02", endDateStr, tz)
		if err != nil {
			log.WithFields(log.Fields{
				"Function":     "handler/strategy.go:RunStrategy",
				"Strategy":     shortcode,
				"StartDateStr": startDateStr,
				"EndDateStr":   endDateStr,
				"Error":        err,
			}).Error("Cannoy parse end date query parameter")
			return fiber.ErrNotAcceptable
		}
	}

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
		jwtToken := c.Locals("user").(jwt.Token)
		if tiingoToken, ok := jwtToken.Get(`https://pennyvault.com/tiingo_token`); ok {
			credentials["tiingo"] = tiingoToken.(string)
		} else {
			log.WithFields(log.Fields{
				"jwtToken": tiingoToken,
				"error":    "jwt token does not have expected claim: https://pennyvault.com/tiingo_token",
			})
			return fiber.ErrBadRequest
		}

		manager := data.NewManager(credentials)
		manager.Begin = startDate
		manager.End = endDate

		params := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(c.Query("arguments", "{}")), &params); err != nil {
			log.Println(err)
			return fiber.ErrBadRequest
		}

		stratObject, err := strat.Factory(params)
		if err != nil {
			log.Println(err)
			return fiber.ErrBadRequest
		}

		start := time.Now()
		p, err := stratObject.Compute(&manager)
		if err != nil {
			log.Println(err)
			return fiber.ErrBadRequest
		}
		stop := time.Now()
		stratComputeDur := stop.Sub(start).Round(time.Millisecond)

		// calculate the portfolio's performance
		start = time.Now()
		performance, err := p.CalculatePerformance(manager.End)
		if err != nil {
			log.Println(err)
			return fiber.ErrBadRequest
		}
		stop = time.Now()
		calcPerfDur := stop.Sub(start).Round(time.Millisecond)

		start = time.Now()
		performance.BuildMetricsBundle()
		stop = time.Now()
		metricCalcDur := stop.Sub(start).Round(time.Millisecond)

		log.WithFields(log.Fields{
			"StratCalcDur":  stratComputeDur,
			"PerfCalcDur":   calcPerfDur,
			"MetricCalcDur": metricCalcDur,
		}).Info("Strategy calculated")

		return c.JSON(performance)
	}

	return fiber.ErrNotFound
}
