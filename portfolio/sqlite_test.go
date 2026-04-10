package portfolio_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	_ "modernc.org/sqlite"
)

var _ = Describe("SQLite", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "pvbt-sqlite-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	Describe("round-trip", func() {
		It("writes and reads back a populated account", func() {
			spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}

			acct := portfolio.New(
				portfolio.WithCash(10000, time.Time{}),
				portfolio.WithBenchmark(spy),
			)

			acct.SetMetadata("strategy", "momentum")
			acct.SetMetadata("run_id", "abc-123")

			// Record a buy transaction.
			t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			acct.Record(portfolio.Transaction{
				Date:   t0,
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    10,
				Price:  450.0,
				Amount: -4500.0,
			})

			// Update prices to build equity curve.
			acct.SetRiskFreeValue(91.50)
			df0 := buildDF(t0,
				[]asset.Asset{spy},
				[]float64{450.0},
				[]float64{450.0},
			)
			acct.UpdatePrices(df0)

			t1 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
			acct.SetRiskFreeValue(91.55)
			df1 := buildDF(t1,
				[]asset.Asset{spy},
				[]float64{455.0},
				[]float64{455.0},
			)
			acct.UpdatePrices(df1)

			// Record a dividend.
			t2 := time.Date(2025, 4, 15, 0, 0, 0, 0, time.UTC)
			acct.Record(portfolio.Transaction{
				Date:   t2,
				Asset:  spy,
				Type:   asset.DividendTransaction,
				Qty:    0,
				Price:  0,
				Amount: 15.0,
			})

			// Append a metric row.
			acct.AppendMetric(portfolio.MetricRow{
				Date:   t1,
				Name:   "sharpe",
				Window: "since_inception",
				Value:  1.5,
			})
			acct.AppendMetric(portfolio.MetricRow{
				Date:   t1,
				Name:   "max_drawdown",
				Window: "1yr",
				Value:  -0.05,
			})

			// Write to SQLite.
			dbPath := filepath.Join(tmpDir, "test.db")
			err := acct.ToSQLite(dbPath)
			Expect(err).NotTo(HaveOccurred())

			// Read back.
			restored, err := portfolio.FromSQLite(dbPath)
			Expect(err).NotTo(HaveOccurred())

			// Verify cash.
			Expect(restored.Cash()).To(Equal(acct.Cash()))

			// Verify perf data.
			origPD := acct.PerfData()
			resPD := restored.PerfData()
			Expect(resPD).NotTo(BeNil())
			perfA := asset.Asset{CompositeFigi: "_PORTFOLIO_", Ticker: "_PORTFOLIO_"}
			Expect(resPD.Column(perfA, data.PortfolioEquity)).To(Equal(origPD.Column(perfA, data.PortfolioEquity)))
			Expect(resPD.Times()).To(HaveLen(len(origPD.Times())))
			for i, t := range resPD.Times() {
				Expect(t.Equal(origPD.Times()[i])).To(BeTrue(),
					"perf data time mismatch at index %d", i)
			}

			// Verify transactions.
			origTxns := acct.Transactions()
			resTxns := restored.Transactions()
			Expect(resTxns).To(HaveLen(len(origTxns)))
			for i, tx := range resTxns {
				Expect(tx.Type).To(Equal(origTxns[i].Type))
				Expect(tx.Amount).To(Equal(origTxns[i].Amount))
				Expect(tx.Qty).To(Equal(origTxns[i].Qty))
				Expect(tx.Price).To(Equal(origTxns[i].Price))
				Expect(tx.Asset).To(Equal(origTxns[i].Asset))
			}

			// Verify holdings.
			origHoldings := acct.Holdings()
			resHoldings := restored.Holdings()
			Expect(resHoldings).To(Equal(origHoldings))

			// Verify tax lots.
			origLots := acct.TaxLots()
			resLots := restored.TaxLots()
			Expect(resLots).To(HaveLen(len(origLots)))
			for ast, lots := range origLots {
				Expect(resLots[ast]).To(HaveLen(len(lots)))
				for j, lot := range lots {
					Expect(resLots[ast][j].Qty).To(Equal(lot.Qty))
					Expect(resLots[ast][j].Price).To(Equal(lot.Price))
				}
			}

			// Verify benchmark and risk-free prices via perfData.
			Expect(resPD.Column(perfA, data.PortfolioBenchmark)).To(Equal(origPD.Column(perfA, data.PortfolioBenchmark)))
			Expect(resPD.Column(perfA, data.PortfolioRiskFree)).To(Equal(origPD.Column(perfA, data.PortfolioRiskFree)))

			// Verify benchmark identity.
			Expect(restored.Benchmark()).To(Equal(acct.Benchmark()))

			// Verify metadata.
			Expect(restored.GetMetadata("strategy")).To(Equal("momentum"))
			Expect(restored.GetMetadata("run_id")).To(Equal("abc-123"))

			// Verify metrics (order may differ due to DB sorting).
			resMetrics := restored.Metrics()
			origMetrics := acct.Metrics()
			Expect(resMetrics).To(HaveLen(len(origMetrics)))
			metricSet := make(map[string]float64)
			for _, m := range origMetrics {
				key := m.Name + "|" + m.Window + "|" + m.Date.Format("2006-01-02")
				metricSet[key] = m.Value
			}
			for _, m := range resMetrics {
				key := m.Name + "|" + m.Window + "|" + m.Date.Format("2006-01-02")
				Expect(metricSet).To(HaveKey(key))
				Expect(m.Value).To(Equal(metricSet[key]))
			}
		})

		It("round-trips annotations", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			ts1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
			ts2 := time.Date(2024, 2, 15, 16, 0, 0, 0, time.UTC)
			acct.Annotate(ts1, "SPY/Momentum", "0.87")
			acct.Annotate(ts1, "bond_fraction", "0.3")
			acct.Annotate(ts2, "SPY/Momentum", "0.92")

			path := filepath.Join(tmpDir, "annotations.db")
			Expect(acct.ToSQLite(path)).To(Succeed())

			restored, err := portfolio.FromSQLite(path)
			Expect(err).NotTo(HaveOccurred())

			annotations := restored.Annotations()
			Expect(annotations).To(HaveLen(3))
			Expect(annotations[0].Timestamp).To(Equal(ts1))
			Expect(annotations[0].Key).To(Equal("SPY/Momentum"))
			Expect(annotations[0].Value).To(Equal("0.87"))
			Expect(annotations[2].Timestamp).To(Equal(ts2))
		})

		It("round-trips tax lot ID", func() {
			spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}

			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    5,
				Price:  500.0,
				Amount: -2500.0,
			})

			// Capture the original lot ID before persisting.
			origLots := acct.TaxLots()
			Expect(origLots).To(HaveKey(spy))
			Expect(origLots[spy]).To(HaveLen(1))
			origID := origLots[spy][0].ID
			Expect(origID).NotTo(BeEmpty())

			path := filepath.Join(tmpDir, "lotid.db")
			Expect(acct.ToSQLite(path)).To(Succeed())

			restored, err := portfolio.FromSQLite(path)
			Expect(err).NotTo(HaveOccurred())

			resLots := restored.TaxLots()
			Expect(resLots).To(HaveKey(spy))
			Expect(resLots[spy]).To(HaveLen(1))
			Expect(resLots[spy][0].ID).To(Equal(origID))
		})

		It("round-trips transaction justification", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:          time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:         asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"},
				Type:          asset.BuyTransaction,
				Qty:           10,
				Price:         500,
				Amount:        -5000,
				Justification: "momentum crossover",
			})

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
				Asset:  asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"},
				Type:   asset.SellTransaction,
				Qty:    5,
				Price:  510,
				Amount: 2550,
			})

			path := filepath.Join(tmpDir, "justification.db")
			Expect(acct.ToSQLite(path)).To(Succeed())

			restored, err := portfolio.FromSQLite(path)
			Expect(err).NotTo(HaveOccurred())

			txns := restored.Transactions()
			// First is the deposit from WithCash, then our two trades.
			Expect(txns).To(HaveLen(3))
			Expect(txns[1].Justification).To(Equal("momentum crossover"))
			Expect(txns[2].Justification).To(BeEmpty())
		})

		It("round-trips perfData frequency", func() {
			spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}

			acct := portfolio.New(
				portfolio.WithCash(10000, time.Time{}),
				portfolio.WithBenchmark(spy),
			)

			priceTime := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
			priceDF, err := data.NewDataFrame(
				[]time.Time{priceTime},
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose, data.AdjClose},
				data.Daily,
				[][]float64{{500}, {500}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(priceDF)

			path := filepath.Join(tmpDir, "freq.db")
			Expect(acct.ToSQLite(path)).To(Succeed())

			restored, err := portfolio.FromSQLite(path)
			Expect(err).NotTo(HaveOccurred())

			perfData := restored.PerfData()
			Expect(perfData).NotTo(BeNil())
			Expect(perfData.Frequency()).To(Equal(data.Daily))
		})
	})

	Describe("empty portfolio round-trip", func() {
		It("writes and reads back an empty account", func() {
			acct := portfolio.New()
			dbPath := filepath.Join(tmpDir, "empty.db")

			err := acct.ToSQLite(dbPath)
			Expect(err).NotTo(HaveOccurred())

			restored, err := portfolio.FromSQLite(dbPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Cash()).To(Equal(0.0))
			Expect(restored.PerfData()).To(BeNil())
			Expect(restored.Transactions()).To(BeEmpty())
			Expect(restored.Metrics()).To(BeEmpty())
		})
	})

	Describe("transaction type round-trip", func() {
		It("persists split, interest, and journal transaction types", func() {
			spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			txnDate := time.Date(2024, 3, 22, 0, 0, 0, 0, time.UTC)

			acct.Record(portfolio.Transaction{
				Date:  txnDate,
				Asset: spy,
				Type:  asset.SplitTransaction,
				Qty:   20,
				Price: 250,
			})
			acct.Record(portfolio.Transaction{
				Date:   txnDate,
				Asset:  spy,
				Type:   asset.InterestTransaction,
				Amount: 12.50,
			})
			acct.Record(portfolio.Transaction{
				Date:   txnDate,
				Asset:  spy,
				Type:   asset.JournalTransaction,
				Amount: 500,
			})

			path := filepath.Join(tmpDir, "txntypes.db")
			Expect(acct.ToSQLite(path)).To(Succeed())

			restored, err := portfolio.FromSQLite(path)
			Expect(err).NotTo(HaveOccurred())

			txns := restored.Transactions()
			// First txn is the deposit from WithCash, then our three.
			Expect(txns).To(HaveLen(4))
			Expect(txns[1].Type).To(Equal(asset.SplitTransaction))
			Expect(txns[2].Type).To(Equal(asset.InterestTransaction))
			Expect(txns[3].Type).To(Equal(asset.JournalTransaction))
		})
	})

	Describe("error cases", func() {
		It("returns an error for nonexistent file", func() {
			_, err := portfolio.FromSQLite(filepath.Join(tmpDir, "noexist.db"))
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for wrong schema version", func() {
			dbPath := filepath.Join(tmpDir, "badversion.db")

			// Create a database with wrong schema version.
			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT)`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO metadata (key, value) VALUES ('schema_version', '99')`)
			Expect(err).NotTo(HaveOccurred())
			db.Close()

			_, err = portfolio.FromSQLite(dbPath)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported schema version"))
		})
	})
})
