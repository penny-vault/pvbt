package signal_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("Momentum", func() {
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

	It("computes percent change over the full window", func() {
		// AAPL prices: 100, 105, 110, 115, 120 => (120-100)/100 = 0.20
		// GOOG prices: 200, 190, 180, 170, 160 => (160-200)/200 = -0.20
		times := make([]time.Time, 5)
		for i := range times {
			times[i] = now.AddDate(0, 0, i-4)
		}
		vals := []float64{
			100, 105, 110, 115, 120, // AAPL/Close
			200, 190, 180, 170, 160, // GOOG/Close
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.Momentum(ctx, u, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.MomentumSignal}))
		Expect(result.Value(aapl, signal.MomentumSignal)).To(BeNumerically("~", 0.20, 1e-10))
		Expect(result.Value(goog, signal.MomentumSignal)).To(BeNumerically("~", -0.20, 1e-10))
	})

	It("uses custom metric when provided", func() {
		times := make([]time.Time, 3)
		for i := range times {
			times[i] = now.AddDate(0, 0, i-2)
		}
		vals := []float64{50, 60, 75} // single asset AdjClose: (75-50)/50 = 0.50
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Momentum(ctx, u, portfolio.Days(2), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MomentumSignal)).To(BeNumerically("~", 0.50, 1e-10))
	})

	It("returns error on degenerate window (fewer than 2 rows)", func() {
		times := []time.Time{now}
		vals := []float64{100}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Momentum(ctx, u, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Momentum(ctx, u, portfolio.Days(10))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
