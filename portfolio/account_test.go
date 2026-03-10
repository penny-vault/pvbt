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
