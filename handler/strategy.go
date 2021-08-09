package handler

import (
	"main/backtest"
	"main/data"
	"main/strategies"
	"runtime/debug"
	"time"

	"github.com/goccy/go-json"

	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
)

// ListStrategies get a list of all strategies
func ListStrategies(c *fiber.Ctx) error {
	log.Info("ListStrategies")
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
		}).Warn("could not load timezone")
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
		}).Error("cannot parse start date query parameter")
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
			}).Error("cannot parse end date query parameter")
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

	credentials := make(map[string]string)
	credentials["tiingo"] = c.Locals("tiingoToken").(string)
	manager := data.NewManager(credentials)

	params := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(c.Query("arguments", "{}")), &params); err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}

	b, err := backtest.New(shortcode, params, startDate, endDate, &manager)
	if err != nil {
		if err.Error() == "strategy not found" {
			return fiber.ErrNotFound
		}

		return fiber.ErrBadRequest
	}

	go b.Serialize(c.Locals("userID").(string))

	measurements := b.Performance.Measurements
	b.Performance.Measurements = nil // set measurements to nil for serialization
	serialized, err := b.Performance.MarshalBinary()
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("serialization failed for performance")
		return fiber.ErrInternalServerError
	}
	b.Performance.Measurements = measurements

	c.Set("Content-type", "application/x-colfer")
	return c.Send(serialized)
}
