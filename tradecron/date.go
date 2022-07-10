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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

var (
	ErrDurationParseError = errors.New("could not parse duration string")
)

type CronMode int16

const (
	RegularHours CronMode = iota
	ExtendedHours
	AllHours
)

const (
	AtOpen     = "@open"
	AtClose    = "@close"
	AtMonthEnd = "@monthend"
)

type TradeCron struct {
	Schedule       cron.Schedule
	ScheduleString string
	Flag           string
	Offset         time.Duration
	Mode           CronMode
}

var holidays map[int][]time.Time
var lastHolidayLoad time.Time

var marketOpenHour = 9
var marketOpenMin = 30
var marketCloseHour = 16
var marketExtendedOpenHour = 7
var marketExtendedOpenMin = 0
var marketExtendedCloseHour = 20

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
			log.Error().Stack().Err(err).Msg("could not rollback tranasaction")
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
		log.Error().Stack().Err(err).Msg("could not commit transaction")
	}
	return nil
}

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

// IsTradeDay returns true if the specified date is a valid trading day (i.e. not a market holiday or weekend)
func IsTradeDay(t time.Time) bool {
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}

	if IsMarketHoliday(t) {
		return false
	}

	return true
}

// NextMonth returns the first day of the next month
func NextMonth(t time.Time) time.Time {
	y := t.Year()
	m := t.Month()
	if m == time.December {
		y++
		m = time.January
	} else {
		m++
	}
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}

// MonthEnd returns the last trading day of the specified month
func MonthEnd(t time.Time) time.Time {
	var nyc *time.Location
	var err error
	if nyc, err = time.LoadLocation("America/New_York"); err != nil {
		log.Panic().Err(err).Msg("could not load nyc timezone")
	}

	firstOfMonth := time.Date(t.Year(), t.Month(), 1, marketCloseHour, 0, 0, 0, nyc)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	for !IsTradeDay(lastOfMonth) {
		lastOfMonth = lastOfMonth.AddDate(0, 0, -1)
	}

	return lastOfMonth
}

// TradeCron enables market aware scheduling. It supports the standard
// CRON format of: Minutes Hours DayOfMonth Month DayOfWeek
//     e.g. */5 * * * * = Run every 5 minutes from 0930 to 1600 EST
//
// Additional market-aware modifiers are supported:
//     @open [timespec]       - Run at market open; timespec is a string parseable by
//                              time.ParseDuration and is added to the @open specification
//     @close [timespec]      - Run at market close
//     @monthend [timespec]   - Run at market close or timespec on last trading day of month
func New(scheduleStr string, mode CronMode) (*TradeCron, error) {
	specParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	var schedule cron.Schedule
	var offset = time.Duration(0)
	var flag string
	var err error

	identifier := strings.Split(scheduleStr, " ")
	switch identifier[0] {
	case AtOpen:
		flag = "@open"
		schedule, err = specParser.Parse(fmt.Sprintf("%d %d * * *", marketCloseHour, marketOpenHour))
	case AtClose:
		flag = "@close"
		schedule, err = specParser.Parse("0 16 * * *")
	case "@monthend":
		flag = "@monthend"
		schedule, err = specParser.Parse("0 16 * * *")
	default:
		flag = ""
		schedule, err = specParser.Parse(scheduleStr)
	}

	if err != nil {
		return nil, err
	}

	if len(identifier) == 2 && flag != "" {
		offset, err = time.ParseDuration(identifier[1])
		if err != nil {
			return nil, ErrDurationParseError
		}
	}

	tc := &TradeCron{
		Schedule:       schedule,
		ScheduleString: scheduleStr,
		Flag:           flag,
		Offset:         offset,
		Mode:           mode,
	}

	return tc, err
}

// Next returns the next tradeable date
func (tc *TradeCron) Next(forDate time.Time) time.Time {
	var nyc *time.Location
	var err error
	if nyc, err = time.LoadLocation("America/New_York"); err != nil {
		log.Panic().Err(err).Msg("could not load nyc timezone")
	}

	switch tc.Flag {
	case AtMonthEnd:
		// get last trading day of month
		now := forDate.In(nyc)
		monthend := MonthEnd(now)
		monthend = monthend.Add(tc.Offset)
		if now.After(monthend) {
			now = NextMonth(now)
			monthend = MonthEnd(now)
			monthend = monthend.Add(tc.Offset)
		}
		return monthend
	case AtOpen:
		t := forDate.In(nyc)
		if t.After(time.Date(t.Year(), t.Month(), t.Day(), 9, 30, 0, 0, nyc)) {
			t = t.AddDate(0, 0, 1)
		}
		for !IsTradeDay(t) {
			t = t.AddDate(0, 0, 1)
		}
		return t
	case AtClose:
		t := forDate.In(nyc)
		if t.After(time.Date(t.Year(), t.Month(), t.Day(), 16, 0, 0, 0, nyc)) {
			t = t.AddDate(0, 0, 1)
		}
		for !IsTradeDay(t) {
			t = t.AddDate(0, 0, 1)
		}
		return t
	default:
		t := forDate.In(nyc)
		for !IsTradeDay(t) {
			t = t.AddDate(0, 0, 1)
		}
		t = tc.Schedule.Next(t)

		// if the time is outside the execution hours (actual hours depends on mode)
		switch tc.Mode {
		case RegularHours:
			marketOpen := time.Date(t.Year(), t.Month(), t.Day(), marketOpenHour, marketOpenMin, 0, 0, nyc)
			marketClose := time.Date(t.Year(), t.Month(), t.Day(), marketCloseHour, 0, 0, 0, nyc)
			if t.Before(marketOpen) {
				// date is before marketOpen snap to marketOpen
				t = marketOpen
			} else if t.After(marketClose) {
				// date is after marketClose, snap to marketOpen for next trade day
				t = marketOpen
				t = t.AddDate(0, 0, 1)
			}
		case ExtendedHours:
			marketOpen := time.Date(t.Year(), t.Month(), t.Day(), marketExtendedOpenHour, marketExtendedOpenMin, 0, 0, nyc)
			marketClose := time.Date(t.Year(), t.Month(), t.Day(), marketExtendedCloseHour, 0, 0, 0, nyc)
			if t.Before(marketOpen) {
				// date is before marketOpen snap to marketOpen
				t = marketOpen
			} else if t.After(marketClose) {
				// date is after marketClose, snap to marketOpen for next trade day
				t = marketOpen
				t = t.AddDate(0, 0, 1)
			}
		default:
			// don't do anything if all trading hours are allowed
		}

		for !IsTradeDay(t) {
			t = t.AddDate(0, 0, 1)
		}

		return t
	}
}
