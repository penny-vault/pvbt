package fill_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/fill"
)

var _ = Describe("CloseFill", func() {
	var (
		aapl asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		date = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("fills at the close price", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{data.MetricClose: 150.0})
		model := fill.Close()

		result, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(150.0))
		Expect(result.Quantity).To(Equal(100.0))
		Expect(result.Partial).To(BeFalse())
	})

	It("returns an error when close price is zero", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{data.MetricClose: 0})
		model := fill.Close()

		_, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).To(HaveOccurred())
	})

	It("returns an error when close price is NaN", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{data.MetricClose: math.NaN()})
		model := fill.Close()

		_, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 50}, bar)

		Expect(err).To(HaveOccurred())
	})
})
