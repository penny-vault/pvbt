package broker_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("MarketImpact", func() {
	var (
		aapl asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		date = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	Context("small order with LargeCap preset", func() {
		It("produces minimal price impact and full fill", func() {
			// 1% of volume = 10_000 shares out of 1_000_000
			volume := 1_000_000.0
			qty := 10_000.0
			price := 150.0

			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: price,
				data.Volume:      volume,
			})
			initial := broker.FillResult{Price: price, Quantity: qty}
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: qty}
			adj := broker.MarketImpact(broker.LargeCap)

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Partial).To(BeFalse())
			Expect(result.Quantity).To(Equal(qty))

			// participation = 10_000 / 1_000_000 = 0.01
			// impact = 0.1 * sqrt(0.01) = 0.1 * 0.1 = 0.01
			// expected price = 150 * (1 + 0.01) = 151.5
			expectedPrice := price * (1.0 + broker.LargeCap.Coefficient*math.Sqrt(qty/volume))
			Expect(result.Price).To(BeNumerically("~", expectedPrice, 1e-9))
		})
	})

	Context("large order exceeding SmallCap threshold", func() {
		It("produces partial fill with quantity capped at volume * threshold", func() {
			// SmallCap threshold = 0.02, so >2% of volume triggers partial fill
			volume := 100_000.0
			qty := 5_000.0 // 5% of volume, exceeds 2% threshold
			price := 50.0

			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: price,
				data.Volume:      volume,
			})
			initial := broker.FillResult{Price: price, Quantity: qty}
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: qty}
			adj := broker.MarketImpact(broker.SmallCap)

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Partial).To(BeTrue())

			cappedQty := math.Floor(volume * broker.SmallCap.PartialThreshold)
			Expect(result.Quantity).To(Equal(cappedQty))

			// recompute participation with capped quantity
			participation := cappedQty / volume
			impact := broker.SmallCap.Coefficient * math.Sqrt(participation)
			expectedPrice := price * (1.0 + impact)
			Expect(result.Price).To(BeNumerically("~", expectedPrice, 1e-9))
		})
	})

	Context("MicroCap vs LargeCap impact comparison", func() {
		It("MicroCap produces larger price impact than LargeCap for the same order", func() {
			volume := 500_000.0
			qty := 2_500.0 // 0.5% of volume, below both thresholds
			price := 100.0

			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: price,
				data.Volume:      volume,
			})
			initial := broker.FillResult{Price: price, Quantity: qty}
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: qty}

			largeCap := broker.MarketImpact(broker.LargeCap)
			microCap := broker.MarketImpact(broker.MicroCap)

			largeResult, err := largeCap.Adjust(context.Background(), order, bar, initial)
			Expect(err).NotTo(HaveOccurred())

			microResult, err := microCap.Adjust(context.Background(), order, bar, initial)
			Expect(err).NotTo(HaveOccurred())

			Expect(microResult.Price).To(BeNumerically(">", largeResult.Price))
		})
	})

	Context("buy side price adjustment", func() {
		It("increases price by price * coefficient * sqrt(qty/volume)", func() {
			volume := 1_000_000.0
			qty := 10_000.0
			price := 200.0

			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: price,
				data.Volume:      volume,
			})
			initial := broker.FillResult{Price: price, Quantity: qty}
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: qty}
			adj := broker.MarketImpact(broker.LargeCap)

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())

			participation := qty / volume
			impact := broker.LargeCap.Coefficient * math.Sqrt(participation)
			expectedPrice := price * (1.0 + impact)
			Expect(result.Price).To(BeNumerically("~", expectedPrice, 1e-9))
			Expect(result.Price).To(BeNumerically(">", price))
		})
	})

	Context("sell side price adjustment", func() {
		It("decreases price by price * coefficient * sqrt(qty/volume)", func() {
			volume := 1_000_000.0
			qty := 10_000.0
			price := 200.0

			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: price,
				data.Volume:      volume,
			})
			initial := broker.FillResult{Price: price, Quantity: qty}
			order := broker.Order{Asset: aapl, Side: broker.Sell, Qty: qty}
			adj := broker.MarketImpact(broker.LargeCap)

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())

			participation := qty / volume
			impact := broker.LargeCap.Coefficient * math.Sqrt(participation)
			expectedPrice := price * (1.0 - impact)
			Expect(result.Price).To(BeNumerically("~", expectedPrice, 1e-9))
			Expect(result.Price).To(BeNumerically("<", price))
		})
	})

	Context("error cases", func() {
		It("returns error when volume is zero", func() {
			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: 150.0,
				data.Volume:      0,
			})
			initial := broker.FillResult{Price: 150.0, Quantity: 100}
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 100}
			adj := broker.MarketImpact(broker.LargeCap)

			_, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("volume"))
		})

		It("returns error when volume is NaN (no volume data)", func() {
			// Build bar without volume metric so Value returns NaN
			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: 150.0,
			})
			initial := broker.FillResult{Price: 150.0, Quantity: 100}
			order := broker.Order{Asset: aapl, Side: broker.Buy, Qty: 100}
			adj := broker.MarketImpact(broker.LargeCap)

			_, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("volume"))
		})
	})

	Context("partial fill flag", func() {
		It("sets Partial true and reduces Quantity when threshold exceeded", func() {
			volume := 100_000.0
			qty := 5_000.0 // 5% > LargeCap threshold of 5%, use SmallCap (2%)
			price := 75.0

			bar := buildBar(date, aapl, map[data.Metric]float64{
				data.MetricClose: price,
				data.Volume:      volume,
			})
			initial := broker.FillResult{Price: price, Quantity: qty}
			order := broker.Order{Asset: aapl, Side: broker.Sell, Qty: qty}
			adj := broker.MarketImpact(broker.SmallCap)

			result, err := adj.Adjust(context.Background(), order, bar, initial)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Partial).To(BeTrue())
			Expect(result.Quantity).To(BeNumerically("<", qty))
			Expect(result.Quantity).To(Equal(math.Floor(volume * broker.SmallCap.PartialThreshold)))
		})
	})
})
