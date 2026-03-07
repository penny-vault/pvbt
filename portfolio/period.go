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

package portfolio

// PeriodUnit identifies the calendar unit of a Period.
type PeriodUnit int

const (
	UnitDay PeriodUnit = iota
	UnitMonth
	UnitYear
)

// Period represents a calendar-aware duration used for performance metric
// windows. Unlike time.Duration, it handles variable-length units like
// months and years correctly.
type Period struct {
	N    int
	Unit PeriodUnit
}

// Days returns a Period of n calendar days.
func Days(n int) Period { return Period{N: n, Unit: UnitDay} }

// Months returns a Period of n calendar months.
func Months(n int) Period { return Period{N: n, Unit: UnitMonth} }

// Years returns a Period of n calendar years.
func Years(n int) Period { return Period{N: n, Unit: UnitYear} }
