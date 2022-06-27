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

package data

import (
	"time"

	dataframe "github.com/rocketlaunchr/dataframe-go"
)

func dateMatchForFrequency(currDate time.Time, nextDate time.Time, frequency Frequency) bool {
	switch frequency {
	case FrequencyDaily:
		return true
	case FrequencyWeekly:
		currYear, currWeek := currDate.ISOWeek()
		nextYear, nextWeek := nextDate.ISOWeek()
		if currYear != nextYear || currWeek != nextWeek {
			return true
		}
	case FrequencyMonthly:
		if currDate.Month() != nextDate.Month() {
			return true
		}
	case FrequencyAnnualy:
		if currDate.Year() != nextDate.Year() {
			return true
		}
	}
	return false
}

func partitionArray(xs []string, chunkSize int) [][]string {
	if len(xs) == 0 {
		return nil
	}
	divided := make([][]string, (len(xs)+chunkSize-1)/chunkSize)
	prev := 0
	i := 0
	till := len(xs) - chunkSize
	for prev < till {
		next := prev + chunkSize
		divided[i] = xs[prev:next]
		prev = next
		i++
	}
	divided[i] = xs[prev:]
	return divided
}

type quoteResult struct {
	Ticker string
	Data   *dataframe.DataFrame
	Err    error
}
