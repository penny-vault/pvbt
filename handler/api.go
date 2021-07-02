package handler

import (
	"main/data"
	"main/portfolio"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

type PingResponse struct {
	Status  string `json:"status" example:"success"`
	Message string `json:"message" example:"API is alive"`
	Time    string `json:"time" example:"2021-06-19T08:09:10.115924-05:00"`
}

func Ping(c *fiber.Ctx) error {
	var response PingResponse
	now, err := time.Now().MarshalText()
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("error while getting time in ping")
		response = PingResponse{
			Status:  "error",
			Message: err.Error(),
			Time:    string(now),
		}
	} else {
		response = PingResponse{
			Status:  "success",
			Message: "API is alive",
			Time:    string(now),
		}
	}
	return c.JSON(response)
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

	snapToStart, err := strconv.ParseBool(c.Query("snapToStart", "true"))
	if err != nil {
		log.WithFields(
			log.Fields{
				"Error":       err,
				"StatusCode":  fiber.ErrBadRequest,
				"SnapToStart": c.Query("snapToStart"),
				"Uri":         "/v1/benchmark",
			}).Warn("/v1/benchmark called with invalid snapToStart")
		return fiber.ErrBadRequest
	}

	ticker := c.Params("ticker")

	if snapToStart {
		securityStart, err := manager.GetData(ticker)
		if err != nil {
			log.WithFields(log.Fields{
				"Symbol": ticker,
				"Error":  err,
			}).Warn("Could not load symbol data")
			return fiber.ErrBadRequest
		}
		row := securityStart.Row(0, true, dataframe.SeriesName)
		startDate = row[data.DateIdx].(time.Time)
	}

	benchmarkTicker := strings.ToUpper(ticker)

	dates := dataframe.NewSeriesTime(data.DateIdx, &dataframe.SeriesInit{Size: 1}, startDate)
	tickers := dataframe.NewSeriesString(portfolio.TickerName, &dataframe.SeriesInit{Size: 1}, benchmarkTicker)
	targetPortfolio := dataframe.NewDataFrame(dates, tickers)

	p := portfolio.NewPortfolio(ticker, &manager)
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
