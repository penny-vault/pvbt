package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Withdrawal Metrics", func() {
	var spy asset.Asset

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	})

	// buildModerateAccount creates an Account with 300 daily data points
	// (~14 months) at 0.02% daily growth. This yields differentiated
	// withdrawal rates: PWR < SWR, both below the 20% ceiling.
	buildModerateAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(100_000))
		price := 100_000.0
		start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

		for i := range 300 {
			d := start.AddDate(0, 0, i)
			if i > 0 {
				growth := price * 0.0002
				a.Record(portfolio.Transaction{
					Date:   d,
					Type:   portfolio.DepositTransaction,
					Amount: growth,
				})
				price += growth
			}
			df := buildDF(d, []asset.Asset{spy}, []float64{450 + float64(i)}, []float64{448 + float64(i)})
			a.UpdatePrices(df)
		}

		return a
	}

	// buildFlatAccount creates an Account with 300 daily data points and
	// zero growth -- the equity curve is constant at 100,000.
	buildFlatAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(100_000))
		start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

		for i := range 300 {
			d := start.AddDate(0, 0, i)
			df := buildDF(d, []asset.Asset{spy}, []float64{450}, []float64{448})
			a.UpdatePrices(df)
		}

		return a
	}

	// buildShortAccount creates an Account with only 10 data points --
	// fewer than the 22 needed for monthlyReturnsFromEquity to produce
	// any monthly returns.
	buildShortAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(10_000))
		start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

		for i := range 10 {
			d := start.AddDate(0, 0, i)
			df := buildDF(d, []asset.Asset{spy}, []float64{450}, []float64{448})
			a.UpdatePrices(df)
		}

		return a
	}

	Describe("SafeWithdrawalRate", func() {
		It("returns 0 when equity curve is too short for monthly returns", func() {
			a := buildShortAccount()
			Expect(portfolio.SafeWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("returns 0.033 for a flat equity curve (seed=42)", func() {
			// A flat curve has 0% monthly returns. The simulation can
			// still survive small withdrawal rates because the bootstrap
			// just replays 0% returns, so the portfolio only shrinks by
			// the withdrawal amount. 3.3% over 30 years barely survives.
			a := buildFlatAccount()
			Expect(portfolio.SafeWithdrawalRate.Compute(a, nil)).To(
				BeNumerically("~", 0.033, 0.001))
		})

		It("returns 0.063 for a moderate-growth equity curve (seed=42)", func() {
			a := buildModerateAccount()
			Expect(portfolio.SafeWithdrawalRate.Compute(a, nil)).To(
				BeNumerically("~", 0.063, 0.001))
		})

		It("returns nil from ComputeSeries", func() {
			a := buildModerateAccount()
			Expect(portfolio.SafeWithdrawalRate.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("PerpetualWithdrawalRate", func() {
		It("returns 0 when equity curve is too short for monthly returns", func() {
			a := buildShortAccount()
			Expect(portfolio.PerpetualWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("returns 0 for a flat equity curve (seed=42)", func() {
			// With 0% returns, any withdrawal at all means the ending
			// balance will be less than the starting balance, so no
			// perpetual rate is sustainable.
			a := buildFlatAccount()
			Expect(portfolio.PerpetualWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("returns 0.049 for a moderate-growth equity curve (seed=42)", func() {
			a := buildModerateAccount()
			Expect(portfolio.PerpetualWithdrawalRate.Compute(a, nil)).To(
				BeNumerically("~", 0.049, 0.001))
		})

		It("returns nil from ComputeSeries", func() {
			a := buildModerateAccount()
			Expect(portfolio.PerpetualWithdrawalRate.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("DynamicWithdrawalRate", func() {
		It("returns 0 when equity curve is too short for monthly returns", func() {
			a := buildShortAccount()
			Expect(portfolio.DynamicWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("returns 0.200 for a flat equity curve (seed=42)", func() {
			// Dynamic withdrawal adapts downward as the portfolio drops,
			// so even with 0% returns the portfolio never fully depletes
			// (withdrawal shrinks proportionally), allowing the maximum
			// rate to pass.
			a := buildFlatAccount()
			Expect(portfolio.DynamicWithdrawalRate.Compute(a, nil)).To(
				BeNumerically("~", 0.200, 0.001))
		})

		It("returns 0.200 for a moderate-growth equity curve (seed=42)", func() {
			a := buildModerateAccount()
			Expect(portfolio.DynamicWithdrawalRate.Compute(a, nil)).To(
				BeNumerically("~", 0.200, 0.001))
		})

		It("returns nil from ComputeSeries", func() {
			a := buildModerateAccount()
			Expect(portfolio.DynamicWithdrawalRate.ComputeSeries(a, nil)).To(BeNil())
		})
	})

	Describe("ordering invariant", func() {
		It("PerpetualWithdrawalRate <= SafeWithdrawalRate <= DynamicWithdrawalRate", func() {
			a := buildModerateAccount()
			pwr := portfolio.PerpetualWithdrawalRate.Compute(a, nil)
			swr := portfolio.SafeWithdrawalRate.Compute(a, nil)
			dwr := portfolio.DynamicWithdrawalRate.Compute(a, nil)

			Expect(pwr).To(BeNumerically("<=", swr))
			Expect(swr).To(BeNumerically("<=", dwr))
		})
	})
})
