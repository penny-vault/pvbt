package portfolio_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
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
})
