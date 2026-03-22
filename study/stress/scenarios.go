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
			Name:        "1973-74 Oil Embargo Bear Market",
			Description: "Prolonged bear market driven by the OPEC oil embargo and stagflation",
			Start:       time.Date(1973, 1, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(1974, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "Volcker Tightening",
			Description: "Federal Reserve raised rates to 20% to break double-digit inflation",
			Start:       time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(1982, 8, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "1987 Black Monday",
			Description: "Single-day market crash of 22%, the largest one-day percentage decline in history",
			Start:       time.Date(1987, 10, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(1987, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "1994 Bond Massacre",
			Description: "Unexpected Federal Reserve rate hikes caused the worst bond market losses in decades",
			Start:       time.Date(1994, 2, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(1994, 11, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "1998 LTCM / Russian Crisis",
			Description: "Russian debt default and Long-Term Capital Management collapse triggered a global liquidity crisis",
			Start:       time.Date(1998, 8, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(1998, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "Dot-com Bubble",
			Description: "Speculative mania in technology stocks; strategies not chasing momentum appeared to underperform",
			Start:       time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2000, 3, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "Dot-com Bust",
			Description: "Collapse of the dot-com bubble",
			Start:       time.Date(2000, 3, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2002, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "9/11",
			Description: "Markets closed for four trading days then reopened to sharp declines",
			Start:       time.Date(2001, 9, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2001, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2008 Financial Crisis",
			Description: "Global financial crisis triggered by subprime mortgage collapse",
			Start:       time.Date(2008, 9, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2009, 3, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2010 Flash Crash",
			Description: "Dow dropped nearly 1,000 points intraday before recovering within minutes",
			Start:       time.Date(2010, 5, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2010, 6, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "Euro Debt Crisis",
			Description: "Sovereign debt contagion across Greece, Ireland, Portugal, Spain, and Italy",
			Start:       time.Date(2010, 4, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2011, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2011 Debt Ceiling Crisis",
			Description: "US debt ceiling standoff and S&P downgrade",
			Start:       time.Date(2011, 7, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2011, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2015-2017 Low-Volatility Grind",
			Description: "Extended low-volatility period with no clear trends",
			Start:       time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2017, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2018 Q4 Selloff",
			Description: "Sharp equity decline driven by Fed tightening fears and trade war escalation",
			Start:       time.Date(2018, 10, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2018, 12, 31, 0, 0, 0, 0, time.UTC),
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
			Name:        "2023 Regional Banking Crisis",
			Description: "Collapse of Silicon Valley Bank and Signature Bank triggered fears of systemic contagion",
			Start:       time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2023, 5, 31, 0, 0, 0, 0, time.UTC),
		},
	}
}
