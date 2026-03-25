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

var _ = Describe("MonthlyReturns", func() {
	It("computes month-over-month returns with NaN for the first month", func() {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		acct := portfolio.New(portfolio.WithCash(100, time.Time{}))

		// Build 3 months of data: Jan=100, Feb=110, Mar=105
		dates := []time.Time{
			time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC),
		}
		equityValues := []float64{100, 110, 105}

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

		years, grid, err := acct.MonthlyReturns(data.PortfolioEquity)
		Expect(err).NotTo(HaveOccurred())
		Expect(years).To(Equal([]int{2024}))
		Expect(grid).To(HaveLen(1))

		row := grid[0]
		// Jan (index 0): NaN (no prior month)
		Expect(math.IsNaN(row[0])).To(BeTrue())
		// Feb (index 1): (110-100)/100 = 0.10
		Expect(row[1]).To(BeNumerically("~", 0.10, 1e-6))
		// Mar (index 2): (105-110)/110 = -0.04545...
		Expect(row[2]).To(BeNumerically("~", -0.04545454545, 1e-6))
		// Apr-Dec: NaN
		for month := 3; month < 12; month++ {
			Expect(math.IsNaN(row[month])).To(BeTrue())
		}
	})

	It("returns nil when perfData is nil", func() {
		acct := portfolio.New()
		years, grid, err := acct.MonthlyReturns(data.PortfolioEquity)
		Expect(err).NotTo(HaveOccurred())
		Expect(years).To(BeNil())
		Expect(grid).To(BeNil())
	})
})
