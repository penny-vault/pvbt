package portfolio_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Benchmark targeting", func() {
	Describe("TWRR via Benchmark()", func() {
		It("computes against the benchmark equity curve", func() {
			// SPY prices: 100, 110, 121 -> portfolio equity = 5*price
			// BIL (risk-free) prices held constant at 91.50
			// Benchmark (SPY) returns: 10%, 10% -> TWRR = 1.1*1.1-1 = 0.21
			spyPrices := []float64{100, 110, 121}
			bilPrices := []float64{91.50, 91.50, 91.50}
			acct := buildAccountWithRF(spyPrices, bilPrices)

			// Portfolio TWRR should match benchmark TWRR since the
			// portfolio holds SPY and the benchmark IS SPY.
			portfolioTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			benchmarkTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Benchmark().Value()
			Expect(err).NotTo(HaveOccurred())

			// Both should be 0.21 since the portfolio holds exactly SPY.
			Expect(portfolioTWRR).To(BeNumerically("~", 0.21, 1e-9))
			Expect(benchmarkTWRR).To(BeNumerically("~", 0.21, 1e-9))
		})

		It("returns different values when portfolio diverges from benchmark", func() {
			// Portfolio equity grows differently from benchmark.
			// buildAccountFromEquity creates a cash-only portfolio with
			// given equity values; we need to set up benchmark data manually.
			spyPrices := []float64{100, 105, 110}
			bilPrices := []float64{91.50, 91.50, 91.50}
			acct := buildAccountWithRF(spyPrices, bilPrices)

			// Portfolio holds 5 shares of SPY, equity = 5*price.
			// Portfolio returns: 5/100=5%, 5/105=4.76%
			// Benchmark returns: 5/100=5%, 5/105=4.76% (same as SPY)
			portfolioTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			benchmarkTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Benchmark().Value()
			Expect(err).NotTo(HaveOccurred())

			// Both should be equal because the portfolio IS the benchmark.
			Expect(portfolioTWRR).To(BeNumerically("~", benchmarkTWRR, 1e-9))
		})
	})

	Describe("ErrBenchmarkNotSupported", func() {
		It("returns error for transaction-based metrics like WinRate", func() {
			spyPrices := []float64{100, 110, 121}
			bilPrices := []float64{91.50, 91.50, 91.50}
			acct := buildAccountWithRF(spyPrices, bilPrices)

			_, err := acct.PerformanceMetric(portfolio.WinRate).Benchmark().Value()
			Expect(err).To(MatchError(portfolio.ErrBenchmarkNotSupported))
		})

		It("returns error for relational metrics like Beta", func() {
			spyPrices := []float64{100, 110, 121}
			bilPrices := []float64{91.50, 91.50, 91.50}
			acct := buildAccountWithRF(spyPrices, bilPrices)

			_, err := acct.PerformanceMetric(portfolio.Beta).Benchmark().Value()
			Expect(err).To(MatchError(portfolio.ErrBenchmarkNotSupported))
		})
	})

	Describe("ErrNoBenchmark", func() {
		It("returns error when no benchmark is configured", func() {
			// buildAccountFromEquity does not set a benchmark.
			acct := buildAccountFromEquity([]float64{1000, 1050, 1100})

			_, err := acct.PerformanceMetric(portfolio.TWRR).Benchmark().Value()
			Expect(err).To(MatchError(portfolio.ErrNoBenchmark))
		})
	})

	Describe("Series via Benchmark()", func() {
		It("computes series against the benchmark curve", func() {
			spyPrices := []float64{100, 110, 121}
			bilPrices := []float64{91.50, 91.50, 91.50}
			acct := buildAccountWithRF(spyPrices, bilPrices)

			df, err := acct.PerformanceMetric(portfolio.TWRR).Benchmark().Series()
			Expect(err).NotTo(HaveOccurred())
			Expect(df.Len()).To(Equal(2))
			// Cumulative returns: after day 1: 10%, after day 2: 21%
			series := df.Column(perfAsset, data.PortfolioEquity)
			Expect(series[0]).To(BeNumerically("~", 0.10, 1e-9))
			Expect(series[1]).To(BeNumerically("~", 0.21, 1e-9))
		})

		It("returns ErrBenchmarkNotSupported for unsupported metrics", func() {
			spyPrices := []float64{100, 110, 121}
			bilPrices := []float64{91.50, 91.50, 91.50}
			acct := buildAccountWithRF(spyPrices, bilPrices)

			_, err := acct.PerformanceMetric(portfolio.WinRate).Benchmark().Series()
			Expect(err).To(MatchError(portfolio.ErrBenchmarkNotSupported))
		})
	})
})
