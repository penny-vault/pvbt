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
		})
	})
})
