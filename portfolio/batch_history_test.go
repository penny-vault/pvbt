package portfolio_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
	_ "modernc.org/sqlite"
)

var _ = Describe("batch history round-trip", func() {
	It("persists and restores batches with their timestamps", func() {
		ctx := context.Background()
		ts1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		ts2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		mb := newMockBroker()
		mb.defaultFill = &broker.Fill{Price: 100.0, FilledAt: ts1}

		acct := portfolio.New(portfolio.WithCash(100_000, ts1), portfolio.WithBroker(mb))
		acct.UpdatePrices(buildDF(ts1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))

		b1 := acct.NewBatch(ts1)
		Expect(b1.Order(ctx, spy, portfolio.Buy, 10)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, b1)).To(Succeed())

		b2 := acct.NewBatch(ts2)
		Expect(acct.ExecuteBatch(ctx, b2)).To(Succeed())

		tmp := filepath.Join(GinkgoT().TempDir(), "out.db")
		Expect(acct.ToSQLite(tmp)).To(Succeed())
		defer os.Remove(tmp)

		restored, err := portfolio.FromSQLite(tmp)
		Expect(err).NotTo(HaveOccurred())

		batches := portfolio.GetAccountBatches(restored)
		Expect(batches).To(HaveLen(2))
		Expect(batches[0].BatchID).To(Equal(1))
		Expect(batches[0].Timestamp.UTC()).To(Equal(ts1))
		Expect(batches[1].BatchID).To(Equal(2))
		Expect(batches[1].Timestamp.UTC()).To(Equal(ts2))
	})

	It("persists and restores transaction BatchID", func() {
		ctx := context.Background()
		ts := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		mb := newMockBroker()
		mb.defaultFill = &broker.Fill{Price: 100.0, FilledAt: ts}

		acct := portfolio.New(portfolio.WithCash(100_000, ts), portfolio.WithBroker(mb))
		acct.UpdatePrices(buildDF(ts, []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))

		b1 := acct.NewBatch(ts)
		Expect(b1.Order(ctx, spy, portfolio.Buy, 10)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, b1)).To(Succeed())

		tmp := filepath.Join(GinkgoT().TempDir(), "out.db")
		Expect(acct.ToSQLite(tmp)).To(Succeed())
		defer os.Remove(tmp)

		restored, err := portfolio.FromSQLite(tmp)
		Expect(err).NotTo(HaveOccurred())

		var seenBuy bool
		for _, txn := range restored.Transactions() {
			if txn.Type == asset.BuyTransaction {
				Expect(txn.BatchID).To(Equal(1))
				seenBuy = true
			}
		}
		Expect(seenBuy).To(BeTrue())
	})

	It("persists and restores annotation BatchID", func() {
		ctx := context.Background()
		ts := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

		mb := newMockBroker()
		acct := portfolio.New(portfolio.WithCash(100_000, ts), portfolio.WithBroker(mb))

		b1 := acct.NewBatch(ts)
		b1.Annotate("score", "0.42")
		Expect(acct.ExecuteBatch(ctx, b1)).To(Succeed())

		tmp := filepath.Join(GinkgoT().TempDir(), "out.db")
		Expect(acct.ToSQLite(tmp)).To(Succeed())
		defer os.Remove(tmp)

		restored, err := portfolio.FromSQLite(tmp)
		Expect(err).NotTo(HaveOccurred())

		anns := restored.Annotations()
		Expect(anns).To(HaveLen(1))
		Expect(anns[0].Key).To(Equal("score"))
		Expect(anns[0].BatchID).To(Equal(1))
	})

	It("supports reconstructing holdings after each batch via SQL replay", func() {
		ctx := context.Background()
		ts1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		ts2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		mb := newMockBroker()
		mb.defaultFill = &broker.Fill{Price: 100.0, FilledAt: ts1}

		acct := portfolio.New(portfolio.WithCash(100_000, ts1), portfolio.WithBroker(mb))
		acct.UpdatePrices(buildDF(ts1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))

		// Batch 1: buy 10 SPY.
		b1 := acct.NewBatch(ts1)
		Expect(b1.Order(ctx, spy, portfolio.Buy, 10)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, b1)).To(Succeed())

		// Batch 2: sell 4 SPY.
		b2 := acct.NewBatch(ts2)
		Expect(b2.Order(ctx, spy, portfolio.Sell, 4)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, b2)).To(Succeed())

		tmp := filepath.Join(GinkgoT().TempDir(), "out.db")
		Expect(acct.ToSQLite(tmp)).To(Succeed())
		defer os.Remove(tmp)

		db, err := sql.Open("sqlite", tmp)
		Expect(err).NotTo(HaveOccurred())
		defer db.Close()

		query := `
	        SELECT ticker,
	               SUM(CASE type WHEN 'buy' THEN quantity
	                             WHEN 'sell' THEN -quantity
	                             WHEN 'split' THEN quantity ELSE 0 END) AS qty
	        FROM transactions
	        WHERE batch_id > 0 AND batch_id <= ?
	        GROUP BY ticker
	        HAVING qty != 0`

		checkQty := func(n int, expected float64) {
			rows, err := db.Query(query, n)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()

			Expect(rows.Next()).To(BeTrue())
			var (
				ticker string
				qty    float64
			)
			Expect(rows.Scan(&ticker, &qty)).To(Succeed())
			Expect(ticker).To(Equal("SPY"))
			Expect(qty).To(BeNumerically("~", expected, 1e-9))
		}

		checkQty(1, 10)
		checkQty(2, 6)
	})
})
