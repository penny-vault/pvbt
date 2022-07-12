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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

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

// MonthEnd returns the last trading day of the specified month
func MonthEnd(t time.Time) time.Time {
	var nyc *time.Location
	var err error
	if nyc, err = time.LoadLocation("America/New_York"); err != nil {
		log.Panic().Err(err).Msg("could not load nyc timezone")
	}

	firstOfMonth := time.Date(t.Year(), t.Month(), 1, 16, 0, 0, 0, nyc)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	for !IsTradeDay(lastOfMonth) {
		lastOfMonth = lastOfMonth.AddDate(0, 0, -1)
	}

	return lastOfMonth
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

// expandBriefFormat expands a timespec that has fields ommitted for brevity
func expandBriefFormat(spec string) string {
	tokens := strings.Split(spec, " ")

	// count the number of special tokens
	special := 0
	for _, token := range tokens {
		if token[0] == '@' {
			special++
		}
	}

	expectedLength := 5 + special
	for len(tokens) < expectedLength {
		tokens = append(tokens, "*")
	}

	return strings.Join(tokens, " ")
}

// parseTimeRelativeTo parse a set of tokens relative to the specified time
func parseTimeRelativeTo(tokens []string, hours int, minutes int) (string, error) {
	// parse minutes
	var mins int
	var err error
	if tokens[0] == "*" {
		mins = 0
	} else {
		if mins, err = strconv.Atoi(tokens[0]); err != nil {
			log.Error().Str("MinutesToken", tokens[0]).Msg("could not parse minutes token")
			return "", ErrMalformedTimeSpec
		}
	}

	// parse hours
	var hrs int
	if tokens[1] == "*" {
		hrs = 0
	} else {
		if hrs, err = strconv.Atoi(tokens[1]); err != nil {
			log.Error().Str("HoursToken", tokens[1]).Msg("could not parse hours token")
			return "", ErrMalformedTimeSpec
		}
	}

	// apply mins and hours
	mins += minutes

	// if mins is actually hours, roll over to hours
	if mins > 59 || mins < -59 {
		hrs += (mins / 60)
		mins = mins % 60
	}

	hrs += hours

	if mins < 0 {
		mins = 60 + mins
		hrs -= 1
	}

	if hrs < 0 {
		return "", ErrFieldOutOfBounds
	}

	if hrs > 23 {
		return "", ErrFieldOutOfBounds
	}

	result := fmt.Sprintf("%d %d %s %s %s", mins, hrs, tokens[2], tokens[3], tokens[4])
	return result, nil
}
