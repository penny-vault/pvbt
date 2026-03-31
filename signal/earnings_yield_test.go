package signal_test

import (
	"context"
	"errors"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("EarningsYield", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		goog asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("computes EPS divided by Price", func() {
		// AAPL: EPS=5, Price=100 => yield = 0.05
		// GOOG: EPS=10, Price=200 => yield = 0.05
		times := []time.Time{now}
		vals := [][]float64{
			{5},   // AAPL/EarningsPerShare
			{100}, // AAPL/Price
			{10},  // GOOG/EarningsPerShare
			{200}, // GOOG/Price
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
			[]data.Metric{data.EarningsPerShare, data.Price}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.EarningsYieldSignal}))
		Expect(result.Value(aapl, signal.EarningsYieldSignal)).To(BeNumerically("~", 0.05, 1e-10))
		Expect(result.Value(goog, signal.EarningsYieldSignal)).To(BeNumerically("~", 0.05, 1e-10))
	})

	It("returns error when EarningsPerShare metric is missing", func() {
		times := []time.Time{now}
		vals := [][]float64{{100}} // only Price, no EPS
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("EarningsPerShare"))
	})

	It("returns error when Price metric is missing", func() {
		times := []time.Time{now}
		vals := [][]float64{{5}} // only EPS, no Price
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.EarningsPerShare}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("Price"))
	})

	It("returns NaN when price is zero", func() {
		// EPS=5, Price=0: division by zero should produce NaN, not +Inf.
		times := []time.Time{now}
		vals := [][]float64{
			{5}, // AAPL/EarningsPerShare
			{0}, // AAPL/Price = 0
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.EarningsPerShare, data.Price}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.EarningsYieldSignal))).To(BeTrue())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("db down")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("db down"))
	})
})
