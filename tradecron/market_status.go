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

package tradecron

import (
	"sync"
	"time"
)

var (
	holidays      map[int64]int
	holidayLocker sync.RWMutex

	nyc     *time.Location
	nycOnce sync.Once
)

// HolidaysInitialized returns true if SetMarketHolidays has been called.
func HolidaysInitialized() bool {
	holidayLocker.RLock()
	defer holidayLocker.RUnlock()

	return holidays != nil
}

// requireHolidays panics if SetMarketHolidays has not been called. This
// catches misconfigured commands that forget to load the holiday calendar,
// which would silently treat every day as a trading day.
func requireHolidays() {
	if holidays == nil {
		panic("tradecron: market holidays not initialized -- call SetMarketHolidays before using market calendar functions")
	}
}

// MarketHoliday describes a single market holiday or early-close day.
type MarketHoliday struct {
	Date       time.Time
	EarlyClose bool
	CloseTime  int // e.g., 1300; 0 for full holiday
}

// MarketStatus tracks market hours and holidays for determining trading availability.
type MarketStatus struct {
	marketHours *MarketHours
	tz          *time.Location
}

// SetMarketHolidays replaces the global holiday calendar with the provided data.
func SetMarketHolidays(items []MarketHoliday) {
	nyc := mustLoadNewYork()

	holidayLocker.Lock()
	defer holidayLocker.Unlock()

	holidays = make(map[int64]int, len(items))
	for _, h := range items {
		dt := time.Date(h.Date.Year(), h.Date.Month(), h.Date.Day(), 0, 0, 0, 0, nyc)
		if h.EarlyClose {
			holidays[dt.Unix()] = h.CloseTime
		} else {
			holidays[dt.Unix()] = 0
		}
	}
}

// EarlyClose returns close time of an early close market day, e.g. 1300
func (ms *MarketStatus) EarlyClose(checkTime time.Time) int {
	holidayLocker.RLock()
	defer holidayLocker.RUnlock()

	requireHolidays()

	d := time.Date(checkTime.Year(), checkTime.Month(), checkTime.Day(), 0, 0, 0, 0, ms.tz)
	if marketClose, ok := holidays[d.Unix()]; ok {
		return marketClose
	}

	return 0
}

// IsMarketHoliday returns true if the specified date is a market holiday
func (ms *MarketStatus) IsMarketHoliday(checkTime time.Time) bool {
	holidayLocker.RLock()
	defer holidayLocker.RUnlock()

	requireHolidays()

	d := time.Date(checkTime.Year(), checkTime.Month(), checkTime.Day(), 0, 0, 0, 0, ms.tz)

	marketHoliday, isHoliday := holidays[d.Unix()]
	if marketHoliday != 0 {
		// Non-zero means early close, not a full holiday.
		return false
	}

	return isHoliday
}

// IsMarketOpen returns true if the specified time is during market hours
// (i.e. not a market holiday or weekend)
func (ms *MarketStatus) IsMarketOpen(checkTime time.Time) bool {
	if !ms.IsMarketDay(checkTime) {
		return false
	}

	// check time
	closeTime := ms.marketHours.Close

	earlyClose := ms.EarlyClose(checkTime)
	if earlyClose != 0 {
		closeTime = earlyClose
	}

	timeOfDay := checkTime.Hour()*100 + checkTime.Minute()
	if timeOfDay < ms.marketHours.Open || timeOfDay > closeTime {
		return false
	}

	return true
}

// IsMarketDay returns true if the specified date is a valid trading day
// (i.e. not a market holiday or weekend)
func (ms *MarketStatus) IsMarketDay(checkTime time.Time) bool {
	if checkTime.Weekday() == time.Saturday || checkTime.Weekday() == time.Sunday {
		return false
	}

	isHoliday := ms.IsMarketHoliday(checkTime)

	return !isHoliday
}

// NewMarketStatus creates a new MarketStatus for the given market hours.
func NewMarketStatus(hours *MarketHours) *MarketStatus {
	return &MarketStatus{
		marketHours: hours,
		tz:          mustLoadNewYork(),
	}
}

// NextFirstTradingDayOfMonth returns the first trading day of the next month
func (ms *MarketStatus) NextFirstTradingDayOfMonth(t time.Time) time.Time {
	// construct a new date for the first of the month
	firstOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, ms.tz)
	// add a month to the date
	firstOfMonth = firstOfMonth.AddDate(0, 1, 0)

	// Check if the market is open on the date
	marketOpen := false

	for !marketOpen {
		marketOpen = ms.IsMarketDay(firstOfMonth)
		if !marketOpen {
			firstOfMonth = firstOfMonth.AddDate(0, 0, 1)
		}
	}

	return firstOfMonth
}

// NextFirstTradingDayOfWeek returns the first trading day of the week.
func (ms *MarketStatus) NextFirstTradingDayOfWeek(t time.Time) time.Time {
	daysToWeekBegin := (8 - t.Weekday()) % 7
	weekStart := t.AddDate(0, 0, int(daysToWeekBegin))

	marketOpen := false

	for !marketOpen {
		marketOpen = ms.IsMarketDay(weekStart)
		if !marketOpen {
			weekStart = weekStart.AddDate(0, 0, 1)
		}
	}

	// adjust weekStart to midnight
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, ms.tz)

	return weekStart
}

// NextLastTradingDayOfMonth returns the next last trading day of month that
// is on or after t's calendar date. If t is already past the last trading
// day of its month, the last trading day of the following month is returned.
func (ms *MarketStatus) NextLastTradingDayOfMonth(t time.Time) time.Time {
	tDate := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ms.tz)
	firstOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, ms.tz)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	marketOpen := false

	for !marketOpen {
		marketOpen = ms.IsMarketDay(lastOfMonth)
		if !marketOpen {
			lastOfMonth = lastOfMonth.AddDate(0, 0, -1)
		}
	}

	// If the last trading day is before t, advance to the next month.
	if lastOfMonth.Before(tDate) {
		nextMonth := firstOfMonth.AddDate(0, 1, 0)
		return ms.NextLastTradingDayOfMonth(nextMonth)
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

// mustLoadNewYork returns the America/New_York timezone location.
// The result is cached after the first call.
func mustLoadNewYork() *time.Location {
	nycOnce.Do(func() {
		var err error

		nyc, err = time.LoadLocation("America/New_York")
		if err != nil {
			panic("tradecron: could not load America/New_York timezone: " + err.Error())
		}
	})

	return nyc
}
