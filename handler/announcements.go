// Copyright 2021-2025
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

	"github.com/gofiber/fiber/v2"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

type Announcement struct {
	ID      string   `json:"id"`
	Date    string   `json:"date"`
	Expires string   `json:"expires"`
	Message string   `json:"message"`
	Tags    []string `json:"tags"`
}

func GetAnnouncements(c *fiber.Ctx) error {
	ctx := context.Background()
	userID := c.Locals("userID").(string)
	subLog := log.With().Str("UserID", userID).Str("Endpoint", "GetAnnouncements").Logger()
	query := `SELECT
		id::string,
		event_date,
		expires,
		announcement,
		tags
	FROM announcements WHERE expires > now() ORDER BY event_date DESC`
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}

	rows, err := trx.Query(ctx, query)
	if err != nil {
		subLog.Warn().Stack().Err(err).Str("Query", query).Msg("database query failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrInternalServerError
	}

	announcements := make([]Announcement, 0, 10)
	for rows.Next() {
		a := Announcement{}
		err := rows.Scan(&a.ID, &a.Date, &a.Expires, &a.Message, &a.Tags)
		if err != nil {
			subLog.Warn().Err(err).Msg("could not scan announcement")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
		}
		announcements = append(announcements, a)
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit database transaction")
	}
	return c.JSON(announcements)
}
