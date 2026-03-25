package portfolio_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Margin Accounting", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		now = time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	})

	Describe("MarginRatio", func() {
		It("returns NaN when there are no short positions", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, now))
			df := buildDF(now, []asset.Asset{spy}, []float64{450}, []float64{450})
			acct.UpdatePrices(df)

			Expect(math.IsNaN(acct.MarginRatio())).To(BeTrue())
		})

		It("computes the correct margin ratio with a short position", func() {
			// Short 100 shares at $150. Cash received = 100*150 = 15000.
			// Starting cash = 100_000, after short sale cash = 115_000.
			// SMV = 100 * 150 = 15_000
			// Equity = cash + LMV - SMV = 115_000 + 0 - 15_000 = 100_000
			// Ratio = equity / SMV = 100_000 / 15_000 = 6.6667
			acct := portfolio.New(portfolio.WithCash(115_000, now))
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    100,
				Price:  150,
				Amount: 0, // cash already accounted for
			})

			df := buildDF(now, []asset.Asset{spy}, []float64{150}, []float64{150})
			acct.UpdatePrices(df)

			Expect(acct.MarginRatio()).To(BeNumerically("~", 100_000.0/15_000.0, 0.001))
		})
	})

	Describe("ShortMarketValue", func() {
		It("returns 0 when there are no short positions", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, now))
			df := buildDF(now, []asset.Asset{spy}, []float64{450}, []float64{450})
			acct.UpdatePrices(df)

			Expect(acct.ShortMarketValue()).To(Equal(0.0))
		})

		It("computes the absolute market value of short positions", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, now))
			// Manually set a short position of -50 shares.
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    50,
				Price:  200,
				Amount: 0,
			})

			df := buildDF(now, []asset.Asset{spy}, []float64{200}, []float64{200})
			acct.UpdatePrices(df)

			Expect(acct.ShortMarketValue()).To(BeNumerically("~", 10_000.0, 0.01))
		})

		It("returns 0 when prices are nil", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, now))
			Expect(acct.ShortMarketValue()).To(Equal(0.0))
		})
	})

	Describe("LongMarketValue", func() {
		It("computes the market value of long positions", func() {
			acct := portfolio.New(portfolio.WithCash(0, now))
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  300,
				Amount: -3000,
			})

			df := buildDF(now, []asset.Asset{spy}, []float64{300}, []float64{300})
			acct.UpdatePrices(df)

			Expect(acct.LongMarketValue()).To(BeNumerically("~", 3_000.0, 0.01))
		})

		It("excludes short positions from long market value", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, now))
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    20,
				Price:  250,
				Amount: 0,
			})

			df := buildDF(now, []asset.Asset{spy}, []float64{250}, []float64{250})
			acct.UpdatePrices(df)

			Expect(acct.LongMarketValue()).To(Equal(0.0))
		})
	})

	Describe("Equity", func() {
		It("computes equity with mixed long and short positions", func() {
			// Start with 50_000 cash.
			// Buy 10 AAPL at 200: Amount = -2000, cash becomes 48_000.
			// Short sell 20 SPY at 150: Amount = +3000 (proceeds), cash becomes 51_000.
			// LMV = 10 * 200 = 2_000
			// SMV = 20 * 150 = 3_000
			// Equity = cash + LMV - SMV = 51_000 + 2_000 - 3_000 = 50_000
			acct := portfolio.New(portfolio.WithCash(50_000, now))
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  aapl,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  200,
				Amount: -2000,
			})
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    20,
				Price:  150,
				Amount: 3000,
			})

			df := buildDF(now, []asset.Asset{aapl, spy}, []float64{200, 150}, []float64{200, 150})
			acct.UpdatePrices(df)

			Expect(acct.Equity()).To(BeNumerically("~", 50_000.0, 0.01))
		})
	})

	Describe("MarginDeficiency", func() {
		It("returns 0 when the account is healthy", func() {
			// Cash 115_000, short 100 shares at 150.
			// SMV = 15_000, Equity = 100_000
			// Required = SMV * (1 + 0.30) = 19_500
			// Equity 100_000 >> 19_500 => deficiency = 0
			acct := portfolio.New(portfolio.WithCash(115_000, now))
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    100,
				Price:  150,
				Amount: 0,
			})

			df := buildDF(now, []asset.Asset{spy}, []float64{150}, []float64{150})
			acct.UpdatePrices(df)

			Expect(acct.MarginDeficiency()).To(Equal(0.0))
		})

		It("returns 0 when there are no short positions", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, now))
			df := buildDF(now, []asset.Asset{spy}, []float64{450}, []float64{450})
			acct.UpdatePrices(df)

			Expect(acct.MarginDeficiency()).To(Equal(0.0))
		})
	})

	Describe("BuyingPower", func() {
		It("computes buying power as cash minus initial margin on shorts", func() {
			// Cash = 115_000, short 100 at 150 => SMV = 15_000
			// BuyingPower = 115_000 - 15_000 * 0.50 = 107_500
			acct := portfolio.New(portfolio.WithCash(115_000, now))
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    100,
				Price:  150,
				Amount: 0,
			})

			df := buildDF(now, []asset.Asset{spy}, []float64{150}, []float64{150})
			acct.UpdatePrices(df)

			Expect(acct.BuyingPower()).To(BeNumerically("~", 107_500.0, 0.01))
		})
	})

	Describe("Configurable rates", func() {
		It("uses custom initial margin rate", func() {
			acct := portfolio.New(
				portfolio.WithCash(115_000, now),
				portfolio.WithInitialMargin(0.60),
			)
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    100,
				Price:  150,
				Amount: 0,
			})

			df := buildDF(now, []asset.Asset{spy}, []float64{150}, []float64{150})
			acct.UpdatePrices(df)

			// BuyingPower = 115_000 - 15_000 * 0.60 = 106_000
			Expect(acct.BuyingPower()).To(BeNumerically("~", 106_000.0, 0.01))
		})

		It("uses custom maintenance margin rate for deficiency", func() {
			// Use a very high maintenance margin to trigger a deficiency.
			// Cash = 20_000, short 100 at 150 => SMV = 15_000
			// Equity = 20_000 - 15_000 = 5_000
			// Required = 15_000 * 0.90 = 13_500
			// Deficiency = 13_500 - 5_000 = 8_500
			acct := portfolio.New(
				portfolio.WithCash(20_000, now),
				portfolio.WithMaintenanceMargin(0.90),
			)
			acct.Record(portfolio.Transaction{
				Date:   now,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    100,
				Price:  150,
				Amount: 0,
			})

			df := buildDF(now, []asset.Asset{spy}, []float64{150}, []float64{150})
			acct.UpdatePrices(df)

			Expect(acct.MarginDeficiency()).To(BeNumerically("~", 8_500.0, 0.01))
		})
	})
})
