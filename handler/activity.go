// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
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
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

type Activity struct {
	ID          string   `json:"id"`
	PortfolioID string   `json:"portfolio_id"`
	Date        string   `json:"date"`
	Activity    string   `json:"message"`
	Tags        []string `json:"tags"`
}

func GetAllActivity(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	sqlQuery := `SELECT
		id::string,
		portfolio_id::string,
		event_date,
		activity,
		tags
	FROM activity WHERE user_id=$1 ORDER BY event_date`
	return GetActivity(c, sqlQuery, userID)
}

func GetPortfolioActivity(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)
	sqlQuery := `SELECT
		id::string,
		portfolio_id::string,
		event_date,
		activity,
		tags
	FROM activity WHERE portfolio_id=$1, user_id=$2 ORDER BY event_date`
	return GetActivity(c, sqlQuery, portfolioID, userID)
}

func GetActivity(c *fiber.Ctx, sqlQuery string, args ...any) error {
	userID := c.Locals("userID").(string)
	subLog := log.With().Str("UserID", userID).Str("Endpoint", "GetActivity").Logger()
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}

	rows, err := trx.Query(context.Background(), sqlQuery, args...)
	if err != nil {
		subLog.Warn().Stack().Err(err).Str("Query", sqlQuery).Msg("datdabase query failed")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrInternalServerError
	}

	activities := make([]Activity, 0, 10)
	for rows.Next() {
		a := Activity{}
		err := rows.Scan(&a.ID, &a.PortfolioID, &a.Date, &a.Activity, &a.Tags)
		if err != nil {
			subLog.Warn().Err(err).Msg("could not scan activity")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
		}
		activities = append(activities, a)
	}

	if err := trx.Commit(context.Background()); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit database transaction")
	}
	return c.JSON(activities)
}
