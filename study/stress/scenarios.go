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

package stress

import "time"

// Scenario describes a historical market stress period with a name, description,
// and the date range over which the stress event occurred.
type Scenario struct {
	Name        string
	Description string
	Start       time.Time
	End         time.Time
}

// DefaultScenarios returns the built-in set of historical market stress scenarios
// used when no custom scenarios are provided to New.
func DefaultScenarios() []Scenario {
	return []Scenario{
		{
			Name:        "2008 Financial Crisis",
			Description: "Global financial crisis triggered by subprime mortgage collapse",
			Start:       time.Date(2008, 9, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2009, 3, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "COVID Crash",
			Description: "Rapid market decline due to COVID-19 pandemic",
			Start:       time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2020, 3, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2022 Rate Hiking Cycle",
			Description: "Federal Reserve aggressive rate hikes to combat inflation",
			Start:       time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2022, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "Dot-com Bust",
			Description: "Collapse of the dot-com bubble",
			Start:       time.Date(2000, 3, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2002, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2015-2017 Low-Volatility Grind",
			Description: "Extended low-volatility period with no clear trends",
			Start:       time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2017, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2011 Debt Ceiling Crisis",
			Description: "US debt ceiling standoff and S&P downgrade",
			Start:       time.Date(2011, 7, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2011, 10, 31, 0, 0, 0, 0, time.UTC),
		},
	}
}
