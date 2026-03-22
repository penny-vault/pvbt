package portfolio_test

import (
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Benchmark metrics with NaN in returns", func() {
	// This reproduces the panic from stat.Covariance when portfolio
	// and benchmark return slices have different lengths after
	// independent NaN removal.
	//
	// Equity curve has a NaN-producing gap at index 2 (price = NaN),
	// benchmark has a NaN-producing gap at index 3 (price = NaN).
	// Independent removeNaN would yield slices of length 3 and 3 but
	// at different positions, causing length mismatch after the NaN
	// at different indices drops different elements.

	Describe("NaN benchmark price does not panic", func() {
		var acct *portfolio.Account

		BeforeEach(func() {
			// 7 data points. Benchmark has NaN at index 3 which produces
			// NaN return at index 3. Portfolio returns are clean.
			// Without aligned NaN removal, pCol has 6 returns, bCol has 5,
			// and stat.Covariance panics.
			acct = benchAcct(
				[]float64{1000, 1050, 1020, 1080, 1060, 1100, 1120},
				[]float64{100, 104, 101, math.NaN(), 105, 109, 112},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05, 100.06},
			)
		})

		It("Beta does not panic and returns a value", func() {
			val, err := acct.PerformanceMetric(portfolio.Beta).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(math.IsNaN(val)).To(BeFalse())
		})

		It("RSquared does not panic and returns a value", func() {
			val, err := acct.PerformanceMetric(portfolio.RSquared).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(math.IsNaN(val)).To(BeFalse())
		})

		It("TrackingError does not panic and returns a value", func() {
			val, err := acct.PerformanceMetric(portfolio.TrackingError).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(math.IsNaN(val)).To(BeFalse())
		})

		It("InformationRatio does not panic and returns a value", func() {
			val, err := acct.PerformanceMetric(portfolio.InformationRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(math.IsNaN(val)).To(BeFalse())
		})

		It("Alpha does not panic and returns a value", func() {
			val, err := acct.PerformanceMetric(portfolio.Alpha).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(math.IsNaN(val)).To(BeFalse())
		})
	})
})
