package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("TaxMetrics", func() {
	var (
		spy asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	})

	Describe("realized gains", func() {
		It("computes STCG for positions held less than one year", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 shares at $100
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Sell 50 shares at $120 after < 1 year => STCG = 50 * (120 - 100) = 1000
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  120.0,
				Amount: 6_000.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(1_000.0))
			Expect(tm.LTCG).To(Equal(0.0))
		})

		It("computes LTCG for positions held more than one year", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 shares at $100 on Jan 1, 2023
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Sell 50 shares at $130 on Feb 1, 2024 (> 1 year) => LTCG = 50 * (130 - 100) = 1500
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  130.0,
				Amount: 6_500.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.LTCG).To(Equal(1_500.0))
			Expect(tm.STCG).To(Equal(0.0))
		})

		It("splits gains between STCG and LTCG for mixed holding periods", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 shares at $100 on Jan 1, 2023
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Sell 50 at $120 on Jun 1, 2023 (STCG: 50 * 20 = 1000)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  120.0,
				Amount: 6_000.0,
			})

			// Sell 50 at $130 on Feb 1, 2024 (LTCG: 50 * 30 = 1500)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  130.0,
				Amount: 6_500.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(1_000.0))
			Expect(tm.LTCG).To(Equal(1_500.0))
		})

		It("handles capital losses", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Sell at $80 for a loss => STCG = 100 * (80 - 100) = -2000
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    100,
				Price:  80.0,
				Amount: 8_000.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(-2_000.0))
		})
	})

	Describe("dividends", func() {
		It("sums qualified dividends", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.DividendTransaction,
				Amount: 200.0,
			})

			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.DividendTransaction,
				Amount: 150.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.QualifiedDividends).To(Equal(350.0))
		})
	})

	Describe("unrealized gains", func() {
		It("computes unrealized STCG for positions held less than one year", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			buyDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
			a.Record(portfolio.Transaction{
				Date:   buyDate,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Current price is $110, held < 1 year => unrealized STCG = 100 * (110 - 100) = 1000
			now := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
			df := buildDF(now, []asset.Asset{spy}, []float64{110.0}, []float64{110.0})
			a.UpdatePrices(df)

			tm := a.TaxMetrics()
			Expect(tm.UnrealizedSTCG).To(Equal(1_000.0))
			Expect(tm.UnrealizedLTCG).To(Equal(0.0))
		})

		It("computes unrealized LTCG for positions held more than one year", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			buyDate := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
			a.Record(portfolio.Transaction{
				Date:   buyDate,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Current price is $130, held > 1 year => unrealized LTCG = 100 * (130 - 100) = 3000
			now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
			df := buildDF(now, []asset.Asset{spy}, []float64{130.0}, []float64{130.0})
			a.UpdatePrices(df)

			tm := a.TaxMetrics()
			Expect(tm.UnrealizedLTCG).To(Equal(3_000.0))
			Expect(tm.UnrealizedSTCG).To(Equal(0.0))
		})
	})

	Describe("TaxCostRatio", func() {
		It("computes tax cost ratio from estimated taxes and total gain", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 at $100
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Update prices to establish initial equity
			df1 := buildDF(
				time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
			)
			a.UpdatePrices(df1)

			// Sell 50 at $120 (STCG = 1000)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  120.0,
				Amount: 6_000.0,
			})

			// Sell remaining 50 at $130 after > 1 year (LTCG = 1500)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  130.0,
				Amount: 6_500.0,
			})

			// Dividend of $200
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.DividendTransaction,
				Amount: 200.0,
			})

			// Update prices to establish final equity (no holdings left)
			df2 := buildDF(
				time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				[]asset.Asset{spy}, []float64{130.0}, []float64{130.0},
			)
			a.UpdatePrices(df2)

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(1_000.0))
			Expect(tm.LTCG).To(Equal(1_500.0))
			Expect(tm.QualifiedDividends).To(Equal(200.0))

			// Estimated tax = 0.25*1000 + 0.15*1500 + 0.15*200 = 250 + 225 + 30 = 505
			// Total gain = final equity - initial equity = 52700 - 50000 = 2700
			// TaxCostRatio = 505 / 2700 ~ 0.18703703...
			Expect(tm.TaxCostRatio).To(BeNumerically("~", 505.0/2700.0, 1e-9))
		})

		It("returns zero TaxCostRatio when there is no gain", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy and sell at a loss
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			df1 := buildDF(
				time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
			)
			a.UpdatePrices(df1)

			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    100,
				Price:  80.0,
				Amount: 8_000.0,
			})

			df2 := buildDF(
				time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				[]asset.Asset{spy}, []float64{80.0}, []float64{80.0},
			)
			a.UpdatePrices(df2)

			tm := a.TaxMetrics()
			Expect(tm.TaxCostRatio).To(Equal(0.0))
		})
	})

	Describe("multiple assets", func() {
		It("tracks FIFO gains independently per asset", func() {
			a := portfolio.New(portfolio.WithCash(100_000))
			spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
			aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

			// Buy 50 SPY at $200
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    50,
				Price:  200.0,
				Amount: -10_000.0,
			})

			// Buy 30 AAPL at $150
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  aapl,
				Type:   portfolio.BuyTransaction,
				Qty:    30,
				Price:  150.0,
				Amount: -4_500.0,
			})

			// Sell 50 SPY at $220 (STCG = 50*(220-200) = 1000)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  220.0,
				Amount: 11_000.0,
			})

			// Sell 30 AAPL at $170 (STCG = 30*(170-150) = 600)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 8, 1, 0, 0, 0, 0, time.UTC),
				Asset:  aapl,
				Type:   portfolio.SellTransaction,
				Qty:    30,
				Price:  170.0,
				Amount: 5_100.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(1_600.0))
			Expect(tm.LTCG).To(Equal(0.0))
		})
	})

	Describe("partial lot consumption", func() {
		It("consumes first lot fully and second lot partially via FIFO", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 shares at $10 on day 1
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  10.0,
				Amount: -1_000.0,
			})

			// Buy 50 shares at $12 on day 2
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 3, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    50,
				Price:  12.0,
				Amount: -600.0,
			})

			// Sell 120 shares at $15 on day 3
			// FIFO: 100 shares from lot 1 => gain = 100*(15-10) = 500
			//        20 shares from lot 2 => gain =  20*(15-12) =  60
			// Total STCG = 560
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 3, 3, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    120,
				Price:  15.0,
				Amount: 1_800.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(560.0))
			Expect(tm.LTCG).To(Equal(0.0))
		})
	})

	Describe("capital losses and TaxCostRatio", func() {
		It("records negative STCG and TaxCostRatio is zero when STCG is negative", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 shares at $100
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Establish initial equity
			df1 := buildDF(
				time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
			)
			a.UpdatePrices(df1)

			// Sell 100 at $80 => STCG = 100*(80-100) = -2000
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    100,
				Price:  80.0,
				Amount: 8_000.0,
			})

			df2 := buildDF(
				time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				[]asset.Asset{spy}, []float64{80.0}, []float64{80.0},
			)
			a.UpdatePrices(df2)

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(-2_000.0))
			// totalGain is negative (48000 - 50000 = -2000), so TaxCostRatio stays 0.
			Expect(tm.TaxCostRatio).To(Equal(0.0))
		})
	})

	Describe("edge cases", func() {
		It("treats exactly 365 days as STCG (boundary: > 365 required for LTCG)", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 shares at $100 on Jan 1, 2023
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Sell 100 shares at $120 on Jan 1, 2024 (exactly 365 days) => STCG = 100*(120-100) = 2000
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    100,
				Price:  120.0,
				Amount: 12_000.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(2_000.0))
			Expect(tm.LTCG).To(Equal(0.0))
		})

		It("treats 366 days as LTCG", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 shares at $100 on Jan 1, 2023
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Sell 100 shares at $120 on Jan 2, 2024 (366 days) => LTCG = 100*(120-100) = 2000
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    100,
				Price:  120.0,
				Amount: 12_000.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.LTCG).To(Equal(2_000.0))
			Expect(tm.STCG).To(Equal(0.0))
		})

		It("treats buy and sell on same date as STCG", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 50 shares at $100 and sell same day at $105 => STCG = 50*(105-100) = 250
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    50,
				Price:  100.0,
				Amount: -5_000.0,
			})

			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  105.0,
				Amount: 5_250.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(250.0))
			Expect(tm.LTCG).To(Equal(0.0))
		})

		It("returns zero TaxMetrics when there are no transactions", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(0.0))
			Expect(tm.LTCG).To(Equal(0.0))
			Expect(tm.QualifiedDividends).To(Equal(0.0))
			Expect(tm.UnrealizedSTCG).To(Equal(0.0))
			Expect(tm.UnrealizedLTCG).To(Equal(0.0))
			Expect(tm.TaxCostRatio).To(Equal(0.0))
		})

		It("tracks FIFO per asset with interleaved buys and sells", func() {
			a := portfolio.New(portfolio.WithCash(100_000))
			aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

			// Buy 50 SPY @$100 on Jan 1
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    50,
				Price:  100.0,
				Amount: -5_000.0,
			})

			// Buy 30 AAPL @$150 on Feb 1
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  aapl,
				Type:   portfolio.BuyTransaction,
				Qty:    30,
				Price:  150.0,
				Amount: -4_500.0,
			})

			// Sell 50 SPY @$120 on May 1 => STCG = 50*(120-100) = 1000
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  120.0,
				Amount: 6_000.0,
			})

			// Buy 20 AAPL @$160 on Jun 1
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  aapl,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  160.0,
				Amount: -3_200.0,
			})

			// Sell 50 AAPL @$170 on Aug 1
			// FIFO: 30 from first lot => 30*(170-150) = 600
			//        20 from second lot => 20*(170-160) = 200
			// Total AAPL STCG = 800
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC),
				Asset:  aapl,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  170.0,
				Amount: 8_500.0,
			})

			// Total STCG = 1000 + 800 = 1800
			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(1_800.0))
			Expect(tm.LTCG).To(Equal(0.0))
		})
	})

	Describe("complete scenario", func() {
		It("computes all tax metrics correctly", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy 100 shares at $100 on Jan 1, 2023
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Sell 50 at $120 on Jun 1, 2023 (STCG = 1000)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  120.0,
				Amount: 6_000.0,
			})

			// Sell 50 at $130 on Feb 1, 2024 (LTCG = 1500)
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  130.0,
				Amount: 6_500.0,
			})

			// Dividend of $200 on Mar 1, 2024
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.DividendTransaction,
				Amount: 200.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(1_000.0))
			Expect(tm.LTCG).To(Equal(1_500.0))
			Expect(tm.QualifiedDividends).To(Equal(200.0))
		})
	})
})
