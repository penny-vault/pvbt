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

package tradecron

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

var holidays map[int][]time.Time
var lastHolidayLoad time.Time

// IsMarketHoliday returns true if the specified date is a market holiday
func IsMarketHoliday(t time.Time) bool {
	var nyc *time.Location
	var err error
	if nyc, err = time.LoadLocation("America/New_York"); err != nil {
		log.Panic().Err(err).Msg("could not load nyc timezone")
	}

	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, nyc)
	for _, day := range holidays[t.Year()] {
		if d.Equal(day) {
			return true
		}
	}
	return false
}

// LoadMarketHolidays retrieves market holidays from the database
func LoadMarketHolidays() error {
	var rows pgx.Rows
	var err error

	var nyc *time.Location
	if nyc, err = time.LoadLocation("America/New_York"); err != nil {
		log.Panic().Err(err).Msg("could not load nyc timezone")
	}

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		return err
	}

	if lastHolidayLoad.After(time.Date(1929, 1, 1, 0, 0, 0, 0, nyc)) {
		rows, err = trx.Query(context.Background(), "SELECT event_date FROM market_holidays WHERE event_date > $1 ORDER BY event_date ASC", lastHolidayLoad)
	} else {
		rows, err = trx.Query(context.Background(), "SELECT event_date FROM market_holidays ORDER BY event_date ASC")
	}

	if err != nil {
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback tranasaction")
		}
		return err
	}

	var dt time.Time
	for rows.Next() {
		err = rows.Scan(&dt)
		if err != nil {
			return err
		}
		lastHolidayLoad = dt
		if _, ok := holidays[dt.Year()]; !ok {
			holidays[dt.Year()] = []time.Time{dt}
		} else {
			holidays[dt.Year()] = append(holidays[dt.Year()], dt)
		}
	}

	if err := trx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Msg("could not commit transaction")
	}
	return nil
}
