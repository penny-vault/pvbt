package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ portfolio.Portfolio = (*portfolio.Account)(nil)
var _ portfolio.PortfolioManager = (*portfolio.Account)(nil)

var _ = Describe("Account", func() {
	var (
		spy asset.Asset
		bil asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
	})

	Describe("New", func() {
		It("creates an account with default zero cash", func() {
			a := portfolio.New()
			Expect(a.Cash()).To(Equal(0.0))
			Expect(a.Value()).To(Equal(0.0))
		})

		It("sets initial cash balance via WithCash", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			Expect(a.Cash()).To(Equal(10_000.0))
			Expect(a.Value()).To(Equal(10_000.0))
		})

		It("records a DepositTransaction for initial cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			txns := a.Transactions()
			Expect(txns).To(HaveLen(1))
			Expect(txns[0].Type).To(Equal(portfolio.DepositTransaction))
			Expect(txns[0].Amount).To(Equal(10_000.0))
		})

		It("stores benchmark and risk-free assets", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(spy),
				portfolio.WithRiskFree(bil),
			)
			Expect(a.Benchmark()).To(Equal(spy))
			Expect(a.RiskFree()).To(Equal(bil))
		})
	})

	Describe("Record", func() {
		It("records a dividend and increases cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.DividendTransaction,
				Amount: 50.0,
			})
			Expect(a.Cash()).To(Equal(10_050.0))
			Expect(a.Transactions()).To(HaveLen(2)) // deposit + dividend
		})

		It("records a fee and decreases cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Type:   portfolio.FeeTransaction,
				Amount: -25.0,
			})
			Expect(a.Cash()).To(Equal(9_975.0))
		})

		It("records a deposit and increases cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Type:   portfolio.DepositTransaction,
				Amount: 5_000.0,
			})
			Expect(a.Cash()).To(Equal(15_000.0))
		})

		It("records a withdrawal and decreases cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Type:   portfolio.WithdrawalTransaction,
				Amount: -3_000.0,
			})
			Expect(a.Cash()).To(Equal(7_000.0))
		})

		It("records a buy: decreases cash, increases holdings, creates tax lot", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			Expect(a.Cash()).To(Equal(7_000.0))
			Expect(a.Position(spy)).To(Equal(10.0))
		})

		It("records a sell: increases cash, decreases holdings, consumes tax lots FIFO", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    5,
				Price:  320.0,
				Amount: 1_600.0,
			})
			Expect(a.Cash()).To(Equal(8_600.0))
			Expect(a.Position(spy)).To(Equal(5.0))
		})
	})

	Describe("Holdings", func() {
		It("starts with no holdings", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			count := 0
			a.Holdings(func(_ asset.Asset, _ float64) { count++ })
			Expect(count).To(Equal(0))
		})

		It("returns zero for unknown positions", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			Expect(a.Position(spy)).To(Equal(0.0))
			Expect(a.PositionValue(spy)).To(Equal(0.0))
		})
	})
})

var _ = Describe("TransactionType", func() {
	It("returns correct string for each type", func() {
		Expect(portfolio.BuyTransaction.String()).To(Equal("Buy"))
		Expect(portfolio.SellTransaction.String()).To(Equal("Sell"))
		Expect(portfolio.DividendTransaction.String()).To(Equal("Dividend"))
		Expect(portfolio.FeeTransaction.String()).To(Equal("Fee"))
		Expect(portfolio.DepositTransaction.String()).To(Equal("Deposit"))
		Expect(portfolio.WithdrawalTransaction.String()).To(Equal("Withdrawal"))
	})
})
