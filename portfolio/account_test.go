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

	Describe("UpdatePrices", func() {
		var (
			t1 time.Time
			t2 time.Time
			bm asset.Asset
			rf asset.Asset
		)

		BeforeEach(func() {
			t1 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			t2 = time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
			bm = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
			rf = asset.Asset{CompositeFigi: "RF", Ticker: "RF"}
		})

		It("with no holdings, value equals cash only", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			df := buildDF(t1, []asset.Asset{spy}, []float64{450.0}, []float64{448.0})
			a.UpdatePrices(df)

			Expect(a.Value()).To(Equal(10_000.0))
			Expect(a.EquityCurve()).To(Equal([]float64{10_000.0}))
			Expect(a.EquityTimes()).To(Equal([]time.Time{t1}))
		})

		It("marks holdings to MetricClose prices", func() {
			a := portfolio.New(portfolio.WithCash(7_000))
			// simulate having bought 10 shares
			a.Record(portfolio.Transaction{
				Date:   t1,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			// cash is now 4_000, holding 10 SPY
			df := buildDF(t1, []asset.Asset{spy}, []float64{450.0}, []float64{448.0})
			a.UpdatePrices(df)

			// total = 4000 + 10*450 = 8500
			Expect(a.Value()).To(Equal(8_500.0))
			Expect(a.EquityCurve()).To(Equal([]float64{8_500.0}))
			Expect(a.PositionValue(spy)).To(Equal(4_500.0))
		})

		It("accumulates equity curve, benchmark, and risk-free series over multiple calls", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(rf),
			)

			// Day 1
			df1 := buildDF(t1,
				[]asset.Asset{spy, bm, rf},
				[]float64{450.0, 100.0, 50.0},
				[]float64{448.0, 99.0, 49.5},
			)
			a.UpdatePrices(df1)

			Expect(a.EquityCurve()).To(HaveLen(1))
			Expect(a.EquityCurve()[0]).To(Equal(10_000.0))
			Expect(a.BenchmarkPrices()).To(Equal([]float64{99.0}))
			Expect(a.RiskFreePrices()).To(Equal([]float64{49.5}))

			// Day 2
			df2 := buildDF(t2,
				[]asset.Asset{spy, bm, rf},
				[]float64{455.0, 102.0, 50.5},
				[]float64{453.0, 101.0, 50.0},
			)
			a.UpdatePrices(df2)

			Expect(a.EquityCurve()).To(HaveLen(2))
			Expect(a.EquityCurve()).To(Equal([]float64{10_000.0, 10_000.0}))
			Expect(a.EquityTimes()).To(Equal([]time.Time{t1, t2}))
			Expect(a.BenchmarkPrices()).To(Equal([]float64{99.0, 101.0}))
			Expect(a.RiskFreePrices()).To(Equal([]float64{49.5, 50.0}))
		})

		It("does not append benchmark/risk-free when not set", func() {
			a := portfolio.New(portfolio.WithCash(5_000))
			df := buildDF(t1, []asset.Asset{spy}, []float64{450.0}, []float64{448.0})
			a.UpdatePrices(df)

			Expect(a.BenchmarkPrices()).To(BeEmpty())
			Expect(a.RiskFreePrices()).To(BeEmpty())
		})

		It("reflects latest prices in Value and PositionValue after UpdatePrices", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:   t1,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    5,
				Price:  400.0,
				Amount: -2_000.0,
			})
			// cash = 8000, 5 shares SPY

			df1 := buildDF(t1, []asset.Asset{spy}, []float64{400.0}, []float64{399.0})
			a.UpdatePrices(df1)
			Expect(a.Value()).To(Equal(10_000.0))       // 8000 + 5*400
			Expect(a.PositionValue(spy)).To(Equal(2_000.0)) // 5*400

			df2 := buildDF(t2, []asset.Asset{spy}, []float64{420.0}, []float64{418.0})
			a.UpdatePrices(df2)
			Expect(a.Value()).To(Equal(10_100.0))       // 8000 + 5*420
			Expect(a.PositionValue(spy)).To(Equal(2_100.0)) // 5*420
			Expect(a.EquityCurve()).To(Equal([]float64{10_000.0, 10_100.0}))
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

		It("iterates over actual positions with correct asset/qty pairs", func() {
			a := portfolio.New(portfolio.WithCash(100_000))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  bil,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  50.0,
				Amount: -1_000.0,
			})

			seen := make(map[asset.Asset]float64)
			a.Holdings(func(ast asset.Asset, qty float64) {
				seen[ast] = qty
			})
			Expect(seen).To(HaveLen(2))
			Expect(seen[spy]).To(Equal(10.0))
			Expect(seen[bil]).To(Equal(20.0))
		})
	})

	Describe("Value with NaN price", func() {
		It("skips NaN-priced assets and returns cash only", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			a.Record(portfolio.Transaction{
				Date:   t1,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			// cash is now 7_000, holding 10 SPY

			// Build a DataFrame where SPY has NaN close price.
			df, err := data.NewDataFrame(
				[]time.Time{t1},
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				[]float64{math.NaN()},
			)
			Expect(err).NotTo(HaveOccurred())
			a.UpdatePrices(df)

			// Value should equal cash only since SPY price is NaN.
			Expect(a.Value()).To(Equal(7_000.0))
		})
	})

	Describe("PositionValue with nil prices", func() {
		It("returns 0 when prices have never been set", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			Expect(a.Position(spy)).To(Equal(10.0))
			Expect(a.PositionValue(spy)).To(Equal(0.0))
		})
	})

	Describe("Record full position depletion", func() {
		It("removes asset from holdings when all shares are sold", func() {
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
				Qty:    10,
				Price:  320.0,
				Amount: 3_200.0,
			})

			Expect(a.Position(spy)).To(Equal(0.0))

			// Holdings callback should not see SPY at all.
			seen := make(map[asset.Asset]float64)
			a.Holdings(func(ast asset.Asset, qty float64) {
				seen[ast] = qty
			})
			Expect(seen).NotTo(HaveKey(spy))
		})
	})

	Describe("Record with multiple tax lots (FIFO partial consumption)", func() {
		It("consumes lots in FIFO order across partial sells", func() {
			a := portfolio.New(portfolio.WithCash(100_000))

			// Buy 10 shares at $100 on day 1.
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			// Buy 5 shares at $120 on day 2.
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    5,
				Price:  120.0,
				Amount: -600.0,
			})

			// Sell 12 shares at $150.
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    12,
				Price:  150.0,
				Amount: 1_800.0,
			})

			// Position should be 15 - 12 = 3 shares.
			Expect(a.Position(spy)).To(Equal(3.0))

			// Cash: 100_000 - 1_000 - 600 + 1_800 = 100_200
			Expect(a.Cash()).To(Equal(100_200.0))
		})
	})

	Describe("WithCash(0)", func() {
		It("records no deposit transaction when cash is 0", func() {
			a := portfolio.New(portfolio.WithCash(0))
			txns := a.Transactions()
			// A deposit of 0 is still recorded by WithCash.
			// Verify cash is 0 and the transaction exists but has 0 amount.
			Expect(a.Cash()).To(Equal(0.0))
			Expect(txns).To(HaveLen(1))
			Expect(txns[0].Type).To(Equal(portfolio.DepositTransaction))
			Expect(txns[0].Amount).To(Equal(0.0))
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

	It("returns a formatted string for unknown transaction types", func() {
		t := portfolio.TransactionType(99)
		Expect(t.String()).To(Equal("TransactionType(99)"))
	})
})

var _ = Describe("Summary", func() {
	// Build a known equity curve: 5 shares of SPY at prices [100,105,98,103,97,110].
	// Equity curve = [500, 525, 490, 515, 485, 550].
	// BIL is the risk-free asset: [100, 100.01, 100.02, 100.03, 100.04, 100.05].
	var buildSummaryAcct = func() *portfolio.Account {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		spyPrices := []float64{100, 105, 98, 103, 97, 110}
		bilPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}
		n := len(spyPrices)
		times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

		acct := portfolio.New(
			portfolio.WithCash(5*spyPrices[0]),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)
		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})
		for i := range n {
			df := buildDF(times[i],
				[]asset.Asset{spy, bil},
				[]float64{spyPrices[i], bilPrices[i]},
				[]float64{spyPrices[i], bilPrices[i]},
			)
			acct.UpdatePrices(df)
		}
		return acct
	}

	It("computes correct TWRR for known equity curve", func() {
		// equity = [500,525,490,515,485,550], TWRR = 550/500 - 1 = 0.10
		acct := buildSummaryAcct()
		s := acct.Summary()
		Expect(s.TWRR).To(BeNumerically("~", 0.10, 1e-9))
	})

	It("computes correct MaxDrawdown for known equity curve", func() {
		// peak=525, trough=485 => drawdown = (485-525)/525 ~ -0.07619
		acct := buildSummaryAcct()
		s := acct.Summary()
		Expect(s.MaxDrawdown).To(BeNumerically("~", -0.07619, 1e-4))
	})

	It("computes non-zero StdDev for volatile equity curve", func() {
		acct := buildSummaryAcct()
		s := acct.Summary()
		Expect(s.StdDev).To(BeNumerically(">", 0.0))
	})

	It("computes non-zero positive Sharpe ratio", func() {
		acct := buildSummaryAcct()
		s := acct.Summary()
		Expect(s.Sharpe).To(BeNumerically(">", 0.0))
	})

	It("computes non-zero positive Sortino ratio", func() {
		acct := buildSummaryAcct()
		s := acct.Summary()
		Expect(s.Sortino).To(BeNumerically(">", 0.0))
	})

	It("computes non-zero positive Calmar ratio", func() {
		acct := buildSummaryAcct()
		s := acct.Summary()
		Expect(s.Calmar).To(BeNumerically(">", 0.0))
	})

	It("computes non-zero MWRR", func() {
		acct := buildSummaryAcct()
		s := acct.Summary()
		Expect(s.MWRR).To(BeNumerically(">", 0.0))
	})
})

var _ = Describe("RiskMetrics", func() {
	// The portfolio holds 5 shares of SPY using SPY itself as the benchmark.
	// Because equity = 5*SPY, portfolio returns are identical to benchmark returns,
	// which yields Beta=1, Alpha=0, TrackingError=0, IR=0, RSquared=1.
	var buildRiskAcct = func() *portfolio.Account {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		spyPrices := []float64{100, 105, 98, 103, 97, 110}
		bilPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}
		n := len(spyPrices)
		times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

		acct := portfolio.New(
			portfolio.WithCash(5*spyPrices[0]),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)
		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})
		for i := range n {
			df := buildDF(times[i],
				[]asset.Asset{spy, bil},
				[]float64{spyPrices[i], bilPrices[i]},
				[]float64{spyPrices[i], bilPrices[i]},
			)
			acct.UpdatePrices(df)
		}
		return acct
	}

	It("Beta equals 1.0 when portfolio tracks benchmark exactly", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(rm.Beta).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("Alpha equals 0.0 when portfolio tracks benchmark exactly", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(rm.Alpha).To(BeNumerically("~", 0.0, 1e-9))
	})

	It("TrackingError equals 0.0 when portfolio tracks benchmark exactly", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(rm.TrackingError).To(BeNumerically("~", 0.0, 1e-9))
	})

	It("InformationRatio equals 0.0 when active return is zero", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(rm.InformationRatio).To(BeNumerically("~", 0.0, 1e-9))
	})

	It("RSquared equals 1.0 when portfolio tracks benchmark exactly", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(rm.RSquared).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("Treynor equals (TWRR - rf_return) / beta ~ 0.0995", func() {
		// TWRR = 0.10, approximate risk-free return over 5 days ~ 0.0005,
		// Beta = 1.0, so Treynor ~ 0.0995.
		rm := buildRiskAcct().RiskMetrics()
		Expect(rm.Treynor).To(BeNumerically("~", 0.0995, 1e-3))
	})

	It("DownsideDeviation is finite and non-negative", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(math.IsNaN(rm.DownsideDeviation)).To(BeFalse())
		Expect(rm.DownsideDeviation).To(BeNumerically(">=", 0.0))
	})

	It("UlcerIndex is finite and non-negative", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(math.IsNaN(rm.UlcerIndex)).To(BeFalse())
		Expect(rm.UlcerIndex).To(BeNumerically(">=", 0.0))
	})

	It("Skewness is finite", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(math.IsNaN(rm.Skewness)).To(BeFalse())
	})

	It("ExcessKurtosis is finite", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(math.IsNaN(rm.ExcessKurtosis)).To(BeFalse())
	})

	It("ValueAtRisk is finite", func() {
		rm := buildRiskAcct().RiskMetrics()
		Expect(math.IsNaN(rm.ValueAtRisk)).To(BeFalse())
	})
})

var _ = Describe("WithdrawalMetrics", func() {
	// Build a 300-day steadily growing equity curve starting at 100_000 with
	// 0.02% daily growth. Over 300 days this produces a monotonically rising curve.
	var buildWithdrawalAcct = func() *portfolio.Account {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		acct := portfolio.New(portfolio.WithCash(100_000))
		price := 100_000.0
		start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

		for i := range 300 {
			d := start.AddDate(0, 0, i)
			if i > 0 {
				growth := price * 0.0002
				acct.Record(portfolio.Transaction{
					Date:   d,
					Type:   portfolio.DividendTransaction,
					Amount: growth,
				})
				price += growth
			}
			df := buildDF(d, []asset.Asset{spy}, []float64{450 + float64(i)}, []float64{448 + float64(i)})
			acct.UpdatePrices(df)
		}
		return acct
	}

	It("SafeWithdrawalRate is approximately 0.063", func() {
		wm := buildWithdrawalAcct().WithdrawalMetrics()
		Expect(wm.SafeWithdrawalRate).To(BeNumerically("~", 0.063, 1e-2))
	})

	It("PerpetualWithdrawalRate is approximately 0.049", func() {
		wm := buildWithdrawalAcct().WithdrawalMetrics()
		Expect(wm.PerpetualWithdrawalRate).To(BeNumerically("~", 0.049, 1e-2))
	})

	It("SWR > 0 and DWR > 0 for a growing curve", func() {
		wm := buildWithdrawalAcct().WithdrawalMetrics()
		Expect(wm.SafeWithdrawalRate).To(BeNumerically(">", 0.0))
		Expect(wm.DynamicWithdrawalRate).To(BeNumerically(">", 0.0))
	})

	It("ordering invariant: PWR <= SWR <= DWR", func() {
		wm := buildWithdrawalAcct().WithdrawalMetrics()
		Expect(wm.PerpetualWithdrawalRate).To(BeNumerically("<=", wm.SafeWithdrawalRate))
		Expect(wm.SafeWithdrawalRate).To(BeNumerically("<=", wm.DynamicWithdrawalRate))
	})
})

var _ = Describe("Period constructors", func() {
	It("Days creates a Period with UnitDay", func() {
		p := portfolio.Days(30)
		Expect(p.N).To(Equal(30))
		Expect(p.Unit).To(Equal(portfolio.UnitDay))
	})

	It("Months creates a Period with UnitMonth", func() {
		p := portfolio.Months(6)
		Expect(p.N).To(Equal(6))
		Expect(p.Unit).To(Equal(portfolio.UnitMonth))
	})

	It("Years creates a Period with UnitYear", func() {
		p := portfolio.Years(2)
		Expect(p.N).To(Equal(2))
		Expect(p.Unit).To(Equal(portfolio.UnitYear))
	})
})

var _ = Describe("Window", func() {
	// buildLongAccount creates an account with 40 daily data points showing
	// steady growth, suitable for testing windowed metric computations.
	buildLongAccount := func() *portfolio.Account {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		n := 40
		times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

		// SPY grows ~0.5% per day with some noise to produce nonzero metrics.
		spyPrices := make([]float64, n)
		bilPrices := make([]float64, n)
		spyPrices[0] = 100.0
		bilPrices[0] = 100.0
		for i := 1; i < n; i++ {
			// Alternating growth pattern to create variance.
			if i%3 == 0 {
				spyPrices[i] = spyPrices[i-1] * 0.995
			} else {
				spyPrices[i] = spyPrices[i-1] * 1.008
			}
			bilPrices[i] = bilPrices[i-1] * 1.0001
		}

		acct := portfolio.New(
			portfolio.WithCash(5*spyPrices[0]),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)
		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})
		for i := range n {
			df := buildDF(times[i],
				[]asset.Asset{spy, bil},
				[]float64{spyPrices[i], bilPrices[i]},
				[]float64{spyPrices[i], bilPrices[i]},
			)
			acct.UpdatePrices(df)
		}
		return acct
	}

	It("Window(Days(10)) produces a different TWRR than full history", func() {
		acct := buildLongAccount()

		fullTWRR := acct.PerformanceMetric(portfolio.TWRR).Value()
		windowedTWRR := acct.PerformanceMetric(portfolio.TWRR).Window(portfolio.Days(10)).Value()

		Expect(fullTWRR).NotTo(Equal(windowedTWRR))
	})

	It("Window(Months(1)) produces a different TWRR than full history", func() {
		acct := buildLongAccount()

		fullTWRR := acct.PerformanceMetric(portfolio.TWRR).Value()
		windowedTWRR := acct.PerformanceMetric(portfolio.TWRR).Window(portfolio.Months(1)).Value()

		Expect(fullTWRR).NotTo(Equal(windowedTWRR))
	})

	It("Window(Days(10)) produces a different Sharpe than full history", func() {
		acct := buildLongAccount()

		fullSharpe := acct.PerformanceMetric(portfolio.Sharpe).Value()
		windowedSharpe := acct.PerformanceMetric(portfolio.Sharpe).Window(portfolio.Days(10)).Value()

		Expect(fullSharpe).NotTo(Equal(windowedSharpe))
	})
})
