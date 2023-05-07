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
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v4"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

type SecurityDetail struct {
	CompositeFigi string `json:"compositeFigi"`
	Cusip         string `json:"cusip"`
	Name          string `json:"name"`
	Ticker        string `json:"ticker"`
}

func parseRange(r string) (int, int, error) {
	if r == "" {
		return 100, 0, nil
	}

	re := regexp.MustCompile(`((\w+)=)?(\d+)-(\d+)`)
	res := re.FindStringSubmatch(r)

	if res == nil {
		return 10, 0, fiber.ErrRequestedRangeNotSatisfiable
	}

	if len(res) == 5 && res[2] != "items" {
		return 10, 0, fiber.ErrRequestedRangeNotSatisfiable
	}

	begin, err := strconv.ParseInt(res[3], 10, 32)
	if err != nil {
		log.Error().Err(err).Msg("could not parse limit")
		return 10, 0, fiber.ErrRequestedRangeNotSatisfiable
	}

	end, err := strconv.ParseInt(res[4], 10, 32)
	if err != nil {
		log.Error().Err(err).Msg("could not parse offset")
		return 10, 0, fiber.ErrRequestedRangeNotSatisfiable
	}

	if end <= begin {
		log.Error().Int64("Begin", begin).Int64("End", end).Msg("range error: end <= begin")
		return 10, 0, fiber.ErrRequestedRangeNotSatisfiable
	}

	limit := int(end - begin + 1)
	offset := int(begin)

	return limit, offset, nil
}

func LookupSecurity(c *fiber.Ctx) error {
	var err error
	symbol := c.Params("symbol")
	symbol, err = url.QueryUnescape(symbol)
	if err != nil {
		log.Error().Err(err).Msg("could not unencode ticker")
		return fiber.ErrBadRequest
	}
	subLog := log.With().Str("Endpoint", "LookupSecurityByTicker").Logger()

	var security *data.Security
	if len(symbol) == 12 {
		// it's a figi
		security, err = data.SecurityFromFigi(symbol)
		if err != nil {
			subLog.Error().Str("Ticker", symbol).Msg("could not find ticker")
			return fiber.ErrNotFound
		}
	} else {
		// if it's not a figi it must be a ticker
		security, err = data.SecurityFromTicker(symbol)
		if err != nil {
			subLog.Error().Str("Ticker", symbol).Msg("could not find ticker")
			return fiber.ErrNotFound
		}
	}

	return c.JSON(security)
}

func SearchSecurity(c *fiber.Ctx) error {
	ctx := context.Background()
	query := c.Query("q")
	rangeHeader := c.Get("range")
	limit, offset, err := parseRange(rangeHeader)
	if limit > 100 || err != nil {
		log.Error().Int("Limit", limit).Msg("range header error")
		return fiber.ErrRequestedRangeNotSatisfiable
	}

	subLog := log.With().Str("Query", query).Str("Endpoint", "SearchSecurity").Logger()

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("could not get transaction when querying trading days")
		return fiber.ErrInternalServerError
	}

	var rows pgx.Rows

	if query != "" {
		var err error
		query = fmt.Sprintf("%s%%", query)
		sql := fmt.Sprintf("SELECT composite_figi, cusip, ticker, name, 1.0 AS rank from assets where active='t' and ticker ilike $1 ORDER BY ticker LIMIT %d OFFSET %d;", limit, offset)
		rows, err = trx.Query(ctx, sql, query)
		if err != nil {
			subLog.Warn().Stack().Str("SQL", sql).Err(err).Str("Query", sql).Msg("database query failed")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}

			return fiber.ErrInternalServerError
		}
	} else {
		var err error
		sql := fmt.Sprintf("SELECT composite_figi, cusip, ticker, name, 1.0 AS rank from assets where active='t' ORDER BY ticker LIMIT %d OFFSET %d;", limit, offset)
		rows, err = trx.Query(ctx, sql)
		if err != nil {
			subLog.Warn().Stack().Str("SQL", sql).Err(err).Str("Query", sql).Msg("database query failed")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}

			return fiber.ErrInternalServerError
		}
	}

	securities := make([]*SecurityDetail, 0)
	for rows.Next() {
		var ticker string
		var compositeFigi string
		var cusip string
		var name string
		var rank float64
		err := rows.Scan(&compositeFigi, &cusip, &ticker, &name, &rank)
		if err != nil {
			log.Error().Err(err).Msg("could not scan database results")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return err
		}
		s := &SecurityDetail{
			CompositeFigi: compositeFigi,
			Cusip:         cusip,
			Name:          name,
			Ticker:        ticker,
		}
		securities = append(securities, s)
	}

	if len(securities) == 0 {
		// use full text search
		query = fmt.Sprintf("%s:*", query)
		sql := fmt.Sprintf("SELECT composite_figi, cusip, ticker, name, ts_rank_cd(search, to_tsquery('simple', $1)) AS rank from assets where active='t' and to_tsquery('simple', $1) @@ search ORDER BY rank desc LIMIT %d OFFSET %d;", limit, offset)
		rows, err = trx.Query(ctx, sql, query)
		if err != nil {
			subLog.Warn().Stack().Str("SQL", sql).Err(err).Str("Query", sql).Msg("database query failed")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}

			return fiber.ErrInternalServerError
		}
		for rows.Next() {
			var ticker string
			var compositeFigi string
			var cusip string
			var name string
			var rank float64
			err := rows.Scan(&compositeFigi, &cusip, &ticker, &name, &rank)
			if err != nil {
				log.Error().Err(err).Msg("could not scan database results")
				if err := trx.Rollback(ctx); err != nil {
					log.Error().Stack().Err(err).Msg("could not rollback transaction")
				}
				return err
			}
			s := &SecurityDetail{
				CompositeFigi: compositeFigi,
				Cusip:         cusip,
				Name:          name,
				Ticker:        ticker,
			}
			securities = append(securities, s)
		}
	}

	if err := trx.Commit(ctx); err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not commit transaction")
	}

	beginRange := offset
	endRange := offset + len(securities) - 1
	count := "*"
	if len(securities) < limit {
		count = fmt.Sprintf("%d", len(securities)+offset)
	}
	c.Append("Content-Range", fmt.Sprintf("items %d-%d/%s", beginRange, endRange, count))
	return c.JSON(securities)
}
