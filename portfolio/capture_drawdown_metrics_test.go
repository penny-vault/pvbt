package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Capture and Drawdown Metrics", func() {
	var (
		spy asset.Asset
		bm  asset.Asset
		bil asset.Asset
		a   *portfolio.Account
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bm = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		// Build account with 25 data points.
		// Benchmark has ups and downs. Portfolio amplifies moves (higher beta).
		// Benchmark prices: oscillates with an overall upward trend.
		benchPrices := []float64{
			100, 103, 101, 105, 102, 107, 104, 109, 103, 111,
			108, 113, 106, 115, 110, 118, 112, 120, 114, 122,
			116, 124, 118, 126, 120,
		}
		// Portfolio equity scaling: amplified moves relative to benchmark.
		eqPrices := []float64{
			100, 105, 100, 108, 99, 112, 102, 114, 97, 118,
			106, 120, 101, 123, 108, 128, 110, 130, 111, 133,
			115, 136, 117, 140, 118,
		}
		rfPrices := make([]float64, 25)
		for i := range rfPrices {
			rfPrices[i] = 100 + float64(i)*0.01
		}

		a = portfolio.New(
			portfolio.WithCash(10_000),
			portfolio.WithBenchmark(bm),
			portfolio.WithRiskFree(bil),
		)

		baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

		for i := 0; i < 25; i++ {
			t := baseDate.AddDate(0, 0, i)
			eqVal := 10_000.0 * eqPrices[i] / eqPrices[0]
			if i > 0 {
				prevEq := 10_000.0 * eqPrices[i-1] / eqPrices[0]
				diff := eqVal - prevEq
				if diff > 0 {
					a.Record(portfolio.Transaction{
						Date:   t,
						Type:   portfolio.DividendTransaction,
						Amount: diff,
					})
				} else {
					a.Record(portfolio.Transaction{
						Date:   t,
						Type:   portfolio.FeeTransaction,
						Amount: diff,
					})
				}
			}

			df := buildDF(t,
				[]asset.Asset{spy, bm, bil},
				[]float64{eqPrices[i], benchPrices[i], rfPrices[i]},
				[]float64{eqPrices[i], benchPrices[i], rfPrices[i]},
			)
			a.UpdatePrices(df)
		}
	})

	Describe("UpsideCaptureRatio", func() {
		It("returns a positive value for a portfolio that participates in up markets", func() {
			v := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(v).To(BeNumerically(">", 0.0))
		})

		It("returns 0 when there are no up periods in benchmark", func() {
			// Create account with monotonically declining benchmark.
			acct := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(bil),
			)
			benchDown := []float64{100, 99, 98, 97, 96}
			baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			for i := 0; i < 5; i++ {
				t := baseDate.AddDate(0, 0, i)
				df := buildDF(t,
					[]asset.Asset{spy, bm, bil},
					[]float64{100, benchDown[i], 100},
					[]float64{100, benchDown[i], 100},
				)
				acct.UpdatePrices(df)
			}
			v := acct.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(v).To(Equal(0.0))
		})
	})

	Describe("DownsideCaptureRatio", func() {
		It("returns a positive value for a portfolio that participates in down markets", func() {
			v := a.PerformanceMetric(portfolio.DownsideCaptureRatio).Value()
			Expect(v).To(BeNumerically(">", 0.0))
		})

		It("returns 0 when there are no down periods in benchmark", func() {
			// Create account with monotonically rising benchmark.
			acct := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(bil),
			)
			benchUp := []float64{100, 101, 102, 103, 104}
			baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			for i := 0; i < 5; i++ {
				t := baseDate.AddDate(0, 0, i)
				df := buildDF(t,
					[]asset.Asset{spy, bm, bil},
					[]float64{100, benchUp[i], 100},
					[]float64{100, benchUp[i], 100},
				)
				acct.UpdatePrices(df)
			}
			v := acct.PerformanceMetric(portfolio.DownsideCaptureRatio).Value()
			Expect(v).To(Equal(0.0))
		})
	})

	Describe("AvgDrawdown", func() {
		It("returns a negative value for an equity curve with dips", func() {
			v := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(v).To(BeNumerically("<", 0.0))
		})

		It("returns 0 when equity curve never dips", func() {
			// Create account with monotonically rising equity.
			acct := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(bil),
			)
			baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			for i := 0; i < 5; i++ {
				t := baseDate.AddDate(0, 0, i)
				if i > 0 {
					acct.Record(portfolio.Transaction{
						Date:   t,
						Type:   portfolio.DividendTransaction,
						Amount: 100.0,
					})
				}
				df := buildDF(t,
					[]asset.Asset{spy, bm, bil},
					[]float64{100, 100, 100},
					[]float64{100, 100, 100},
				)
				acct.UpdatePrices(df)
			}
			v := acct.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(v).To(Equal(0.0))
		})
	})
})
