// Copyright 2021-2026
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

package data

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PeriodUnit identifies the calendar unit of a Period.
type PeriodUnit int

const (
	UnitDay PeriodUnit = iota
	UnitMonth
	UnitYear
	UnitYTD         // year-to-date: from Jan 1 of the current year
	UnitMTD         // month-to-date: from the 1st of the current month
	UnitWTD         // week-to-date: from the most recent Monday
	UnitMinuteBar   // last N 1-minute bars before ref
	UnitDailyAtTime // last N trading days, sampling the 1-minute bar at TimeOfDay
)

// Period represents a calendar-aware duration used for performance metric
// windows. Unlike time.Duration, it handles variable-length units like
// months and years correctly.
//
// For UnitDailyAtTime, TimeOfDay carries the wall-clock minute(s) of the
// trading day to sample. For all other units, TimeOfDay is empty.
type Period struct {
	N         int
	Unit      PeriodUnit
	TimeOfDay []TimeOfDay
}

// TimeOfDay is a wall-clock minute within the trading day, expressed in
// market-local time (Eastern). Hour is 0-23, Minute is 0-59.
type TimeOfDay struct {
	Hour   int
	Minute int
}

// MinutesSinceMidnight returns the time-of-day as minutes past midnight
// (0-1439). Used to push down sparse intraday filters to ClickHouse.
func (t TimeOfDay) MinutesSinceMidnight() int { return t.Hour*60 + t.Minute }

// Days returns a Period of n calendar days.
func Days(n int) Period { return Period{N: n, Unit: UnitDay} }

// Months returns a Period of n calendar months.
func Months(n int) Period { return Period{N: n, Unit: UnitMonth} }

// Years returns a Period of n calendar years.
func Years(n int) Period { return Period{N: n, Unit: UnitYear} }

// YTD returns a Period representing year-to-date.
func YTD() Period { return Period{N: 0, Unit: UnitYTD} }

// MTD returns a Period representing month-to-date.
func MTD() Period { return Period{N: 0, Unit: UnitMTD} }

// WTD returns a Period representing week-to-date.
func WTD() Period { return Period{N: 0, Unit: UnitWTD} }

// MinuteBars returns a Period representing the last N 1-minute bars before
// the engine's current time. The result contains a contiguous, dense set
// of minute bars and is served from the intraday data source.
func MinuteBars(n int) Period { return Period{N: n, Unit: UnitMinuteBar} }

// DailyAtTime returns a Period that, for each of the last N trading days,
// pulls the 1-minute bar at the specified wall-clock time-of-day (Eastern).
// timeOfDay is given as "HH:MM" -- e.g. "10:00" or "15:30". Multiple times
// may be specified by passing a comma-separated list, e.g. "10:00,14:00";
// the result interleaves rows in chronological order.
//
// DailyAtTime is sparse: rather than scanning every minute bar in the
// range, only the bars matching timeOfDay are returned. The time-of-day
// predicate is pushed down to the underlying intraday store.
//
// The function panics if timeOfDay is malformed; this is an authoring
// error and would never silently produce wrong data.
func DailyAtTime(timeOfDay string, count int) Period {
	tods, err := parseTimesOfDay(timeOfDay)
	if err != nil {
		panic(fmt.Sprintf("data.DailyAtTime: %v", err))
	}

	return Period{N: count, Unit: UnitDailyAtTime, TimeOfDay: tods}
}

// parseTimesOfDay parses a comma-separated list of "HH:MM" strings into
// TimeOfDay values. Returns an error on malformed input.
func parseTimesOfDay(spec string) ([]TimeOfDay, error) {
	parts := strings.Split(spec, ",")
	out := make([]TimeOfDay, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)

		hh, mm, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, fmt.Errorf("time-of-day %q: expected HH:MM", trimmed)
		}

		hour, err := strconv.Atoi(hh)
		if err != nil {
			return nil, fmt.Errorf("time-of-day %q: hour: %w", trimmed, err)
		}

		minute, err := strconv.Atoi(mm)
		if err != nil {
			return nil, fmt.Errorf("time-of-day %q: minute: %w", trimmed, err)
		}

		if hour < 0 || hour > 23 {
			return nil, fmt.Errorf("time-of-day %q: hour out of range [0,23]", trimmed)
		}

		if minute < 0 || minute > 59 {
			return nil, fmt.Errorf("time-of-day %q: minute out of range [0,59]", trimmed)
		}

		out = append(out, TimeOfDay{Hour: hour, Minute: minute})
	}

	return out, nil
}

// Before returns the start time of the period ending at ref.
func (p Period) Before(ref time.Time) time.Time {
	switch p.Unit {
	case UnitDay:
		return ref.AddDate(0, 0, -p.N)
	case UnitMonth:
		// Snap to the 1st of ref's month, then go back N-1 months.
		// This guarantees the window always starts at a month boundary
		// so that a monthly downsample yields exactly N rows.
		first := time.Date(ref.Year(), ref.Month(), 1,
			ref.Hour(), ref.Minute(), ref.Second(), ref.Nanosecond(), ref.Location())

		return first.AddDate(0, -(p.N - 1), 0)
	case UnitYear:
		return ref.AddDate(-p.N, 0, 0)
	case UnitYTD:
		return time.Date(ref.Year(), 1, 1, 0, 0, 0, 0, ref.Location())
	case UnitMTD:
		return time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
	case UnitWTD:
		offset := int(ref.Weekday()) - int(time.Monday)
		if offset < 0 {
			offset += 7
		}

		return ref.AddDate(0, 0, -offset)
	case UnitMinuteBar:
		return ref.Add(-time.Duration(p.N) * time.Minute)
	case UnitDailyAtTime:
		// Walk back N trading-equivalent calendar days. The intraday
		// fetch path uses the date range [start, ref] together with the
		// TimeOfDay predicate to materialize exactly N samples per
		// time-of-day per asset; calendar-day arithmetic here is a safe
		// upper bound (weekend/holiday rows simply do not exist in the
		// intraday store).
		return ref.AddDate(0, 0, -p.N)
	default:
		return ref
	}
}

// IsIntraday reports whether this period requests intraday-frequency data.
// Periods with intraday units route to the intraday backend; all other
// units route to the daily/end-of-day path.
func (p Period) IsIntraday() bool {
	return p.Unit == UnitMinuteBar || p.Unit == UnitDailyAtTime
}
