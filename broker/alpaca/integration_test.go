package alpaca_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/alpaca"
)

var _ = Describe("Integration", Label("integration"), func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		alpacaBroker  *alpaca.AlpacaBroker
	)

	BeforeEach(func() {
		if os.Getenv("ALPACA_API_KEY") == "" {
			Skip("ALPACA_API_KEY not set")
		}

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		alpacaBroker = alpaca.New(alpaca.WithPaper())
		Expect(alpacaBroker.Connect(ctx)).To(Succeed())
	})

	AfterEach(func() {
		if alpacaBroker != nil {
			alpacaBroker.Close()
		}
		cancel()
	})

	It("connects and retrieves balance", func() {
		balance, err := alpacaBroker.Balance(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(balance.NetLiquidatingValue).To(BeNumerically(">", 0))
	})

	It("retrieves positions", func() {
		positions, err := alpacaBroker.Positions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(positions).NotTo(BeNil())
	})

	It("retrieves orders", func() {
		orders, err := alpacaBroker.Orders(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(orders).NotTo(BeNil())
	})

	It("submits and cancels a limit order", Label("orders"), func() {
		err := alpacaBroker.Submit(ctx, broker.Order{
			Asset:       asset.Asset{Ticker: "AAPL"},
			Side:        broker.Buy,
			Qty:         1,
			OrderType:   broker.Limit,
			LimitPrice:  1.00,
			TimeInForce: broker.Day,
		})
		Expect(err).NotTo(HaveOccurred())

		orders, err := alpacaBroker.Orders(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(orders).NotTo(BeEmpty())

		err = alpacaBroker.Cancel(ctx, orders[0].ID)
		Expect(err).NotTo(HaveOccurred())
	})
})
