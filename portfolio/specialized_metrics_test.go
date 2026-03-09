package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Specialized Metrics", func() {
	var (
		spy asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	})

	// buildAccount creates an Account with a generally rising equity curve
	// that includes drawdowns, over 25 data points.
	buildAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(10_000))

		// A rising curve with some dips to produce drawdowns and negative returns.
		equityValues := []float64{
			10000, 10100, 10050, 10200, 10150,
			10300, 10250, 10400, 10350, 10500,
			10450, 10600, 10550, 10700, 10650,
			10800, 10750, 10900, 10850, 11000,
			10950, 11100, 11050, 11200, 11300,
		}

		for i := 0; i < len(equityValues); i++ {
			d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
			if i > 0 {
				diff := equityValues[i] - equityValues[i-1]
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
			df := buildDF(d, []asset.Asset{spy}, []float64{450}, []float64{448})
			a.UpdatePrices(df)
		}

		return a
	}

	Describe("UlcerIndex", func() {
		It("returns a positive value for an equity curve with drawdowns", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(result).To(BeNumerically(">", 0))
		})

		It("returns zero for a monotonically rising equity curve", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			for i := 0; i < 5; i++ {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				if i > 0 {
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.DividendTransaction,
						Amount: 100,
					})
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{450}, []float64{448})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(result).To(BeNumerically("~", 0, 1e-9))
		})
	})

	Describe("ValueAtRisk", func() {
		It("returns a negative value for an equity curve with losses", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(result).To(BeNumerically("<", 0))
		})
	})

	Describe("KRatio", func() {
		It("returns a positive value for a generally rising equity curve", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(result).To(BeNumerically(">", 0))
		})
	})

	Describe("KellerRatio", func() {
		It("returns a positive value for a rising curve with moderate drawdown", func() {
			a := buildAccount()
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically(">", 0))
		})

		It("returns zero when total return is negative", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			equityValues := []float64{10000, 9800, 9600, 9500}
			for i, v := range equityValues {
				d := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
				if i > 0 {
					diff := v - equityValues[i-1]
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.FeeTransaction,
						Amount: diff,
					})
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{450}, []float64{448})
				a.UpdatePrices(df)
			}
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically("==", 0))
		})
	})
})
