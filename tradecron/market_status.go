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

package tradecron

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

var (
	maxDuration = 24 * 7 * time.Hour
)

type MarketStatus struct {
	holidays    map[int64]int
	marketHours *MarketHours
	lastRefresh time.Time
	tz          *time.Location
}

// EarlyClose returns close time of an early close market day, e.g. 1300
func (ms *MarketStatus) EarlyClose(t time.Time) (int, error) {
	err := ms.refresh()
	if err != nil {
		return 0, err
	}

	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ms.tz)
	if close, ok := ms.holidays[d.Unix()]; ok {
		return close, nil
	}
	return 0, nil
}

// IsMarketHoliday returns true if the specified date is a market holiday
func (ms *MarketStatus) IsMarketHoliday(t time.Time) (bool, error) {
	err := ms.refresh()
	if err != nil {
		return false, err
	}

	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ms.tz)
	close, ok := ms.holidays[d.Unix()]
	if close != 0 {
		return false, nil
	}
	return ok, nil
}

// IsMarketOpen returns true if the specified time is during market hours
// (i.e. not a market holiday or weekend)
func (ms *MarketStatus) IsMarketOpen(t time.Time) (bool, error) {
	marketDay, err := ms.IsMarketDay(t)
	if err != nil {
		return false, err
	}

	if !marketDay {
		return marketDay, nil
	}

	// check time
	closeTime := ms.marketHours.Close
	earlyClose, err := ms.EarlyClose(t)
	if err != nil {
		return false, err
	}
	if earlyClose != 0 {
		closeTime = earlyClose
	}

	timeOfDay := t.Hour()*100 + t.Minute()
	if timeOfDay < ms.marketHours.Open || timeOfDay > closeTime {
		return false, nil
	}

	return true, nil
}

// IsMarketOpen returns true if the specified date is a valid trading day
// (i.e. not a market holiday or weekend)
func (ms *MarketStatus) IsMarketDay(t time.Time) (bool, error) {
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false, nil
	}

	isHoliday, err := ms.IsMarketHoliday(t)
	if err != nil {
		return false, err
	}

	if isHoliday {
		return false, nil
	}

	return true, nil
}

// LoadMarketHolidays retrieves market holidays from the database
func NewMarketStatus(hours *MarketHours) *MarketStatus {
	nyc := common.GetTimezone()
	return &MarketStatus{
		holidays:    make(map[int64]int),
		marketHours: hours,
		tz:          nyc,
	}
}

// NextFirstTradingDayOfMonth returns the first trading day of the next month
func (ms *MarketStatus) NextFirstTradingDayOfMonth(t time.Time) (time.Time, error) {
	// construct a new date for the first of the month
	d := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, ms.tz)
	// add a month to the date
	d = d.AddDate(0, 1, 0)
	// Check if the market is open on the date
	marketOpen := false
	var err error
	for !marketOpen {
		marketOpen, err = ms.IsMarketDay(d)
		if err != nil {
			return d, err
		}
		if !marketOpen {
			d = d.AddDate(0, 0, 1)
		}
	}
	return d, err
}

// NextFirstTradingDayOfWeek returns the first trading day of the week.
func (ms *MarketStatus) NextFirstTradingDayOfWeek(t time.Time) (time.Time, error) {
	var err error
	days := (8 - t.Weekday()) % 7
	t2 := t.AddDate(0, 0, int(days))
	marketOpen := false
	for !marketOpen {
		marketOpen, err = ms.IsMarketDay(t2)
		if err != nil {
			return t2, err
		}
		if !marketOpen {
			t2 = t2.AddDate(0, 0, 1)
		}
	}

	// adjust t2 to midnight
	t2 = time.Date(t2.Year(), t2.Month(), t2.Day(), 0, 0, 0, 0, ms.tz)

	return t2, nil
}

// NextLastTradingDayOfMonth returns the last trading day of the specified month; where a trading day is defined
// as a day the market is open
func (ms *MarketStatus) NextLastTradingDayOfMonth(t time.Time) (time.Time, error) {
	firstOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, ms.tz)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	marketOpen := false
	var err error

	for !marketOpen {
		marketOpen, err = ms.IsMarketDay(lastOfMonth)
		if err != nil {
			return time.Time{}, err
		}

		if !marketOpen {
			lastOfMonth = lastOfMonth.AddDate(0, 0, -1)
		}
	}

	return lastOfMonth, nil
}

// NextLastTradingDayOfWeek returns the next last trading day of week
func (ms *MarketStatus) NextLastTradingDayOfWeek(t time.Time) (time.Time, error) {
	daysToFriday := time.Friday - t.Weekday()
	lastDayOfWeek := t.AddDate(0, 0, int(daysToFriday))

	marketOpen := false
	var err error

	for !marketOpen {
		marketOpen, err = ms.IsMarketDay(lastDayOfWeek)
		if err != nil {
			return time.Time{}, err
		}

		if !marketOpen {
			lastDayOfWeek = lastDayOfWeek.AddDate(0, 0, -1)
		}
	}

	// adjust lastDayOfWeek to midnight
	lastDayOfWeek = time.Date(lastDayOfWeek.Year(), lastDayOfWeek.Month(), lastDayOfWeek.Day(), 0, 0, 0, 0, ms.tz)

	return lastDayOfWeek, nil
}

func (ms *MarketStatus) refresh() error {
	now := time.Now()
	now.In(ms.tz)

	// if the max duration has not elapsed, just return
	if maxDuration > now.Sub(ms.lastRefresh) {
		return nil
	}

	var rows pgx.Rows
	var err error

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		log.Error().Err(err).Msg("could not get database transaction")
		return err
	}

	sql := "SELECT event_date, early_close, extract(hours from close_time)::int * 100 + extract(minutes from close_time)::int AS close_time FROM market_holidays ORDER BY event_date ASC"
	if rows, err = trx.Query(context.Background(), sql); err != nil {
		log.Error().Err(err).Msg("could not load market holidays")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback tranasaction")
		}
		return err
	}

	var dt time.Time
	var earlyClose bool
	var closeTime int
	for rows.Next() {
		err = rows.Scan(&dt, &earlyClose, &closeTime)
		if err != nil {
			log.Error().Err(err).Msg("could not scan database values")
			return err
		}
		// make sure dt is in the right timezone and at midnight
		dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 0, 0, 0, 0, ms.tz)
		if earlyClose {
			ms.holidays[dt.Unix()] = closeTime
		} else {
			ms.holidays[dt.Unix()] = 0
		}
	}

	if err := trx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Msg("could not commit transaction")
	}

	ms.lastRefresh = now
	return nil
}
