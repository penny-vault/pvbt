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
			// Deficiency is the notional that must be covered to restore
			// SMV*rate <= equity: SMV - equity/rate
			//                   = 15_000 - 5_000/0.90 = 9_444.44
			acct := portfolio.New(
				portfolio.WithCash(20_000, now),
				portfolio.WithMaintenanceMargin(0.90),
				portfolio.WithMaxLeverage(10.0), // disable the leverage check for this test
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

			Expect(acct.MarginDeficiency()).To(BeNumerically("~", 9_444.44, 0.01))
		})
	})

	Describe("GrossLeverage", func() {
		It("returns 0 when there are no positions", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, now))
			df := buildDF(now, []asset.Asset{spy}, []float64{450}, []float64{450})
			acct.UpdatePrices(df)

			Expect(acct.GrossLeverage()).To(Equal(0.0))
		})

		It("computes (LMV+SMV)/equity for mixed positions", func() {
			// Long 100 SPY at 100 (LMV=10_000), short 50 AAPL at 200 (SMV=10_000).
			// cash = 50_000 - 10_000 + 10_000 = 50_000.
			// equity = cash + LMV - SMV = 50_000 + 10_000 - 10_000 = 50_000.
			// gross/equity = 20_000/50_000 = 0.4.
			acct := portfolio.New(
				portfolio.WithCash(50_000, now),
				portfolio.WithMaxLeverage(2.0),
			)
			acct.Record(portfolio.Transaction{Date: now, Asset: spy, Type: asset.BuyTransaction, Qty: 100, Price: 100, Amount: -10_000})
			acct.Record(portfolio.Transaction{Date: now, Asset: aapl, Type: asset.SellTransaction, Qty: 50, Price: 200, Amount: 10_000})

			df := buildDF(now,
				[]asset.Asset{spy, aapl},
				[]float64{100, 200},
				[]float64{100, 200},
			)
			acct.UpdatePrices(df)

			Expect(acct.GrossLeverage()).To(BeNumerically("~", 0.4, 0.001))
		})

		It("returns NaN when equity is non-positive while positions exist", func() {
			// Cash 5_000, short 100 at 60 => SMV=6_000, equity=5_000-6_000=-1_000.
			acct := portfolio.New(
				portfolio.WithCash(5_000, now),
				portfolio.WithMaxLeverage(10.0),
			)
			acct.Record(portfolio.Transaction{Date: now, Asset: spy, Type: asset.SellTransaction, Qty: 100, Price: 60})

			df := buildDF(now, []asset.Asset{spy}, []float64{60}, []float64{60})
			acct.UpdatePrices(df)

			Expect(math.IsNaN(acct.GrossLeverage())).To(BeTrue())
		})
	})

	Describe("MaxLeverage", func() {
		It("returns the default 1.0 when not configured", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, now))
			Expect(acct.MaxLeverage()).To(Equal(1.0))
		})

		It("returns the configured cap", func() {
			acct := portfolio.New(
				portfolio.WithCash(100_000, now),
				portfolio.WithMaxLeverage(2.5),
			)
			Expect(acct.MaxLeverage()).To(Equal(2.5))
		})
	})

	Describe("LeverageHeadroom", func() {
		It("returns max*equity when no positions are open", func() {
			acct := portfolio.New(
				portfolio.WithCash(100_000, now),
				portfolio.WithMaxLeverage(2.0),
			)
			Expect(acct.LeverageHeadroom()).To(BeNumerically("~", 200_000.0, 0.01))
		})

		It("subtracts current gross from the cap", func() {
			// Long 100 SPY at 100 => LMV=10_000, cash=90_000, equity=100_000.
			// headroom = 2*100_000 - 10_000 = 190_000.
			acct := portfolio.New(
				portfolio.WithCash(100_000, now),
				portfolio.WithMaxLeverage(2.0),
			)
			acct.Record(portfolio.Transaction{Date: now, Asset: spy, Type: asset.BuyTransaction, Qty: 100, Price: 100, Amount: -10_000})

			df := buildDF(now, []asset.Asset{spy}, []float64{100}, []float64{100})
			acct.UpdatePrices(df)

			Expect(acct.LeverageHeadroom()).To(BeNumerically("~", 190_000.0, 0.01))
		})

		It("is negative when the account is over the cap", func() {
			// Cash 5_000 + buy 100 at 100 => cash=-5_000, LMV=10_000, equity=5_000.
			// headroom = 1*5_000 - 10_000 = -5_000.
			acct := portfolio.New(portfolio.WithCash(5_000, now))
			acct.Record(portfolio.Transaction{Date: now, Asset: spy, Type: asset.BuyTransaction, Qty: 100, Price: 100, Amount: -10_000})

			df := buildDF(now, []asset.Asset{spy}, []float64{100}, []float64{100})
			acct.UpdatePrices(df)

			Expect(acct.LeverageHeadroom()).To(BeNumerically("~", -5_000.0, 0.01))
		})
	})

	Describe("MarginDeficiency leverage breach", func() {
		It("returns the gross excess when (LMV+SMV)/equity exceeds the cap", func() {
			// Cash 5_000 + buy 100 at 100 => cash=-5_000, LMV=10_000, equity=5_000.
			// gross/equity=2.0; with default cap=1.0, deficiency=10_000 - 1*5_000=5_000.
			acct := portfolio.New(portfolio.WithCash(5_000, now))
			acct.Record(portfolio.Transaction{Date: now, Asset: spy, Type: asset.BuyTransaction, Qty: 100, Price: 100, Amount: -10_000})

			df := buildDF(now, []asset.Asset{spy}, []float64{100}, []float64{100})
			acct.UpdatePrices(df)

			Expect(acct.MarginDeficiency()).To(BeNumerically("~", 5_000.0, 0.01))
		})

		It("treats gross as fully deficient when equity is non-positive", func() {
			// Cash 5_000 + short 100 at 60 => SMV=6_000, equity=-1_000.
			acct := portfolio.New(portfolio.WithCash(5_000, now))
			acct.Record(portfolio.Transaction{Date: now, Asset: spy, Type: asset.SellTransaction, Qty: 100, Price: 60})

			df := buildDF(now, []asset.Asset{spy}, []float64{60}, []float64{60})
			acct.UpdatePrices(df)

			// With negative equity, the leverage path returns the full gross.
			Expect(acct.MarginDeficiency()).To(BeNumerically(">=", 6_000.0))
		})
	})
})
