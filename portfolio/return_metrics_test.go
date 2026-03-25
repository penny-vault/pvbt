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

var _ = Describe("Return Metrics", func() {
	var (
		spy asset.Asset
		bm  asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bm = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
	})

	// buildReturnAccount creates an account with the given equity curve.
	// It uses DividendTransaction (positive) and FeeTransaction (negative)
	// to move cash so equity = cash. These transaction types are invisible
	// to the TWRR flow filter, which only looks at deposits/withdrawals.
	buildReturnAccount := func(dates []time.Time, equity []float64) *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(equity[0], time.Time{}))
		for ii, dd := range dates {
			if ii > 0 {
				diff := equity[ii] - equity[ii-1]
				if diff > 0 {
					a.Record(portfolio.Transaction{
						Date:   dd,
						Type:   asset.DividendTransaction,
						Amount: diff,
					})
				} else if diff < 0 {
					a.Record(portfolio.Transaction{
						Date:   dd,
						Type:   asset.FeeTransaction,
						Amount: diff,
					})
				}
			}
			df := buildDF(dd, []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df)
		}
		return a
	}

	Describe("TWRR", func() {
		It("compounds sub-period returns for a 5-point equity curve", func() {
			// Equity: [100, 110, 105, 115, 120]
			// Returns: 10/100=0.10, -5/110=-1/22, 10/105=2/21, 5/115=1/23
			// Product: (110/100)*(105/110)*(115/105)*(120/115) = 120/100 = 1.20
			// TWRR = 1.20 - 1 = 0.20
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 5)
			a := buildReturnAccount(dates, []float64{100, 110, 105, 115, 120})

			result, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.20, 1e-9))
		})

		It("computes cumulative return at each point in the series", func() {
			// Equity: [100, 110, 105, 115, 120]
			// Period returns: r0=10/100, r1=-5/110, r2=10/105, r3=5/115
			// Cumulative product at each step:
			//   cum[0] = (110/100) - 1 = 0.10
			//   cum[1] = (110/100)*(105/110) - 1 = 105/100 - 1 = 0.05
			//   cum[2] = (105/100)*(115/105) - 1 = 115/100 - 1 = 0.15
			//   cum[3] = (115/100)*(120/115) - 1 = 120/100 - 1 = 0.20
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 5)
			a := buildReturnAccount(dates, []float64{100, 110, 105, 115, 120})

			df, err := a.PerformanceMetric(portfolio.TWRR).Series()
			Expect(err).NotTo(HaveOccurred())
			Expect(df.Len()).To(Equal(4))
			series := df.Column(perfAsset, data.PortfolioEquity)
			Expect(series[0]).To(BeNumerically("~", 0.10, 1e-9))
			Expect(series[1]).To(BeNumerically("~", 0.05, 1e-9))
			Expect(series[2]).To(BeNumerically("~", 0.15, 1e-9))
			Expect(series[3]).To(BeNumerically("~", 0.20, 1e-9))
		})

		It("returns negative total return for a declining equity curve", func() {
			// Equity: [100, 90, 80]
			// Product: (90/100)*(80/90) = 80/100 = 0.80
			// TWRR = 0.80 - 1 = -0.20
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
			a := buildReturnAccount(dates, []float64{100, 90, 80})

			result, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", -0.20, 1e-9))
		})

		It("returns 0 for a single data point", func() {
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 1)
			a := buildReturnAccount(dates, []float64{100})

			result, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("isolates market growth from deposit-driven equity changes", func() {
			// Day 0: deposit 10000, equity = 10000
			// Day 1: organic growth +1000, equity = 11000 (no deposit)
			// Day 2: deposit 5000 + organic growth +1000, equity = 17000
			//
			// Sub-period 1 (day 0->1): no flow
			//   preFlowEquity = 11000, return = 11000/10000 - 1 = 0.10
			// Sub-period 2 (day 1->2): flow = 5000 (deposit)
			//   preFlowEquity = 17000 - 5000 = 12000, return = 12000/11000 - 1
			//   subPeriodStart resets to 17000
			// TWRR = (11000/10000) * (12000/11000) - 1 = 12000/10000 - 1 = 0.20
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)

			aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df0)

			// Day 1: organic growth +1000
			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DividendTransaction,
				Amount: 1000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df1)

			// Day 2: deposit 5000 + organic growth +1000
			aa.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.DepositTransaction,
				Amount: 5000,
			})
			aa.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.DividendTransaction,
				Amount: 1000,
			})
			df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df2)

			result, err := aa.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.20, 1e-9))
		})

		It("handles withdrawals correctly", func() {
			// Day 0: deposit 10000, equity = 10000
			// Day 1: market loss -1000, equity = 9000
			// Day 2: withdrawal 3000 + market gain +1000, equity = 7000
			//
			// Sub-period 1 (day 0->1): no flow
			//   preFlowEquity = 9000, return = 9000/10000 - 1 = -0.10
			// Sub-period 2 (day 1->2): flow = -3000 (withdrawal)
			//   preFlowEquity = 7000 - (-3000) = 10000, return = 10000/9000 - 1
			// TWRR = (9000/10000) * (10000/9000) - 1 = 0.0
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)

			aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df0)

			// Day 1: market loss -1000
			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.FeeTransaction,
				Amount: -1000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df1)

			// Day 2: withdrawal 3000 + market gain +1000
			aa.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.WithdrawalTransaction,
				Amount: -3000,
			})
			aa.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.DividendTransaction,
				Amount: 1000,
			})
			df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df2)

			result, err := aa.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("produces correct cumulative series with external flows", func() {
			// Same scenario as "isolates market growth from deposit-driven equity changes"
			// Day 0: deposit 10000, equity = 10000
			// Day 1: organic growth +1000, equity = 11000
			// Day 2: deposit 5000 + organic growth +1000, equity = 17000
			//
			// cum[0] = (11000/10000) - 1 = 0.10
			// cum[1] = (11000/10000) * (12000/11000) - 1 = 0.20
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)

			aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df0)

			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DividendTransaction,
				Amount: 1000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df1)

			aa.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.DepositTransaction,
				Amount: 5000,
			})
			aa.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.DividendTransaction,
				Amount: 1000,
			})
			df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df2)

			seriesDF, err := aa.PerformanceMetric(portfolio.TWRR).Series()
			Expect(err).NotTo(HaveOccurred())
			Expect(seriesDF.Len()).To(Equal(2))
			series := seriesDF.Column(perfAsset, data.PortfolioEquity)
			Expect(series[0]).To(BeNumerically("~", 0.10, 1e-9))
			Expect(series[1]).To(BeNumerically("~", 0.20, 1e-9))
		})

		It("diverges from MWRR when deposit timing matters", func() {
			// Day 0: deposit 10000, equity = 10000
			// Day 183: market grew 20% (+2000), then deposit 10000, equity = 22000
			// Day 367: market dropped 10% (-2200), equity = 19800
			//
			// TWRR = (12000/10000) * (19800/22000) - 1 = 1.2 * 0.9 - 1 = 0.08
			// MWRR will be lower (more money exposed to the loss)
			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 7, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df0)

			// Day 183: market grew 20% (+2000), then deposit 10000
			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DividendTransaction,
				Amount: 2000,
			})
			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DepositTransaction,
				Amount: 10_000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df1)

			// Day 367: market dropped 10% (-2200)
			aa.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.FeeTransaction,
				Amount: -2200,
			})
			df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df2)

			twrr, err := aa.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(twrr).To(BeNumerically("~", 0.08, 1e-9))

			mwrr, err := aa.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(mwrr).To(BeNumerically("<", twrr),
				"MWRR should be lower than TWRR when more capital is exposed to losses")
		})

		It("sums multiple same-day flows correctly", func() {
			// Day 0: deposit 10000, equity = 10000
			// Day 1: deposit 3000 + withdrawal 1000 + organic growth 500, equity = 12500
			//   net flow = 3000 + (-1000) = 2000
			//   preFlowEquity = 12500 - 2000 = 10500
			//   return = 10500/10000 - 1 = 0.05
			// TWRR = 0.05
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 2)

			aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df0)

			// Day 1: deposit 3000, withdrawal 1000, organic growth 500
			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DepositTransaction,
				Amount: 3000,
			})
			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.WithdrawalTransaction,
				Amount: -1000,
			})
			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DividendTransaction,
				Amount: 500,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df1)

			result, err := aa.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.05, 1e-9))
		})

		It("handles zero equity at sub-period start without division by zero", func() {
			// Day 0: no cash, equity = 0
			// Day 1: deposit 10000, equity = 10000
			//   flow = 10000, preFlowEquity = 10000 - 10000 = 0
			//   subPeriodStart was 0 => return = 0 (guard)
			//   subPeriodStart resets to 10000
			// Day 2: organic growth +1000, equity = 11000
			//   no flow, preFlowEquity = 11000
			//   return = 11000/10000 - 1 = 0.10
			// TWRR = (1+0) * (1+0.10) - 1 = 0.10
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)

			aa := portfolio.New(portfolio.WithCash(0, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df0)

			// Day 1: deposit 10000
			aa.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DepositTransaction,
				Amount: 10_000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df1)

			// Day 2: organic growth +1000
			aa.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.DividendTransaction,
				Amount: 1000,
			})
			df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
			aa.UpdatePrices(df2)

			result, err := aa.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.10, 1e-9))
		})
	})

	Describe("MWRR", func() {
		It("matches TWRR when there are no mid-stream cash flows", func() {
			// Build account manually with DividendTransaction for organic
			// growth so MWRR sees only the initial deposit as a cash flow.
			// Equity: [10000, 11000] over 367 days (Jan 2 2024 to Jan 3 2025).
			// Flows: -10000 at t0 (synthetic), +11000 at t1.
			// XIRR solves: -10000 + 11000/(1+r)^(367/365) = 0
			//   (1+r)^(367/365) = 1.1
			//   r = 1.1^(365/367) - 1 = 0.09942880667...
			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df0)

			// Organic growth via dividend (not an external cash flow for MWRR).
			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DividendTransaction,
				Amount: 1000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df1)

			result, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			expected := math.Pow(1.1, 365.0/367.0) - 1
			Expect(result).To(BeNumerically("~", expected, 1e-9))
		})

		It("agrees with MWRR when deposits produce no market growth", func() {
			// Day 0: deposit 10000, equity=10000
			// Day 183 (Jul 3): deposit 500 + deposit 5000 -> equity 15500
			// Day 367 (Jan 3 next year): deposit 1000 -> equity 16500
			//
			// Total deposits: 10000 + 500 + 5000 + 1000 = 16500
			// Terminal value: 16500
			// All equity growth comes from deposits, not market returns.
			//
			// TWRR strips deposits from the equity curve, so each sub-period
			// return is 0. MWRR also sees zero return because total cash in
			// equals terminal value. Both metrics correctly report 0.
			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 7, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			// Day 0: initial state
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df0)

			// Day 183: deposit 500 then deposit 5000 -> equity 15500
			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DepositTransaction,
				Amount: 500,
			})
			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DepositTransaction,
				Amount: 5000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df1)

			// Day 367: deposit 1000 -> equity 16500
			a.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   asset.DepositTransaction,
				Amount: 1000,
			})
			df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df2)

			// Verify equity curve is as expected.
			Expect(a.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10_000, 15_500, 16_500}))

			result, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			twrrResult, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			// TWRR = 0.0 because deposits are stripped; no market growth occurred.
			Expect(twrrResult).To(BeNumerically("~", 0.0, 1e-9))

			// MWRR = 0.0 because total deposits (16500) equal terminal value (16500).
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns 0 for a single data point", func() {
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 1)
			a := buildReturnAccount(dates, []float64{10_000})

			result, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("returns nil for ComputeSeries (MWRR is a scalar metric)", func() {
			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df0)

			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DividendTransaction,
				Amount: 1000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df1)

			df, err := a.PerformanceMetric(portfolio.MWRR).Series()
			Expect(err).NotTo(HaveOccurred())
			Expect(df).To(BeNil())
		})

		It("returns positive MWRR when terminal value exceeds total deposits", func() {
			// Day 0: deposit 10000, equity=10000
			// Day 367: dividend 1500 (organic growth, not a deposit) -> equity 11500
			//
			// Flows: -10000 at t0 (synthetic), +11500 at t1
			// XIRR solves: -10000 + 11500/(1+r)^(367/365) = 0
			//   (1+r)^(367/365) = 1.15
			//   r = 1.15^(365/367) - 1
			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df0)

			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DividendTransaction,
				Amount: 1500,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df1)

			result, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			expected := math.Pow(1.15, 365.0/367.0) - 1
			Expect(result).To(BeNumerically("~", expected, 1e-9))
		})

		It("returns negative MWRR when terminal value is less than deposits", func() {
			// Day 0: deposit 10000, equity=10000
			// Day 367: fee of 2000 (negative dividend) reduces equity to 8000.
			// We use DividendTransaction (not Withdrawal) so MWRR sees no
			// external cash flow -- only the initial deposit and terminal value.
			//
			// Flows: -10000 at d=0 (synthetic), +8000 at d=367 (terminal)
			// XIRR solves: -10000 + 8000/(1+r)^(367/365) = 0
			//   (1+r)^(367/365) = 0.8
			//   r = 0.8^(365/367) - 1 (negative)
			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df0)

			// Simulate loss via negative dividend (fee/expense).
			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   asset.DividendTransaction,
				Amount: -2000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df1)

			// Equity curve: [10000, 8000]
			Expect(a.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10_000, 8_000}))

			result, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			// Flows: -10000 at d=0 (synthetic), +8000 at d=367 (terminal)
			// -10000 + 8000/(1+r)^(367/365) = 0
			// (1+r)^(367/365) = 0.8
			// r = 0.8^(365/367) - 1 (negative)
			expected := math.Pow(0.8, 365.0/367.0) - 1
			Expect(result).To(BeNumerically("~", expected, 1e-9))
			Expect(result).To(BeNumerically("<", 0.0))
		})
	})

	// recordBuy is a helper that records a buy transaction on the account.
	// It decreases cash by price*qty and increases holdings.
	recordBuy := func(a *portfolio.Account, ast asset.Asset, d time.Time, qty, price float64) {
		a.Record(portfolio.Transaction{
			Date:   d,
			Asset:  ast,
			Type:   asset.BuyTransaction,
			Qty:    qty,
			Price:  price,
			Amount: -(price * qty),
		})
	}

	// recordSell is a helper that records a sell transaction on the account.
	// It increases cash by price*qty and decreases holdings.
	recordSell := func(a *portfolio.Account, ast asset.Asset, d time.Time, qty, price float64) {
		a.Record(portfolio.Transaction{
			Date:   d,
			Asset:  ast,
			Type:   asset.SellTransaction,
			Qty:    qty,
			Price:  price,
			Amount: price * qty,
		})
	}

	Describe("TWRR and MWRR with real portfolio mechanics", func() {
		It("reflects actual price returns for buy-and-hold with no mid-stream cash flows", func() {
			// Setup: Buy 100 shares of SPY at $100 with $10000 initial cash.
			// Prices over 5 days: 100, 110, 105, 115, 125.
			// Since all cash is invested and there are no deposits/withdrawals
			// after the initial one, the equity curve tracks price changes exactly.
			//
			// Equity curve: [10000, 11000, 10500, 11500, 12500]
			//   (cash=0 throughout, equity = 100 shares * price)
			//
			// TWRR = product(equity[i+1]/equity[i]) - 1
			//      = (11000/10000)*(10500/11000)*(11500/10500)*(12500/11500) - 1
			//      = 12500/10000 - 1 = 0.25
			//
			// This matches the simple price return: 125/100 - 1 = 0.25.

			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 5)
			prices := []float64{100, 110, 105, 115, 125}

			a := portfolio.New(portfolio.WithCash(10000, time.Time{}))
			recordBuy(a, spy, dates[0], 100, 100)

			for _, d := range dates {
				df := buildDF(d, []asset.Asset{spy}, []float64{prices[0]}, []float64{prices[0]})
				prices = prices[1:]
				a.UpdatePrices(df)
			}

			// Verify equity curve.
			Expect(a.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10000, 11000, 10500, 11500, 12500}))

			twrrVal, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(twrrVal).To(BeNumerically("~", 0.25, 1e-9))

			// MWRR should also be positive (we gained 25% in absolute terms).
			mwrrVal, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(mwrrVal).To(BeNumerically(">", 0.0))
		})

		It("shows MWRR exceeds asset return when deposit occurs before a rally", func() {
			// Scenario: initial buy, then deposit more cash and buy more shares
			// just before prices rally. Good timing benefits MWRR.
			//
			// Timeline (quarterly over 1 year):
			//   Day 0 (Jan 2 2024): deposit 10000, buy 100@100. Cash=0. Eq=10000.
			//   Day 1 (Jul 2 2024): deposit 10000, buy 90@110 (cost=9900).
			//     Cash=100, holdings=190. UpdatePrices at 110: Eq=100+190*110=21000.
			//   Day 2 (Jan 2 2025): price=150. Eq=100+190*150=28600.
			//
			// Equity: [10000, 21000, 28600]
			// TWRR strips the deposit:
			//   r1 = (21000-10000)/10000 = 0.10 (market moved 100->110)
			//   r2 = 28600/21000 - 1 = 0.3619
			//   TWRR = 1.10 * (28600/21000) - 1 = 1573/1050 - 1
			//
			// MWRR flows:
			//   -10000 at Jan 2 2024 (initial deposit, zero date -> times[0])
			//   -10000 at Jul 2 2024 (mid-stream deposit)
			//   +28600 at Jan 2 2025 (terminal)
			//
			// The asset itself returned 150/100 - 1 = 50%.
			// Because we deposited MORE money before the rally (110->150, a 36%
			// gain on the second tranche), the money-weighted return should
			// exceed the asset's simple return (annualized).

			d0 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			d1 := time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC)
			d2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			a := portfolio.New(portfolio.WithCash(10000, time.Time{}))
			recordBuy(a, spy, d0, 100, 100)
			a.UpdatePrices(buildDF(d0, []asset.Asset{spy}, []float64{100}, []float64{100}))

			// Deposit and buy more before the rally, then snapshot equity.
			a.Record(portfolio.Transaction{
				Date:   d1,
				Type:   asset.DepositTransaction,
				Amount: 10000,
			})
			recordBuy(a, spy, d1, 90, 110) // cost = 9900, cash = 100
			a.UpdatePrices(buildDF(d1, []asset.Asset{spy}, []float64{110}, []float64{110}))

			a.UpdatePrices(buildDF(d2, []asset.Asset{spy}, []float64{150}, []float64{150}))

			Expect(a.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10000, 21000, 28600}))

			// TWRR = 1.10 * (28600/21000) - 1 = 1573/1050 - 1
			twrrVal, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(twrrVal).To(BeNumerically("~", 1573.0/1050.0-1, 1e-9))

			mwrrVal, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(mwrrVal).To(BeNumerically(">", 0.0))

			// The annualized asset return over 366 days (leap year) is:
			//   assetCAGR = (150/100)^(365/366) - 1
			// MWRR should exceed this because money was added before the rally.
			assetCAGR := math.Pow(150.0/100.0, 365.0/366.0) - 1
			Expect(mwrrVal).To(BeNumerically(">", assetCAGR))
		})

		It("shows MWRR falls below asset return when deposit occurs before a decline", func() {
			// Same structure as above, but prices DROP after the deposit.
			//
			// Timeline:
			//   Day 0 (Jan 2 2024): deposit 10000, buy 100@100. Cash=0. Eq=10000.
			//   Day 1 (Jul 2 2024): deposit 10000, buy 90@110 (cost=9900).
			//     Cash=100, holdings=190. UpdatePrices at 110: Eq=100+190*110=21000.
			//   Day 2 (Jan 2 2025): price=90. Eq=100+190*90=17200.
			//
			// Equity: [10000, 21000, 17200]
			// TWRR strips the deposit:
			//   r1 = (21000-10000)/10000 = 0.10 (market moved 100->110)
			//   r2 = 17200/21000 - 1
			//   TWRR = 1.1 * (17200/21000) - 1
			//
			// MWRR flows:
			//   -10000 at Jan 2 2024
			//   -10000 at Jul 2 2024
			//   +17200 at Jan 2 2025
			//
			// The asset returned 90/100 - 1 = -10%.
			// The investor deposited more before the decline (110->90, an 18%
			// loss on the second tranche), so MWRR should be BELOW the asset
			// return -- worse timing means worse money-weighted return.

			d0 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			d1 := time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC)
			d2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			a := portfolio.New(portfolio.WithCash(10000, time.Time{}))
			recordBuy(a, spy, d0, 100, 100)
			a.UpdatePrices(buildDF(d0, []asset.Asset{spy}, []float64{100}, []float64{100}))

			// Deposit and buy more before the decline, then snapshot equity.
			a.Record(portfolio.Transaction{
				Date:   d1,
				Type:   asset.DepositTransaction,
				Amount: 10000,
			})
			recordBuy(a, spy, d1, 90, 110)
			a.UpdatePrices(buildDF(d1, []asset.Asset{spy}, []float64{110}, []float64{110}))

			a.UpdatePrices(buildDF(d2, []asset.Asset{spy}, []float64{90}, []float64{90}))

			Expect(a.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10000, 21000, 17200}))

			// TWRR = 1.1 * (17200/21000) - 1
			twrrVal, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(twrrVal).To(BeNumerically("~", 1.1*17200.0/21000.0-1, 1e-9))

			mwrrVal, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			// Asset CAGR over 366 days: (90/100)^(365/366) - 1 (negative).
			// MWRR should be below this because more money was exposed to the decline.
			assetCAGR := math.Pow(90.0/100.0, 365.0/366.0) - 1
			Expect(mwrrVal).To(BeNumerically("<", assetCAGR))
		})

		It("handles withdrawal mid-stream with partial position sale", func() {
			// Scenario: buy shares, prices rise, sell some and withdraw, prices continue.
			//
			// Timeline:
			//   Day 0 (Jan 2 2024): deposit 10000, buy 100@100. Cash=0. Eq=10000.
			//   Day 1 (Apr 2 2024): sell 50@120 (proceeds=6000). Cash=6000, holdings=50.
			//     Withdraw 5000. Cash=1000, holdings=50.
			//     UpdatePrices at 120: Eq=1000+50*120=7000.
			//   Day 2 (Jul 2 2024): price=130. Eq=1000+50*130=7500.
			//   Day 3 (Jan 2 2025): price=140. Eq=1000+50*140=8000.
			//
			// Equity: [10000, 7000, 7500, 8000]
			// TWRR strips the withdrawal (-5000):
			//   r1 = (7000-(-5000))/10000 - 1 = 12000/10000 - 1 = 0.20
			//   r2 = 7500/7000 - 1
			//   r3 = 8000/7500 - 1
			//   TWRR = 1.2 * (7500/7000) * (8000/7500) - 1 = 1.2 * 8/7 - 1
			//
			// MWRR flows:
			//   -10000 at Jan 2 2024 (initial deposit)
			//   +5000 at Apr 2 2024 (withdrawal; Amount is -5000, negated = +5000)
			//   +8000 at Jan 2 2025 (terminal)

			d0 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			d1 := time.Date(2024, 4, 2, 0, 0, 0, 0, time.UTC)
			d2 := time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC)
			d3 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			a := portfolio.New(portfolio.WithCash(10000, time.Time{}))
			recordBuy(a, spy, d0, 100, 100)
			a.UpdatePrices(buildDF(d0, []asset.Asset{spy}, []float64{100}, []float64{100}))

			// Sell 50 shares at 120, then withdraw 5000, then snapshot equity.
			recordSell(a, spy, d1, 50, 120)
			a.Record(portfolio.Transaction{
				Date:   d1,
				Type:   asset.WithdrawalTransaction,
				Amount: -5000,
			})
			a.UpdatePrices(buildDF(d1, []asset.Asset{spy}, []float64{120}, []float64{120}))

			a.UpdatePrices(buildDF(d2, []asset.Asset{spy}, []float64{130}, []float64{130}))
			a.UpdatePrices(buildDF(d3, []asset.Asset{spy}, []float64{140}, []float64{140}))

			Expect(a.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10000, 7000, 7500, 8000}))

			// TWRR = 1.2 * (7500/7000) * (8000/7500) - 1 = 1.2 * 8/7 - 1
			twrrVal, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(twrrVal).To(BeNumerically("~", 1.2*8.0/7.0-1, 1e-9))

			// MWRR: investor put in 10000, took out 5000 midway, left with 8000.
			// Net gain = 5000 + 8000 - 10000 = 3000 (positive).
			// So MWRR should be positive.
			mwrrVal, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(mwrrVal).To(BeNumerically(">", 0.0))
		})

		It("differentiates MWRR across multiple deposits with varying timing", func() {
			// Two accounts with the same total invested ($20000), same prices,
			// but deposits timed differently. Prices rise steadily, then fall.
			//
			// Prices: 100, 120, 140, 110 (quarterly over 9 months)
			// Account A deposits early (before the rise) -- good timing.
			// Account B deposits late (before the fall) -- bad timing.
			//
			// Account A:
			//   d0: deposit 10000, buy 100@100. Cash=0. Eq=10000.
			//   d1: deposit 10000, buy 83@120 (cost=9960). Cash=40, holdings=183.
			//     UpdatePrices at 120: Eq=40+183*120=22000.
			//   d2: price=140. Eq=40+183*140=25660.
			//   d3: price=110. Eq=40+183*110=20170.
			//
			// Account B:
			//   d0: deposit 10000, buy 100@100. Cash=0. Eq=10000.
			//   d1: price=120. Eq=12000.
			//   d2: deposit 10000, buy 71@140 (cost=9940). Cash=60, holdings=171.
			//     UpdatePrices at 140: Eq=60+171*140=24000.
			//   d3: price=110. Eq=60+171*110=18870.

			d0 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			d1 := time.Date(2024, 4, 2, 0, 0, 0, 0, time.UTC)
			d2 := time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC)
			d3 := time.Date(2024, 10, 2, 0, 0, 0, 0, time.UTC)

			// --- Account A: deposit early (d1, before continued rise) ---
			acctA := portfolio.New(portfolio.WithCash(10000, time.Time{}))
			recordBuy(acctA, spy, d0, 100, 100)
			acctA.UpdatePrices(buildDF(d0, []asset.Asset{spy}, []float64{100}, []float64{100}))

			acctA.Record(portfolio.Transaction{Date: d1, Type: asset.DepositTransaction, Amount: 10000})
			recordBuy(acctA, spy, d1, 83, 120) // cost=9960, cash=40, holdings=183
			acctA.UpdatePrices(buildDF(d1, []asset.Asset{spy}, []float64{120}, []float64{120}))

			acctA.UpdatePrices(buildDF(d2, []asset.Asset{spy}, []float64{140}, []float64{140}))
			acctA.UpdatePrices(buildDF(d3, []asset.Asset{spy}, []float64{110}, []float64{110}))

			// Equity A: [10000, 22000, 25660, 20170]
			Expect(acctA.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10000, 22000, 25660, 20170}))

			// --- Account B: deposit late (d2, before the decline) ---
			acctB := portfolio.New(portfolio.WithCash(10000, time.Time{}))
			recordBuy(acctB, spy, d0, 100, 100)
			acctB.UpdatePrices(buildDF(d0, []asset.Asset{spy}, []float64{100}, []float64{100}))

			acctB.UpdatePrices(buildDF(d1, []asset.Asset{spy}, []float64{120}, []float64{120}))

			acctB.Record(portfolio.Transaction{Date: d2, Type: asset.DepositTransaction, Amount: 10000})
			recordBuy(acctB, spy, d2, 71, 140) // cost=9940, cash=60, holdings=171
			acctB.UpdatePrices(buildDF(d2, []asset.Asset{spy}, []float64{140}, []float64{140}))

			acctB.UpdatePrices(buildDF(d3, []asset.Asset{spy}, []float64{110}, []float64{110}))

			// Equity B: [10000, 12000, 24000, 18870]
			Expect(acctB.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10000, 12000, 24000, 18870}))

			mwrrA, err := acctA.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			mwrrB, err := acctB.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			// Account A deposited at d1 (price=120). The second tranche rose
			// to 140 then fell to 110, netting a loss of 10/120 = -8.3%.
			// But the first tranche gained 10% (110/100), so overall positive.
			//
			// Account B deposited at d2 (price=140). The second tranche fell
			// to 110, losing 30/140 = -21.4%. The investor put more money in
			// just before the decline, so dollar-weighted return is worse.
			//
			// MWRR flows for A: -10000 at d0, -10000 at d1, +20170 at d3.
			// MWRR flows for B: -10000 at d0, -10000 at d2, +18870 at d3.
			//
			// A deposited earlier but with a higher terminal value; B deposited
			// later (closer to the loss) with lower terminal value. MWRR(A) > MWRR(B).
			Expect(mwrrA).To(BeNumerically(">", mwrrB))

			// TWRR strips deposits, so both accounts reflect the same underlying
			// asset performance (100->110 = 10% over the full period).
			twrrA, err := acctA.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			twrrB, err := acctB.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			// TWRR A = 1.2 * (25660/22000) * (20170/25660) - 1 = 1.2 * 20170/22000 - 1
			Expect(twrrA).To(BeNumerically("~", 1.2*20170.0/22000.0-1, 1e-9))

			// TWRR B = (12000/10000) * (24000-10000)/12000 * (18870/24000) - 1
			//        = 1.2 * (14000/12000) * (18870/24000) - 1
			Expect(twrrB).To(BeNumerically("~", 1.2*14000.0/12000.0*18870.0/24000.0-1, 1e-9))
		})

		It("correctly tracks equity when buying and selling multiple times", func() {
			// A round-trip trade scenario: buy, price goes up, sell at profit,
			// then buy again at a different price.
			//
			// Day 0 (Jan 2 2024): deposit 10000, buy 100@100. Cash=0. Eq=10000.
			// Day 1 (Apr 2 2024): price=120. Eq=12000.
			//   Sell all 100@120. Cash=12000, holdings=0.
			// Day 2 (Jul 2 2024): price=90. Eq=12000 (all cash).
			//   Buy 133@90 (cost=11970). Cash=30, holdings=133.
			// Day 3 (Oct 2 2024): price=100. Eq=30+133*100=13330.
			// Day 4 (Jan 2 2025): price=110. Eq=30+133*110=14660.
			//
			// Equity: [10000, 12000, 12000, 13330, 14660]
			// TWRR = (12000/10000)*(12000/12000)*(13330/12000)*(14660/13330) - 1
			//      = 1.2 * 1.0 * (13330/12000) * (14660/13330) - 1
			//      = 14660/10000 - 1 = 0.466

			d0 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			d1 := time.Date(2024, 4, 2, 0, 0, 0, 0, time.UTC)
			d2 := time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC)
			d3 := time.Date(2024, 10, 2, 0, 0, 0, 0, time.UTC)
			d4 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			a := portfolio.New(portfolio.WithCash(10000, time.Time{}))
			recordBuy(a, spy, d0, 100, 100)
			a.UpdatePrices(buildDF(d0, []asset.Asset{spy}, []float64{100}, []float64{100}))

			a.UpdatePrices(buildDF(d1, []asset.Asset{spy}, []float64{120}, []float64{120}))
			recordSell(a, spy, d1, 100, 120) // proceeds=12000

			a.UpdatePrices(buildDF(d2, []asset.Asset{spy}, []float64{90}, []float64{90}))
			recordBuy(a, spy, d2, 133, 90) // cost=11970, cash=30

			a.UpdatePrices(buildDF(d3, []asset.Asset{spy}, []float64{100}, []float64{100}))
			a.UpdatePrices(buildDF(d4, []asset.Asset{spy}, []float64{110}, []float64{110}))

			Expect(a.PerfData().Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10000, 12000, 12000, 13330, 14660}))

			twrrVal, err := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(twrrVal).To(BeNumerically("~", 0.466, 1e-9))

			// MWRR: only flow is initial deposit -10000, terminal 14660.
			// Positive gain, so MWRR > 0.
			mwrrVal, err := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(mwrrVal).To(BeNumerically(">", 0.0))
		})
	})

	Describe("ActiveReturn", func() {
		// buildAccountWithBenchmark creates an account with both a portfolio equity
		// curve and benchmark prices.
		buildAccountWithBenchmark := func(
			dates []time.Time,
			equity []float64,
			benchPrices []float64,
		) *portfolio.Account {
			a := portfolio.New(
				portfolio.WithCash(equity[0], time.Time{}),
				portfolio.WithBenchmark(bm),
			)
			for i, d := range dates {
				if i > 0 {
					diff := equity[i] - equity[i-1]
					if diff > 0 {
						a.Record(portfolio.Transaction{
							Date:   d,
							Type:   asset.DepositTransaction,
							Amount: diff,
						})
					} else if diff < 0 {
						a.Record(portfolio.Transaction{
							Date:   d,
							Type:   asset.WithdrawalTransaction,
							Amount: diff,
						})
					}
				}
				df := buildDF(d,
					[]asset.Asset{spy, bm},
					[]float64{100, benchPrices[i]},
					[]float64{100, benchPrices[i]},
				)
				a.UpdatePrices(df)
			}
			return a
		}

		It("computes portfolio total return minus benchmark total return", func() {
			// Portfolio equity: [1000, 1100, 1200]
			// Benchmark:        [50,   52,   54]
			//
			// ActiveReturn.Compute uses (end/start)-1 for each:
			//   portReturn  = 1200/1000 - 1 = 0.20
			//   benchReturn = 54/50 - 1     = 0.08
			//   active      = 0.20 - 0.08   = 0.12
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
			a := buildAccountWithBenchmark(dates,
				[]float64{1000, 1100, 1200},
				[]float64{50, 52, 54},
			)

			result, err := a.PerformanceMetric(portfolio.ActiveReturn).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.12, 1e-9))
		})

		It("returns 0 when portfolio perfectly tracks benchmark", func() {
			// Both have the same percentage returns at each step.
			// Portfolio: [1000, 1100, 1210]  -> returns: 10%, 10%
			// Benchmark: [50,   55,   60.5]  -> returns: 10%, 10%
			//
			// portReturn  = 1210/1000 - 1 = 0.21
			// benchReturn = 60.5/50 - 1   = 0.21
			// active      = 0.0
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
			a := buildAccountWithBenchmark(dates,
				[]float64{1000, 1100, 1210},
				[]float64{50, 55, 60.5},
			)

			result, err := a.PerformanceMetric(portfolio.ActiveReturn).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("computes cumulative active return series as portfolio minus benchmark cumulative returns", func() {
			// Portfolio equity: [100, 110, 105]
			// Benchmark:        [50,  52,  54]
			//
			// Portfolio returns: r0=10/100=0.10, r1=-5/110=-1/22
			// Benchmark returns: r0=2/50=0.04,   r1=2/52=1/26
			//
			// Cumulative portfolio: cum_p[0]=0.10, cum_p[1]=(110/100)*(105/110)-1=0.05
			// Cumulative benchmark: cum_b[0]=0.04, cum_b[1]=(52/50)*(54/52)-1=0.08
			//
			// Series[i] = (cumPort - 1) - (cumBench - 1) = cumPort - cumBench
			//   series[0] = 0.10 - 0.04 = 0.06
			//   series[1] = 0.05 - 0.08 = -0.03
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
			a := buildAccountWithBenchmark(dates,
				[]float64{100, 110, 105},
				[]float64{50, 52, 54},
			)

			df, err := a.PerformanceMetric(portfolio.ActiveReturn).Series()
			Expect(err).NotTo(HaveOccurred())
			Expect(df.Len()).To(Equal(2))
			series := df.Column(perfAsset, data.PortfolioEquity)
			Expect(series[0]).To(BeNumerically("~", 0.06, 1e-9))
			Expect(series[1]).To(BeNumerically("~", -0.03, 1e-9))
		})
	})
})
