package portfolio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("LotSelection", func() {
	It("has four lot selection methods", func() {
		Expect(portfolio.LotFIFO).To(BeNumerically(">=", 0))
		Expect(portfolio.LotLIFO).To(BeNumerically(">=", 0))
		Expect(portfolio.LotHighestCost).To(BeNumerically(">=", 0))
		Expect(portfolio.LotSpecificID).To(BeNumerically(">=", 0))
	})

	It("defaults to FIFO", func() {
		Expect(portfolio.LotFIFO).To(Equal(portfolio.LotSelection(0)))
	})
})

var _ = Describe("WithLotSelection modifier", func() {
	It("sets LotSelection on broker.Order via batch.Order()", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		timestamp := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

		df := buildDF(timestamp, []asset.Asset{spy}, []float64{150.0}, []float64{150.0})
		acct.UpdatePrices(df)

		// Record a buy so we have something to sell.
		acct.Record(portfolio.Transaction{
			Date:   timestamp,
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    10,
			Price:  150.0,
			Amount: -1500.0,
		})

		batch := portfolio.NewBatch(timestamp, acct)
		err := batch.Order(context.Background(), spy, portfolio.Sell, 5,
			portfolio.WithLotSelection(portfolio.LotHighestCost))
		Expect(err).ToNot(HaveOccurred())
		Expect(batch.Orders).To(HaveLen(1))
		Expect(batch.Orders[0].LotSelection).To(Equal(int(portfolio.LotHighestCost)))
	})
})
