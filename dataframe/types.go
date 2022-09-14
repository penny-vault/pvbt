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

import "time"

type DataFrame struct {
	dates []time.Time
	vals  [][]float64
}

// Defines
type Frequency string

const (
	Daily      Frequency = "Daily"
	WeekBegin  Frequency = "WeekBegin"
	WeekEnd    Frequency = "WeekEnd"
	MonthBegin Frequency = "MonthBegin"
	MonthEnd   Frequency = "MonthEnd"
	YearBegin  Frequency = "YearBegin"
	YearEnd    Frequency = "YearEnd"
)
