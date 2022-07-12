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
	OpenHour    int
	OpenMinute  int
	CloseHour   int
	CloseMinute int
}

type TradeCron struct {
	Schedule       cron.Schedule
	ScheduleString string
	TimeSpec       string
	TimeFlag       string
	DateFlag       string
}

var (
	RegularHours = MarketHours{
		OpenHour:    9,
		OpenMinute:  30,
		CloseHour:   16,
		CloseMinute: 0,
	}
	ExtendedHours = MarketHours{
		OpenHour:    7,
		OpenMinute:  0,
		CloseHour:   20,
		CloseMinute: 0,
	}
)

// TradeCron enables market aware scheduling. It supports the standard
// CRON format of: Minutes(Min) Hours(H) DayOfMonth(DoM) Month(M) DayOfWeek(DoW)
// See: https://en.wikipedia.org/wiki/Cron
//
// Additional market-aware modifiers are supported:
//     @open       - Run at market open; replaces Minute and Hour field
//                   e.g., @open * * *
//     @close      - Run at market close; replaces Minute and Hour field
//     @weekbegin  - Run on first trading day of week; replaces DayOfMonth field
//     @weekend    - Run on last trading day of week; replaces DayOfMonth field
//     @monthbegin - Run at market open or timespec on first trading day of month
//     @monthend   - Run at market close or timespec on last trading day of month
//
// Examples:
//     - every 5 minutes: */5 * * * *
//     - market open on tuesdays: @open * * 2
//     - 15 minutes after market open: 15 @open * * *
//     - market open on first trading day of week: @weekbegin
//     - market open on last trading day of month: @open @monthend
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
			if timeSpec, err = parseTimeRelativeTo(timeSpecTokens, hours.OpenHour, hours.OpenMinute); err != nil {
				return nil, err
			}
			timeFlag = AtOpen
		case AtClose:
			if timeFlag != "" {
				return nil, ErrConflictingModifiers
			}
			if timeSpec, err = parseTimeRelativeTo(timeSpecTokens, hours.CloseHour, hours.CloseMinute); err != nil {
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

	switch tc.DateFlag {
	case AtMonthEnd:
		// get last trading day of month
		now := forDate.In(nyc)
		monthend := MonthEnd(now)
		if now.After(monthend) {
			now = NextMonth(now)
			monthend = MonthEnd(now)
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

		/*
			// if the time is outside the execution hours (actual hours depends on mode)
			switch tc.TimeMode {
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
		*/

		for !IsTradeDay(t) {
			t = t.AddDate(0, 0, 1)
		}

		return t
	}
}