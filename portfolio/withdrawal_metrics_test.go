package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Withdrawal Metrics", func() {
	var (
		spy asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
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

	// buildLongAccount creates an Account with a steadily growing equity curve
	// over 60 data points (representing roughly 60 trading days).
	buildLongAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(100_000))

		// Steady ~0.3% daily growth to simulate a strong equity curve.
		price := 100_000.0
		for i := 0; i < 60; i++ {
			d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
			if i > 0 {
				growth := price * 0.003
				a.Record(portfolio.Transaction{
					Date:   d,
					Type:   portfolio.DividendTransaction,
					Amount: growth,
				})
				price += growth
			}
			df := buildDF(d, []asset.Asset{spy}, []float64{450 + float64(i)}, []float64{448 + float64(i)})
			a.UpdatePrices(df)
		}

		return a
	}

	// buildShortAccount creates an Account with fewer than 12 data points.
	buildShortAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(10_000))

		for i := 0; i < 5; i++ {
			d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
			if i > 0 {
				a.Record(portfolio.Transaction{
					Date:   d,
					Type:   portfolio.DividendTransaction,
					Amount: 50,
				})
			}
			df := buildDF(d, []asset.Asset{spy}, []float64{450}, []float64{448})
			a.UpdatePrices(df)
		}

		return a
	}

	Describe("SafeWithdrawalRate", func() {
		It("has the correct name", func() {
			Expect(portfolio.SafeWithdrawalRate.Name()).To(Equal("SafeWithdrawalRate"))
		})

		It("returns 0 when equity curve has fewer than 12 points", func() {
			a := buildShortAccount()
			Expect(portfolio.SafeWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("returns a value between 0 and 0.20 for a growing equity curve", func() {
			a := buildLongAccount()
			rate := portfolio.SafeWithdrawalRate.Compute(a, nil)
			Expect(rate).To(BeNumerically(">=", 0.0))
			Expect(rate).To(BeNumerically("<=", 0.20))
		})

		It("returns a positive rate for a growing equity curve", func() {
			a := buildLongAccount()
			rate := portfolio.SafeWithdrawalRate.Compute(a, nil)
			Expect(rate).To(BeNumerically(">", 0.0))
		})

		It("returns nil for ComputeSeries", func() {
			a := buildLongAccount()
			Expect(portfolio.SafeWithdrawalRate.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("PerpetualWithdrawalRate", func() {
		It("has the correct name", func() {
			Expect(portfolio.PerpetualWithdrawalRate.Name()).To(Equal("PerpetualWithdrawalRate"))
		})

		It("returns 0 when equity curve has fewer than 12 points", func() {
			a := buildShortAccount()
			Expect(portfolio.PerpetualWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("returns a value <= SafeWithdrawalRate", func() {
			a := buildLongAccount()
			swr := portfolio.SafeWithdrawalRate.Compute(a, nil)
			pwr := portfolio.PerpetualWithdrawalRate.Compute(a, nil)
			Expect(pwr).To(BeNumerically("<=", swr))
		})

		It("returns nil for ComputeSeries", func() {
			a := buildLongAccount()
			Expect(portfolio.PerpetualWithdrawalRate.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("DynamicWithdrawalRate", func() {
		It("has the correct name", func() {
			Expect(portfolio.DynamicWithdrawalRate.Name()).To(Equal("DynamicWithdrawalRate"))
		})

		It("returns 0 when equity curve has fewer than 12 points", func() {
			a := buildShortAccount()
			Expect(portfolio.DynamicWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("returns a positive rate for a growing equity curve", func() {
			a := buildLongAccount()
			rate := portfolio.DynamicWithdrawalRate.Compute(a, nil)
			Expect(rate).To(BeNumerically(">", 0.0))
		})

		It("returns a value >= SafeWithdrawalRate", func() {
			a := buildLongAccount()
			swr := portfolio.SafeWithdrawalRate.Compute(a, nil)
			dwr := portfolio.DynamicWithdrawalRate.Compute(a, nil)
			Expect(dwr).To(BeNumerically(">=", swr))
		})

		It("returns nil for ComputeSeries", func() {
			a := buildLongAccount()
			Expect(portfolio.DynamicWithdrawalRate.ComputeSeries(a, nil)).To(BeNil())
		})
	})
})
