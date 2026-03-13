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

package engine

import (
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/portfolio"
)

// standardWindows returns the fixed set of metric windows.
func standardWindows() []portfolio.Period {
	return []portfolio.Period{
		portfolio.Years(5),
		portfolio.Years(3),
		portfolio.Years(1),
		portfolio.YTD(),
		portfolio.MTD(),
		portfolio.WTD(),
	}
}

// computeMetrics computes all registered metrics on the account for
// the given date across all standard windows plus since-inception.
func computeMetrics(acct *portfolio.Account, date time.Time) {
	for _, m := range acct.RegisteredMetrics() {
		// Since inception (nil window).
		val, err := m.Compute(acct, nil)
		if err == nil {
			acct.AppendMetric(portfolio.MetricRow{
				Date:   date,
				Name:   m.Name(),
				Window: "since_inception",
				Value:  val,
			})
		}

		// Standard windows.
		for _, w := range standardWindows() {
			wCopy := w
			val, err := m.Compute(acct, &wCopy)
			if err == nil {
				acct.AppendMetric(portfolio.MetricRow{
					Date:   date,
					Name:   m.Name(),
					Window: windowLabel(w),
					Value:  val,
				})
			}
		}
	}
}

// windowLabel returns a human-readable label for a Period.
func windowLabel(p portfolio.Period) string {
	switch p.Unit {
	case portfolio.UnitYear:
		return fmt.Sprintf("%dyr", p.N)
	case portfolio.UnitMonth:
		return fmt.Sprintf("%dmo", p.N)
	case portfolio.UnitDay:
		return fmt.Sprintf("%dd", p.N)
	case portfolio.UnitYTD:
		return "ytd"
	case portfolio.UnitMTD:
		return "mtd"
	case portfolio.UnitWTD:
		return "wtd"
	default:
		return "unknown"
	}
}
