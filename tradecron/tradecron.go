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

package tradecron

import (
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

const (
	AtOpen       = "@open"
	AtClose      = "@close"
	AtWeekBegin  = "@weekbegin"
	AtWeekEnd    = "@weekend"
	AtMonthBegin = "@monthbegin"
	AtMonthEnd   = "@monthend"
)

type MarketHours struct {
	Open  int
	Close int
}

type TradeCron struct {
	Schedule       cron.Schedule
	ScheduleString string
	TimeSpec       string
	TimeFlag       string
	DateFlag       string
	marketStatus   *MarketStatus
}

var (
	RegularHours = MarketHours{
		Open:  930,
		Close: 1600,
	}
	ExtendedHours = MarketHours{
		Open:  700,
		Close: 2000,
	}
)

// TradeCron enables market aware scheduling. It supports schedules via the standard
// CRON format of: Minutes(Min) Hours(H) DayOfMonth(DoM) Month(M) DayOfWeek(DoW)
// See: https://en.wikipedia.org/wiki/Cron
//
// '*' wildcards only execute during market open hours
//
// Additional market-aware modifiers are supported:
//
//	@open       - Run at market open; replaces Minute and Hour field
//	              e.g., @open * * *
//	@close      - Run at market close; replaces Minute and Hour field
//	@weekbegin  - Run on first trading day of week; replaces DayOfMonth field
//	@weekend    - Run on last trading day of week; replaces DayOfMonth field
//	@monthbegin - Run at market open or timespec on first trading day of month
//	@monthend   - Run at market close or timespec on last trading day of month
//
// Examples:
//   - every 5 minutes: */5 * * * *
//   - market open on tuesdays: @open * * 2
//   - 15 minutes after market open: 15 @open * * *
//   - market open on first trading day of week: @weekbegin
//   - market open on last trading day of month: @open @monthend
func New(cronSpec string, hours MarketHours) (*TradeCron, error) {
	specParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	scheduleStr := strings.TrimSpace(cronSpec)
	scheduleStr = expandBriefFormat(scheduleStr)

	// separate special tokens from timespec
	tokens := strings.Split(scheduleStr, " ")

	timeSpecTokens := make([]string, 0, 5)
	specialTokens := make([]string, 0, 2)
	for _, token := range tokens {
		if token[0] == '@' {
			specialTokens = append(specialTokens, token)
		} else {
			timeSpecTokens = append(timeSpecTokens, token)
		}
	}

	var timeSpec string
	var timeFlag string
	var dateFlag string
	var err error
	for _, token := range specialTokens {
		switch token {
		case AtOpen:
			if timeFlag != "" {
				return nil, ErrConflictingModifiers
			}
			if timeSpec, err = parseTimeRelativeTo(timeSpecTokens, hours.Open/100, hours.Open%100); err != nil {
				return nil, err
			}
			timeFlag = AtOpen
		case AtClose:
			if timeFlag != "" {
				return nil, ErrConflictingModifiers
			}
			if timeSpec, err = parseTimeRelativeTo(timeSpecTokens, hours.Close/100, hours.Close%100); err != nil {
				return nil, err
			}
			timeFlag = AtClose
		case AtWeekBegin:
			if dateFlag != "" {
				return nil, ErrConflictingModifiers
			}
			dateFlag = AtWeekBegin
		case AtWeekEnd:
			if dateFlag != "" {
				return nil, ErrConflictingModifiers
			}
			dateFlag = AtWeekEnd
		case AtMonthBegin:
			if dateFlag != "" {
				return nil, ErrConflictingModifiers
			}
			dateFlag = AtMonthBegin
		case AtMonthEnd:
			if dateFlag != "" {
				return nil, ErrConflictingModifiers
			}
			dateFlag = AtMonthEnd
		default:
			return nil, ErrUnknownModifier
		}
	}

	if timeSpec == "" {
		timeSpec = strings.Join(timeSpecTokens, " ")
	}

	schedule, err := specParser.Parse(timeSpec)
	if err != nil {
		log.Error().Err(err).Str("TimeSpec", timeSpec).Str("TradeCronSpec", cronSpec).Msg("robfig/cron could not parse timespec")
		return nil, err
	}

	tc := &TradeCron{
		Schedule:       schedule,
		ScheduleString: cronSpec,
		TimeSpec:       timeSpec,
		DateFlag:       dateFlag,
		TimeFlag:       timeFlag,
		marketStatus:   NewMarketStatus(&hours),
	}

	return tc, err
}

// IsTradeDay evaluates the given date against the schedule and returns true if the date falls
// on a trading day according to the schedule. The time portion of the schedule is ignored when
// evaluating this function
func (tc *TradeCron) IsTradeDay(forDate time.Time) bool {
	t1 := time.Date(forDate.Year(), forDate.Month(), forDate.Day(), 0, 0, 0, 0, tc.marketStatus.tz)
	t2 := t1.AddDate(0, 0, -1)
	t2 = time.Date(t2.Year(), t2.Month(), t2.Day(), 23, 59, 59, 999_999_999, tc.marketStatus.tz)
	next := tc.Next(t2)
	nextDate := time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, tc.marketStatus.tz)
	return nextDate.Equal(t1)
}

// Next returns the next tradeable date
func (tc *TradeCron) Next(forDate time.Time) time.Time {
	var checkDate time.Time

	nextDate := tc.Schedule.Next(forDate)

	// if there is a date flag in the schedule then fast-forward to the next possible date for checking
	switch tc.DateFlag {
	case AtWeekBegin:
		checkDate = tc.marketStatus.NextFirstTradingDayOfWeek(forDate)
		firstTradingDay := time.Date(checkDate.Year(), checkDate.Month(), checkDate.Day(), nextDate.Hour(), nextDate.Minute(), nextDate.Second(), nextDate.Nanosecond(), nextDate.Location())
		if nextDate.After(firstTradingDay) {
			// bump forward because next date is still after dt
			checkDate = tc.marketStatus.NextFirstTradingDayOfWeek(nextDate)
		}
	case AtWeekEnd:
		checkDate = tc.marketStatus.NextLastTradingDayOfWeek(forDate)
		firstTradingDay := time.Date(checkDate.Year(), checkDate.Month(), checkDate.Day(), nextDate.Hour(), nextDate.Minute(), nextDate.Second(), nextDate.Nanosecond(), nextDate.Location())
		if nextDate.After(firstTradingDay) {
			// bump forward because next date is still after dt
			checkDate = tc.marketStatus.NextLastTradingDayOfWeek(nextDate)
		}
	case AtMonthBegin:
		// get the first trading day of the current month
		lastMonth := time.Date(forDate.Year(), forDate.Month(), 1, 23, 59, 59, 999_999_999, tc.marketStatus.tz).AddDate(0, 0, -1)
		firstTradingDayOfThisMonth := tc.marketStatus.NextFirstTradingDayOfMonth(lastMonth)
		dateOnly := time.Date(forDate.Year(), forDate.Month(), forDate.Day(), 0, 0, 0, 0, tc.marketStatus.tz)
		if firstTradingDayOfThisMonth.After(dateOnly) || firstTradingDayOfThisMonth.Equal(dateOnly) {
			checkDate = firstTradingDayOfThisMonth
		} else {
			// it's after the first trading day of this month, get next month first trading day
			checkDate = tc.marketStatus.NextFirstTradingDayOfMonth(forDate)
			firstTradingDay := time.Date(checkDate.Year(), checkDate.Month(), checkDate.Day(), nextDate.Hour(), nextDate.Minute(), nextDate.Second(), nextDate.Nanosecond(), nextDate.Location())
			if nextDate.After(firstTradingDay) {
				// bump forward because next date is still after dt
				checkDate = tc.marketStatus.NextFirstTradingDayOfMonth(nextDate)
			}
		}
	case AtMonthEnd:
		checkDate = tc.marketStatus.NextLastTradingDayOfMonth(forDate)
		firstTradingDay := time.Date(checkDate.Year(), checkDate.Month(), checkDate.Day(), nextDate.Hour(), nextDate.Minute(), nextDate.Second(), nextDate.Nanosecond(), nextDate.Location())
		if nextDate.After(firstTradingDay) {
			// bump forward because next date is still after dt
			checkDate = tc.marketStatus.NextLastTradingDayOfMonth(nextDate)
		}
	default:
		checkDate = forDate
	}

	marketOpen := false
	maxIters := 5000
	actualIters := 0
	for !marketOpen {
		checkDate = tc.Schedule.Next(checkDate)
		marketOpen = tc.marketStatus.IsMarketOpen(checkDate)
		if actualIters > maxIters {
			log.Panic().Str("TimeSpec", tc.TimeSpec).Msg("something is wrong with tradecron schedule as it appears to be in an infinite loop")
		}
		actualIters++
	}

	return checkDate
}
