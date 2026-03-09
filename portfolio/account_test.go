package portfolio_test

import (
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
