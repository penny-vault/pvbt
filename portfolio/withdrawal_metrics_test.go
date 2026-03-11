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

	Describe("declining equity curve", func() {
		buildDecliningAccount := func() *portfolio.Account {
			a := portfolio.New(portfolio.WithCash(100_000))
			start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
			price := 100_000.0

			for i := range 300 {
				d := start.AddDate(0, 0, i)
				if i > 0 {
					loss := price * 0.0001
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.WithdrawalTransaction,
						Amount: -loss,
					})
					price -= loss
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{450 - float64(i)*0.1}, []float64{448 - float64(i)*0.1})
				a.UpdatePrices(df)
			}

			return a
		}

		It("SafeWithdrawalRate is lower than for a growing curve", func() {
			declining := buildDecliningAccount()
			growing := buildModerateAccount()

			declSWR := portfolio.SafeWithdrawalRate.Compute(declining, nil)
			growSWR := portfolio.SafeWithdrawalRate.Compute(growing, nil)

			Expect(declSWR).To(BeNumerically("<", growSWR))
		})

		It("PerpetualWithdrawalRate is 0 for declining curve", func() {
			a := buildDecliningAccount()
			Expect(portfolio.PerpetualWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})
	})

	Describe("equity curve just above minimum length", func() {
		// Build an account with exactly 12 equity points. This passes the
		// len(equity) < 12 gate in each Compute method, but
		// monthlyReturnsFromEquity requires 22+ points to produce any
		// monthly return, so the second guard (len(monthly) == 0) triggers.
		buildMinLengthAccount := func() *portfolio.Account {
			a := portfolio.New(portfolio.WithCash(10_000))
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

			for i := range 12 {
				d := start.AddDate(0, 0, i)
				if i > 0 {
					growth := 10_000.0 * 0.001
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.DepositTransaction,
						Amount: growth,
					})
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{450 + float64(i)}, []float64{448 + float64(i)})
				a.UpdatePrices(df)
			}

			return a
		}

		It("SafeWithdrawalRate returns 0 for exactly 12 equity points", func() {
			a := buildMinLengthAccount()
			Expect(portfolio.SafeWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("PerpetualWithdrawalRate returns 0 for exactly 12 equity points", func() {
			a := buildMinLengthAccount()
			Expect(portfolio.PerpetualWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("DynamicWithdrawalRate returns 0 for exactly 12 equity points", func() {
			a := buildMinLengthAccount()
			Expect(portfolio.DynamicWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})
	})

	Describe("steeply declining curve", func() {
		// Build an account with 300 daily data points losing ~0.5% per day.
		// The monthly returns are deeply negative, so no withdrawal rate
		// should be sustainable for either SWR or PWR.
		buildSteepDeclineAccount := func() *portfolio.Account {
			a := portfolio.New(portfolio.WithCash(100_000))
			start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
			price := 100_000.0

			for i := range 300 {
				d := start.AddDate(0, 0, i)
				if i > 0 {
					loss := price * 0.005
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.WithdrawalTransaction,
						Amount: -loss,
					})
					price -= loss
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{450 - float64(i)*0.5}, []float64{448 - float64(i)*0.5})
				a.UpdatePrices(df)
			}

			return a
		}

		It("SafeWithdrawalRate is 0 for a steeply declining curve", func() {
			a := buildSteepDeclineAccount()
			Expect(portfolio.SafeWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("PerpetualWithdrawalRate is 0 for a steeply declining curve", func() {
			a := buildSteepDeclineAccount()
			Expect(portfolio.PerpetualWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})

		It("DynamicWithdrawalRate hits ceiling for a steeply declining curve", func() {
			// Dynamic withdrawal adapts downward as the portfolio drops,
			// so the portfolio never fully depletes even with negative
			// returns. This allows the maximum 20% rate to pass.
			a := buildSteepDeclineAccount()
			Expect(portfolio.DynamicWithdrawalRate.Compute(a, nil)).To(
				BeNumerically("~", 0.200, 0.001))
		})
	})

	Describe("very high growth curve", func() {
		// Build an account with 300 daily data points at 0.2% daily growth
		// (10x the moderate account). This should produce higher withdrawal
		// rates than the moderate-growth account.
		buildHighGrowthAccount := func() *portfolio.Account {
			a := portfolio.New(portfolio.WithCash(100_000))
			price := 100_000.0
			start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

			for i := range 300 {
				d := start.AddDate(0, 0, i)
				if i > 0 {
					growth := price * 0.002
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.DepositTransaction,
						Amount: growth,
					})
					price += growth
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{450 + float64(i)*2}, []float64{448 + float64(i)*2})
				a.UpdatePrices(df)
			}

			return a
		}

		It("SafeWithdrawalRate exceeds the moderate-growth rate", func() {
			high := buildHighGrowthAccount()
			moderate := buildModerateAccount()

			highSWR := portfolio.SafeWithdrawalRate.Compute(high, nil)
			modSWR := portfolio.SafeWithdrawalRate.Compute(moderate, nil)

			Expect(highSWR).To(BeNumerically(">", modSWR))
		})

		It("PerpetualWithdrawalRate exceeds the moderate-growth rate", func() {
			high := buildHighGrowthAccount()
			moderate := buildModerateAccount()

			highPWR := portfolio.PerpetualWithdrawalRate.Compute(high, nil)
			modPWR := portfolio.PerpetualWithdrawalRate.Compute(moderate, nil)

			Expect(highPWR).To(BeNumerically(">", modPWR))
		})

		It("ordering invariant holds: PWR <= SWR <= DWR", func() {
			a := buildHighGrowthAccount()
			pwr := portfolio.PerpetualWithdrawalRate.Compute(a, nil)
			swr := portfolio.SafeWithdrawalRate.Compute(a, nil)
			dwr := portfolio.DynamicWithdrawalRate.Compute(a, nil)

			Expect(pwr).To(BeNumerically("<=", swr))
			Expect(swr).To(BeNumerically("<=", dwr))
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
