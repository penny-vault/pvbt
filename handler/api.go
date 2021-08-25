// Copyright 2021 JD Fergason
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"main/common"
	"main/data"
	"main/portfolio"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

// ApiKey returns a hex-encoded JSON string containing the userID and tiingoToken
type ApiKeyResponse struct {
	Token string `json:"token"`
}

func ApiKey(c *fiber.Ctx) error {
	// get tiingo token from jwt claims
	pvToken := make(map[string]string)
	pvToken["userID"] = c.Locals("userID").(string)
	pvToken["tiingo"] = c.Locals("tiingoToken").(string)

	jsonPVToken, err := json.Marshal(pvToken)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not encode pvToken")
		return fiber.ErrBadRequest
	}

	// gzip result
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err = zw.Write(jsonPVToken)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not gzip data")
		return fiber.ErrInternalServerError
	}

	if err := zw.Close(); err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not gzip data")
		return fiber.ErrInternalServerError
	}

	// encrypt it
	encryptedToken, err := common.Encrypt(buf.Bytes())
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not encrypt data")
		return fiber.ErrBadRequest
	}

	resp := ApiKeyResponse{
		Token: hex.EncodeToString(encryptedToken),
	}

	return c.JSON(resp)
}

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
	credentials["tiingo"] = c.Locals("tiingoToken").(string)

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
	tickers := dataframe.NewSeriesString(common.TickerName, &dataframe.SeriesInit{Size: 1}, benchmarkTicker)
	targetPortfolio := dataframe.NewDataFrame(dates, tickers)

	p := portfolio.NewPortfolio(ticker, startDate, 10000, &manager)
	err = p.TargetPortfolio(targetPortfolio)
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

	return c.JSON(performance)
}
