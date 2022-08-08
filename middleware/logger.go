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

package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// NewLogger creates a new middleware handler
func NewLogger() fiber.Handler {
	// Set variables
	var (
		start, stop time.Time
		once        sync.Once
		errHandler  fiber.ErrorHandler
	)

	// Return new handler
	return func(c *fiber.Ctx) (err error) {
		// Set error handler once
		once.Do(func() {
			// override error handler
			errHandler = c.App().Config().ErrorHandler
		})

		// Set latency start time
		start = time.Now()

		// Handle request, store err for logging
		chainErr := c.Next()

		// Manually call error handler
		if chainErr != nil {
			if err := errHandler(c, chainErr); err != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		// Set latency stop time
		stop = time.Now()

		subLog := log.With().
			Int("StatusCode", c.Response().StatusCode()).
			Dur("Latency", stop.Sub(start).Round(time.Millisecond)).
			Str("IP", c.IP()).
			Str("Method", c.Method()).
			Str("Path", c.Path()).
			Str("Referer", c.Get(fiber.HeaderReferer)).
			Str("Protocol", c.Protocol()).
			Str("XForwardedFor", c.Get(fiber.HeaderXForwardedFor)).
			Str("Host", c.Hostname()).
			Str("URL", c.OriginalURL()).
			Str("UserAgent", c.Get(fiber.HeaderUserAgent)).
			Int("NumBytesReceived", len(c.Request().Body())).
			Int("NumBytesSent", len(c.Response().Body())).
			Str("Route", c.Route().Path).
			Bytes("RequestBody", c.Body()).
			Str("QueryStringParams", c.Request().URI().QueryArgs().String()).Logger()

		code := c.Response().StatusCode()
		switch {
		case code >= fiber.StatusOK && code < fiber.StatusMultipleChoices:
			subLog.Info().Msg("Processed HTTP request")
		case code >= fiber.StatusMultipleChoices && code < fiber.StatusBadRequest:
			subLog.Info().Msg("Forward HTTP request")
		case code >= fiber.StatusBadRequest && code < fiber.StatusInternalServerError:
			subLog.Warn().Stack().Msg("Bad HTTP request")
		default:
			subLog.Error().Stack().Msg("Internal Server Error")
		}

		return nil
	}
}
