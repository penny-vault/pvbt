package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Distribution Metrics", func() {
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

	// buildAccount creates an Account with a 21-point equity curve that has
	// a mix of ups and downs. The overall trend is upward with a dip in the
	// middle to create interesting distribution characteristics.
	buildAccount := func() *portfolio.Account {
		a := portfolio.New(
			portfolio.WithCash(10_000),
			portfolio.WithBenchmark(bm),
			portfolio.WithRiskFree(bil),
		)

		// 21 data points: mostly rising with some drops
		equityValues := []float64{
			10000, 10050, 10120, 10080, 10150,
			10200, 10180, 10250, 10300, 10270,
			10350, 10400, 10380, 10450, 10500,
			10480, 10550, 10600, 10580, 10650,
			10700,
		}

		baseDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

		for i, eq := range equityValues {
			d := baseDate.AddDate(0, 0, i)
			if i > 0 {
				diff := eq - equityValues[i-1]
				if diff > 0 {
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.DividendTransaction,
						Amount: diff,
					})
				} else if diff < 0 {
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.FeeTransaction,
						Amount: diff,
					})
				}
			}
			benchVal := 100.0 + float64(i)*0.5
			rfVal := 50.0 + float64(i)*0.01
			df := buildDF(d,
				[]asset.Asset{spy, bm, bil},
				[]float64{450, benchVal, rfVal},
				[]float64{448, benchVal, rfVal},
			)
			a.UpdatePrices(df)
		}

		return a
	}

	Describe("ExcessKurtosis", func() {
		It("has the correct name", func() {
			Expect(portfolio.ExcessKurtosis.Name()).To(Equal("ExcessKurtosis"))
		})

		It("returns a finite number for a mixed equity curve", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.ExcessKurtosis).Value()
			Expect(result).To(BeNumerically("<", 100))
			Expect(result).To(BeNumerically(">", -100))
		})

		It("returns zero for a flat equity curve", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			for i := 0; i < 5; i++ {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				df := buildDF(d, []asset.Asset{spy}, []float64{100}, []float64{100})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.ExcessKurtosis).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns zero when fewer than 4 data points", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			// 3 equity points -> 2 returns, which is < 4
			for i := 0; i < 3; i++ {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				df := buildDF(d, []asset.Asset{spy}, []float64{float64(100 + i)}, []float64{float64(100 + i)})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.ExcessKurtosis).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns nil for ComputeSeries", func() {
			a := buildAccount()
			series := a.PerformanceMetric(portfolio.ExcessKurtosis).Series()
			Expect(series).To(BeNil())
		})
	})

	Describe("Skewness", func() {
		It("has the correct name", func() {
			Expect(portfolio.Skewness.Name()).To(Equal("Skewness"))
		})

		It("returns a finite number for a mixed equity curve", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.Skewness).Value()
			Expect(result).To(BeNumerically("<", 100))
			Expect(result).To(BeNumerically(">", -100))
		})

		It("returns zero for a flat equity curve", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			for i := 0; i < 5; i++ {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				df := buildDF(d, []asset.Asset{spy}, []float64{100}, []float64{100})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.Skewness).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns zero when fewer than 3 data points", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			for i := 0; i < 2; i++ {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				df := buildDF(d, []asset.Asset{spy}, []float64{float64(100 + i)}, []float64{float64(100 + i)})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.Skewness).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns nil for ComputeSeries", func() {
			a := buildAccount()
			series := a.PerformanceMetric(portfolio.Skewness).Series()
			Expect(series).To(BeNil())
		})
	})

	Describe("NPositivePeriods", func() {
		It("has the correct name", func() {
			Expect(portfolio.NPositivePeriods.Name()).To(Equal("NPositivePeriods"))
		})

		It("returns a value between 0 and 1", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			Expect(result).To(BeNumerically(">", 0.0))
			Expect(result).To(BeNumerically("<=", 1.0))
		})

		It("matches the expected fraction of positive return days", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			// From the equity curve, count positive returns:
			// 21 equity points -> 20 returns
			// Ups: indices 1,2,4,5,7,8,10,11,13,14,16,17,19,20 = 14 positive
			// Downs: indices 3,6,9,12,15,18 = 6 negative
			// Fraction: 14/20 = 0.70
			Expect(result).To(BeNumerically("~", 0.70, 1e-9))
		})

		It("returns zero for a flat equity curve", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			for i := 0; i < 5; i++ {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				df := buildDF(d, []asset.Asset{spy}, []float64{100}, []float64{100})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns nil for ComputeSeries", func() {
			a := buildAccount()
			series := a.PerformanceMetric(portfolio.NPositivePeriods).Series()
			Expect(series).To(BeNil())
		})
	})

	Describe("GainLossRatio", func() {
		It("has the correct name", func() {
			Expect(portfolio.GainLossRatio.Name()).To(Equal("GainLossRatio"))
		})

		It("returns a positive value when there are both gains and losses", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically(">", 0.0))
		})

		It("returns zero when all returns are positive (no losses)", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			for i := 0; i < 5; i++ {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				price := float64(100 + i*10)
				if i > 0 {
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.DividendTransaction,
						Amount: 10.0 * float64(i),
					})
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{price}, []float64{price})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns zero when there are no returns", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			d := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			df := buildDF(d, []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df)
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns nil for ComputeSeries", func() {
			a := buildAccount()
			series := a.PerformanceMetric(portfolio.GainLossRatio).Series()
			Expect(series).To(BeNil())
		})
	})
})
