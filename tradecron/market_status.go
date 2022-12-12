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
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

var (
	holidays      map[int64]int
	holidayLocker sync.RWMutex
)

type MarketStatus struct {
	marketHours *MarketHours
	tz          *time.Location
}

// EarlyClose returns close time of an early close market day, e.g. 1300
func (ms *MarketStatus) EarlyClose(t time.Time) int {
	holidayLocker.RLock()
	defer holidayLocker.RUnlock()

	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ms.tz)
	if close, ok := holidays[d.Unix()]; ok {
		return close
	}
	return 0
}

// IsMarketHoliday returns true if the specified date is a market holiday
func (ms *MarketStatus) IsMarketHoliday(t time.Time) bool {
	holidayLocker.RLock()
	defer holidayLocker.RUnlock()

	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ms.tz)
	close, ok := holidays[d.Unix()]
	if close != 0 {
		return false
	}
	return ok
}

// IsMarketOpen returns true if the specified time is during market hours
// (i.e. not a market holiday or weekend)
func (ms *MarketStatus) IsMarketOpen(t time.Time) bool {
	marketDay := ms.IsMarketDay(t)
	if !marketDay {
		return marketDay
	}

	// check time
	closeTime := ms.marketHours.Close
	earlyClose := ms.EarlyClose(t)
	if earlyClose != 0 {
		closeTime = earlyClose
	}

	timeOfDay := t.Hour()*100 + t.Minute()
	if timeOfDay < ms.marketHours.Open || timeOfDay > closeTime {
		return false
	}

	return true
}

// IsMarketOpen returns true if the specified date is a valid trading day
// (i.e. not a market holiday or weekend)
func (ms *MarketStatus) IsMarketDay(t time.Time) bool {
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}

	isHoliday := ms.IsMarketHoliday(t)
	return !isHoliday
}

// LoadMarketHolidays retrieves market holidays from the database
func NewMarketStatus(hours *MarketHours) *MarketStatus {
	nyc := common.GetTimezone()
	return &MarketStatus{
		marketHours: hours,
		tz:          nyc,
	}
}

// NextFirstTradingDayOfMonth returns the first trading day of the next month
func (ms *MarketStatus) NextFirstTradingDayOfMonth(t time.Time) time.Time {
	// construct a new date for the first of the month
	d := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, ms.tz)
	// add a month to the date
	d = d.AddDate(0, 1, 0)
	// Check if the market is open on the date
	marketOpen := false
	for !marketOpen {
		marketOpen = ms.IsMarketDay(d)
		if !marketOpen {
			d = d.AddDate(0, 0, 1)
		}
	}
	return d
}

// NextFirstTradingDayOfWeek returns the first trading day of the week.
func (ms *MarketStatus) NextFirstTradingDayOfWeek(t time.Time) time.Time {
	daysToWeekBegin := (8 - t.Weekday()) % 7
	t2 := t.AddDate(0, 0, int(daysToWeekBegin))
	marketOpen := false
	for !marketOpen {
		marketOpen = ms.IsMarketDay(t2)
		if !marketOpen {
			t2 = t2.AddDate(0, 0, 1)
		}
	}

	// adjust t2 to midnight
	t2 = time.Date(t2.Year(), t2.Month(), t2.Day(), 0, 0, 0, 0, ms.tz)

	return t2
}

// NextLastTradingDayOfMonth returns the last trading day of the specified month; where a trading day is defined
// as a day the market is open
func (ms *MarketStatus) NextLastTradingDayOfMonth(t time.Time) time.Time {
	firstOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, ms.tz)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	marketOpen := false

	for !marketOpen {
		marketOpen = ms.IsMarketDay(lastOfMonth)
		if !marketOpen {
			lastOfMonth = lastOfMonth.AddDate(0, 0, -1)
		}
	}

	return lastOfMonth
}

// NextLastTradingDayOfWeek returns the next last trading day of week
func (ms *MarketStatus) NextLastTradingDayOfWeek(t time.Time) time.Time {
	daysToFriday := time.Friday - t.Weekday()
	lastDayOfWeek := t.AddDate(0, 0, int(daysToFriday))

	marketOpen := false

	for !marketOpen {
		marketOpen = ms.IsMarketDay(lastDayOfWeek)
		if !marketOpen {
			lastDayOfWeek = lastDayOfWeek.AddDate(0, 0, -1)
		}
	}

	// adjust lastDayOfWeek to midnight
	lastDayOfWeek = time.Date(lastDayOfWeek.Year(), lastDayOfWeek.Month(), lastDayOfWeek.Day(), 0, 0, 0, 0, ms.tz)
	return lastDayOfWeek
}

func LoadMarketHolidays() {
	ctx := context.Background()
	var rows pgx.Rows

	nyc := common.GetTimezone()

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		log.Panic().Err(err).Msg("could not get database transaction")
	}

	sql := "SELECT event_date, early_close, extract(hours from close_time)::int * 100 + extract(minutes from close_time)::int AS close_time FROM market_holidays ORDER BY event_date ASC"
	if rows, err = trx.Query(ctx, sql); err != nil {
		log.Panic().Err(err).Msg("could not load market holidays")
		if err := trx.Rollback(ctx); err != nil {
			log.Panic().Err(err).Msg("could not rollback tranasaction")
		}
	}

	holidayLocker.Lock()
	defer holidayLocker.Unlock()

	holidays = make(map[int64]int)

	var dt time.Time
	var earlyClose bool
	var closeTime int
	for rows.Next() {
		err = rows.Scan(&dt, &earlyClose, &closeTime)
		if err != nil {
			log.Panic().Err(err).Msg("could not scan database values")
		}
		// make sure dt is in the right timezone and at midnight
		dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 0, 0, 0, 0, nyc)
		if earlyClose {
			holidays[dt.Unix()] = closeTime
		} else {
			holidays[dt.Unix()] = 0
		}
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("could not commit transaction")
	}
}
