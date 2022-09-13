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

package data

import (
	"time"

	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
)

// FilterDays takes an array of dates and filters it based on the requested frequency
func FilterDays(frequency Frequency, res []time.Time) []time.Time {
	days := make([]time.Time, 0, 252)

	var schedule *tradecron.TradeCron
	var err error

	switch frequency {
	case FrequencyDaily:
		schedule, err = tradecron.New("@close * * *", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close * * *").Msg("could not build tradecron schedule")
		}
	case FrequencyWeekly:
		schedule, err = tradecron.New("@close @weekend", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @weekend").Msg("could not build tradecron schedule")
		}
	case FrequencyMonthly:
		schedule, err = tradecron.New("@close @monthend", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthend").Msg("could not build tradecron schedule")
		}
	case FrequencyAnnually:
		schedule, err = tradecron.New("@close @monthend 12 *", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthend 12 *").Msg("could not build tradecron schedule")
		}
	}

	for _, xx := range res {
		if schedule.IsTradeDay(xx) {
			days = append(days, xx)
		}
	}
	return days
}
