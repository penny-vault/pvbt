package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("AnnualReturns", func() {
	It("computes year-over-year returns using first value as baseline for year one", func() {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		acct := portfolio.New(portfolio.WithCash(100, time.Time{}))

		// Equity curve: Jan 2024=100, Dec 2024=120, Dec 2025=150
		dates := []time.Time{
			time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		equityValues := []float64{100, 120, 150}

		for idx, date := range dates {
			if idx > 0 {
				diff := equityValues[idx] - equityValues[idx-1]
				if diff > 0 {
					acct.Record(portfolio.Transaction{
						Date:   date,
						Type:   asset.DepositTransaction,
						Amount: diff,
					})
				} else if diff < 0 {
					acct.Record(portfolio.Transaction{
						Date:   date,
						Type:   asset.WithdrawalTransaction,
						Amount: diff,
					})
				}
			}
			df := buildDF(date, []asset.Asset{spy}, []float64{450}, []float64{448})
			acct.UpdatePrices(df)
		}

		years, returns, err := acct.AnnualReturns(data.PortfolioEquity)
		Expect(err).NotTo(HaveOccurred())
		Expect(years).To(Equal([]int{2024, 2025}))
		Expect(returns).To(HaveLen(2))
		// 2024: (120-100)/100 = 0.20
		Expect(returns[0]).To(BeNumerically("~", 0.20, 1e-6))
		// 2025: (150-120)/120 = 0.25
		Expect(returns[1]).To(BeNumerically("~", 0.25, 1e-6))
	})

	It("returns nil when perfData is nil", func() {
		acct := portfolio.New()
		years, returns, err := acct.AnnualReturns(data.PortfolioEquity)
		Expect(err).NotTo(HaveOccurred())
		Expect(years).To(BeNil())
		Expect(returns).To(BeNil())
	})
})
