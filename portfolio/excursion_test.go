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

var _ = Describe("Clone", func() {
	var (
		acme asset.Asset
	)

	BeforeEach(func() {
		acme = asset.Asset{CompositeFigi: "ACME", Ticker: "ACME"}
	})

	It("deep-copies excursions and tradeDetails", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

		acct.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:  acme,
			Type:   portfolio.BuyTransaction,
			Qty:    10,
			Price:  100.0,
			Amount: -1_000.0,
		})

		df, err := data.NewDataFrame(
			[]time.Time{time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
			[]asset.Asset{acme},
			[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
			data.Daily,
			[]float64{110.0, 110.0, 115.0, 90.0},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdateExcursions(df)

		clone := acct.Clone()

		// Excursions should be independent copies
		cloneExc := clone.Excursions()
		Expect(cloneExc).To(HaveKey(acme))
		Expect(cloneExc[acme].HighPrice).To(Equal(115.0))

		// Mutating clone should not affect original
		df2, err := data.NewDataFrame(
			[]time.Time{time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)},
			[]asset.Asset{acme},
			[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
			data.Daily,
			[]float64{120.0, 120.0, 130.0, 85.0},
		)
		Expect(err).NotTo(HaveOccurred())
		clone.UpdateExcursions(df2)

		origExc := acct.Excursions()
		Expect(origExc[acme].HighPrice).To(Equal(115.0))
		Expect(clone.Excursions()[acme].HighPrice).To(Equal(130.0))
	})
})

var _ = Describe("WithPortfolioSnapshot", func() {
	var (
		acme asset.Asset
	)

	BeforeEach(func() {
		acme = asset.Asset{CompositeFigi: "ACME", Ticker: "ACME"}
	})

	It("restores excursions and tradeDetails from snapshot", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

		acct.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:  acme,
			Type:   portfolio.BuyTransaction,
			Qty:    10,
			Price:  100.0,
			Amount: -1_000.0,
		})

		df, err := data.NewDataFrame(
			[]time.Time{time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
			[]asset.Asset{acme},
			[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
			data.Daily,
			[]float64{110.0, 110.0, 115.0, 90.0},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdateExcursions(df)

		// Create new account from snapshot
		restored := portfolio.New(portfolio.WithPortfolioSnapshot(acct))

		exc := restored.Excursions()
		Expect(exc).To(HaveKey(acme))
		Expect(exc[acme].HighPrice).To(Equal(115.0))
		Expect(exc[acme].LowPrice).To(Equal(90.0))

		// Also verify trade details round-trip
		details := restored.TradeDetails()
		Expect(details).To(BeEmpty()) // no sells yet, so no trade details
	})

	It("restores tradeDetails from snapshot", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

		acct.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:  acme,
			Type:   portfolio.BuyTransaction,
			Qty:    10,
			Price:  100.0,
			Amount: -1_000.0,
		})

		df, err := data.NewDataFrame(
			[]time.Time{time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
			[]asset.Asset{acme},
			[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
			data.Daily,
			[]float64{110.0, 110.0, 115.0, 90.0},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdateExcursions(df)

		// Close position to generate a TradeDetail
		acct.Record(portfolio.Transaction{
			Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			Asset:  acme,
			Type:   portfolio.SellTransaction,
			Qty:    10,
			Price:  110.0,
			Amount: 1_100.0,
		})

		Expect(acct.TradeDetails()).To(HaveLen(1))

		restored := portfolio.New(portfolio.WithPortfolioSnapshot(acct))

		restoredDetails := restored.TradeDetails()
		Expect(restoredDetails).To(HaveLen(1))
		Expect(restoredDetails[0].Asset).To(Equal(acme))
		Expect(restoredDetails[0].MFE).To(BeNumerically("~", 0.15, 1e-9))
		Expect(restoredDetails[0].MAE).To(BeNumerically("~", -0.10, 1e-9))
	})
})

var _ = Describe("ExcursionRecord", func() {
	var (
		acme asset.Asset
	)

	BeforeEach(func() {
		acme = asset.Asset{CompositeFigi: "ACME", Ticker: "ACME"}
	})

	Describe("initialization on buy", func() {
		It("creates an excursion record when a position is opened", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			exc := acct.Excursions()
			Expect(exc).To(HaveKey(acme))
			Expect(exc[acme].EntryPrice).To(Equal(100.0))
			Expect(exc[acme].HighPrice).To(Equal(100.0))
			Expect(exc[acme].LowPrice).To(Equal(100.0))
		})
	})

	Describe("position adds", func() {
		It("keeps existing record when adding to a position", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    5,
				Price:  110.0,
				Amount: -550.0,
			})

			exc := acct.Excursions()
			Expect(exc[acme].EntryPrice).To(Equal(100.0))
		})
	})

	Describe("UpdateExcursions", func() {
		It("updates running high and low from DataFrame", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			// Day 2: High=108, Low=95
			df, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{acme},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily,
				[]float64{102.0, 102.0, 108.0, 95.0},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df)

			exc := acct.Excursions()
			Expect(exc[acme].HighPrice).To(Equal(108.0))
			Expect(exc[acme].LowPrice).To(Equal(95.0))
		})

		It("accumulates extremes across multiple days", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			// Day 2: High=105, Low=98
			df1, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{acme},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily,
				[]float64{102.0, 102.0, 105.0, 98.0},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df1)

			// Day 3: High=110, Low=99 (new high, but low is above previous low)
			df2, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{acme},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily,
				[]float64{107.0, 107.0, 110.0, 99.0},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df2)

			exc := acct.Excursions()
			Expect(exc[acme].HighPrice).To(Equal(110.0))
			Expect(exc[acme].LowPrice).To(Equal(98.0)) // from day 2
		})

		It("skips update when high or low is NaN", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			// Day 2: High=108, Low=95 (establishes extremes)
			df1, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{acme},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily,
				[]float64{102.0, 102.0, 108.0, 95.0},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df1)

			// Day 3: NaN high and low (missing data)
			df2, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{acme},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily,
				[]float64{102.0, 102.0, math.NaN(), math.NaN()},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df2)

			exc := acct.Excursions()
			Expect(exc[acme].HighPrice).To(Equal(108.0))
			Expect(exc[acme].LowPrice).To(Equal(95.0))
		})
	})

	Describe("TradeDetail", func() {
		It("produces a TradeDetail on full position close", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			// Simulate price movement: High=115, Low=90
			df, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{acme},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily,
				[]float64{110.0, 110.0, 115.0, 90.0},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df)

			// Close position
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  110.0,
				Amount: 1_100.0,
			})

			details := acct.TradeDetails()
			Expect(details).To(HaveLen(1))

			td := details[0]
			Expect(td.Asset).To(Equal(acme))
			Expect(td.EntryDate).To(Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)))
			Expect(td.ExitDate).To(Equal(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)))
			Expect(td.EntryPrice).To(Equal(100.0))
			Expect(td.ExitPrice).To(Equal(110.0))
			Expect(td.Qty).To(Equal(10.0))
			Expect(td.PnL).To(Equal(100.0))         // (110-100)*10
			Expect(td.HoldDays).To(Equal(31.0))
			Expect(td.MFE).To(BeNumerically("~", 0.15, 1e-9))  // (115-100)/100
			Expect(td.MAE).To(BeNumerically("~", -0.10, 1e-9)) // (90-100)/100
		})

		It("produces TradeDetail on partial close and keeps excursion record", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  100.0,
				Amount: -2_000.0,
			})

			df, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{acme},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily,
				[]float64{110.0, 110.0, 112.0, 95.0},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df)

			// Partial sell
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  110.0,
				Amount: 1_100.0,
			})

			details := acct.TradeDetails()
			Expect(details).To(HaveLen(1))
			Expect(details[0].Qty).To(Equal(10.0))
			Expect(details[0].MFE).To(BeNumerically("~", 0.12, 1e-9))  // (112-100)/100
			Expect(details[0].MAE).To(BeNumerically("~", -0.05, 1e-9)) // (95-100)/100

			// Excursion record still exists
			exc := acct.Excursions()
			Expect(exc).To(HaveKey(acme))
		})

		It("produces MFE=0 MAE=0 for same-day open/close", func() {
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
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  102.0,
				Amount: 1_020.0,
			})

			details := acct.TradeDetails()
			Expect(details).To(HaveLen(1))
			Expect(details[0].MFE).To(Equal(0.0))
			Expect(details[0].MAE).To(Equal(0.0))
		})
	})

	Describe("MFE/MAE summary metrics", func() {
		var acct *portfolio.Account

		BeforeEach(func() {
			acme := asset.Asset{CompositeFigi: "ACME", Ticker: "ACME"}
			widg := asset.Asset{CompositeFigi: "WIDG", Ticker: "WIDG"}
			acct = portfolio.New(portfolio.WithCash(50_000, time.Time{}))

			// Trade 1: ACME buy@100, high=115, low=90, sell@110
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})
			df1, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{acme},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily, []float64{105.0, 105.0, 115.0, 90.0},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df1)
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  110.0,
				Amount: 1_100.0,
			})

			// Trade 2: WIDG buy@50, high=55, low=42, sell@45
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				Asset:  widg,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  50.0,
				Amount: -1_000.0,
			})
			df2, err := data.NewDataFrame(
				[]time.Time{time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
				[]asset.Asset{widg},
				[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow},
				data.Daily, []float64{48.0, 48.0, 55.0, 42.0},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdateExcursions(df2)
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
				Asset:  widg,
				Type:   portfolio.SellTransaction,
				Qty:    20,
				Price:  45.0,
				Amount: 900.0,
			})
		})

		// Trade 1: MFE = (115-100)/100 = 0.15, MAE = (90-100)/100 = -0.10
		// Trade 2: MFE = (55-50)/50 = 0.10, MAE = (42-50)/50 = -0.16

		It("computes AverageMFE", func() {
			val, err := acct.PerformanceMetric(portfolio.AverageMFE).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", 0.125, 1e-9))
		})

		It("computes AverageMAE", func() {
			val, err := acct.PerformanceMetric(portfolio.AverageMAE).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", -0.13, 1e-9))
		})
	})

	Describe("with no trades", func() {
		It("returns zero for averages", func() {
			emptyAcct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			tradeMetrics, err := emptyAcct.TradeMetrics()
			Expect(err).NotTo(HaveOccurred())
			Expect(tradeMetrics.AverageMFE).To(Equal(0.0))
			Expect(tradeMetrics.AverageMAE).To(Equal(0.0))
		})
	})

	Describe("position close", func() {
		It("removes excursion record when position is fully closed", func() {
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
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  120.0,
				Amount: 1_200.0,
			})

			exc := acct.Excursions()
			Expect(exc).NotTo(HaveKey(acme))
		})

		It("keeps excursion record on partial close", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  100.0,
				Amount: -2_000.0,
			})

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  120.0,
				Amount: 1_200.0,
			})

			exc := acct.Excursions()
			Expect(exc).To(HaveKey(acme))
			Expect(exc[acme].EntryPrice).To(Equal(100.0))
		})
	})
})
