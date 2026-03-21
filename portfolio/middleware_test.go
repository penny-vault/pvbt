package portfolio_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

// testMiddleware records that it was called and optionally removes buy orders.
type testMiddleware struct {
	called    bool
	removeBuy bool
}

func (m *testMiddleware) Process(_ context.Context, batch *portfolio.Batch) error {
	m.called = true

	if m.removeBuy {
		var filtered []broker.Order
		for _, order := range batch.Orders {
			if order.Side != broker.Buy {
				filtered = append(filtered, order)
			}
		}

		batch.Orders = filtered
		batch.Annotate("risk:test", "removed all buy orders")
	}

	return nil
}

// errorMiddleware always returns an error.
type errorMiddleware struct {
	err error
}

func (m *errorMiddleware) Process(_ context.Context, _ *portfolio.Batch) error {
	return m.err
}

// orderTrackingMiddleware records the order count when it runs.
type orderTrackingMiddleware struct {
	orderCount int
}

func (m *orderTrackingMiddleware) Process(_ context.Context, batch *portfolio.Batch) error {
	m.orderCount = len(batch.Orders)
	return nil
}

var _ = Describe("Middleware", func() {
	var (
		spy  asset.Asset
		t1   time.Time
		fill time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t1 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		fill = time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)
	})

	It("runs middleware in order during ExecuteBatch", func() {
		mb := newMockBroker()
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy: {{OrderID: "batch-" + "1704153600000000000-0", Price: 100.0, Qty: 10, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))
		df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		mw := &testMiddleware{}
		acct.Use(mw)

		batch := acct.NewBatch(t1)
		batch.Order(context.Background(), spy, portfolio.Buy, 10)

		err := acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())
		Expect(mw.called).To(BeTrue())
		Expect(mb.submitted).To(HaveLen(1))
	})

	It("middleware can remove orders from the batch", func() {
		mb := newMockBroker()

		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))
		df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		mw := &testMiddleware{removeBuy: true}
		acct.Use(mw)

		batch := acct.NewBatch(t1)
		batch.Order(context.Background(), spy, portfolio.Buy, 10)

		err := acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// All buy orders were removed, so nothing should be submitted.
		Expect(mb.submitted).To(BeEmpty())

		// Position should be unchanged (no shares bought).
		Expect(acct.Position(spy)).To(Equal(0.0))
	})

	It("middleware error aborts batch", func() {
		mb := newMockBroker()

		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))
		df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		mwErr := errors.New("risk limit exceeded")
		acct.Use(&errorMiddleware{err: mwErr})

		batch := acct.NewBatch(t1)
		batch.Order(context.Background(), spy, portfolio.Buy, 10)

		err := acct.ExecuteBatch(context.Background(), batch)
		Expect(err).To(MatchError(mwErr))

		// No orders should have been submitted.
		Expect(mb.submitted).To(BeEmpty())
	})

	It("ExecuteBatch records annotations", func() {
		mb := newMockBroker()

		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))
		df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		batch := acct.NewBatch(t1)
		batch.Annotate("strategy:signal", "momentum=0.5")

		err := acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		annotations := acct.Annotations()
		Expect(annotations).To(HaveLen(1))
		Expect(annotations[0].Key).To(Equal("strategy:signal"))
		Expect(annotations[0].Value).To(Equal("momentum=0.5"))
		Expect(annotations[0].Timestamp).To(Equal(t1))
	})

	It("ExecuteBatch with no middleware executes normally", func() {
		mb := newMockBroker()
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy: {{OrderID: "batch-" + "1704153600000000000-0", Price: 100.0, Qty: 10, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))
		df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		batch := acct.NewBatch(t1)
		batch.Order(context.Background(), spy, portfolio.Buy, 10)

		err := acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// Order should be submitted and fill recorded.
		Expect(mb.submitted).To(HaveLen(1))
		Expect(acct.Position(spy)).To(Equal(10.0))
		Expect(acct.Cash()).To(Equal(10_000.0 - 100.0*10.0))
	})

	It("runs multiple middleware in registration order", func() {
		mb := newMockBroker()

		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))
		df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		// First middleware records order count before filtering.
		tracker := &orderTrackingMiddleware{}
		// Second middleware removes buy orders.
		remover := &testMiddleware{removeBuy: true}

		acct.Use(tracker, remover)

		batch := acct.NewBatch(t1)
		batch.Order(context.Background(), spy, portfolio.Buy, 10)

		err := acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// Tracker ran first, saw 1 order.
		Expect(tracker.orderCount).To(Equal(1))
		// Remover ran second, removed the buy.
		Expect(remover.called).To(BeTrue())
		// No orders submitted because remover filtered them out.
		Expect(mb.submitted).To(BeEmpty())
	})

	It("DrainFills records pending fills as transactions", func() {
		mb := newMockBroker()
		// Allow submit without immediate fill delivery.
		mb.submitFn = func(_ broker.Order) error { return nil }

		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))
		df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		// Submit a batch with no immediate fill.
		batch := acct.NewBatch(t1)
		batch.Order(context.Background(), spy, portfolio.Buy, 5)

		err := acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// Position is 0 because no fill was delivered yet.
		Expect(acct.Position(spy)).To(Equal(0.0))

		// Now deliver a fill to the channel.
		orderID := batch.Orders[0].ID
		mb.fillCh <- broker.Fill{OrderID: orderID, Price: 100.0, Qty: 5, FilledAt: fill}

		// DrainFills picks it up.
		err = acct.DrainFills(context.Background())
		Expect(err).NotTo(HaveOccurred())

		Expect(acct.Position(spy)).To(Equal(5.0))
		Expect(acct.Cash()).To(Equal(10_000.0 - 500.0))
	})

	It("CancelOpenOrders cancels all pending orders tracked by the account", func() {
		mb := newMockBroker()
		canceledIDs := []string{}
		mb.cancelFn = func(orderID string) error {
			canceledIDs = append(canceledIDs, orderID)
			return nil
		}

		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))

		// Seed pending orders directly (the account only tracks open/submitted orders).
		acct.SetPendingOrder(broker.Order{ID: "o1", Status: broker.OrderOpen})
		acct.SetPendingOrder(broker.Order{ID: "o3", Status: broker.OrderSubmitted})

		err := acct.CancelOpenOrders(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// All orders in pendingOrders are cancelled.
		Expect(canceledIDs).To(ConsistOf("o1", "o3"))

		// pendingOrders is cleared after cancellation.
		Expect(acct.PendingOrderIDs()).To(BeEmpty())
	})
})

var _ = Describe("Middleware annotation from middleware", func() {
	It("middleware annotations flow through to portfolio", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

		mb := newMockBroker()
		acct := portfolio.New(portfolio.WithCash(10_000, t1), portfolio.WithBroker(mb))
		df := buildDF(t1, []asset.Asset{spy}, []float64{100.0}, []float64{100.0})
		acct.UpdatePrices(df)

		// Middleware that removes buys also annotates the batch.
		mw := &testMiddleware{removeBuy: true}
		acct.Use(mw)

		batch := acct.NewBatch(t1)
		batch.Order(context.Background(), spy, portfolio.Buy, 10)

		err := acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		annotations := acct.Annotations()
		Expect(annotations).To(HaveLen(1))
		Expect(annotations[0].Key).To(Equal("risk:test"))
		Expect(annotations[0].Value).To(Equal("removed all buy orders"))
	})
})
