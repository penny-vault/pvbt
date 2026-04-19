package portfolio_test

import (
	"context"
	"errors"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var perfAsset = asset.Asset{CompositeFigi: "_PORTFOLIO_", Ticker: "_PORTFOLIO_"}

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
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			Expect(a.Cash()).To(Equal(10_000.0))
			Expect(a.Value()).To(Equal(10_000.0))
		})

		It("records a DepositTransaction for initial cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			txns := a.Transactions()
			Expect(txns).To(HaveLen(1))
			Expect(txns[0].Type).To(Equal(asset.DepositTransaction))
			Expect(txns[0].Amount).To(Equal(10_000.0))
		})

		It("stores benchmark asset", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
				portfolio.WithBenchmark(spy),
			)
			Expect(a.Benchmark()).To(Equal(spy))
		})

		It("returns empty short lots for new account", func() {
			acct := portfolio.New()
			var count int
			acct.ShortLots(func(ast asset.Asset, lots []portfolio.TaxLot) {
				count += len(lots)
			})
			Expect(count).To(Equal(0))
		})
	})

	Describe("SetBroker", func() {
		It("replaces the broker on the account", func() {
			// The mockBroker type is defined in order_test.go and is
			// available in the portfolio_test package.
			mb1 := newMockBroker()
			mb2 := newMockBroker()

			a := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
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
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.DividendTransaction,
				Amount: 50.0,
			})
			Expect(a.Cash()).To(Equal(10_050.0))
			Expect(a.Transactions()).To(HaveLen(2)) // deposit + dividend
		})

		It("records a fee and decreases cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Type:   asset.FeeTransaction,
				Amount: -25.0,
			})
			Expect(a.Cash()).To(Equal(9_975.0))
		})

		It("records a deposit and increases cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Type:   asset.DepositTransaction,
				Amount: 5_000.0,
			})
			Expect(a.Cash()).To(Equal(15_000.0))
		})

		It("records a withdrawal and decreases cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Type:   asset.WithdrawalTransaction,
				Amount: -3_000.0,
			})
			Expect(a.Cash()).To(Equal(7_000.0))
		})

		It("records a buy: decreases cash, increases holdings, creates tax lot", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			Expect(a.Cash()).To(Equal(7_000.0))
			Expect(a.Position(spy)).To(Equal(10.0))
		})

		It("records a sell: increases cash, decreases holdings, consumes tax lots FIFO", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    5,
				Price:  320.0,
				Amount: 1_600.0,
			})
			Expect(a.Cash()).To(Equal(8_600.0))
			Expect(a.Position(spy)).To(Equal(5.0))
		})

		It("creates short lots when selling without long positions", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:  testAsset,
				Type:   asset.SellTransaction,
				Qty:    100,
				Price:  150.0,
				Amount: 15000.0,
			})

			Expect(acct.Position(testAsset)).To(Equal(-100.0))
			Expect(acct.Cash()).To(Equal(115000.0))

			var shortLotCount int
			var shortLotQty float64
			var shortLotPrice float64
			acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
				if a == testAsset {
					shortLotCount = len(lots)
					if len(lots) > 0 {
						shortLotQty = lots[0].Qty
						shortLotPrice = lots[0].Price
					}
				}
			})
			Expect(shortLotCount).To(Equal(1))
			Expect(shortLotQty).To(Equal(100.0))
			Expect(shortLotPrice).To(Equal(150.0))

			// No TradeDetail should be generated for a pure short open.
			Expect(acct.TradeDetails()).To(BeEmpty())
		})

		It("closes long lots then creates short lots for the remainder", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

			// Buy 50 shares
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
				Asset:  testAsset,
				Type:   asset.BuyTransaction,
				Qty:    50,
				Price:  140.0,
				Amount: -7000.0,
			})

			// Sell 80 shares -- closes 50 long, opens 30 short
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:  testAsset,
				Type:   asset.SellTransaction,
				Qty:    80,
				Price:  150.0,
				Amount: 12000.0,
			})

			Expect(acct.Position(testAsset)).To(Equal(-30.0))

			// Long lots should be fully consumed
			longLots := acct.UnrealizedLots(testAsset)
			Expect(longLots).To(BeEmpty())

			// Short lots should have the remainder
			var shortLotQty float64
			acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
				if a == testAsset {
					for _, lot := range lots {
						shortLotQty += lot.Qty
					}
				}
			})
			Expect(shortLotQty).To(Equal(30.0))

			// TradeDetail should show the long close.
			details := acct.TradeDetails()
			Expect(details).To(HaveLen(1))
			Expect(details[0].Direction).To(Equal(portfolio.TradeLong))
			Expect(details[0].EntryPrice).To(Equal(140.0))
			Expect(details[0].ExitPrice).To(Equal(150.0))
			Expect(details[0].Qty).To(Equal(50.0))
			Expect(details[0].PnL).To(Equal(500.0)) // (150-140) * 50
		})

		It("covers short lots on buy when short position exists", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

			// Open short: sell 100 at 150
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
				Asset:  testAsset,
				Type:   asset.SellTransaction,
				Qty:    100,
				Price:  150.0,
				Amount: 15000.0,
			})

			// Cover: buy 100 at 140 (profit $10/share)
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
				Asset:  testAsset,
				Type:   asset.BuyTransaction,
				Qty:    100,
				Price:  140.0,
				Amount: -14000.0,
			})

			Expect(acct.Position(testAsset)).To(Equal(0.0))
			Expect(acct.Cash()).To(Equal(101000.0)) // 100k + 15k - 14k

			var shortLotCount int
			acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
				if a == testAsset {
					shortLotCount = len(lots)
				}
			})
			Expect(shortLotCount).To(Equal(0))

			details := acct.TradeDetails()
			Expect(details).To(HaveLen(1))
			Expect(details[0].Direction).To(Equal(portfolio.TradeShort))
			Expect(details[0].PnL).To(Equal(1000.0)) // (150 - 140) * 100
			Expect(details[0].EntryPrice).To(Equal(150.0))
			Expect(details[0].ExitPrice).To(Equal(140.0))
		})

		It("partially covers short then creates long lots for remainder", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

			// Open short: sell 50 at 150
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
				Asset:  testAsset,
				Type:   asset.SellTransaction,
				Qty:    50,
				Price:  150.0,
				Amount: 7500.0,
			})

			// Buy 80: covers 50 short, opens 30 long
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
				Asset:  testAsset,
				Type:   asset.BuyTransaction,
				Qty:    80,
				Price:  140.0,
				Amount: -11200.0,
			})

			Expect(acct.Position(testAsset)).To(Equal(30.0))

			var shortLotCount int
			acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
				if a == testAsset {
					shortLotCount = len(lots)
				}
			})
			Expect(shortLotCount).To(Equal(0))

			longLots := acct.UnrealizedLots(testAsset)
			totalLongQty := 0.0
			for _, lot := range longLots {
				totalLongQty += lot.Qty
			}
			Expect(totalLongQty).To(Equal(30.0))
		})
	})

	Describe("UpdatePrices", func() {
		var (
			t1 time.Time
			t2 time.Time
			bm asset.Asset
		)

		BeforeEach(func() {
			t1 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			t2 = time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
			bm = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
		})

		It("with no holdings, value equals cash only", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			df := buildDF(t1, []asset.Asset{spy}, []float64{450.0}, []float64{448.0})
			a.UpdatePrices(df)

			Expect(a.Value()).To(Equal(10_000.0))
			pd := a.PerfData()
			Expect(pd).NotTo(BeNil())
			Expect(pd.Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10_000.0}))
			Expect(pd.Times()).To(Equal([]time.Time{t1}))
		})

		It("marks holdings to MetricClose prices", func() {
			a := portfolio.New(portfolio.WithCash(7_000, time.Time{}))
			// simulate having bought 10 shares
			a.Record(portfolio.Transaction{
				Date:   t1,
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			// cash is now 4_000, holding 10 SPY
			df := buildDF(t1, []asset.Asset{spy}, []float64{450.0}, []float64{448.0})
			a.UpdatePrices(df)

			// total = 4000 + 10*450 = 8500
			Expect(a.Value()).To(Equal(8_500.0))
			pd := a.PerfData()
			Expect(pd).NotTo(BeNil())
			Expect(pd.Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{8_500.0}))
			Expect(a.PositionValue(spy)).To(Equal(4_500.0))
		})

		It("accumulates equity curve, benchmark, and risk-free series over multiple calls", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
				portfolio.WithBenchmark(bm),
			)

			// Day 1
			a.SetRiskFreeValue(49.5)
			df1 := buildDF(t1,
				[]asset.Asset{spy, bm},
				[]float64{450.0, 100.0},
				[]float64{448.0, 99.0},
			)
			a.UpdatePrices(df1)

			pd := a.PerfData()
			Expect(pd).NotTo(BeNil())
			Expect(pd.Len()).To(Equal(1))
			Expect(pd.Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10_000.0}))
			Expect(pd.Column(perfAsset, data.PortfolioBenchmark)).To(Equal([]float64{99.0}))
			Expect(pd.Column(perfAsset, data.PortfolioRiskFree)).To(Equal([]float64{49.5}))

			// Day 2
			a.SetRiskFreeValue(50.0)
			df2 := buildDF(t2,
				[]asset.Asset{spy, bm},
				[]float64{455.0, 102.0},
				[]float64{453.0, 101.0},
			)
			a.UpdatePrices(df2)

			Expect(pd.Len()).To(Equal(2))
			Expect(pd.Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10_000.0, 10_000.0}))
			Expect(pd.Times()).To(Equal([]time.Time{t1, t2}))
			Expect(pd.Column(perfAsset, data.PortfolioBenchmark)).To(Equal([]float64{99.0, 101.0}))
			Expect(pd.Column(perfAsset, data.PortfolioRiskFree)).To(Equal([]float64{49.5, 50.0}))
		})

		It("stores zero benchmark/risk-free when not set", func() {
			a := portfolio.New(portfolio.WithCash(5_000, time.Time{}))
			df := buildDF(t1, []asset.Asset{spy}, []float64{450.0}, []float64{448.0})
			a.UpdatePrices(df)

			pd := a.PerfData()
			Expect(pd).NotTo(BeNil())
			Expect(pd.Column(perfAsset, data.PortfolioBenchmark)).To(Equal([]float64{0}))
			Expect(pd.Column(perfAsset, data.PortfolioRiskFree)).To(Equal([]float64{0}))
		})

		It("reflects latest prices in Value and PositionValue after UpdatePrices", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   t1,
				Asset:  spy,
				Type:   asset.BuyTransaction,
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
			pd := a.PerfData()
			Expect(pd).NotTo(BeNil())
			Expect(pd.Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10_000.0, 10_100.0}))
		})

		It("appends NaN benchmark price to keep arrays aligned with equity curve", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
				portfolio.WithBenchmark(bm),
			)

			// Day 1: normal prices
			a.SetRiskFreeValue(49.5)
			df1 := buildDF(t1,
				[]asset.Asset{spy, bm},
				[]float64{450.0, 100.0},
				[]float64{448.0, 99.0},
			)
			a.UpdatePrices(df1)
			pd := a.PerfData()
			Expect(pd).NotTo(BeNil())
			Expect(pd.Column(perfAsset, data.PortfolioBenchmark)).To(Equal([]float64{99.0}))
			Expect(pd.Column(perfAsset, data.PortfolioRiskFree)).To(Equal([]float64{49.5}))

			// Day 2: benchmark has NaN AdjClose and NaN Close
			a.SetRiskFreeValue(50.0)
			df2, err := data.NewDataFrame(
				[]time.Time{t2},
				[]asset.Asset{spy, bm},
				[]data.Metric{data.MetricClose, data.AdjClose},
				data.Daily,
				[][]float64{
					{455.0}, {453.0}, // spy: close, adjclose
					{math.NaN()}, {math.NaN()}, // bm: close, adjclose (NaN)
				},
			)
			Expect(err).NotTo(HaveOccurred())
			a.UpdatePrices(df2)

			// NaN is appended to keep benchmark aligned with equity curve.
			benchCol := pd.Column(perfAsset, data.PortfolioBenchmark)
			Expect(benchCol).To(HaveLen(2))
			Expect(math.IsNaN(benchCol[1])).To(BeTrue())
			Expect(pd.Column(perfAsset, data.PortfolioRiskFree)).To(Equal([]float64{49.5, 50.0}))
			Expect(pd.Len()).To(Equal(2))
		})

		It("stores risk-free value from SetRiskFreeValue in perf data", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
				portfolio.WithBenchmark(bm),
			)

			// Day 1: normal prices
			a.SetRiskFreeValue(49.5)
			df1 := buildDF(t1,
				[]asset.Asset{spy, bm},
				[]float64{450.0, 100.0},
				[]float64{448.0, 99.0},
			)
			a.UpdatePrices(df1)

			// Day 2: set a different risk-free value
			a.SetRiskFreeValue(50.0)
			df2 := buildDF(t2,
				[]asset.Asset{spy, bm},
				[]float64{455.0, 102.0},
				[]float64{453.0, 101.0},
			)
			a.UpdatePrices(df2)

			pd := a.PerfData()
			Expect(pd).NotTo(BeNil())
			Expect(pd.Column(perfAsset, data.PortfolioBenchmark)).To(Equal([]float64{99.0, 101.0}))
			Expect(pd.Column(perfAsset, data.PortfolioRiskFree)).To(Equal([]float64{49.5, 50.0}))
			Expect(pd.Len()).To(Equal(2))
		})

		It("preserves prices when UpdatePrices receives an empty DataFrame", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   t1,
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			// cash = 7000, 10 shares SPY

			df1 := buildDF(t1, []asset.Asset{spy}, []float64{450.0}, []float64{448.0})
			a.UpdatePrices(df1)
			Expect(a.Value()).To(Equal(11_500.0)) // 7000 + 10*450

			// Pass an empty DataFrame (simulates missing data on the last step).
			emptyDF, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
			Expect(err).NotTo(HaveOccurred())
			a.UpdatePrices(emptyDF)

			// Value must still reflect the last known prices, not drop to cash-only.
			Expect(a.Value()).To(Equal(11_500.0))
		})
	})

	Describe("PerfData before any UpdatePrices", func() {
		It("returns nil before any UpdatePrices calls", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			Expect(a.PerfData()).To(BeNil())
		})

		It("returns correct accumulated data after multiple UpdatePrices calls", func() {
			bm := asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}

			a := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
				portfolio.WithBenchmark(bm),
			)

			t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			t2 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
			t3 := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)

			a.SetRiskFreeValue(49.0)
			a.UpdatePrices(buildDF(t1,
				[]asset.Asset{spy, bm},
				[]float64{100.0, 200.0},
				[]float64{99.0, 198.0},
			))
			a.SetRiskFreeValue(49.5)
			a.UpdatePrices(buildDF(t2,
				[]asset.Asset{spy, bm},
				[]float64{102.0, 204.0},
				[]float64{101.0, 202.0},
			))
			a.SetRiskFreeValue(50.0)
			a.UpdatePrices(buildDF(t3,
				[]asset.Asset{spy, bm},
				[]float64{104.0, 208.0},
				[]float64{103.0, 206.0},
			))

			pd := a.PerfData()
			Expect(pd).NotTo(BeNil())
			Expect(pd.Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10_000.0, 10_000.0, 10_000.0}))
			Expect(pd.Times()).To(Equal([]time.Time{t1, t2, t3}))
			Expect(pd.Column(perfAsset, data.PortfolioBenchmark)).To(Equal([]float64{198.0, 202.0, 206.0}))
			Expect(pd.Column(perfAsset, data.PortfolioRiskFree)).To(Equal([]float64{49.0, 49.5, 50.0}))
		})
	})

	Describe("Holdings", func() {
		It("starts with no holdings", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			Expect(a.Holdings()).To(HaveLen(0))
		})

		It("returns zero for unknown positions", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			Expect(a.Position(spy)).To(Equal(0.0))
			Expect(a.PositionValue(spy)).To(Equal(0.0))
		})

		It("iterates over actual positions with correct asset/qty pairs", func() {
			a := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  bil,
				Type:   asset.BuyTransaction,
				Qty:    20,
				Price:  50.0,
				Amount: -1_000.0,
			})

			seen := a.Holdings()
			Expect(seen).To(HaveLen(2))
			Expect(seen[spy]).To(Equal(10.0))
			Expect(seen[bil]).To(Equal(20.0))
		})
	})

	Describe("Value with NaN price", func() {
		It("skips NaN-priced assets and returns cash only", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			a.Record(portfolio.Transaction{
				Date:   t1,
				Asset:  spy,
				Type:   asset.BuyTransaction,
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
				data.Daily,
				[][]float64{{math.NaN()}},
			)
			Expect(err).NotTo(HaveOccurred())
			a.UpdatePrices(df)

			// Value should equal cash only since SPY price is NaN.
			Expect(a.Value()).To(Equal(7_000.0))
		})
	})

	Describe("PositionValue with nil prices", func() {
		It("returns 0 when prices have never been set", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.BuyTransaction,
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
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  300.0,
				Amount: -3_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    10,
				Price:  320.0,
				Amount: 3_200.0,
			})

			Expect(a.Position(spy)).To(Equal(0.0))

			// Holdings should not include SPY at all.
			seen := a.Holdings()
			Expect(seen).NotTo(HaveKey(spy))
		})
	})

	Describe("Record with multiple tax lots (FIFO partial consumption)", func() {
		It("consumes lots in FIFO order across partial sells", func() {
			a := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy 10 shares at $100 on day 1.
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			// Buy 5 shares at $120 on day 2.
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    5,
				Price:  120.0,
				Amount: -600.0,
			})

			// Sell 12 shares at $150.
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.SellTransaction,
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

	Describe("WithCash(0, time.Time{})", func() {
		It("records no deposit transaction when cash is 0", func() {
			a := portfolio.New(portfolio.WithCash(0, time.Time{}))
			txns := a.Transactions()
			// A deposit of 0 is still recorded by WithCash.
			// Verify cash is 0 and the transaction exists but has 0 amount.
			Expect(a.Cash()).To(Equal(0.0))
			Expect(txns).To(HaveLen(1))
			Expect(txns[0].Type).To(Equal(asset.DepositTransaction))
			Expect(txns[0].Amount).To(Equal(0.0))
		})
	})
})

var _ = Describe("Account.Clone", func() {
	It("preserves holdings, cash, metadata, and annotations", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}

		acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
		acct.SetMetadata("strategy", "adm")
		acct.Annotate(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), "signal", "0.5")

		acct.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  500,
			Amount: -50_000,
		})

		clone := acct.Clone()

		Expect(clone.Cash()).To(Equal(acct.Cash()))
		Expect(clone.Position(spy)).To(Equal(acct.Position(spy)))
		Expect(clone.GetMetadata("strategy")).To(Equal("adm"))
		Expect(clone.Annotations()).To(HaveLen(1))
	})

	It("isolates clone mutations from original", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
		acct.SetMetadata("key", "original")
		acct.Annotate(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), "signal", "0.5")

		cloneIface := acct.Clone()
		clone := cloneIface.(*portfolio.Account)

		clone.SetMetadata("key", "mutated")
		clone.Annotate(time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC), "other", "1.0")

		Expect(acct.GetMetadata("key")).To(Equal("original"))
		Expect(acct.Annotations()).To(HaveLen(1))
	})

	It("does not panic and isolates group state when group fields are populated", func() {
		ts := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
		testAsset := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

		mb := newMockBroker()
		// Do not deliver fills so the entry order stays pending.
		mb.submitFn = func(_ broker.Order) error { return nil }

		acct := portfolio.New(portfolio.WithCash(50_000, ts), portfolio.WithBroker(mb))
		df := buildDF(ts, []asset.Asset{testAsset}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		batch := acct.NewBatch(ts)
		err := batch.Order(context.Background(), testAsset, portfolio.Buy, 10,
			portfolio.WithBracket(
				portfolio.StopLossPrice(90.0),
				portfolio.TakeProfitPrice(115.0),
			))
		Expect(err).NotTo(HaveOccurred())

		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// The account should have pending orders and group state.
		Expect(acct.PendingOrderIDs()).NotTo(BeEmpty())

		// Clone must not panic.
		cloneIface := acct.Clone()
		Expect(cloneIface).NotTo(BeNil())
		clone := cloneIface.(*portfolio.Account)

		// Mutating the clone's pending orders must not affect the original.
		for _, orderID := range clone.PendingOrderIDs() {
			clone.SetPendingOrder(broker.Order{ID: orderID + "-clone"})
		}
		for _, id := range acct.PendingOrderIDs() {
			Expect(id).NotTo(HaveSuffix("-clone"))
		}
	})
})

var _ = Describe("CancelOpenOrders group cleanup", func() {
	var (
		testAsset asset.Asset
		ts        time.Time
	)

	BeforeEach(func() {
		testAsset = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		ts = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	})

	It("clears pendingOrders, pendingGroups, and deferredExits after cancellation", func() {
		mb := newMockBroker()
		canceledIDs := []string{}
		mb.cancelFn = func(orderID string) error {
			canceledIDs = append(canceledIDs, orderID)
			return nil
		}
		// Do not deliver fills so the entry order stays pending.
		mb.submitFn = func(_ broker.Order) error { return nil }

		acct := portfolio.New(portfolio.WithCash(50_000, ts), portfolio.WithBroker(mb))
		df := buildDF(ts, []asset.Asset{testAsset}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		batch := acct.NewBatch(ts)
		err := batch.Order(context.Background(), testAsset, portfolio.Buy, 10,
			portfolio.WithBracket(
				portfolio.StopLossPrice(90.0),
				portfolio.TakeProfitPrice(115.0),
			))
		Expect(err).NotTo(HaveOccurred())

		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// The entry order is pending (no fill was delivered).
		pendingBefore := acct.PendingOrderIDs()
		Expect(pendingBefore).NotTo(BeEmpty())

		err = acct.CancelOpenOrders(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// broker.Cancel was called for every pending order.
		Expect(canceledIDs).To(ConsistOf(pendingBefore))

		// All group state is cleared.
		Expect(acct.PendingOrderIDs()).To(BeEmpty())
	})

	It("allows a new batch to be submitted cleanly after CancelOpenOrders", func() {
		mb := newMockBroker()
		mb.submitFn = func(ord broker.Order) error {
			// Deliver a fill for the second batch's entry order.
			mb.fillCh <- broker.Fill{OrderID: ord.ID, Price: 100.0, Qty: ord.Qty, FilledAt: ts}
			return nil
		}

		acct := portfolio.New(portfolio.WithCash(50_000, ts), portfolio.WithBroker(mb))
		df := buildDF(ts, []asset.Asset{testAsset}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		// Submit first batch (no fills) then cancel.
		firstBatch := acct.NewBatch(ts)
		firstSubmitFn := mb.submitFn
		mb.submitFn = func(_ broker.Order) error { return nil } // no fills for first batch
		err := firstBatch.Order(context.Background(), testAsset, portfolio.Buy, 5,
			portfolio.WithBracket(
				portfolio.StopLossPrice(90.0),
				portfolio.TakeProfitPrice(115.0),
			))
		Expect(err).NotTo(HaveOccurred())
		Expect(acct.ExecuteBatch(context.Background(), firstBatch)).To(Succeed())

		err = acct.CancelOpenOrders(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(acct.PendingOrderIDs()).To(BeEmpty())

		// Restore fill-delivering submitFn for second batch.
		mb.submitFn = firstSubmitFn

		// Submit a plain second batch; should not panic or error.
		secondBatch := acct.NewBatch(ts)
		err = secondBatch.Order(context.Background(), testAsset, portfolio.Buy, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(acct.ExecuteBatch(context.Background(), secondBatch)).To(Succeed())
	})
})

var _ = Describe("TransactionType", func() {
	It("returns correct string for each type", func() {
		Expect(asset.BuyTransaction.String()).To(Equal("Buy"))
		Expect(asset.SellTransaction.String()).To(Equal("Sell"))
		Expect(asset.DividendTransaction.String()).To(Equal("Dividend"))
		Expect(asset.FeeTransaction.String()).To(Equal("Fee"))
		Expect(asset.DepositTransaction.String()).To(Equal("Deposit"))
		Expect(asset.WithdrawalTransaction.String()).To(Equal("Withdrawal"))
	})

	It("returns a formatted string for unknown transaction types", func() {
		t := asset.TransactionType(99)
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

		spyPrices := []float64{100, 105, 98, 103, 97, 110}
		bilPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}
		n := len(spyPrices)
		times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

		acct := portfolio.New(
			portfolio.WithCash(5*spyPrices[0], time.Time{}),
			portfolio.WithBenchmark(spy),
		)
		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})
		for i := range n {
			acct.SetRiskFreeValue(bilPrices[i])
			df := buildDF(times[i],
				[]asset.Asset{spy},
				[]float64{spyPrices[i]},
				[]float64{spyPrices[i]},
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
	// daySeq(2025-01-02, 6) = Jan 2,3,6,7,8,9 -> 7 calendar days, 5 returns
	// AF = 5 / (7/365.25) = 260.89
	af := 5.0 / (7.0 / 365.25)

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

		spyPrices := []float64{100, 105, 98, 103, 97, 110}
		bilPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}
		n := len(spyPrices)
		times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

		acct := portfolio.New(
			portfolio.WithCash(5*spyPrices[0], time.Time{}),
			portfolio.WithBenchmark(spy),
		)
		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})
		for i := range n {
			acct.SetRiskFreeValue(bilPrices[i])
			df := buildDF(times[i],
				[]asset.Asset{spy},
				[]float64{spyPrices[i]},
				[]float64{spyPrices[i]},
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
	// daySeq(2025-01-02, 6) = Jan 2,3,6,7,8,9 -> 7 calendar days, 5 returns
	af := 5.0 / (7.0 / 365.25)

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

	It("Treynor = 0 for short backtests (< 30 days)", func() {
		// 6 data points spanning ~7 calendar days: too short to annualize.
		rm, err := buildRiskAcct().RiskMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.Treynor).To(Equal(0.0))
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
	// Build a 400-day steadily growing equity curve starting at 100_000 with
	// 0.02% daily growth. Over 400 days (~13 months) this produces a
	// monotonically rising curve with at least one year boundary.
	var buildWithdrawalAcct = func() *portfolio.Account {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
		price := 100_000.0
		start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

		for idx := range 400 {
			date := start.AddDate(0, 0, idx)
			if idx > 0 {
				growth := price * 0.0002
				acct.Record(portfolio.Transaction{
					Date:   date,
					Type:   asset.DividendTransaction,
					Amount: growth,
				})
				price += growth
			}
			df := buildDF(date, []asset.Asset{spy}, []float64{450 + float64(idx)}, []float64{448 + float64(idx)})
			acct.UpdatePrices(df)
		}
		return acct
	}

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
			portfolio.WithCash(5*spyPrices[0], time.Time{}),
			portfolio.WithBenchmark(spy),
		)
		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})
		for i := range n {
			acct.SetRiskFreeValue(bilPrices[i])
			df := buildDF(times[i],
				[]asset.Asset{spy},
				[]float64{spyPrices[i]},
				[]float64{spyPrices[i]},
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

		// Months(1) window: snaps to the 1st of last's month (N=1 means
		// first.AddDate(0, 0, 0) = 1st of current month).
		last := times[len(times)-1]
		cutoff := time.Date(last.Year(), last.Month(), 1,
			last.Hour(), last.Minute(), last.Second(), last.Nanosecond(), last.Location())
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
		windowedTimes := times[startIdx:]

		wRet := helperReturns(windowedEq)
		wRfRet := helperReturns(windowedRf)
		wER := helperExcessReturns(wRet, wRfRet)
		// AF computed from actual windowed timestamps.
		calDays := windowedTimes[len(windowedTimes)-1].Sub(windowedTimes[0]).Hours() / 24
		windowAF := float64(len(windowedTimes)-1) / (calDays / 365.25)
		expectedSharpe := helperMean(wER) / helperStddev(wER) * math.Sqrt(windowAF)

		windowedSharpe, err := acct.PerformanceMetric(portfolio.Sharpe).Window(portfolio.Days(10)).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(windowedSharpe).To(BeNumerically("~", expectedSharpe, 1e-10))

		fullSharpe, err := acct.PerformanceMetric(portfolio.Sharpe).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(fullSharpe).NotTo(BeNumerically("~", windowedSharpe, 1e-10))
	})

	Describe("ExecuteBatch group submission", func() {
		var (
			testAsset asset.Asset
			ts        time.Time
		)

		BeforeEach(func() {
			testAsset = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
			ts = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
		})

		It("submits standalone OCO legs individually when broker lacks GroupSubmitter", func() {
			mb := newMockBroker()
			mb.submitFn = func(ord broker.Order) error {
				mb.fillCh <- broker.Fill{OrderID: ord.ID, Price: 50.0, Qty: ord.Qty, FilledAt: ts}
				return nil
			}

			acct := portfolio.New(
				portfolio.WithCash(10_000, ts),
				portfolio.WithBroker(mb),
			)

			batch := acct.NewBatch(ts)
			err := batch.Order(context.Background(), testAsset, portfolio.Sell, 10,
				portfolio.OCO(portfolio.StopLeg(45.0), portfolio.LimitLeg(55.0)))
			Expect(err).NotTo(HaveOccurred())
			Expect(batch.Groups()).To(HaveLen(1))
			Expect(batch.Groups()[0].Type).To(Equal(broker.GroupOCO))

			// With a plain broker (no GroupSubmitter), OCO legs are submitted individually.
			err = acct.ExecuteBatch(context.Background(), batch)
			Expect(err).NotTo(HaveOccurred())

			Expect(mb.submitted).To(HaveLen(2))
		})

		It("submits OCO legs via GroupSubmitter when broker supports it", func() {
			mgb := newMockGroupBroker()

			acct := portfolio.New(
				portfolio.WithCash(10_000, ts),
				portfolio.WithBroker(mgb),
			)

			batch := acct.NewBatch(ts)
			err := batch.Order(context.Background(), testAsset, portfolio.Sell, 10,
				portfolio.OCO(portfolio.StopLeg(45.0), portfolio.LimitLeg(55.0)))
			Expect(err).NotTo(HaveOccurred())

			err = acct.ExecuteBatch(context.Background(), batch)
			Expect(err).NotTo(HaveOccurred())

			// SubmitGroup called once; individual Submit not called.
			Expect(mgb.submittedGroups).To(HaveLen(1))
			Expect(mgb.submittedGroups[0].groupType).To(Equal(broker.GroupOCO))
			Expect(mgb.submittedGroups[0].orders).To(HaveLen(2))
			Expect(mgb.submitted).To(BeEmpty())
		})

		It("submits bracket entry individually and then exits after entry fill", func() {
			mb := newMockBroker()
			mb.submitFn = func(ord broker.Order) error {
				// Only deliver a fill for the entry order (not for exit orders).
				if ord.GroupRole == broker.RoleEntry {
					mb.fillCh <- broker.Fill{OrderID: ord.ID, Price: 100.0, Qty: ord.Qty, FilledAt: ts}
				}
				return nil
			}

			acct := portfolio.New(
				portfolio.WithCash(50_000, ts),
				portfolio.WithBroker(mb),
			)

			df := buildDF(ts, []asset.Asset{testAsset}, []float64{100.0}, []float64{100.0})
			acct.UpdatePrices(df)

			batch := acct.NewBatch(ts)
			err := batch.Order(context.Background(), testAsset, portfolio.Buy, 10,
				portfolio.WithBracket(
					portfolio.StopLossPrice(90.0),
					portfolio.TakeProfitPrice(115.0),
				))
			Expect(err).NotTo(HaveOccurred())
			Expect(batch.Groups()).To(HaveLen(1))
			Expect(batch.Groups()[0].Type).To(Equal(broker.GroupBracket))

			err = acct.ExecuteBatch(context.Background(), batch)
			Expect(err).NotTo(HaveOccurred())

			// Entry is submitted first, then entry fill triggers exit submission.
			// With a non-GroupSubmitter broker, exits are submitted individually.
			Expect(mb.submitted).To(HaveLen(3))
			Expect(mb.submitted[0].GroupRole).To(Equal(broker.RoleEntry))

			// Verify exit orders were submitted with correct prices and roles.
			var hasStopLoss, hasTakeProfit bool
			for _, ord := range mb.submitted[1:] {
				switch ord.GroupRole {
				case broker.RoleStopLoss:
					hasStopLoss = true
					Expect(ord.StopPrice).To(Equal(90.0))
					Expect(ord.Side).To(Equal(broker.Sell))
				case broker.RoleTakeProfit:
					hasTakeProfit = true
					Expect(ord.LimitPrice).To(Equal(115.0))
					Expect(ord.Side).To(Equal(broker.Sell))
				}
			}
			Expect(hasStopLoss).To(BeTrue())
			Expect(hasTakeProfit).To(BeTrue())
		})
	})

	Describe("DrainFills two-phase logic", func() {
		var (
			testAsset asset.Asset
			ts        time.Time
		)

		BeforeEach(func() {
			testAsset = asset.Asset{CompositeFigi: "TEST", Ticker: "TEST"}
			ts = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		})

		It("submits bracket exit orders when entry fill arrives via GroupSubmitter broker", func() {
			mgb := newMockGroupBroker()

			// Configure submitFn to deliver a fill for the entry order.
			mgb.submitFn = func(ord broker.Order) error {
				mgb.fillCh <- broker.Fill{OrderID: ord.ID, Price: 100.0, Qty: ord.Qty, FilledAt: ts}
				return nil
			}

			acct := portfolio.New(
				portfolio.WithCash(50_000, ts),
				portfolio.WithBroker(mgb),
			)

			df := buildDF(ts, []asset.Asset{testAsset}, []float64{100.0}, []float64{100.0})
			acct.UpdatePrices(df)

			stopPct := 5.0  // 5% below entry -> PercentOffset = -0.05
			takePct := 10.0 // 10% above entry -> PercentOffset = +0.10

			batch := acct.NewBatch(ts)
			err := batch.Order(context.Background(), testAsset, portfolio.Buy, 10,
				portfolio.WithBracket(
					portfolio.StopLossPercent(stopPct),
					portfolio.TakeProfitPercent(takePct),
				))
			Expect(err).NotTo(HaveOccurred())

			err = acct.ExecuteBatch(context.Background(), batch)
			Expect(err).NotTo(HaveOccurred())

			// Entry was submitted individually, then DrainFills (called inside
			// ExecuteBatch) should have triggered bracket exit submission via
			// SubmitGroup.
			Expect(mgb.submittedGroups).To(HaveLen(1))
			Expect(mgb.submittedGroups[0].groupType).To(Equal(broker.GroupOCO))
			Expect(mgb.submittedGroups[0].orders).To(HaveLen(2))

			exitOrders := mgb.submittedGroups[0].orders

			// Find stop-loss and take-profit orders.
			var stopLoss, takeProfit broker.Order
			for _, ord := range exitOrders {
				switch ord.GroupRole {
				case broker.RoleStopLoss:
					stopLoss = ord
				case broker.RoleTakeProfit:
					takeProfit = ord
				}
			}

			// Verify stop loss price = fillPrice * (1 - stopPct/100) = 100 * 0.95 = 95.
			Expect(stopLoss.StopPrice).To(BeNumerically("~", 95.0, 0.001))
			// Verify take profit price = fillPrice * (1 + takePct/100) = 100 * 1.10 = 110.
			Expect(takeProfit.LimitPrice).To(BeNumerically("~", 110.0, 0.001))

			// Exit orders have side opposite to entry (entry=Buy -> exit=Sell).
			Expect(stopLoss.Side).To(Equal(broker.Sell))
			Expect(takeProfit.Side).To(Equal(broker.Sell))

			// TIF = GTC.
			Expect(stopLoss.TimeInForce).To(Equal(broker.GTC))
			Expect(takeProfit.TimeInForce).To(Equal(broker.GTC))
		})

		It("cancels OCO sibling on fill without calling broker.Cancel for GroupSubmitter broker", func() {
			mgb := newMockGroupBroker()

			cancelCalled := false
			mgb.cancelFn = func(_ string) error {
				cancelCalled = true
				return nil
			}

			acct := portfolio.New(
				portfolio.WithCash(50_000, ts),
				portfolio.WithBroker(mgb),
			)

			df := buildDF(ts, []asset.Asset{testAsset}, []float64{100.0}, []float64{100.0})
			acct.UpdatePrices(df)

			// Manually set up an OCO group in pendingOrders and pendingGroups.
			// Simulate two OCO orders already tracked by the account.
			orderA := broker.Order{
				ID:        "oco-a",
				Asset:     testAsset,
				Side:      broker.Sell,
				Qty:       10,
				OrderType: broker.Stop,
				StopPrice: 95.0,
				GroupID:   "oco-group-1",
				GroupRole: broker.RoleStopLoss,
			}
			orderB := broker.Order{
				ID:         "oco-b",
				Asset:      testAsset,
				Side:       broker.Sell,
				Qty:        10,
				OrderType:  broker.Limit,
				LimitPrice: 110.0,
				GroupID:    "oco-group-1",
				GroupRole:  broker.RoleTakeProfit,
			}

			acct.SetPendingOrder(orderA)
			acct.SetPendingOrder(orderB)
			acct.SetPendingGroup(&broker.OrderGroup{
				ID:       "oco-group-1",
				Type:     broker.GroupOCO,
				OrderIDs: []string{"oco-a", "oco-b"},
			})

			// Simulate a fill arriving for orderA.
			mgb.fillCh <- broker.Fill{OrderID: "oco-a", Price: 95.0, Qty: 10, FilledAt: ts}

			err := acct.DrainFills(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// The sibling (orderB) should be removed from pendingOrders
			// WITHOUT calling broker.Cancel (since broker is a GroupSubmitter).
			Expect(cancelCalled).To(BeFalse())
			Expect(acct.PendingOrderIDs()).NotTo(ContainElement("oco-b"))
			Expect(acct.PendingOrderIDs()).NotTo(ContainElement("oco-a"))
		})
	})
})

var _ = Describe("SyncTransactions", func() {
	var (
		spy asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	})

	It("applies broker dividend transactions to the account", func() {
		date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		acct := portfolio.New(portfolio.WithCash(50_000, date))

		// Establish a holding so the dividend has context.
		acct.Record(portfolio.Transaction{
			Date:   date,
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  400,
			Amount: -40_000,
		})

		divDate := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
		err := acct.SyncTransactions([]broker.Transaction{
			{
				ID:     "div-001",
				Date:   divDate,
				Asset:  spy,
				Type:   asset.DividendTransaction,
				Qty:    100,
				Price:  1.50,
				Amount: 150.0,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		txns := acct.Transactions()
		// deposit + buy + dividend = 3
		Expect(txns).To(HaveLen(3))

		divTxn := txns[2]
		Expect(divTxn.Type).To(Equal(asset.DividendTransaction))
		Expect(divTxn.Amount).To(Equal(150.0))
		Expect(divTxn.ID).To(Equal("div-001"))
		Expect(acct.Cash()).To(BeNumerically("~", 10_150.0, 0.01))
	})

	It("deduplicates transactions by ID", func() {
		date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		acct := portfolio.New(portfolio.WithCash(50_000, date))

		feeTxn := broker.Transaction{
			ID:     "fee-001",
			Date:   date,
			Type:   asset.FeeTransaction,
			Amount: -25.0,
		}

		err := acct.SyncTransactions([]broker.Transaction{feeTxn})
		Expect(err).NotTo(HaveOccurred())

		// Sync the same transaction again.
		err = acct.SyncTransactions([]broker.Transaction{feeTxn})
		Expect(err).NotTo(HaveOccurred())

		txns := acct.Transactions()
		// deposit + one fee = 2 (not 3)
		Expect(txns).To(HaveLen(2))
		Expect(acct.Cash()).To(BeNumerically("~", 49_975.0, 0.01))
	})

	It("applies broker split transactions to account holdings", func() {
		date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		acct := portfolio.New(portfolio.WithCash(50_000, date))

		// Establish a 100-share position.
		acct.Record(portfolio.Transaction{
			Date:   date,
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  400,
			Amount: -40_000,
		})

		splitDate := time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC)
		err := acct.SyncTransactions([]broker.Transaction{
			{
				ID:    "split-001",
				Date:  splitDate,
				Asset: spy,
				Type:  asset.SplitTransaction,
				Price: 2.0, // 2-for-1 split factor
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(acct.Position(spy)).To(Equal(200.0))
	})
})

var _ = Describe("ApplySplit", func() {
	var (
		acme asset.Asset
	)

	BeforeEach(func() {
		acme = asset.Asset{CompositeFigi: "ACME", Ticker: "ACME"}
	})

	It("adjusts long position and tax lots for a 2:1 split", func() {
		date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		acct := portfolio.New(portfolio.WithCash(50_000, date))
		acct.Record(portfolio.Transaction{
			Date:   date,
			Asset:  acme,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  200,
			Amount: -20_000,
		})

		splitDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		err := acct.ApplySplit(acme, splitDate, 2.0)
		Expect(err).NotTo(HaveOccurred())

		Expect(acct.Position(acme)).To(Equal(200.0))

		lots := acct.TaxLots()[acme]
		Expect(lots).To(HaveLen(1))
		Expect(lots[0].Qty).To(Equal(200.0))
		Expect(lots[0].Price).To(Equal(100.0))

		txns := acct.Transactions()
		lastTxn := txns[len(txns)-1]
		Expect(lastTxn.Type).To(Equal(asset.SplitTransaction))
		Expect(lastTxn.Qty).To(Equal(200.0))
		Expect(lastTxn.Price).To(Equal(2.0))
		Expect(lastTxn.Amount).To(Equal(0.0))
	})

	It("adjusts short position and short lots for a 2:1 split", func() {
		date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		acct := portfolio.New(portfolio.WithCash(50_000, date))
		acct.Record(portfolio.Transaction{
			Date:   date,
			Asset:  acme,
			Type:   asset.SellTransaction,
			Qty:    100,
			Price:  200,
			Amount: 20_000,
		})

		splitDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		err := acct.ApplySplit(acme, splitDate, 2.0)
		Expect(err).NotTo(HaveOccurred())

		Expect(acct.Position(acme)).To(Equal(-200.0))

		var shortLots []portfolio.TaxLot
		acct.ShortLots(func(ast asset.Asset, lots []portfolio.TaxLot) {
			if ast == acme {
				shortLots = lots
			}
		})
		Expect(shortLots).To(HaveLen(1))
		Expect(shortLots[0].Qty).To(Equal(200.0))
		Expect(shortLots[0].Price).To(Equal(100.0))
	})

	It("returns error when split factor is zero", func() {
		date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		acct := portfolio.New(portfolio.WithCash(50_000, date))
		acct.Record(portfolio.Transaction{
			Date:   date,
			Asset:  acme,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  200,
			Amount: -20_000,
		})

		err := acct.ApplySplit(acme, date, 0)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("split factor cannot be zero"))
	})
})

var _ = Describe("batch history", func() {
	It("assigns sequential BatchIDs starting at 1 and stamps orders", func() {
		ctx := context.Background()
		ts1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		ts2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		mb := newMockBroker()
		mb.defaultFill = &broker.Fill{Price: 100.0, FilledAt: ts1}

		acct := portfolio.New(portfolio.WithCash(100_000, ts1), portfolio.WithBroker(mb))
		acct.UpdatePrices(buildDF(ts1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))

		batch1 := acct.NewBatch(ts1)
		Expect(batch1.Order(ctx, spy, portfolio.Buy, 10)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, batch1)).To(Succeed())

		batch2 := acct.NewBatch(ts2)
		Expect(batch2.Order(ctx, spy, portfolio.Sell, 5)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, batch2)).To(Succeed())

		batches := portfolio.GetAccountBatches(acct)
		Expect(batches).To(HaveLen(2))
		Expect(batches[0].BatchID).To(Equal(1))
		Expect(batches[1].BatchID).To(Equal(2))
		Expect(batches[0].Timestamp).To(Equal(ts1))
		Expect(batches[1].Timestamp).To(Equal(ts2))
	})

	It("resets currentBatchID to 0 after ExecuteBatch returns", func() {
		ctx := context.Background()
		ts := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

		mb := newMockBroker()
		acct := portfolio.New(portfolio.WithCash(100_000, ts), portfolio.WithBroker(mb))

		batch := acct.NewBatch(ts)
		Expect(acct.ExecuteBatch(ctx, batch)).To(Succeed())

		Expect(portfolio.GetAccountCurrentBatchID(acct)).To(BeZero())
	})

	It("preserves batch history and resets currentBatchID when middleware rejects the batch", func() {
		ctx := context.Background()
		ts := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		rejectErr := errors.New("rejected by middleware")

		mb := newMockBroker()
		acct := portfolio.New(portfolio.WithCash(100_000, ts), portfolio.WithBroker(mb))
		acct.Use(&errorMiddleware{err: rejectErr})

		batch := acct.NewBatch(ts)
		err := acct.ExecuteBatch(ctx, batch)
		Expect(err).To(MatchError(rejectErr))

		Expect(portfolio.GetAccountCurrentBatchID(acct)).To(BeZero())

		batches := portfolio.GetAccountBatches(acct)
		Expect(batches).To(HaveLen(1))
		Expect(batches[0].BatchID).To(Equal(1))
	})

	It("copies order.BatchID onto the recorded transaction", func() {
		ctx := context.Background()
		ts := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		mb := newMockBroker()
		mb.defaultFill = &broker.Fill{Price: 100.0, FilledAt: ts}

		acct := portfolio.New(portfolio.WithCash(100_000, ts), portfolio.WithBroker(mb))
		acct.UpdatePrices(buildDF(ts, []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))

		batch := acct.NewBatch(ts)
		Expect(batch.Order(ctx, spy, portfolio.Buy, 10)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, batch)).To(Succeed())

		var tradeTxn portfolio.Transaction
		for _, txn := range acct.Transactions() {
			if txn.Type == asset.BuyTransaction {
				tradeTxn = txn
				break
			}
		}
		Expect(tradeTxn.BatchID).To(Equal(1))
	})
})
