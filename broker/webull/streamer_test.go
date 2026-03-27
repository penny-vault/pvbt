package webull_test

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/webull"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FillStreamer", func() {
	var (
		fills chan broker.Fill
		fs    *webull.FillStreamerForTestType
	)

	BeforeEach(func() {
		fills = make(chan broker.Fill, 16)
	})

	Describe("handleTradeEvent", func() {
		BeforeEach(func() {
			fs = webull.NewFillStreamerForTest(fills, nil)
		})

		It("sends a fill for a FILLED event", func() {
			fs.HandleTradeEvent("order-1", "FILLED", 10, 150.25)

			Eventually(fills).Should(Receive(SatisfyAll(
				HaveField("OrderID", "order-1"),
				HaveField("Price", 150.25),
				HaveField("Qty", 10.0),
			)))
		})

		It("sends a fill for a FINAL_FILLED event", func() {
			fs.HandleTradeEvent("order-2", "FINAL_FILLED", 5, 99.50)

			Eventually(fills).Should(Receive(SatisfyAll(
				HaveField("OrderID", "order-2"),
				HaveField("Price", 99.50),
				HaveField("Qty", 5.0),
			)))
		})

		It("does not send a fill for non-fill events", func() {
			fs.HandleTradeEvent("order-3", "CANCEL_SUCCESS", 0, 0)

			Consistently(fills, "100ms").ShouldNot(Receive())
		})

		It("does not send a fill for PENDING events", func() {
			fs.HandleTradeEvent("order-4", "PENDING", 0, 0)

			Consistently(fills, "100ms").ShouldNot(Receive())
		})

		It("deduplicates fills with the same cumulative qty", func() {
			fs.HandleTradeEvent("order-5", "FILLED", 10, 100.0)
			Eventually(fills).Should(Receive())

			fs.HandleTradeEvent("order-5", "FILLED", 10, 100.0)
			Consistently(fills, "100ms").ShouldNot(Receive())
		})

		It("sends a delta fill when cumulative qty increases", func() {
			fs.HandleTradeEvent("order-6", "FILLED", 5, 100.0)

			var firstFill broker.Fill
			Eventually(fills).Should(Receive(&firstFill))
			Expect(firstFill.Qty).To(Equal(5.0))

			fs.HandleTradeEvent("order-6", "FINAL_FILLED", 10, 102.0)

			var secondFill broker.Fill
			Eventually(fills).Should(Receive(&secondFill))
			Expect(secondFill.Qty).To(Equal(5.0))
			Expect(secondFill.Price).To(Equal(102.0))
		})

		It("tracks cumulative filled qty per order", func() {
			fs.HandleTradeEvent("order-7", "FILLED", 10, 100.0)
			Eventually(fills).Should(Receive())

			Expect(fs.CumulFilledForTest("order-7")).To(Equal(10.0))
		})
	})

	Describe("pollMissedFills", func() {
		It("sends fills from polled orders not yet seen", func() {
			pollFn := func(_ context.Context) ([]webull.OrderResponseExport, error) {
				return []webull.OrderResponseExport{
					{ID: "poll-1", Status: "FILLED", FilledQty: "20", FilledPrice: "55.00"},
					{ID: "poll-2", Status: "PARTIALLY_FILLED", FilledQty: "3", FilledPrice: "80.00"},
				}, nil
			}

			fs = webull.NewFillStreamerForTest(fills, pollFn)
			fs.PollMissedFills(context.Background())

			var fill1, fill2 broker.Fill
			Eventually(fills).Should(Receive(&fill1))
			Eventually(fills).Should(Receive(&fill2))

			Expect(fill1.OrderID).To(Equal("poll-1"))
			Expect(fill1.Qty).To(Equal(20.0))
			Expect(fill2.OrderID).To(Equal("poll-2"))
			Expect(fill2.Qty).To(Equal(3.0))
		})

		It("does not duplicate fills already delivered via stream", func() {
			pollFn := func(_ context.Context) ([]webull.OrderResponseExport, error) {
				return []webull.OrderResponseExport{
					{ID: "dup-1", Status: "FILLED", FilledQty: "10", FilledPrice: "50.00"},
				}, nil
			}

			fs = webull.NewFillStreamerForTest(fills, pollFn)

			// Simulate an already-delivered stream fill.
			fs.HandleTradeEvent("dup-1", "FILLED", 10, 50.00)
			Eventually(fills).Should(Receive())

			// Poll should not produce another fill.
			fs.PollMissedFills(context.Background())
			Consistently(fills, "100ms").ShouldNot(Receive())
		})

		It("sends delta when poll shows higher cumulative than stream", func() {
			pollFn := func(_ context.Context) ([]webull.OrderResponseExport, error) {
				return []webull.OrderResponseExport{
					{ID: "delta-1", Status: "FILLED", FilledQty: "15", FilledPrice: "60.00"},
				}, nil
			}

			fs = webull.NewFillStreamerForTest(fills, pollFn)

			// Stream delivered partial fill of 5.
			fs.HandleTradeEvent("delta-1", "FILLED", 5, 58.00)
			Eventually(fills).Should(Receive())

			// Poll sees cumulative 15, so delta should be 10.
			fs.PollMissedFills(context.Background())

			var deltaFill broker.Fill
			Eventually(fills).Should(Receive(&deltaFill))
			Expect(deltaFill.Qty).To(Equal(10.0))
			Expect(deltaFill.Price).To(Equal(60.0))
		})

		It("skips orders with non-fill status", func() {
			pollFn := func(_ context.Context) ([]webull.OrderResponseExport, error) {
				return []webull.OrderResponseExport{
					{ID: "skip-1", Status: "CANCELLED", FilledQty: "0", FilledPrice: "0"},
					{ID: "skip-2", Status: "PENDING", FilledQty: "0", FilledPrice: "0"},
				}, nil
			}

			fs = webull.NewFillStreamerForTest(fills, pollFn)
			fs.PollMissedFills(context.Background())

			Consistently(fills, "100ms").ShouldNot(Receive())
		})

		It("handles poll errors gracefully", func() {
			pollFn := func(_ context.Context) ([]webull.OrderResponseExport, error) {
				return nil, fmt.Errorf("network error")
			}

			fs = webull.NewFillStreamerForTest(fills, pollFn)
			fs.PollMissedFills(context.Background())

			Consistently(fills, "100ms").ShouldNot(Receive())
		})

		It("does nothing when pollOrders is nil", func() {
			fs = webull.NewFillStreamerForTest(fills, nil)
			fs.PollMissedFills(context.Background())

			Consistently(fills, "100ms").ShouldNot(Receive())
		})
	})
})
