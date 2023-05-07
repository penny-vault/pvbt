// Copyright 2021-2023
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
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
	"context"
	"encoding/hex"
	"runtime"
	"strconv"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/rs/zerolog/log"
)

// ApiKey returns a hex-encoded JSON string containing the userID
type APIKeyResponse struct {
	Token string `json:"token"`
}

func GetAPIKey(c *fiber.Ctx) error {
	pvToken := make(map[string]string)
	pvToken["userID"] = c.Locals("userID").(string)

	jsonPVToken, err := json.Marshal(pvToken)
	if err != nil {
		log.Warn().Stack().Err(err).Msg("could not encode pvToken")
		return fiber.ErrBadRequest
	}

	// gzip result
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err = zw.Write(jsonPVToken)
	if err != nil {
		log.Warn().Stack().Err(err).Msg("could not gzip data")
		return fiber.ErrInternalServerError
	}

	if err := zw.Close(); err != nil {
		log.Warn().Stack().Err(err).Msg("could not close gzipper")
		return fiber.ErrInternalServerError
	}

	// encrypt it
	encryptedToken, err := common.Encrypt(buf.Bytes())
	if err != nil {
		log.Warn().Stack().Err(err).Msg("could not encrypt data")
		return fiber.ErrBadRequest
	}

	resp := APIKeyResponse{
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
		log.Error().Stack().Err(err).Msg("error while getting time in ping")
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
	var err error
	// Parse date strings
	startDateStr := c.Query("startDate", "1990-01-01")
	endDateStr := c.Query("endDate", "now")

	subLog := log.With().Str("StartDateStr", startDateStr).Str("EndDateStr", endDateStr).Logger()

	var startDate time.Time
	var endDate time.Time

	tz := common.GetTimezone()

	startDate, err = time.ParseInLocation("2006-01-02", startDateStr, tz)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("cannot parse start date query parameter")
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
			subLog.Error().Stack().Err(err).Msg("cannot parse end date query parameter")
			return fiber.ErrNotAcceptable
		}
	}

	defer func() {
		if err := recover(); err != nil {
			stackSlice := make([]byte, 1024)
			runtime.Stack(stackSlice, false)
			subLog.Error().Stack().Msg("caught panic in /v1/benchmark")
			resp = fiber.ErrInternalServerError
		}
	}()

	snapToStart, err := strconv.ParseBool(c.Query("snapToStart", "true"))
	if err != nil {
		subLog.Warn().Stack().Int("StatusCode", fiber.ErrBadRequest.Code).Err(err).Str("SnapToStart", c.Query("snapToStart")).Str("Uri", "/v1/benchmark").Msg("/v1/benchmark called with invalid snapToStart")
		return fiber.ErrBadRequest
	}

	compositeFigi := c.Params("compositeFigi")
	security, err := data.SecurityFromFigi(compositeFigi)
	if err != nil {
		return fiber.ErrNotFound
	}

	if snapToStart {
		securityStart, err := data.NewDataRequest(security).Metrics(data.MetricAdjustedClose).Between(startDate, endDate)
		if err != nil {
			return fiber.ErrBadRequest
		}
		df := securityStart[data.SecurityMetric{
			SecurityObject: *security,
			MetricObject:   data.MetricAdjustedClose,
		}.String()]
		startDate = df.Dates[0]
	}

	target := data.PortfolioPlan{
		{
			Date: startDate,
			Members: map[data.Security]float64{
				*security: 1.0,
			},
		},
	}

	p := portfolio.NewPortfolio(security.Ticker, startDate, 10000)
	err = p.TargetPortfolio(context.Background(), target)
	if err != nil {
		return fiber.ErrBadRequest
	}

	// calculate the portfolio's performance
	performance := portfolio.NewPerformance(p.Portfolio)
	err = performance.CalculateThrough(context.Background(), p, endDate)
	if err != nil {
		return fiber.ErrBadRequest
	}

	return c.JSON(performance)
}
