// Copyright 2021-2022
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
	"context"
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/penny-vault/pv-api/backtest"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ListStrategies get a list of all strategies
func ListStrategies(c *fiber.Ctx) error {
	log.Info().Msg("ListStrategies")
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

// RunStrategy executes the strategy
func RunStrategy(c *fiber.Ctx) (resp error) {
	attrs := opentelemetry.SpanAttributesFromFiber(c)
	ctx, span := otel.Tracer(opentelemetry.Name).Start(context.Background(), "RunStrategy", trace.WithAttributes(attrs...))
	defer span.End()

	shortcode := c.Params("shortcode")
	startDateStr := c.Query("startDate", "1980-01-01")
	endDateStr := c.Query("endDate", "now")
	benchmark := c.Query("benchmark", "VFINX")

	subLog := log.With().Str("Shortcode", shortcode).Str("StartDateQueryStr", startDateStr).Str("EndDateQueryStr", endDateStr).Logger()

	span.SetAttributes(
		attribute.KeyValue{
			Key:   "pv.shortcode",
			Value: attribute.StringValue(shortcode),
		},
		attribute.KeyValue{
			Key:   "pv.StartDate",
			Value: attribute.StringValue(startDateStr),
		},
		attribute.KeyValue{
			Key:   "pv.EndDate",
			Value: attribute.StringValue(endDateStr),
		},
	)

	var startDate time.Time
	var endDate time.Time
	var err error

	tz := common.GetTimezone()

	startDate, err = time.ParseInLocation("2006-01-02", startDateStr, tz)
	if err != nil {
		subLog.Warn().Stack().Msg("cannot parse start date query parameter")
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
			subLog.Warn().Stack().Msg("cannot parse end date query parameter")
			return fiber.ErrNotAcceptable
		}
	}

	defer func() {
		if err := recover(); err != nil {
			log.Error().Stack().Msg("caught exception")
			fmt.Println(err)
			debug.PrintStack()
			resp = fiber.ErrInternalServerError
		}
	}()

	manager := data.NewManager()

	params := map[string]json.RawMessage{}
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		log.Warn().Stack().Err(err).Msg("could not unmarshal body message")
		return fiber.ErrBadRequest
	}

	b, err := backtest.New(ctx, shortcode, params, benchmark, startDate, endDate, manager)
	if err != nil {
		if err.Error() == "strategy not found" {
			return fiber.ErrNotFound
		}
		return fiber.ErrBadRequest
	}

	permanent := false
	if c.Query("permanent", "false") == "true" {
		permanent = true
	}
	go b.Save(c.Locals("userID").(string), permanent)

	portfolioIDStr := hex.EncodeToString(b.PortfolioModel.Portfolio.ID)
	serializedPortfolio, err := b.PortfolioModel.Portfolio.MarshalBinary()
	if err != nil {
		subLog.Error().Stack().Err(err).Str("PortfolioID", portfolioIDStr).Msg("serialization failed for portfolio")
		return err
	}
	err = common.CacheSet(portfolioIDStr, serializedPortfolio)
	if err != nil {
		subLog.Error().Stack().Err(err).Str("PortfolioID", portfolioIDStr).Msg("caching failed for portfolio")
		return err
	}

	serializedPerformance, err := b.Performance.MarshalBinary()
	if err != nil {
		subLog.Error().Stack().Err(err).Str("PortfolioID", portfolioIDStr).Msg("serialization failed for portfolio")
		return err
	}
	err = common.CacheSet(fmt.Sprintf("%s:performance", portfolioIDStr), serializedPerformance)
	if err != nil {
		subLog.Error().Stack().Err(err).Str("PortfolioID", portfolioIDStr).Msg("caching failed for portfolio")
		return err
	}

	measurements := b.Performance.Measurements
	b.Performance.Measurements = nil // set measurements to nil for serialization
	serialized, err := b.Performance.MarshalBinary()
	if err != nil {
		subLog.Error().Stack().Err(err).Str("PortfolioID", portfolioIDStr).Msg("serialization failed for performance")
		return fiber.ErrInternalServerError
	}
	b.Performance.Measurements = measurements

	c.Set("Content-type", "application/x-colfer")
	return c.Send(serialized)
}
