package handler

import (
	"main/data"
	"main/portfolio"
	"runtime"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

func Ping(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success", "message": "API is alive"})
}

func Benchmark(c *fiber.Ctx) (resp error) {
	// Parse date strings
	startDateStr := c.Query("startDate", "1990-01-01")
	endDateStr := c.Query("endDate", "now")

	var startDate time.Time
	var endDate time.Time

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		log.WithFields(log.Fields{
			"StartDateStr": startDateStr,
			"EndDateStr":   endDateStr,
			"Error":        err,
		}).Error("Cannoy parse start date query parameter")
		return fiber.ErrNotAcceptable
	}

	if endDateStr == "now" {
		endDate = time.Now()
		year, month, day := endDate.Date()
		endDate = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	} else {
		var err error
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			log.WithFields(log.Fields{
				"StartDateStr": startDateStr,
				"EndDateStr":   endDateStr,
				"Error":        err,
			}).Error("Cannoy parse end date query parameter")
			return fiber.ErrNotAcceptable
		}
	}

	defer func() {
		if err := recover(); err != nil {
			stackSlice := make([]byte, 1024)
			runtime.Stack(stackSlice, false)
			log.WithFields(log.Fields{
				"error":      err,
				"StackTrace": string(stackSlice),
			}).Error("Caught panic in /v1/benchmark")
			resp = fiber.ErrInternalServerError
		}
	}()

	credentials := make(map[string]string)

	// get tiingo token from jwt claims
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	tiingoToken := claims["https://pennyvault.com/tiingo_token"].(string)

	credentials["tiingo"] = tiingoToken
	manager := data.NewManager(credentials)
	manager.Begin = startDate
	manager.End = endDate

	type BenchmarkArgs struct {
		Ticker      string `json:"ticker"`
		SnapToStart bool   `json:"snapToStart"`
	}

	var args BenchmarkArgs
	if err := json.Unmarshal(c.Body(), &args); err != nil {
		log.WithFields(
			log.Fields{
				"Error":      err,
				"StatusCode": fiber.ErrBadRequest,
				"Body":       c.Body(),
				"Uri":        "/v1/benchmark",
			}).Warn("/v1/benchmark called with invalid args")
		return fiber.ErrBadRequest
	}

	if args.SnapToStart {
		securityStart, err := manager.GetData(args.Ticker)
		if err != nil {
			log.WithFields(log.Fields{
				"Symbol": args.Ticker,
				"Error":  err,
			}).Warn("Could not load symbol data")
			return fiber.ErrBadRequest
		}
		row := securityStart.Row(0, true, dataframe.SeriesName)
		startDate = row[data.DateIdx].(time.Time)
	}

	benchmarkTicker := strings.ToUpper(args.Ticker)

	dates := dataframe.NewSeriesTime(data.DateIdx, &dataframe.SeriesInit{Size: 1}, startDate)
	tickers := dataframe.NewSeriesString(portfolio.TickerName, &dataframe.SeriesInit{Size: 1}, benchmarkTicker)
	targetPortfolio := dataframe.NewDataFrame(dates, tickers)

	p := portfolio.NewPortfolio(args.Ticker, &manager)
	err = p.TargetPortfolio(10000, targetPortfolio)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":      err,
			"StatusCode": fiber.ErrBadRequest,
		}).Warn("Error creating target portfolio")
		return fiber.ErrBadRequest
	}

	// calculate the portfolio's performance
	performance, err := p.CalculatePerformance(manager.End)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}
	performance.BuildMetricsBundle()

	return c.JSON(performance)
}
