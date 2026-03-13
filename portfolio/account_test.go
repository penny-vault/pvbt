package portfolio_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
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

	Describe("SetBroker", func() {
		It("replaces the broker on the account", func() {
			// The mockBroker type is defined in order_test.go and is
			// available in the portfolio_test package.
			mb1 := &mockBroker{}
			mb2 := &mockBroker{}

			a := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBroker(mb1),
			)

			// Give the account a price so Order can work.
			t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
			a.UpdatePrices(df)

			// Submit via the first broker.
			mb1.defaultFill = &broker.Fill{Price: 100.0, FilledAt: t1}
			a.Order(context.Background(), spy, portfolio.Buy, 1)
			Expect(mb1.submitted).To(HaveLen(1))
			Expect(mb2.submitted).To(HaveLen(0))

			// Replace the broker.
			a.SetBroker(mb2)
			mb2.defaultFill = &broker.Fill{Price: 100.0, FilledAt: t1}
			a.Order(context.Background(), spy, portfolio.Buy, 1)
			Expect(mb2.submitted).To(HaveLen(1))
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
			Expect(a.Value()).To(Equal(10_000.0))           // 8000 + 5*400
			Expect(a.PositionValue(spy)).To(Equal(2_000.0)) // 5*400

			df2 := buildDF(t2, []asset.Asset{spy}, []float64{420.0}, []float64{418.0})
			a.UpdatePrices(df2)
			Expect(a.Value()).To(Equal(10_100.0))           // 8000 + 5*420
			Expect(a.PositionValue(spy)).To(Equal(2_100.0)) // 5*420
			Expect(a.EquityCurve()).To(Equal([]float64{10_000.0, 10_100.0}))
		})

		It("appends NaN benchmark price to keep arrays aligned with equity curve", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(rf),
			)

			// Day 1: normal prices
			df1 := buildDF(t1,
				[]asset.Asset{spy, bm, rf},
				[]float64{450.0, 100.0, 50.0},
				[]float64{448.0, 99.0, 49.5},
			)
			a.UpdatePrices(df1)
			Expect(a.BenchmarkPrices()).To(Equal([]float64{99.0}))
			Expect(a.RiskFreePrices()).To(Equal([]float64{49.5}))

			// Day 2: benchmark has NaN AdjClose and NaN Close
			df2, err := data.NewDataFrame(
				[]time.Time{t2},
				[]asset.Asset{spy, bm, rf},
				[]data.Metric{data.MetricClose, data.AdjClose},
				[]float64{
					455.0, 453.0, // spy: close, adjclose
					math.NaN(), math.NaN(), // bm: close, adjclose (NaN)
					50.5, 50.0, // rf: close, adjclose
				},
			)
			Expect(err).NotTo(HaveOccurred())
			a.UpdatePrices(df2)

			// NaN is appended to keep benchmarkPrices aligned with equityCurve.
			Expect(a.BenchmarkPrices()).To(HaveLen(2))
			Expect(math.IsNaN(a.BenchmarkPrices()[1])).To(BeTrue())
			Expect(a.RiskFreePrices()).To(Equal([]float64{49.5, 50.0}))
			Expect(a.EquityCurve()).To(HaveLen(2))
		})

		It("appends NaN risk-free price to keep arrays aligned with equity curve", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(rf),
			)

			// Day 1: normal prices
			df1 := buildDF(t1,
				[]asset.Asset{spy, bm, rf},
				[]float64{450.0, 100.0, 50.0},
				[]float64{448.0, 99.0, 49.5},
			)
			a.UpdatePrices(df1)

			// Day 2: risk-free has NaN AdjClose and NaN Close
			df2, err := data.NewDataFrame(
				[]time.Time{t2},
				[]asset.Asset{spy, bm, rf},
				[]data.Metric{data.MetricClose, data.AdjClose},
				[]float64{
					455.0, 453.0, // spy
					102.0, 101.0, // bm
					math.NaN(), math.NaN(), // rf: NaN
				},
			)
			Expect(err).NotTo(HaveOccurred())
			a.UpdatePrices(df2)

			Expect(a.BenchmarkPrices()).To(Equal([]float64{99.0, 101.0}))
			// NaN is appended to keep riskFreePrices aligned with equityCurve.
			Expect(a.RiskFreePrices()).To(HaveLen(2))
			Expect(math.IsNaN(a.RiskFreePrices()[1])).To(BeTrue())
			Expect(a.EquityCurve()).To(HaveLen(2))
		})
	})

	Describe("EquityCurve, EquityTimes, BenchmarkPrices, RiskFreePrices accessors", func() {
		It("returns empty slices before any UpdatePrices calls", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			Expect(a.EquityCurve()).To(BeEmpty())
			Expect(a.EquityTimes()).To(BeEmpty())
			Expect(a.BenchmarkPrices()).To(BeEmpty())
			Expect(a.RiskFreePrices()).To(BeEmpty())
		})

		It("returns correct accumulated slices after multiple UpdatePrices calls", func() {
			bm := asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
			rf := asset.Asset{CompositeFigi: "RF", Ticker: "RF"}

			a := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(bm),
				portfolio.WithRiskFree(rf),
			)

			t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			t2 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
			t3 := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)

			a.UpdatePrices(buildDF(t1,
				[]asset.Asset{spy, bm, rf},
				[]float64{100.0, 200.0, 50.0},
				[]float64{99.0, 198.0, 49.0},
			))
			a.UpdatePrices(buildDF(t2,
				[]asset.Asset{spy, bm, rf},
				[]float64{102.0, 204.0, 50.5},
				[]float64{101.0, 202.0, 49.5},
			))
			a.UpdatePrices(buildDF(t3,
				[]asset.Asset{spy, bm, rf},
				[]float64{104.0, 208.0, 51.0},
				[]float64{103.0, 206.0, 50.0},
			))

			Expect(a.EquityCurve()).To(Equal([]float64{10_000.0, 10_000.0, 10_000.0}))
			Expect(a.EquityTimes()).To(Equal([]time.Time{t1, t2, t3}))
			Expect(a.BenchmarkPrices()).To(Equal([]float64{198.0, 202.0, 206.0}))
			Expect(a.RiskFreePrices()).To(Equal([]float64{49.0, 49.5, 50.0}))
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

// computeExpectedSummary derives all expected Summary values from first
// principles using the same math as the production code, but expressed
// independently so the test is not just calling the same function.
//
// Fixture:
//
//	SPY prices: [100, 105, 98, 103, 97, 110]
//	Equity curve (5 shares, 0 cash): [500, 525, 490, 515, 485, 550]
//	BIL prices: [100, 100.01, 100.02, 100.03, 100.04, 100.05]
//	Times: daySeq(2025-01-02, 6) -- 6 weekdays starting Thursday Jan 2
//
// The helper functions below mirror the production helpers in
// metric_helpers.go to cross-check the math.
func helperReturns(p []float64) []float64 {
	r := make([]float64, len(p)-1)
	for i := range r {
		r[i] = (p[i+1] - p[i]) / p[i]
	}
	return r
}

func helperMean(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func helperVariance(x []float64) float64 {
	m := helperMean(x)
	s := 0.0
	for _, v := range x {
		d := v - m
		s += d * d
	}
	return s / float64(len(x)-1)
}

func helperStddev(x []float64) float64 { return math.Sqrt(helperVariance(x)) }

func helperExcessReturns(r, rf []float64) []float64 {
	n := len(r)
	if len(rf) < n {
		n = len(rf)
	}
	er := make([]float64, n)
	for i := range n {
		er[i] = r[i] - rf[i]
	}
	return er
}

func helperDrawdownSeries(equity []float64) []float64 {
	dd := make([]float64, len(equity))
	peak := math.Inf(-1)
	for i, v := range equity {
		if v > peak {
			peak = v
		}
		dd[i] = (v - peak) / peak
	}
	return dd
}

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

	// Pre-compute all expected values from the fixture data.
	equity := []float64{500, 525, 490, 515, 485, 550}
	bilPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}
	eqRet := helperReturns(equity)
	rfRet := helperReturns(bilPrices)
	er := helperExcessReturns(eqRet, rfRet)
	af := 252.0 // annualization factor: daily data

	It("computes correct TWRR for known equity curve", func() {
		// TWRR = 550/500 - 1 = 0.10
		acct := buildSummaryAcct()
		s, err := acct.Summary()
		Expect(err).NotTo(HaveOccurred())
		Expect(s.TWRR).To(BeNumerically("~", 0.10, 1e-9))
	})

	It("computes correct MaxDrawdown for known equity curve", func() {
		// peak=525, trough=485 => drawdown = (485-525)/525 = -40/525
		acct := buildSummaryAcct()
		s, err := acct.Summary()
		Expect(err).NotTo(HaveOccurred())
		expectedMaxDD := -40.0 / 525.0
		Expect(s.MaxDrawdown).To(BeNumerically("~", expectedMaxDD, 1e-10))
	})

	It("computes correct StdDev for known equity curve", func() {
		// Annualized StdDev = stddev(equity returns) * sqrt(252)
		acct := buildSummaryAcct()
		s, err := acct.Summary()
		Expect(err).NotTo(HaveOccurred())
		expectedStdDev := helperStddev(eqRet) * math.Sqrt(af)
		Expect(s.StdDev).To(BeNumerically("~", expectedStdDev, 1e-10))
	})

	It("computes correct Sharpe ratio for known equity curve", func() {
		// Sharpe = mean(excess returns) / stddev(excess returns) * sqrt(252)
		acct := buildSummaryAcct()
		s, err := acct.Summary()
		Expect(err).NotTo(HaveOccurred())
		expectedSharpe := helperMean(er) / helperStddev(er) * math.Sqrt(af)
		Expect(s.Sharpe).To(BeNumerically("~", expectedSharpe, 1e-10))
	})

	It("computes correct Sortino ratio for known equity curve", func() {
		// Sortino = mean(excess returns) / downside_deviation * sqrt(252)
		// where downside_deviation = sqrt(sum(min(er_i, 0)^2) / N)
		acct := buildSummaryAcct()
		s, err := acct.Summary()
		Expect(err).NotTo(HaveOccurred())
		sumSq := 0.0
		for _, v := range er {
			if v < 0 {
				sumSq += v * v
			}
		}
		dd := math.Sqrt(sumSq / float64(len(er)))
		expectedSortino := helperMean(er) / dd * math.Sqrt(af)
		Expect(s.Sortino).To(BeNumerically("~", expectedSortino, 1e-10))
	})

	It("computes correct Calmar ratio for known equity curve", func() {
		// Calmar = CAGR / |MaxDrawdown|
		// times span: Jan 2 to Jan 9 = 7 calendar days
		// years = 7 / 365.25
		acct := buildSummaryAcct()
		s, err := acct.Summary()
		Expect(err).NotTo(HaveOccurred())
		years := 7.0 / 365.25
		annReturn := math.Pow(550.0/500.0, 1.0/years) - 1
		dd := helperDrawdownSeries(equity)
		minDD := 0.0
		for _, v := range dd {
			if v < minDD {
				minDD = v
			}
		}
		expectedCalmar := annReturn / math.Abs(minDD)
		Expect(s.Calmar).To(BeNumerically("~", expectedCalmar, 1e-6))
	})

	It("computes correct MWRR for known equity curve", func() {
		// Single deposit of 500 at time 0, terminal value 550 at time end.
		// XIRR: -500 + 550/(1+r)^(7/365) = 0
		// r = (550/500)^(365/7) - 1
		acct := buildSummaryAcct()
		s, err := acct.Summary()
		Expect(err).NotTo(HaveOccurred())
		expectedMWRR := math.Pow(550.0/500.0, 365.0/7.0) - 1
		Expect(s.MWRR).To(BeNumerically("~", expectedMWRR, 1e-4))
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

	// Pre-compute expected values for risk metrics.
	equity := []float64{500, 525, 490, 515, 485, 550}
	bilPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}
	eqRet := helperReturns(equity)
	rfRet := helperReturns(bilPrices)
	er := helperExcessReturns(eqRet, rfRet)
	af := 252.0

	It("Beta equals 1.0 when portfolio tracks benchmark exactly", func() {
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.Beta).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("Alpha equals 0.0 when portfolio tracks benchmark exactly", func() {
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.Alpha).To(BeNumerically("~", 0.0, 1e-9))
	})

	It("TrackingError equals 0.0 when portfolio tracks benchmark exactly", func() {
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.TrackingError).To(BeNumerically("~", 0.0, 1e-9))
	})

	It("InformationRatio equals 0.0 when active return is zero", func() {
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.InformationRatio).To(BeNumerically("~", 0.0, 1e-9))
	})

	It("RSquared equals 1.0 when portfolio tracks benchmark exactly", func() {
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.RSquared).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("Treynor equals (TWRR - rf_return) / beta ~ 0.0995", func() {
		// TWRR = 0.10, approximate risk-free return over 5 days ~ 0.0005,
		// Beta = 1.0, so Treynor ~ 0.0995.
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.Treynor).To(BeNumerically("~", 0.0995, 1e-3))
	})

	It("computes correct DownsideDeviation for known equity curve", func() {
		// DownsideDeviation = stddev(negative excess returns) * sqrt(252)
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		var neg []float64
		for _, v := range er {
			if v < 0 {
				neg = append(neg, v)
			}
		}
		expectedDD := helperStddev(neg) * math.Sqrt(af)
		Expect(rm.DownsideDeviation).To(BeNumerically("~", expectedDD, 1e-10))
	})

	It("returns zero UlcerIndex when equity curve has fewer than 14 points", func() {
		// The risk account has only 6 equity points, which is below the
		// 14-period lookback required by UlcerIndex.
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.UlcerIndex).To(BeNumerically("==", 0))
	})

	It("computes correct Skewness for known equity curve", func() {
		// Skewness = (1/n) * sum((r-mean)^3) / stddev^3
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		n := float64(len(eqRet))
		m := helperMean(eqRet)
		s := helperStddev(eqRet)
		sum3 := 0.0
		for _, v := range eqRet {
			d := v - m
			sum3 += d * d * d
		}
		expectedSkew := sum3 / n / (s * s * s)
		Expect(rm.Skewness).To(BeNumerically("~", expectedSkew, 1e-10))
	})

	It("computes correct ExcessKurtosis for known equity curve", func() {
		// ExcessKurtosis = (1/n) * sum((r-mean)^4) / stddev^4 - 3
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		n := float64(len(eqRet))
		m := helperMean(eqRet)
		s := helperStddev(eqRet)
		sum4 := 0.0
		for _, v := range eqRet {
			d := v - m
			sum4 += d * d * d * d
		}
		expectedKurt := sum4/n/(s*s*s*s) - 3
		Expect(rm.ExcessKurtosis).To(BeNumerically("~", expectedKurt, 1e-10))
	})

	It("computes correct ValueAtRisk for known equity curve", func() {
		// VaR = sorted returns at index floor(0.05 * n)
		// With 5 returns, floor(0.05*5) = floor(0.25) = 0, so VaR = min return.
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		// Sorted returns: find the minimum return.
		// eqRet = [0.05, -35/525, 25/490, -30/515, 65/485]
		// Sorted ascending: -35/525, -30/515, 25/490, 0.05, 65/485
		// idx = floor(0.05 * 5) = 0 => sorted[0] = -35/525
		expectedVaR := -35.0 / 525.0
		Expect(rm.ValueAtRisk).To(BeNumerically("~", expectedVaR, 1e-10))
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
		wm, err := buildWithdrawalAcct().WithdrawalMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(wm.SafeWithdrawalRate).To(BeNumerically("~", 0.063, 1e-2))
	})

	It("PerpetualWithdrawalRate is approximately 0.049", func() {
		wm, err := buildWithdrawalAcct().WithdrawalMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(wm.PerpetualWithdrawalRate).To(BeNumerically("~", 0.049, 1e-2))
	})

	It("SWR > 0 and DWR > 0 for a growing curve", func() {
		wm, err := buildWithdrawalAcct().WithdrawalMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(wm.SafeWithdrawalRate).To(BeNumerically(">", 0.0))
		Expect(wm.DynamicWithdrawalRate).To(BeNumerically(">", 0.0))
	})

	It("ordering invariant: PWR <= SWR <= DWR", func() {
		wm, err := buildWithdrawalAcct().WithdrawalMetrics()
		Expect(err).NotTo(HaveOccurred())
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
	// Returns the account and the raw SPY/BIL price arrays for manual verification.
	buildLongAccountWithPrices := func() (*portfolio.Account, []float64, []float64, []time.Time) {
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

		// Equity curve = 5 * spyPrices
		equityCurve := make([]float64, n)
		for i := range n {
			equityCurve[i] = 5 * spyPrices[i]
		}

		return acct, equityCurve, bilPrices, times
	}

	It("Window(Days(10)) produces correct TWRR for the windowed slice", func() {
		acct, equityCurve, _, times := buildLongAccountWithPrices()

		// Full TWRR from the full equity curve.
		fullTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Value()
		Expect(err).NotTo(HaveOccurred())
		n := len(equityCurve)
		expectedFullTWRR := equityCurve[n-1]/equityCurve[0] - 1
		Expect(fullTWRR).To(BeNumerically("~", expectedFullTWRR, 1e-10))

		// Days(10) window: cutoff = last - 10 days.
		// Find first time >= cutoff.
		last := times[len(times)-1]
		cutoff := last.AddDate(0, 0, -10)
		startIdx := 0
		for i, t := range times {
			if !t.Before(cutoff) {
				startIdx = i
				break
			}
		}
		windowedEq := equityCurve[startIdx:]
		expectedWindowedTWRR := windowedEq[len(windowedEq)-1]/windowedEq[0] - 1

		windowedTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Window(portfolio.Days(10)).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(windowedTWRR).To(BeNumerically("~", expectedWindowedTWRR, 1e-10))
		Expect(fullTWRR).NotTo(BeNumerically("~", windowedTWRR, 1e-10))
	})

	It("Window(Months(1)) produces correct TWRR for the windowed slice", func() {
		acct, equityCurve, _, times := buildLongAccountWithPrices()

		// Months(1) window: cutoff = last - 1 month.
		last := times[len(times)-1]
		cutoff := last.AddDate(0, -1, 0)
		startIdx := 0
		for i, t := range times {
			if !t.Before(cutoff) {
				startIdx = i
				break
			}
		}
		windowedEq := equityCurve[startIdx:]
		expectedWindowedTWRR := windowedEq[len(windowedEq)-1]/windowedEq[0] - 1

		windowedTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Window(portfolio.Months(1)).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(windowedTWRR).To(BeNumerically("~", expectedWindowedTWRR, 1e-10))

		fullTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(fullTWRR).NotTo(BeNumerically("~", windowedTWRR, 1e-10))
	})

	It("Window(Days(10)) produces correct Sharpe for the windowed slice", func() {
		acct, equityCurve, bilPrices, times := buildLongAccountWithPrices()

		// Compute window boundaries.
		last := times[len(times)-1]
		cutoff := last.AddDate(0, 0, -10)
		startIdx := 0
		for i, t := range times {
			if !t.Before(cutoff) {
				startIdx = i
				break
			}
		}
		windowedEq := equityCurve[startIdx:]
		windowedRf := bilPrices[startIdx:]

		wRet := helperReturns(windowedEq)
		wRfRet := helperReturns(windowedRf)
		wER := helperExcessReturns(wRet, wRfRet)
		expectedSharpe := helperMean(wER) / helperStddev(wER) * math.Sqrt(252)

		windowedSharpe, err := acct.PerformanceMetric(portfolio.Sharpe).Window(portfolio.Days(10)).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(windowedSharpe).To(BeNumerically("~", expectedSharpe, 1e-10))

		fullSharpe, err := acct.PerformanceMetric(portfolio.Sharpe).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(fullSharpe).NotTo(BeNumerically("~", windowedSharpe, 1e-10))
	})
})
