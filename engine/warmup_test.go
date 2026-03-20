package engine_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine"
)

var _ = Describe("walkBackTradingDays", func() {
	It("returns the same date for 0 days", func() {
		from := time.Date(2024, 2, 5, 16, 0, 0, 0, time.UTC)
		result, err := engine.WalkBackTradingDaysForTest(from, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Format("2006-01-02")).To(Equal("2024-02-05"))
	})

	It("walks back 5 trading days skipping weekends", func() {
		// Monday 2024-02-12
		from := time.Date(2024, 2, 12, 16, 0, 0, 0, time.UTC)
		result, err := engine.WalkBackTradingDaysForTest(from, 5)
		Expect(err).NotTo(HaveOccurred())
		// 5 trading days back from Feb 12 (Mon): Feb 5 (Mon)
		Expect(result.Format("2006-01-02")).To(Equal("2024-02-05"))
	})

	It("walks back a large number of trading days", func() {
		from := time.Date(2024, 6, 3, 16, 0, 0, 0, time.UTC)
		result, err := engine.WalkBackTradingDaysForTest(from, 252)
		Expect(err).NotTo(HaveOccurred())
		// ~1 year of trading days back from June 2024
		Expect(result.Year()).To(Equal(2023))
	})

	It("returns an error for negative days", func() {
		from := time.Date(2024, 2, 12, 16, 0, 0, 0, time.UTC)
		_, err := engine.WalkBackTradingDaysForTest(from, -1)
		Expect(err).To(HaveOccurred())
	})
})
