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

var _ = Describe("TradeMetrics", func() {
	var (
		acme asset.Asset
		widg asset.Asset
	)

	BeforeEach(func() {
		acme = asset.Asset{CompositeFigi: "ACME", Ticker: "ACME"}
		widg = asset.Asset{CompositeFigi: "WIDG", Ticker: "WIDG"}
	})

	Describe("with three round-trip trades", func() {
		var tm portfolio.TradeMetrics

		BeforeEach(func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			// Trade 1: Buy 10 ACME at $100, Sell at $120, held 30 days -> win $200
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  120.0,
				Amount: 1_200.0,
			})

			// Trade 2: Buy 20 WIDG at $50, Sell at $45, held 60 days -> loss -$100
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  widg,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  50.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
				Asset:  widg,
				Type:   portfolio.SellTransaction,
				Qty:    20,
				Price:  45.0,
				Amount: 900.0,
			})

			// Trade 3: Buy 5 ACME at $200, Sell at $210, held 15 days -> win $50
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    5,
				Price:  200.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 5, 16, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    5,
				Price:  210.0,
				Amount: 1_050.0,
			})

			var err error
			tm, err = a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
		})

		It("computes WinRate correctly", func() {
			Expect(tm.WinRate).To(BeNumerically("~", 2.0/3.0, 1e-4))
		})

		It("computes AverageWin correctly", func() {
			Expect(tm.AverageWin).To(BeNumerically("~", 125.0, 1e-4))
		})

		It("computes AverageLoss correctly", func() {
			Expect(tm.AverageLoss).To(BeNumerically("~", -100.0, 1e-4))
		})

		It("computes ProfitFactor correctly", func() {
			Expect(tm.ProfitFactor).To(BeNumerically("~", 2.5, 1e-4))
		})

		It("computes AverageHoldingPeriod correctly", func() {
			Expect(tm.AverageHoldingPeriod).To(BeNumerically("~", 35.0, 1e-4))
		})

		It("computes GainLossRatio correctly", func() {
			// AverageWin / abs(AverageLoss) = 125 / 100 = 1.25
			Expect(tm.GainLossRatio).To(BeNumerically("~", 1.25, 1e-4))
		})
	})

	Describe("with no trades", func() {
		It("returns zero values", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())

			Expect(tm.WinRate).To(Equal(0.0))
			Expect(tm.AverageWin).To(Equal(0.0))
			Expect(tm.AverageLoss).To(Equal(0.0))
			Expect(math.IsNaN(tm.ProfitFactor)).To(BeTrue())
			Expect(tm.AverageHoldingPeriod).To(Equal(0.0))
			Expect(math.IsNaN(tm.GainLossRatio)).To(BeTrue())
		})
	})

	Describe("with only winning trades", func() {
		It("sets ProfitFactor and GainLossRatio to +Inf (no losses)", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  120.0,
				Amount: 1_200.0,
			})

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tm.WinRate).To(Equal(1.0))
			Expect(tm.AverageWin).To(Equal(200.0))
			Expect(tm.AverageLoss).To(Equal(0.0))
			Expect(math.IsNaN(tm.ProfitFactor)).To(BeTrue())
			Expect(math.IsNaN(tm.GainLossRatio)).To(BeTrue())
		})
	})

	Describe("NPositivePeriods", func() {
		It("computes fraction of positive equity curve returns", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			// Simulate an equity curve: 10000, 10100, 10050, 10200, 10150
			// Returns: +100 (pos), -50 (neg), +150 (pos), -50 (neg) => 2/4 = 0.5
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			// We need to use UpdatePrices to build equity curve. Build
			// minimal DataFrames. Each has ACME with Close and AdjClose.
			buildDF := func(t time.Time, price float64) *data.DataFrame {
				df, err := data.NewDataFrame(
					[]time.Time{t},
					[]asset.Asset{acme},
					[]data.Metric{data.MetricClose, data.AdjClose},
					data.Daily,
					[][]float64{{price}, {price}},
				)
				Expect(err).NotTo(HaveOccurred())
				return df
			}

			// cash=9000, 10 shares ACME
			// equity = 9000 + 10*price
			// To get 10000: price=100, 10100: price=110, 10050: price=105,
			// 10200: price=120, 10150: price=115
			a.UpdatePrices(buildDF(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 100.0))
			a.UpdatePrices(buildDF(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 110.0))
			a.UpdatePrices(buildDF(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), 105.0))
			a.UpdatePrices(buildDF(time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC), 120.0))
			a.UpdatePrices(buildDF(time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC), 115.0))

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tm.NPositivePeriods).To(BeNumerically("~", 0.5, 1e-4))
		})
	})

	Describe("Turnover", func() {
		It("computes annualized turnover from sell volume and mean portfolio value", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			buildDF := func(t time.Time, price float64) *data.DataFrame {
				df, err := data.NewDataFrame(
					[]time.Time{t},
					[]asset.Asset{acme},
					[]data.Metric{data.MetricClose, data.AdjClose},
					data.Daily,
					[][]float64{{price}, {price}},
				)
				Expect(err).NotTo(HaveOccurred())
				return df
			}

			// Day 1: buy 10 shares at 100
			a.UpdatePrices(buildDF(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 100.0))
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			// Day 2
			a.UpdatePrices(buildDF(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 100.0))

			// Day 3: sell 10 shares at 100
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: 1_000.0,
			})
			a.UpdatePrices(buildDF(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), 100.0))

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())

			// Total sell value = 1000
			// Equity curve: [10000, 10000, 10000], mean = 10000
			// Period = 2 days, annualized factor = 365.25/2
			// Turnover = (1000/10000) * (365.25/2) = 0.1 * 182.625 = 18.2625
			Expect(tm.Turnover).To(BeNumerically("~", 18.2625, 0.01))
		})
	})

	Describe("with only losing trades", func() {
		It("returns WinRate=0, AverageWin=0, ProfitFactor=0", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			// Trade 1: Buy 10 ACME at $100, Sell at $90 -> loss -$100
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  90.0,
				Amount: 900.0,
			})

			// Trade 2: Buy 20 WIDG at $50, Sell at $40 -> loss -$200
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  widg,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  50.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				Asset:  widg,
				Type:   portfolio.SellTransaction,
				Qty:    20,
				Price:  40.0,
				Amount: 800.0,
			})

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tm.WinRate).To(Equal(0.0))
			Expect(tm.AverageWin).To(Equal(0.0))
			Expect(tm.ProfitFactor).To(Equal(0.0))
			Expect(tm.AverageLoss).To(Equal(-150.0))
			Expect(math.IsNaN(tm.GainLossRatio)).To(BeTrue())
		})
	})

	Describe("with a break-even trade", func() {
		It("counts break-even (PnL=0) as a loss", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			// Buy 10 ACME at $100, Sell at $100 -> PnL = 0
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: 1_000.0,
			})

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			// PnL=0 is not > 0, so it is counted as a loss
			Expect(tm.WinRate).To(Equal(0.0))
			Expect(tm.AverageWin).To(Equal(0.0))
			Expect(tm.AverageLoss).To(Equal(0.0))
			Expect(math.IsNaN(tm.ProfitFactor)).To(BeTrue())
			Expect(math.IsNaN(tm.GainLossRatio)).To(BeTrue())
		})
	})

	Describe("with multiple assets", func() {
		It("matches FIFO independently per asset", func() {
			a := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
			aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

			// Buy 10 SPY at $200
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  200.0,
				Amount: -2_000.0,
			})

			// Buy 20 AAPL at $150
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
				Asset:  aapl,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  150.0,
				Amount: -3_000.0,
			})

			// Sell 10 SPY at $220 -> PnL = 10*(220-200) = 200 (win), 30 days
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  220.0,
				Amount: 2_200.0,
			})

			// Sell 20 AAPL at $140 -> PnL = 20*(140-150) = -200 (loss), 56 days
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				Asset:  aapl,
				Type:   portfolio.SellTransaction,
				Qty:    20,
				Price:  140.0,
				Amount: 2_800.0,
			})

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tm.WinRate).To(BeNumerically("~", 0.5, 1e-4))
			Expect(tm.AverageWin).To(Equal(200.0))
			Expect(tm.AverageLoss).To(Equal(-200.0))
			Expect(tm.ProfitFactor).To(Equal(1.0))
			Expect(tm.GainLossRatio).To(Equal(1.0))
			// Average holding: SPY 30 days, AAPL 56 days => (30+56)/2 = 43
			Expect(tm.AverageHoldingPeriod).To(BeNumerically("~", 43.0, 1e-4))
		})
	})

	Describe("single winning trade", func() {
		It("sets WinRate=1.0 and +Inf ProfitFactor/GainLossRatio", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			// Buy 10 ACME at $100, Sell at $120 (31 days) -> win $200
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  120.0,
				Amount: 1_200.0,
			})

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tm.WinRate).To(Equal(1.0))
			Expect(tm.AverageWin).To(Equal(200.0))
			Expect(tm.AverageLoss).To(Equal(0.0))
			Expect(math.IsNaN(tm.ProfitFactor)).To(BeTrue())
			Expect(math.IsNaN(tm.GainLossRatio)).To(BeTrue())
		})
	})

	Describe("single losing trade", func() {
		It("sets WinRate=0 and zeroes ProfitFactor/GainLossRatio", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			// Buy 10 ACME at $100, Sell at $80 (31 days) -> loss -$200
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  80.0,
				Amount: 800.0,
			})

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tm.WinRate).To(Equal(0.0))
			Expect(tm.AverageWin).To(Equal(0.0))
			Expect(tm.AverageLoss).To(Equal(-200.0))
			Expect(tm.ProfitFactor).To(Equal(0.0))
			Expect(math.IsNaN(tm.GainLossRatio)).To(BeTrue())
		})
	})

	Describe("NPositivePeriods with flat equity curve", func() {
		It("returns NPositivePeriods=0 when all returns are zero", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			buildDF := func(t time.Time, price float64) *data.DataFrame {
				df, err := data.NewDataFrame(
					[]time.Time{t},
					[]asset.Asset{acme},
					[]data.Metric{data.MetricClose, data.AdjClose},
					data.Daily,
					[][]float64{{price}, {price}},
				)
				Expect(err).NotTo(HaveOccurred())
				return df
			}

			// Call UpdatePrices 5 times with flat price=100; all returns are zero
			a.UpdatePrices(buildDF(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 100.0))
			a.UpdatePrices(buildDF(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 100.0))
			a.UpdatePrices(buildDF(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), 100.0))
			a.UpdatePrices(buildDF(time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC), 100.0))
			a.UpdatePrices(buildDF(time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC), 100.0))

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tm.NPositivePeriods).To(Equal(0.0))
		})
	})

	Describe("FIFO matching with partial fills", func() {
		It("splits a buy lot across multiple sells", func() {
			a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			// Buy 20 at $100
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  100.0,
				Amount: -2_000.0,
			})

			// Sell 10 at $110 (win $100, 10 days)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  110.0,
				Amount: 1_100.0,
			})

			// Sell remaining 10 at $90 (loss -$100, 20 days)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 21, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  90.0,
				Amount: 900.0,
			})

			tm, err := a.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tm.WinRate).To(BeNumerically("~", 0.5, 1e-4))
			Expect(tm.AverageWin).To(BeNumerically("~", 100.0, 1e-4))
			Expect(tm.AverageLoss).To(BeNumerically("~", -100.0, 1e-4))
			Expect(tm.AverageHoldingPeriod).To(BeNumerically("~", 15.0, 1e-4))
		})
	})

	Describe("Long/Short metrics", func() {
		Describe("with a mix of long and short trades", func() {
			var tm portfolio.TradeMetrics

			BeforeEach(func() {
				acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

				// Long trade 1: Buy 10 ACME at $100, Sell at $120 -> win $200
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					Asset:  acme,
					Type:   portfolio.BuyTransaction,
					Qty:    10,
					Price:  100.0,
					Amount: -1_000.0,
				})
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
					Asset:  acme,
					Type:   portfolio.SellTransaction,
					Qty:    10,
					Price:  120.0,
					Amount: 1_200.0,
				})

				// Long trade 2: Buy 20 WIDG at $50, Sell at $45 -> loss -$100
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
					Asset:  widg,
					Type:   portfolio.BuyTransaction,
					Qty:    20,
					Price:  50.0,
					Amount: -1_000.0,
				})
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
					Asset:  widg,
					Type:   portfolio.SellTransaction,
					Qty:    20,
					Price:  45.0,
					Amount: 900.0,
				})

				// Short trade 1: Sell 50 ACME at $150 (short), Buy at $140 -> win $500
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
					Asset:  acme,
					Type:   portfolio.SellTransaction,
					Qty:    50,
					Price:  150.0,
					Amount: 7_500.0,
				})
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC),
					Asset:  acme,
					Type:   portfolio.BuyTransaction,
					Qty:    50,
					Price:  140.0,
					Amount: -7_000.0,
				})

				// Short trade 2: Sell 20 WIDG at $40 (short), Buy at $50 -> loss -$200
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
					Asset:  widg,
					Type:   portfolio.SellTransaction,
					Qty:    20,
					Price:  40.0,
					Amount: 800.0,
				})
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
					Asset:  widg,
					Type:   portfolio.BuyTransaction,
					Qty:    20,
					Price:  50.0,
					Amount: -1_000.0,
				})

				var err error
				tm, err = acct.TradeMetrics()
				Expect(err).NotTo(HaveOccurred())
			})

			It("computes ShortWinRate correctly", func() {
				// 1 winning short out of 2 short trades
				Expect(tm.ShortWinRate).To(BeNumerically("~", 0.5, 1e-4))
			})

			It("computes LongWinRate correctly (excludes short trades)", func() {
				// 1 winning long out of 2 long trades
				Expect(tm.LongWinRate).To(BeNumerically("~", 0.5, 1e-4))
			})

			It("computes ShortProfitFactor correctly", func() {
				// Short gross profit = 500, short gross loss = 200
				// ProfitFactor = 500 / 200 = 2.5
				Expect(tm.ShortProfitFactor).To(BeNumerically("~", 2.5, 1e-4))
			})

			It("computes LongProfitFactor correctly", func() {
				// Long gross profit = 200, long gross loss = 100
				// ProfitFactor = 200 / 100 = 2.0
				Expect(tm.LongProfitFactor).To(BeNumerically("~", 2.0, 1e-4))
			})
		})

		Describe("with no trades", func() {
			It("returns zero for directional win rates and NaN for profit factors", func() {
				acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
				tm, err := acct.TradeMetrics()
				Expect(err).NotTo(HaveOccurred())

				Expect(tm.LongWinRate).To(Equal(0.0))
				Expect(tm.ShortWinRate).To(Equal(0.0))
				Expect(math.IsNaN(tm.LongProfitFactor)).To(BeTrue())
				Expect(math.IsNaN(tm.ShortProfitFactor)).To(BeTrue())
			})
		})

		Describe("with only long trades", func() {
			It("returns zero for short metrics", func() {
				acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					Asset:  acme,
					Type:   portfolio.BuyTransaction,
					Qty:    10,
					Price:  100.0,
					Amount: -1_000.0,
				})
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
					Asset:  acme,
					Type:   portfolio.SellTransaction,
					Qty:    10,
					Price:  120.0,
					Amount: 1_200.0,
				})

				tm, err := acct.TradeMetrics()
				Expect(err).NotTo(HaveOccurred())

				Expect(tm.LongWinRate).To(Equal(1.0))
				Expect(tm.ShortWinRate).To(Equal(0.0))
				Expect(math.IsNaN(tm.LongProfitFactor)).To(BeTrue()) // no losses
				Expect(math.IsNaN(tm.ShortProfitFactor)).To(BeTrue()) // no short trades
			})
		})

		Describe("with only short trades", func() {
			It("returns zero for long metrics", func() {
				acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

				// Short: Sell 50 at $150, Buy at $140 -> win $500
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
					Asset:  acme,
					Type:   portfolio.SellTransaction,
					Qty:    50,
					Price:  150.0,
					Amount: 7_500.0,
				})
				acct.Record(portfolio.Transaction{
					Date:   time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
					Asset:  acme,
					Type:   portfolio.BuyTransaction,
					Qty:    50,
					Price:  140.0,
					Amount: -7_000.0,
				})

				tm, err := acct.TradeMetrics()
				Expect(err).NotTo(HaveOccurred())

				Expect(tm.LongWinRate).To(Equal(0.0))
				Expect(tm.ShortWinRate).To(Equal(1.0))
				Expect(math.IsNaN(tm.LongProfitFactor)).To(BeTrue()) // no long trades
				Expect(math.IsNaN(tm.ShortProfitFactor)).To(BeTrue()) // no losses
			})
		})
	})
})
