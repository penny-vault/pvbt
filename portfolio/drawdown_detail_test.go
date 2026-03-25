package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("DrawdownDetails", func() {
	// buildEquityAccount creates an Account whose equity curve matches the
	// given values on the given dates.
	buildEquityAccount := func(dates []time.Time, equityValues []float64) *portfolio.Account {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		acct := portfolio.New(portfolio.WithCash(equityValues[0], time.Time{}))

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

		return acct
	}

	It("identifies two drawdowns sorted by depth", func() {
		// Equity: 100, 120, 100, 120, 130, 120, 130
		// Drawdown 1: peak=120, trough=100, depth=-16.7%, recovered at 120
		// Drawdown 2: peak=130, trough=120, depth=-7.7%, recovered at 130
		dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 7)
		equity := []float64{100, 120, 100, 120, 130, 120, 130}

		acct := buildEquityAccount(dates, equity)

		drawdowns, err := acct.DrawdownDetails(10)
		Expect(err).NotTo(HaveOccurred())
		Expect(drawdowns).To(HaveLen(2))

		// Most negative first.
		Expect(drawdowns[0].Depth).To(BeNumerically("~", -1.0/6.0, 1e-6))
		Expect(drawdowns[0].Start).To(Equal(dates[1]))
		Expect(drawdowns[0].Trough).To(Equal(dates[2]))
		Expect(drawdowns[0].Recovery).To(Equal(dates[3]))

		Expect(drawdowns[1].Depth).To(BeNumerically("~", -10.0/130.0, 1e-6))
		Expect(drawdowns[1].Start).To(Equal(dates[4]))
		Expect(drawdowns[1].Trough).To(Equal(dates[5]))
		Expect(drawdowns[1].Recovery).To(Equal(dates[6]))
	})

	It("caps results to topN", func() {
		dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 7)
		equity := []float64{100, 120, 100, 120, 130, 120, 130}

		acct := buildEquityAccount(dates, equity)

		drawdowns, err := acct.DrawdownDetails(1)
		Expect(err).NotTo(HaveOccurred())
		Expect(drawdowns).To(HaveLen(1))
		// Should be the deepest one.
		Expect(drawdowns[0].Depth).To(BeNumerically("~", -1.0/6.0, 1e-6))
	})

	It("returns nil when perfData is nil", func() {
		acct := portfolio.New()
		drawdowns, err := acct.DrawdownDetails(5)
		Expect(err).NotTo(HaveOccurred())
		Expect(drawdowns).To(BeNil())
	})

	It("handles unrecovered drawdown at end of data", func() {
		// Equity: 100, 120, 90 -- drawdown never recovers
		dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
		equity := []float64{100, 120, 90}

		acct := buildEquityAccount(dates, equity)

		drawdowns, err := acct.DrawdownDetails(10)
		Expect(err).NotTo(HaveOccurred())
		Expect(drawdowns).To(HaveLen(1))

		Expect(drawdowns[0].Start).To(Equal(dates[1]))
		Expect(drawdowns[0].Trough).To(Equal(dates[2]))
		Expect(drawdowns[0].Recovery.IsZero()).To(BeTrue())
		Expect(drawdowns[0].Depth).To(BeNumerically("~", -30.0/120.0, 1e-6))
		Expect(drawdowns[0].Days).To(Equal(1)) // from index 1 to index 2
	})

	It("handles data with a drawdown from the start that recovers to exactly peak", func() {
		// Equity peaks at first data point and then recovers exactly
		dates := daySeq(time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC), 4)
		equity := []float64{200, 180, 190, 200}

		acct := buildEquityAccount(dates, equity)

		drawdowns, err := acct.DrawdownDetails(10)
		Expect(err).NotTo(HaveOccurred())
		Expect(drawdowns).To(HaveLen(1))
		Expect(drawdowns[0].Depth).To(BeNumerically("~", -20.0/200.0, 1e-6))
		Expect(drawdowns[0].Recovery).To(Equal(dates[3]))
	})
})
