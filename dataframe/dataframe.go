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

package dataframe

import (
	"time"

	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
)

// Len returns the number of rows in the dataframe
func (df *DataFrame) Len() int {
	return len(df.Dates)
}

// ColCount returns the number of columns in the dataframe
func (df *DataFrame) ColCount() int {
	return len(df.ColNames)
}

// Frequency returns a data frame filtered to the requested frequency; note this is not
// an in-place function but creates a copy of the data
func (df *DataFrame) Frequency(frequency Frequency) *DataFrame {
	var schedule *tradecron.TradeCron
	var err error

	switch frequency {
	case Daily:
		schedule, err = tradecron.New("@close * * *", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close * * *").Msg("could not build tradecron schedule")
		}
	case WeekBegin:
		schedule, err = tradecron.New("@close @weekbegin", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @weekbegin").Msg("could not build tradecron schedule")
		}
	case WeekEnd:
		schedule, err = tradecron.New("@close @weekend", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @weekend").Msg("could not build tradecron schedule")
		}
	case MonthBegin:
		schedule, err = tradecron.New("@close @monthbegin", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthbegin").Msg("could not build tradecron schedule")
		}
	case MonthEnd:
		schedule, err = tradecron.New("@close @monthend", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthend").Msg("could not build tradecron schedule")
		}
	case YearBegin:
		schedule, err = tradecron.New("@close @monthend 12 *", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthend 12 *").Msg("could not build tradecron schedule")
		}
	case YearEnd:
		schedule, err = tradecron.New("@close @monthbegin 1 *", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthbegin 1 *").Msg("could not build tradecron schedule")
		}
	}

	newDates := make([]time.Time, 0, len(df.Dates))
	newVals := make([][]float64, len(df.ColNames))
	for rowIdx, xx := range df.Dates {
		if schedule.IsTradeDay(xx) {
			newDates = append(newDates, xx)
			for colIdx := range newVals {
				newVals[colIdx] = append(newVals[colIdx], df.Vals[colIdx][rowIdx])
			}
		}
	}

	newDf := &DataFrame{
		Dates:    newDates,
		ColNames: df.ColNames,
		Vals:     newVals,
	}

	return newDf
}
