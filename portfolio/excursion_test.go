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
