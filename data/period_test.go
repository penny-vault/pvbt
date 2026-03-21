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

package data_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// buildWeekdayDF creates a daily DataFrame of weekday-only data from start
// through end (exclusive), with a single asset and metric.
func buildWeekdayDF(start, end time.Time) *data.DataFrame {
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	var times []time.Time
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		times = append(times, day)
	}
	col := make([]float64, len(times))
	for i := range col {
		col[i] = 100.0 + float64(i)*0.1
	}
	df, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{col})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return df
}

var _ = Describe("Period", func() {
	ref := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	Describe("Before", func() {
		It("subtracts days", func() {
			p := data.Days(10)
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC)))
		})

		It("subtracts months from a month boundary", func() {
			// Months(N) snaps to the 1st of ref's month, then goes back N-1
			// months so a monthly downsample yields exactly N rows.
			// Months(2).Before(Mar 15) -> 1st of Mar - 1 month = Feb 1
			p := data.Months(2)
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("subtracts years", func() {
			p := data.Years(1)
			Expect(p.Before(ref)).To(Equal(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)))
		})

		It("returns Jan 1 for YTD", func() {
			p := data.YTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("returns 1st of month for MTD", func() {
			p := data.MTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("returns most recent Monday for WTD", func() {
			p := data.WTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)))
		})

		It("returns ref for WTD when ref is Monday", func() {
			monday := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
			p := data.WTD()
			Expect(p.Before(monday)).To(Equal(monday))
		})
	})

	Describe("Months boundary consistency", func() {
		Context("Before produces the 1st of the target month", func() {
			DescribeTable("Months(N).Before snaps to month boundary",
				func(refDate time.Time, months int, expectedStart time.Time) {
					p := data.Months(months)
					Expect(p.Before(refDate)).To(Equal(expectedStart))
				},
				// 31-day months
				Entry("Jan 31, Months(7)", time.Date(2024, 1, 31, 16, 0, 0, 0, time.UTC), 7, time.Date(2023, 7, 1, 16, 0, 0, 0, time.UTC)),
				Entry("Oct 31, Months(7)", time.Date(2024, 10, 31, 16, 0, 0, 0, time.UTC), 7, time.Date(2024, 4, 1, 16, 0, 0, 0, time.UTC)),
				Entry("Dec 31, Months(7)", time.Date(2024, 12, 31, 16, 0, 0, 0, time.UTC), 7, time.Date(2024, 6, 1, 16, 0, 0, 0, time.UTC)),
				// 30-day months
				Entry("Apr 30, Months(7)", time.Date(2024, 4, 30, 16, 0, 0, 0, time.UTC), 7, time.Date(2023, 10, 1, 16, 0, 0, 0, time.UTC)),
				Entry("Sep 30, Months(7)", time.Date(2024, 9, 30, 16, 0, 0, 0, time.UTC), 7, time.Date(2024, 3, 1, 16, 0, 0, 0, time.UTC)),
				// February (short month)
				Entry("Feb 28, Months(7)", time.Date(2025, 2, 28, 16, 0, 0, 0, time.UTC), 7, time.Date(2024, 8, 1, 16, 0, 0, 0, time.UTC)),
				Entry("Feb 29 leap, Months(7)", time.Date(2024, 2, 29, 16, 0, 0, 0, time.UTC), 7, time.Date(2023, 8, 1, 16, 0, 0, 0, time.UTC)),
				// Year boundary crossings
				Entry("Mar 31, Months(7) crosses year", time.Date(2024, 3, 29, 16, 0, 0, 0, time.UTC), 7, time.Date(2023, 9, 1, 16, 0, 0, 0, time.UTC)),
				Entry("Jan 31, Months(12) full year", time.Date(2025, 1, 31, 16, 0, 0, 0, time.UTC), 12, time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC)),
				// Small lookbacks
				Entry("Months(1) gives just current month", time.Date(2024, 10, 31, 16, 0, 0, 0, time.UTC), 1, time.Date(2024, 10, 1, 16, 0, 0, 0, time.UTC)),
				Entry("Months(2)", time.Date(2024, 10, 31, 16, 0, 0, 0, time.UTC), 2, time.Date(2024, 9, 1, 16, 0, 0, 0, time.UTC)),
				// Early-close day (13:00 instead of 16:00)
				Entry("Nov 29 early close, Months(7)", time.Date(2024, 11, 29, 13, 0, 0, 0, time.UTC), 7, time.Date(2024, 5, 1, 13, 0, 0, 0, time.UTC)),
				// Mid-month ref (not month-end)
				Entry("mid-month Mar 15, Months(3)", time.Date(2025, 3, 15, 16, 0, 0, 0, time.UTC), 3, time.Date(2025, 1, 1, 16, 0, 0, 0, time.UTC)),
			)
		})

		Context("Months(N) downsampled to monthly always yields exactly N rows", func() {
			var dailyDF *data.DataFrame

			BeforeEach(func() {
				dailyDF = buildWeekdayDF(
					time.Date(2023, 1, 1, 16, 0, 0, 0, time.UTC),
					time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
				)
			})

			DescribeTable("consistent row counts",
				func(monthEnd time.Time, months int) {
					lookback := data.Months(months)
					windowed := dailyDF.Between(lookback.Before(monthEnd), monthEnd)
					monthly := windowed.Downsample(data.Monthly).Last()
					Expect(monthly.Len()).To(Equal(months),
						"Months(%d) from %s: expected %d rows, got %d",
						months, monthEnd.Format("2006-01-02"), months, monthly.Len())
				},
				// Every month of 2024 with Months(7) at 16:00
				Entry("Jan 2024", time.Date(2024, 1, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Feb 2024 (leap)", time.Date(2024, 2, 29, 16, 0, 0, 0, time.UTC), 7),
				Entry("Mar 2024", time.Date(2024, 3, 29, 16, 0, 0, 0, time.UTC), 7),
				Entry("Apr 2024", time.Date(2024, 4, 30, 16, 0, 0, 0, time.UTC), 7),
				Entry("May 2024", time.Date(2024, 5, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Jun 2024", time.Date(2024, 6, 28, 16, 0, 0, 0, time.UTC), 7),
				Entry("Jul 2024", time.Date(2024, 7, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Aug 2024", time.Date(2024, 8, 30, 16, 0, 0, 0, time.UTC), 7),
				Entry("Sep 2024", time.Date(2024, 9, 30, 16, 0, 0, 0, time.UTC), 7),
				Entry("Oct 2024", time.Date(2024, 10, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Nov 2024 (early close 13:00)", time.Date(2024, 11, 29, 13, 0, 0, 0, time.UTC), 7),
				Entry("Dec 2024", time.Date(2024, 12, 31, 16, 0, 0, 0, time.UTC), 7),
				// Into 2025
				Entry("Jan 2025", time.Date(2025, 1, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Feb 2025", time.Date(2025, 2, 28, 16, 0, 0, 0, time.UTC), 7),
				Entry("Mar 2025", time.Date(2025, 3, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Apr 2025", time.Date(2025, 4, 30, 16, 0, 0, 0, time.UTC), 7),
				Entry("May 2025", time.Date(2025, 5, 30, 16, 0, 0, 0, time.UTC), 7),
				Entry("Jun 2025", time.Date(2025, 6, 30, 16, 0, 0, 0, time.UTC), 7),
				Entry("Jul 2025", time.Date(2025, 7, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Aug 2025", time.Date(2025, 8, 29, 16, 0, 0, 0, time.UTC), 7),
				Entry("Sep 2025", time.Date(2025, 9, 30, 16, 0, 0, 0, time.UTC), 7),
				Entry("Oct 2025", time.Date(2025, 10, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Nov 2025", time.Date(2025, 11, 28, 13, 0, 0, 0, time.UTC), 7),
				Entry("Dec 2025", time.Date(2025, 12, 31, 16, 0, 0, 0, time.UTC), 7),
				Entry("Jan 2026", time.Date(2026, 1, 30, 16, 0, 0, 0, time.UTC), 7),
				Entry("Feb 2026", time.Date(2026, 2, 27, 16, 0, 0, 0, time.UTC), 7),
				// Different lookback sizes
				Entry("Months(1) Oct 2024", time.Date(2024, 10, 31, 16, 0, 0, 0, time.UTC), 1),
				Entry("Months(1) Feb 2025", time.Date(2025, 2, 28, 16, 0, 0, 0, time.UTC), 1),
				Entry("Months(3) Oct 2024", time.Date(2024, 10, 31, 16, 0, 0, 0, time.UTC), 3),
				Entry("Months(3) Feb 2025", time.Date(2025, 2, 28, 16, 0, 0, 0, time.UTC), 3),
				Entry("Months(12) Dec 2024", time.Date(2024, 12, 31, 16, 0, 0, 0, time.UTC), 12),
				Entry("Months(12) Feb 2025", time.Date(2025, 2, 28, 16, 0, 0, 0, time.UTC), 12),
			)
		})

		It("different times-of-day on the same date produce the same monthly row count", func() {
			dailyDF := buildWeekdayDF(
				time.Date(2023, 1, 1, 16, 0, 0, 0, time.UTC),
				time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			)
			lookback := data.Months(7)

			// Same calendar date, different times (simulating early-close vs normal)
			timesOfDay := []int{0, 9, 13, 16, 23}
			for _, month := range []time.Month{time.January, time.June, time.October} {
				var counts []int
				for _, hour := range timesOfDay {
					ref := time.Date(2024, month, 28, hour, 0, 0, 0, time.UTC)
					windowed := dailyDF.Between(lookback.Before(ref), ref)
					monthly := windowed.Downsample(data.Monthly).Last()
					counts = append(counts, monthly.Len())
				}
				for i := 1; i < len(counts); i++ {
					Expect(counts[i]).To(Equal(counts[0]),
						"2024-%02d-28: hour %d gave %d rows, hour %d gave %d rows",
						month, timesOfDay[0], counts[0], timesOfDay[i], counts[i])
				}
			}
		})

		It("Months crossing a year boundary still gives correct count", func() {
			dailyDF := buildWeekdayDF(
				time.Date(2023, 1, 1, 16, 0, 0, 0, time.UTC),
				time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			)

			// March lookback of 7 crosses from 2024 into 2023
			ref := time.Date(2024, 3, 29, 16, 0, 0, 0, time.UTC)
			lookback := data.Months(7)
			windowed := dailyDF.Between(lookback.Before(ref), ref)
			monthly := windowed.Downsample(data.Monthly).Last()
			Expect(monthly.Len()).To(Equal(7))

			// Verify the actual months present: Sep, Oct, Nov, Dec (2023), Jan, Feb, Mar (2024)
			monthYears := make([]string, monthly.Len())
			for i, t := range monthly.Times() {
				monthYears[i] = fmt.Sprintf("%d-%02d", t.Year(), t.Month())
			}
			Expect(monthYears).To(Equal([]string{
				"2023-09", "2023-10", "2023-11", "2023-12",
				"2024-01", "2024-02", "2024-03",
			}))
		})
	})
})
