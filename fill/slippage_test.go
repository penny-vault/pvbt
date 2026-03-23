package fill_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/fill"
)

var _ = Describe("Slippage", func() {
	var (
		aapl    asset.Asset
		date    time.Time
		bar     *data.DataFrame
		initial fill.FillResult
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		date = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
		bar = buildBar(date, aapl, map[data.Metric]float64{data.MetricClose: 100.0})
		initial = fill.FillResult{Price: 100.0, Quantity: 50, Partial: true}
	})

	Context("Percent", func() {
		It("increases price for buys", func() {
			adj := fill.Slippage(fill.Percent(0.01))
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 101.0, 1e-9))
		})

		It("decreases price for sells", func() {
			adj := fill.Slippage(fill.Percent(0.01))
			order := broker.Order{Asset: aapl, Side: broker.Sell, Qty: 50}

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 99.0, 1e-9))
		})
	})

	Context("Fixed", func() {
		It("adds to price for buys", func() {
			adj := fill.Slippage(fill.Fixed(0.50))
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 100.50, 1e-9))
		})

		It("subtracts from price for sells", func() {
			adj := fill.Slippage(fill.Fixed(0.50))
			order := broker.Order{Asset: aapl, Side: broker.Sell, Qty: 50}

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 99.50, 1e-9))
		})
	})

	It("passes through Quantity and Partial unchanged", func() {
		adj := fill.Slippage(fill.Percent(0.05))
		order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}

		result, err := adj.Adjust(context.Background(), order, bar, initial)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Quantity).To(Equal(50.0))
		Expect(result.Partial).To(BeTrue())
	})

	It("returns price unchanged with zero slippage", func() {
		adj := fill.Slippage(fill.Percent(0.0))
		order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}

		result, err := adj.Adjust(context.Background(), order, bar, initial)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(100.0))
	})
})
