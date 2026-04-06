package tradecron_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/tradecron"
)

var _ = Describe("TradeCron", func() {
	var nyc *time.Location

	BeforeEach(func() {
		var err error
		nyc, err = time.LoadLocation("America/New_York")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("@daily", func() {
		BeforeEach(func() {
			tradecron.SetMarketHolidays(nil)
		})

		It("fires at market open on a regular trading day", func() {
			tc, err := tradecron.New("@daily", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// Monday Jan 6, 2025 at midnight.
			forDate := time.Date(2025, time.January, 6, 0, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			// Should fire at 9:30 AM on Jan 6.
			want := time.Date(2025, time.January, 6, 9, 30, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("advances to the next trading day after market open", func() {
			tc, err := tradecron.New("@daily", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// After market open on Monday Jan 6.
			forDate := time.Date(2025, time.January, 6, 9, 30, 0, 0, nyc)
			got := tc.Next(forDate)

			// Should fire at 9:30 AM on Tuesday Jan 7.
			want := time.Date(2025, time.January, 7, 9, 30, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("skips weekends", func() {
			tc, err := tradecron.New("@daily", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// Friday Jan 10, 2025 after market open.
			forDate := time.Date(2025, time.January, 10, 9, 30, 0, 0, nyc)
			got := tc.Next(forDate)

			// Should skip Saturday and Sunday, fire Monday Jan 13.
			want := time.Date(2025, time.January, 13, 9, 30, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("skips holidays", func() {
			tradecron.SetMarketHolidays([]tradecron.MarketHoliday{
				{
					Date:       time.Date(2025, time.January, 7, 0, 0, 0, 0, nyc),
					EarlyClose: false,
					CloseTime:  0,
				},
			})

			tc, err := tradecron.New("@daily", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// After market open on Monday Jan 6.
			forDate := time.Date(2025, time.January, 6, 9, 30, 0, 0, nyc)
			got := tc.Next(forDate)

			// Jan 7 is a holiday, should skip to Jan 8.
			want := time.Date(2025, time.January, 8, 9, 30, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})
	})

	Describe("@quarterbegin", func() {
		BeforeEach(func() {
			tradecron.SetMarketHolidays(nil)
		})

		It("fires on the first trading day of Q1", func() {
			tc, err := tradecron.New("@quarterbegin", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// Dec 15, 2024 is mid-Q4.
			forDate := time.Date(2024, time.December, 15, 0, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			// Jan 2, 2025 is the first trading day of Q1 (Jan 1 is a holiday/weekend).
			// Jan 1, 2025 is a Wednesday -- but it's New Year's Day.
			// Without holidays loaded, Jan 1 is a normal Wednesday.
			want := time.Date(2025, time.January, 1, 9, 30, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("fires on the first trading day of Q2", func() {
			tc, err := tradecron.New("@quarterbegin", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// Mar 15, 2025 is mid-Q1.
			forDate := time.Date(2025, time.March, 15, 0, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			// Apr 1, 2025 is a Tuesday.
			want := time.Date(2025, time.April, 1, 9, 30, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("advances to next quarter if current quarter's first day has passed", func() {
			tc, err := tradecron.New("@quarterbegin", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// Feb 15, 2025 -- Q1 already started Jan 1.
			forDate := time.Date(2025, time.February, 15, 0, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			// Next quarter begins Apr 1, 2025 (Tuesday).
			want := time.Date(2025, time.April, 1, 9, 30, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("fires on the first trading day when the quarter starts on a weekend", func() {
			tc, err := tradecron.New("@quarterbegin", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// Jun 15, 2025 -- looking for Q3 start.
			forDate := time.Date(2025, time.June, 15, 0, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			// Jul 1, 2025 is a Tuesday.
			want := time.Date(2025, time.July, 1, 9, 30, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})
	})

	Describe("@quarterend", func() {
		BeforeEach(func() {
			tradecron.SetMarketHolidays(nil)
		})

		It("fires on the last trading day of Q1", func() {
			tc, err := tradecron.New("@quarterend", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// Feb 1, 2025 -- mid-Q1.
			forDate := time.Date(2025, time.February, 1, 0, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			// Mar 31, 2025 is a Monday.
			want := time.Date(2025, time.March, 31, 16, 0, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("fires on the last trading day of Q2", func() {
			tc, err := tradecron.New("@quarterend", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// May 1, 2025 -- mid-Q2.
			forDate := time.Date(2025, time.May, 1, 0, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			// Jun 30, 2025 is a Monday.
			want := time.Date(2025, time.June, 30, 16, 0, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("advances to the next quarter after firing", func() {
			tc, err := tradecron.New("@quarterend", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// After Q1 end close on Mar 31, 2025.
			forDate := time.Date(2025, time.March, 31, 16, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			// Next quarter end is Jun 30, 2025 (Monday).
			want := time.Date(2025, time.June, 30, 16, 0, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("fires on the last trading day when quarter ends on a weekend", func() {
			tc, err := tradecron.New("@quarterend", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			// Aug 1, 2025 -- mid-Q3. Sep 30, 2025 is a Tuesday.
			forDate := time.Date(2025, time.August, 1, 0, 0, 0, 0, nyc)
			got := tc.Next(forDate)

			want := time.Date(2025, time.September, 30, 16, 0, 0, 0, nyc)
			Expect(got).To(Equal(want))
		})

		It("iterates through consecutive quarters", func() {
			tc, err := tradecron.New("@quarterend", tradecron.RegularHours)
			Expect(err).NotTo(HaveOccurred())

			forDate := time.Date(2025, time.January, 1, 0, 0, 0, 0, nyc)

			var months []time.Month
			for range 4 {
				next := tc.Next(forDate)
				months = append(months, next.Month())
				forDate = next.Add(time.Nanosecond)
			}

			Expect(months).To(Equal([]time.Month{
				time.March, time.June, time.September, time.December,
			}))
		})
	})

	Describe("Next", func() {
		Context("when the last trading day of the month is an early-close day", func() {
			BeforeEach(func() {
				// Nov 28, 2024 = Thanksgiving (full holiday)
				// Nov 29, 2024 = day after Thanksgiving (early close at 13:00)
				tradecron.SetMarketHolidays([]tradecron.MarketHoliday{
					{
						Date:       time.Date(2024, time.November, 28, 0, 0, 0, 0, nyc),
						EarlyClose: false,
						CloseTime:  0,
					},
					{
						Date:       time.Date(2024, time.November, 29, 0, 0, 0, 0, nyc),
						EarlyClose: true,
						CloseTime:  1300,
					},
				})
			})

			It("fires @monthend at the actual early close time", func() {
				tc, err := tradecron.New("@monthend", tradecron.RegularHours)
				Expect(err).NotTo(HaveOccurred())

				forDate := time.Date(2024, time.November, 1, 0, 0, 0, 0, nyc)
				got := tc.Next(forDate)

				// Nov 29 is the last trading day; fires at 13:00 (the real close).
				want := time.Date(2024, time.November, 29, 13, 0, 0, 0, nyc)
				Expect(got).To(Equal(want))
			})

			It("advances to the next month after firing at the early close time", func() {
				tc, err := tradecron.New("@monthend", tradecron.RegularHours)
				Expect(err).NotTo(HaveOccurred())

				// After firing at the early close on Nov 29, next should be
				// the last trading day of December, not Dec 2.
				forDate := time.Date(2024, time.November, 29, 13, 0, 0, 0, nyc)
				got := tc.Next(forDate)

				// Dec 31, 2024 is a Tuesday, normal trading day.
				want := time.Date(2024, time.December, 31, 16, 0, 0, 0, nyc)
				Expect(got).To(Equal(want))
			})

			It("reports the early-close day as a trade day for @monthend", func() {
				tc, err := tradecron.New("@monthend", tradecron.RegularHours)
				Expect(err).NotTo(HaveOccurred())

				checkDate := time.Date(2024, time.November, 29, 0, 0, 0, 0, nyc)
				Expect(tc.IsTradeDay(checkDate)).To(BeTrue())
			})
		})

		Context("when the last trading day of the month is a normal day", func() {
			BeforeEach(func() {
				tradecron.SetMarketHolidays(nil)
			})

			It("fires @monthend at the regular close time", func() {
				tc, err := tradecron.New("@monthend", tradecron.RegularHours)
				Expect(err).NotTo(HaveOccurred())

				forDate := time.Date(2024, time.October, 1, 0, 0, 0, 0, nyc)
				got := tc.Next(forDate)

				// Oct 31, 2024 is Thursday, a normal trading day.
				want := time.Date(2024, time.October, 31, 16, 0, 0, 0, nyc)
				Expect(got).To(Equal(want))
			})
		})

		Context("with @close on an early-close day", func() {
			BeforeEach(func() {
				// July 3, 2024 = early close at 13:00 (day before Independence Day)
				// July 4, 2024 = Independence Day (full holiday)
				tradecron.SetMarketHolidays([]tradecron.MarketHoliday{
					{
						Date:       time.Date(2024, time.July, 3, 0, 0, 0, 0, nyc),
						EarlyClose: true,
						CloseTime:  1300,
					},
					{
						Date:       time.Date(2024, time.July, 4, 0, 0, 0, 0, nyc),
						EarlyClose: false,
						CloseTime:  0,
					},
				})
			})

			It("fires at the actual early close time", func() {
				tc, err := tradecron.New("@close * * *", tradecron.RegularHours)
				Expect(err).NotTo(HaveOccurred())

				forDate := time.Date(2024, time.July, 3, 9, 0, 0, 0, nyc)
				got := tc.Next(forDate)

				want := time.Date(2024, time.July, 3, 13, 0, 0, 0, nyc)
				Expect(got).To(Equal(want))
			})

			It("advances past the holiday to the next trading day", func() {
				tc, err := tradecron.New("@close * * *", tradecron.RegularHours)
				Expect(err).NotTo(HaveOccurred())

				// After the early close on July 3, next should be July 5.
				forDate := time.Date(2024, time.July, 3, 13, 0, 0, 0, nyc)
				got := tc.Next(forDate)

				want := time.Date(2024, time.July, 5, 16, 0, 0, 0, nyc)
				Expect(got).To(Equal(want))
			})
		})

		Context("with @weekend on an early-close Friday", func() {
			BeforeEach(func() {
				// Hypothetical early close on a Friday.
				tradecron.SetMarketHolidays([]tradecron.MarketHoliday{
					{
						Date:       time.Date(2024, time.March, 29, 0, 0, 0, 0, nyc),
						EarlyClose: true,
						CloseTime:  1300,
					},
				})
			})

			It("fires at the actual early close time", func() {
				tc, err := tradecron.New("@weekend", tradecron.RegularHours)
				Expect(err).NotTo(HaveOccurred())

				forDate := time.Date(2024, time.March, 25, 0, 0, 0, 0, nyc)
				got := tc.Next(forDate)

				want := time.Date(2024, time.March, 29, 13, 0, 0, 0, nyc)
				Expect(got).To(Equal(want))
			})

			It("advances to the next week after firing at the early close time", func() {
				tc, err := tradecron.New("@weekend", tradecron.RegularHours)
				Expect(err).NotTo(HaveOccurred())

				// After firing at the early close on Friday March 29,
				// next should be Friday April 5, not Monday April 1.
				forDate := time.Date(2024, time.March, 29, 13, 0, 0, 0, nyc)
				got := tc.Next(forDate)

				want := time.Date(2024, time.April, 5, 16, 0, 0, 0, nyc)
				Expect(got).To(Equal(want))
			})
		})

		Context("when November 30 falls on a weekend (no early close)", func() {
			BeforeEach(func() {
				// No holidays — just a plain weekend at month boundary.
				tradecron.SetMarketHolidays(nil)
			})

			DescribeTable("fires exactly once per month across the Nov-Dec boundary",
				func(year int, wantNovDay int, wantDecDay int) {
					tc, err := tradecron.New("@monthend", tradecron.RegularHours)
					Expect(err).NotTo(HaveOccurred())

					// Start just after October's month-end close.
					forDate := time.Date(year, time.October, 1, 0, 0, 0, 0, nyc)

					// Collect the next three month-end dates (Oct, Nov, Dec).
					var dates []time.Time
					for range 3 {
						next := tc.Next(forDate)
						dates = append(dates, next)
						forDate = next.Add(time.Nanosecond)
					}

					// November's month-end should be on wantNovDay.
					Expect(dates[1].Month()).To(Equal(time.November))
					Expect(dates[1].Day()).To(Equal(wantNovDay))

					// December's month-end should be on wantDecDay, NOT the
					// first trading day of December.
					Expect(dates[2].Month()).To(Equal(time.December))
					Expect(dates[2].Day()).To(Equal(wantDecDay))
				},
				// Years where Nov 30 is a weekend.
				Entry("2008: Nov 30 is Sunday", 2008, 28, 31),
				Entry("2013: Nov 30 is Saturday", 2013, 29, 31),
				Entry("2014: Nov 30 is Sunday", 2014, 28, 31),
				Entry("2019: Nov 30 is Saturday", 2019, 29, 31),
				Entry("2024: Nov 30 is Saturday", 2024, 29, 31),
				Entry("2025: Nov 30 is Sunday", 2025, 28, 31),
			)
		})

		Context("when November 30 falls on a weekend WITH Thanksgiving holidays", func() {
			DescribeTable("fires exactly once per month across the Nov-Dec boundary",
				func(year int, thanksgivingDay int, wantNovDay int, wantDecDay int) {
					tradecron.SetMarketHolidays([]tradecron.MarketHoliday{
						{
							Date:       time.Date(year, time.November, thanksgivingDay, 0, 0, 0, 0, nyc),
							EarlyClose: false,
							CloseTime:  0,
						},
						{
							Date:       time.Date(year, time.November, thanksgivingDay+1, 0, 0, 0, 0, nyc),
							EarlyClose: true,
							CloseTime:  1300,
						},
					})

					tc, err := tradecron.New("@monthend", tradecron.RegularHours)
					Expect(err).NotTo(HaveOccurred())

					// Start at beginning of October.
					forDate := time.Date(year, time.October, 1, 0, 0, 0, 0, nyc)

					// Collect the next three month-end dates (Oct, Nov, Dec).
					var dates []time.Time
					for range 3 {
						next := tc.Next(forDate)
						dates = append(dates, next)
						forDate = next.Add(time.Nanosecond)
					}

					// November's month-end should fire on the early close day.
					Expect(dates[1].Month()).To(Equal(time.November))
					Expect(dates[1].Day()).To(Equal(wantNovDay))

					// December's month-end should be end of December, NOT
					// the first trading day of December.
					Expect(dates[2].Month()).To(Equal(time.December))
					Expect(dates[2].Day()).To(Equal(wantDecDay))
				},
				// Thanksgiving day, last Nov trading day, last Dec trading day.
				Entry("2008: Thanksgiving Nov 27", 2008, 27, 28, 31),
				Entry("2013: Thanksgiving Nov 28", 2013, 28, 29, 31),
				Entry("2014: Thanksgiving Nov 27", 2014, 27, 28, 31),
				Entry("2019: Thanksgiving Nov 28", 2019, 28, 29, 31),
				Entry("2024: Thanksgiving Nov 28", 2024, 28, 29, 31),
				Entry("2025: Thanksgiving Nov 27", 2025, 27, 28, 31),
			)
		})
	})
})
