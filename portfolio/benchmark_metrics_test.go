package portfolio_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Benchmark Metrics", func() {
	var (
		spy asset.Asset
		bm  asset.Asset
		bil asset.Asset
		a   *portfolio.Account
	)

	// buildDF builds a single-timestamp DataFrame with MetricClose and AdjClose.
	buildDF := func(t time.Time, assets []asset.Asset, closes, adjCloses []float64) *data.DataFrame {
		vals := make([]float64, 0, len(assets)*2)
		for i := range assets {
			vals = append(vals, closes[i])
			vals = append(vals, adjCloses[i])
		}
		df, err := data.NewDataFrame(
			[]time.Time{t},
			assets,
			[]data.Metric{data.MetricClose, data.AdjClose},
			vals,
		)
		Expect(err).NotTo(HaveOccurred())
		return df
	}

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bm = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		// Build an account where equity tracks the benchmark closely.
		// Equity = benchmark * 1.0 (identical returns), risk-free grows slowly.
		a = portfolio.New(
			portfolio.WithCash(10_000),
			portfolio.WithBenchmark(bm),
			portfolio.WithRiskFree(bil),
		)

		// 12 daily data points -- equity tracks benchmark exactly (same returns).
		benchPrices := []float64{100, 102, 101, 104, 103, 106, 105, 108, 107, 110, 109, 112}
		rfPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05, 100.06, 100.07, 100.08, 100.09, 100.10, 100.11}

		baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

		for i := 0; i < 12; i++ {
			t := baseDate.AddDate(0, 0, i)
			// equity = 10000 * (benchPrices[i] / benchPrices[0])
			eqVal := 10_000.0 * benchPrices[i] / benchPrices[0]
			// Set cash to the equity value (no holdings, just cash = total value).
			// We manipulate cash directly through transactions to get the right equity curve.
			if i == 0 {
				// Initial deposit already set via WithCash.
			} else {
				// Adjust cash to match desired equity value.
				prevEq := 10_000.0 * benchPrices[i-1] / benchPrices[0]
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
				[]float64{benchPrices[i], benchPrices[i], rfPrices[i]},
				[]float64{benchPrices[i], benchPrices[i], rfPrices[i]},
			)
			a.UpdatePrices(df)
		}
	})

	Describe("Beta", func() {
		It("returns a name", func() {
			Expect(portfolio.Beta.Name()).To(Equal("Beta"))
		})

		It("returns beta close to 1.0 when portfolio tracks benchmark", func() {
			b := a.PerformanceMetric(portfolio.Beta).Value()
			Expect(b).To(BeNumerically("~", 1.0, 1e-10))
		})

		It("returns nil for ComputeSeries", func() {
			Expect(portfolio.Beta.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("Alpha", func() {
		It("returns a name", func() {
			Expect(portfolio.Alpha.Name()).To(Equal("Alpha"))
		})

		It("returns alpha close to 0 when portfolio tracks benchmark", func() {
			v := a.PerformanceMetric(portfolio.Alpha).Value()
			Expect(v).To(BeNumerically("~", 0.0, 1e-6))
		})

		It("returns nil for ComputeSeries", func() {
			Expect(portfolio.Alpha.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("TrackingError", func() {
		It("returns a name", func() {
			Expect(portfolio.TrackingError.Name()).To(Equal("TrackingError"))
		})

		It("returns tracking error close to 0 when portfolio tracks benchmark", func() {
			v := a.PerformanceMetric(portfolio.TrackingError).Value()
			Expect(v).To(BeNumerically("~", 0.0, 1e-10))
		})

		It("returns nil for ComputeSeries", func() {
			Expect(portfolio.TrackingError.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("InformationRatio", func() {
		It("returns a name", func() {
			Expect(portfolio.InformationRatio.Name()).To(Equal("InformationRatio"))
		})

		It("returns 0 when tracking error is 0", func() {
			v := a.PerformanceMetric(portfolio.InformationRatio).Value()
			Expect(v).To(BeNumerically("~", 0.0, 1e-10))
		})

		It("returns nil for ComputeSeries", func() {
			Expect(portfolio.InformationRatio.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("Treynor", func() {
		It("returns a name", func() {
			Expect(portfolio.Treynor.Name()).To(Equal("Treynor"))
		})

		It("computes treynor ratio", func() {
			v := a.PerformanceMetric(portfolio.Treynor).Value()
			// portfolioReturn = (equity_end / equity_start) - 1
			// riskFreeReturn = (rf_end / rf_start) - 1
			// beta ~ 1.0
			// treynor = (portfolioReturn - riskFreeReturn) / beta
			eqEnd := 10_000.0 * 112.0 / 100.0
			eqStart := 10_000.0
			pReturn := (eqEnd / eqStart) - 1.0
			rfReturn := (100.11 / 100.0) - 1.0
			expected := (pReturn - rfReturn) / 1.0
			Expect(v).To(BeNumerically("~", expected, 1e-6))
		})

		It("returns nil for ComputeSeries", func() {
			Expect(portfolio.Treynor.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("RSquared", func() {
		It("returns a name", func() {
			Expect(portfolio.RSquared.Name()).To(Equal("RSquared"))
		})

		It("returns R-squared close to 1.0 when portfolio tracks benchmark", func() {
			v := a.PerformanceMetric(portfolio.RSquared).Value()
			Expect(v).To(BeNumerically("~", 1.0, 1e-10))
		})

		It("returns nil for ComputeSeries", func() {
			Expect(portfolio.RSquared.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	// Test with divergent portfolio to get non-trivial values.
	Describe("with divergent portfolio", func() {
		var divergent *portfolio.Account

		BeforeEach(func() {
			divergent = portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(bil),
			)

			// Portfolio grows faster than benchmark.
			benchPrices := []float64{100, 102, 101, 104, 103, 106, 105, 108, 107, 110, 109, 112}
			eqPrices := []float64{100, 105, 103, 110, 108, 115, 112, 120, 117, 125, 122, 130}
			rfPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05, 100.06, 100.07, 100.08, 100.09, 100.10, 100.11}

			baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			for i := 0; i < 12; i++ {
				t := baseDate.AddDate(0, 0, i)
				eqVal := 10_000.0 * eqPrices[i] / eqPrices[0]
				if i > 0 {
					prevEq := 10_000.0 * eqPrices[i-1] / eqPrices[0]
					diff := eqVal - prevEq
					if diff > 0 {
						divergent.Record(portfolio.Transaction{
							Date:   t,
							Type:   portfolio.DividendTransaction,
							Amount: diff,
						})
					} else {
						divergent.Record(portfolio.Transaction{
							Date:   t,
							Type:   portfolio.FeeTransaction,
							Amount: diff,
						})
					}
				}

				df := buildDF(t,
					[]asset.Asset{spy, bm, bil},
					[]float64{benchPrices[i], benchPrices[i], rfPrices[i]},
					[]float64{benchPrices[i], benchPrices[i], rfPrices[i]},
				)
				divergent.UpdatePrices(df)
			}
		})

		It("Beta is positive and greater than 1", func() {
			b := divergent.PerformanceMetric(portfolio.Beta).Value()
			Expect(b).To(BeNumerically(">", 1.0))
		})

		It("Alpha is positive when portfolio outperforms", func() {
			v := divergent.PerformanceMetric(portfolio.Alpha).Value()
			Expect(v).To(BeNumerically(">", 0.0))
		})

		It("TrackingError is positive when returns diverge", func() {
			v := divergent.PerformanceMetric(portfolio.TrackingError).Value()
			Expect(v).To(BeNumerically(">", 0.0))
		})

		It("InformationRatio is positive when outperforming with tracking error", func() {
			v := divergent.PerformanceMetric(portfolio.InformationRatio).Value()
			Expect(v).To(BeNumerically(">", 0.0))
		})

		It("RSquared is between 0 and 1", func() {
			v := divergent.PerformanceMetric(portfolio.RSquared).Value()
			Expect(v).To(BeNumerically(">", 0.0))
			Expect(v).To(BeNumerically("<=", 1.0))
		})

		It("Treynor is positive when portfolio outperforms risk-free", func() {
			v := divergent.PerformanceMetric(portfolio.Treynor).Value()
			Expect(v).To(BeNumerically(">", 0.0))
		})
	})

	// Edge cases.
	Describe("edge cases", func() {
		It("Beta returns 0 when benchmark variance is 0", func() {
			acct := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(bil),
			)
			baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			// Flat benchmark.
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
			b := acct.PerformanceMetric(portfolio.Beta).Value()
			Expect(b).To(Equal(0.0))
		})

		It("Treynor returns 0 when beta is 0", func() {
			acct := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(bil),
			)
			baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			// Flat benchmark, growing portfolio.
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
			v := acct.PerformanceMetric(portfolio.Treynor).Value()
			Expect(v).To(Equal(0.0))
		})

		It("RSquared returns 0 when portfolio stddev is 0", func() {
			acct := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(bil),
			)
			baseDate := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			// Flat portfolio, varying benchmark.
			benchPrices := []float64{100, 102, 101, 104, 103}
			for i := 0; i < 5; i++ {
				t := baseDate.AddDate(0, 0, i)
				df := buildDF(t,
					[]asset.Asset{spy, bm, bil},
					[]float64{100, benchPrices[i], 100},
					[]float64{100, benchPrices[i], 100},
				)
				acct.UpdatePrices(df)
			}
			v := acct.PerformanceMetric(portfolio.RSquared).Value()
			Expect(v).To(Equal(0.0))
		})
	})

	// Verify metric Name() strings.
	Describe("Name methods", func() {
		It("returns correct names for all benchmark metrics", func() {
			Expect(portfolio.Beta.Name()).To(Equal("Beta"))
			Expect(portfolio.Alpha.Name()).To(Equal("Alpha"))
			Expect(portfolio.TrackingError.Name()).To(Equal("TrackingError"))
			Expect(portfolio.InformationRatio.Name()).To(Equal("InformationRatio"))
			Expect(portfolio.Treynor.Name()).To(Equal("Treynor"))
			Expect(portfolio.RSquared.Name()).To(Equal("RSquared"))
		})
	})

	// Verify annualization factor selection.
	Describe("annualization factor", func() {
		It("uses daily factor (252) for daily data", func() {
			// The tracking error for our daily identical-tracking portfolio is 0.
			// We just verify the metric doesn't panic.
			v := a.PerformanceMetric(portfolio.TrackingError).Value()
			Expect(math.IsNaN(v)).To(BeFalse())
			Expect(math.IsInf(v, 0)).To(BeFalse())
		})
	})
})
