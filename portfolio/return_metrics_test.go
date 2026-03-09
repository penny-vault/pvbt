package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Return Metrics", func() {
	var (
		spy asset.Asset
		bm  asset.Asset
		bil asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bm = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
	})

	// buildAccount creates an Account with a rising equity curve over 5 days.
	// Equity values: 10000, 10100, 10300, 10200, 10500
	// Benchmark AdjClose: 100, 101, 103, 102, 105
	buildAccount := func() *portfolio.Account {
		a := portfolio.New(
			portfolio.WithCash(10_000),
			portfolio.WithBenchmark(bm),
			portfolio.WithRiskFree(bil),
		)

		dates := []time.Time{
			time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
		}

		equityValues := []float64{10_000, 10_100, 10_300, 10_200, 10_500}
		benchAdj := []float64{100, 101, 103, 102, 105}
		rfAdj := []float64{50.0, 50.01, 50.02, 50.03, 50.04}

		for i, d := range dates {
			// The account has no holdings, so equity = cash.
			// We set cash directly via deposit/withdrawal to get the desired equity curve.
			if i > 0 {
				diff := equityValues[i] - equityValues[i-1]
				if diff != 0 {
					if diff > 0 {
						a.Record(portfolio.Transaction{
							Date:   d,
							Type:   portfolio.DividendTransaction,
							Amount: diff,
						})
					} else {
						a.Record(portfolio.Transaction{
							Date:   d,
							Type:   portfolio.FeeTransaction,
							Amount: diff,
						})
					}
				}
			}
			df := buildDF(d,
				[]asset.Asset{spy, bm, bil},
				[]float64{450, benchAdj[i], rfAdj[i]},
				[]float64{448, benchAdj[i], rfAdj[i]},
			)
			a.UpdatePrices(df)
		}

		return a
	}

	Describe("TWRR", func() {
		It("returns a positive value for a rising equity curve", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.TWRR).Value()
			// equity went from 10000 to 10500, so total return = 5%
			Expect(result).To(BeNumerically("~", 0.05, 1e-9))
		})

		It("returns zero for a flat equity curve", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			for i := 0; i < 3; i++ {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				df := buildDF(d, []asset.Asset{spy}, []float64{100}, []float64{100})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("computes series with correct length (n-1)", func() {
			a := buildAccount()
			series := a.PerformanceMetric(portfolio.TWRR).Series()
			// 5 equity points -> 4 return periods -> 4 cumulative return values
			Expect(series).To(HaveLen(4))
		})

		It("computes cumulative return series", func() {
			a := buildAccount()
			series := a.PerformanceMetric(portfolio.TWRR).Series()
			// equity: 10000, 10100, 10300, 10200, 10500
			// period returns: 0.01, 0.0198..., -0.0097..., 0.0294...
			// cumulative: (1.01)-1, (1.01*1.0198..)-1, ...
			// final cumulative should equal total return
			Expect(series[len(series)-1]).To(BeNumerically("~", 0.05, 1e-9))
		})
	})

	Describe("MWRR", func() {
		It("returns a positive annualized rate for a growing portfolio with no external flows", func() {
			a := portfolio.New(portfolio.WithCash(10_000))

			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			}
			equityValues := []float64{10_000, 10_500, 11_000}

			for i, d := range dates {
				if i > 0 {
					diff := equityValues[i] - equityValues[i-1]
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.DividendTransaction,
						Amount: diff,
					})
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{450}, []float64{448})
				a.UpdatePrices(df)
			}

			result := a.PerformanceMetric(portfolio.MWRR).Value()
			// With no external cash flows and 10% total return over 1 year,
			// MWRR should be approximately 10%.
			Expect(result).To(BeNumerically(">", 0.0))
			Expect(result).To(BeNumerically("~", 0.10, 0.01))
		})
	})

	Describe("ActiveReturn", func() {
		It("computes the difference between portfolio and benchmark total return", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.ActiveReturn).Value()
			// portfolio return: (10500/10000) - 1 = 0.05
			// benchmark return: (105/100) - 1 = 0.05
			// active return: 0.05 - 0.05 = 0.0
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns positive active return when portfolio outperforms benchmark", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
			)

			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			// portfolio goes from 10000 to 11000 = 10% return
			// benchmark goes from 100 to 105 = 5% return
			// active return = 5%
			a.UpdatePrices(buildDF(dates[0], []asset.Asset{spy, bm}, []float64{450, 100}, []float64{448, 100}))

			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   portfolio.DividendTransaction,
				Amount: 1_000,
			})
			a.UpdatePrices(buildDF(dates[1], []asset.Asset{spy, bm}, []float64{460, 105}, []float64{458, 105}))

			result := a.PerformanceMetric(portfolio.ActiveReturn).Value()
			Expect(result).To(BeNumerically("~", 0.05, 1e-9))
		})

		It("computes a return series with correct length", func() {
			a := buildAccount()
			series := a.PerformanceMetric(portfolio.ActiveReturn).Series()
			// 5 equity points -> 4 return periods
			Expect(series).To(HaveLen(4))
		})

		It("computes element-wise difference of return series", func() {
			a := buildAccount()
			series := a.PerformanceMetric(portfolio.ActiveReturn).Series()
			// Since portfolio and benchmark have same total return structure,
			// check that each element is the difference of portfolio and benchmark returns
			portfolioReturns := a.PerformanceMetric(portfolio.TWRR).Series()
			benchPrices := a.BenchmarkPrices()
			benchReturns := make([]float64, len(benchPrices)-1)
			cumBench := 1.0
			for i := 0; i < len(benchPrices)-1; i++ {
				r := (benchPrices[i+1] - benchPrices[i]) / benchPrices[i]
				cumBench *= (1 + r)
				benchReturns[i] = cumBench - 1
			}
			for i := range series {
				Expect(series[i]).To(BeNumerically("~", portfolioReturns[i]-benchReturns[i], 1e-9))
			}
		})
	})
})
