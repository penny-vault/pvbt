package tastytrade_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/tastytrade"
)

var _ = Describe("Integration", Label("integration"), func() {
	var (
		ctx      context.Context
		cancel   context.CancelFunc
		ttBroker *tastytrade.TastytradeBroker
	)

	BeforeEach(func() {
		if os.Getenv("TASTYTRADE_USERNAME") == "" {
			Skip("TASTYTRADE_USERNAME not set")
		}

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		ttBroker = tastytrade.New(tastytrade.WithSandbox())
		Expect(ttBroker.Connect(ctx)).To(Succeed())
	})

	AfterEach(func() {
		if ttBroker != nil {
			ttBroker.Close()
		}
		cancel()
	})

	It("connects and retrieves balance", func() {
		balance, err := ttBroker.Balance(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(balance.NetLiquidatingValue).To(BeNumerically(">", 0))
	})

	It("retrieves positions", func() {
		positions, err := ttBroker.Positions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(positions).NotTo(BeNil())
	})

	It("retrieves orders", func() {
		orders, err := ttBroker.Orders(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(orders).NotTo(BeNil())
	})

	It("submits and cancels a limit order", Label("orders"), func() {
		err := ttBroker.Submit(ctx, broker.Order{
			Asset:       asset.Asset{Ticker: "AAPL"},
			Side:        broker.Buy,
			Qty:         1,
			OrderType:   broker.Limit,
			LimitPrice:  1.00,
			TimeInForce: broker.Day,
		})
		Expect(err).NotTo(HaveOccurred())

		orders, err := ttBroker.Orders(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(orders).NotTo(BeEmpty())

		err = ttBroker.Cancel(ctx, orders[0].ID)
		Expect(err).NotTo(HaveOccurred())
	})
})
