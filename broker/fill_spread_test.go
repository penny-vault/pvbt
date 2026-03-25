package broker_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("SpreadAware", func() {
	var (
		aapl    asset.Asset
		date    time.Time
		initial broker.FillResult
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		date = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
		initial = broker.FillResult{Price: 100.0, Quantity: 50, Partial: true}
	})

	Context("when bid and ask are present in the bar", func() {
		It("fills buys at the ask price", func() {
			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: 100.0,
				data.Bid:         99.50,
				data.Ask:         100.50,
			})
			adj := broker.SpreadAware()
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 100.50, 1e-9))
		})

		It("fills sells at the bid price", func() {
			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: 100.0,
				data.Bid:         99.50,
				data.Ask:         100.50,
			})
			adj := broker.SpreadAware()
			order := broker.Order{Asset: aapl, Side: broker.Sell, Qty: 50}

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 99.50, 1e-9))
		})
	})

	Context("when bid/ask are absent but BPS fallback is configured", func() {
		It("applies half-spread to current.Price for buys", func() {
			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: 100.0,
			})
			adj := broker.SpreadAware(broker.SpreadBPS(20)) // 20 bps = 0.20%
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}

			// halfSpread = 100.0 * 20 / 10000 = 0.20
			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 100.20, 1e-9))
		})

		It("applies half-spread to current.Price for sells", func() {
			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: 100.0,
			})
			adj := broker.SpreadAware(broker.SpreadBPS(20))
			order := broker.Order{Asset: aapl, Side: broker.Sell, Qty: 50}

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 99.80, 1e-9))
		})

		It("uses current.Price not bar close as the BPS base", func() {
			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: 200.0, // close differs from current.Price
			})
			adj := broker.SpreadAware(broker.SpreadBPS(10)) // 10 bps
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}
			// current.Price is 100.0 (from initial), not 200.0
			// halfSpread = 100.0 * 10 / 10000 = 0.10

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Price).To(BeNumerically("~", 100.10, 1e-9))
		})
	})

	Context("when neither bid/ask nor BPS fallback is available", func() {
		It("returns an error", func() {
			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: 100.0,
			})
			adj := broker.SpreadAware() // no BPS configured
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}

			_, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spread"))
		})
	})

	It("passes through Quantity and Partial unchanged", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{
			data.MetricClose: 100.0,
			data.Bid:         99.50,
			data.Ask:         100.50,
		})
		adj := broker.SpreadAware()
		order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 50}

		result, err := adj.Adjust(context.Background(), order, bar, initial)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Quantity).To(Equal(50.0))
		Expect(result.Partial).To(BeTrue())
	})
})
