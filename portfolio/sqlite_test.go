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
			bil := asset.Asset{Ticker: "BIL", CompositeFigi: "BBG000BIL001"}

			acct := portfolio.New(
				portfolio.WithCash(10000),
				portfolio.WithBenchmark(spy),
				portfolio.WithRiskFree(bil),
			)

			acct.SetMetadata("strategy", "momentum")
			acct.SetMetadata("run_id", "abc-123")

			// Record a buy transaction.
			t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			acct.Record(portfolio.Transaction{
				Date:   t0,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  450.0,
				Amount: -4500.0,
			})

			// Update prices to build equity curve.
			df0 := buildDF(t0,
				[]asset.Asset{spy, bil},
				[]float64{450.0, 91.50},
				[]float64{450.0, 91.50},
			)
			acct.UpdatePrices(df0)

			t1 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
			df1 := buildDF(t1,
				[]asset.Asset{spy, bil},
				[]float64{455.0, 91.55},
				[]float64{455.0, 91.55},
			)
			acct.UpdatePrices(df1)

			// Record a dividend.
			t2 := time.Date(2025, 4, 15, 0, 0, 0, 0, time.UTC)
			acct.Record(portfolio.Transaction{
				Date:   t2,
				Asset:  spy,
				Type:   portfolio.DividendTransaction,
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

			// Verify equity curve.
			Expect(restored.EquityCurve()).To(Equal(acct.EquityCurve()))
			Expect(restored.EquityTimes()).To(HaveLen(len(acct.EquityTimes())))
			for i, t := range restored.EquityTimes() {
				Expect(t.Equal(acct.EquityTimes()[i])).To(BeTrue(),
					"equity time mismatch at index %d", i)
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
			origHoldings := make(map[asset.Asset]float64)
			acct.Holdings(func(a asset.Asset, q float64) {
				origHoldings[a] = q
			})
			resHoldings := make(map[asset.Asset]float64)
			restored.Holdings(func(a asset.Asset, q float64) {
				resHoldings[a] = q
			})
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

			// Verify benchmark and risk-free prices.
			Expect(restored.BenchmarkPrices()).To(Equal(acct.BenchmarkPrices()))
			Expect(restored.RiskFreePrices()).To(Equal(acct.RiskFreePrices()))

			// Verify benchmark and risk-free identity.
			Expect(restored.Benchmark()).To(Equal(acct.Benchmark()))
			Expect(restored.RiskFree()).To(Equal(acct.RiskFree()))

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
			Expect(restored.EquityCurve()).To(BeEmpty())
			Expect(restored.Transactions()).To(BeEmpty())
			Expect(restored.Metrics()).To(BeEmpty())
			Expect(restored.BenchmarkPrices()).To(BeEmpty())
			Expect(restored.RiskFreePrices()).To(BeEmpty())
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

// Ensure data.MetricClose is used (avoid unused import).
var _ = data.MetricClose
