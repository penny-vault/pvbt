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
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("WilliamsR", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("computes hand-calculated Williams %%R correctly", func() {
		highs := []float64{12, 11, 13, 14, 12}
		lows := []float64{9, 8, 10, 11, 9}
		closes := []float64{10, 10, 12, 13, 11}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.WilliamsRSignal}))
		Expect(result.Value(aapl, signal.WilliamsRSignal)).To(BeNumerically("~", -50.0, 1e-10))
	})

	It("returns 0 when close equals highest high", func() {
		highs := []float64{10, 12, 15}
		lows := []float64{8, 9, 11}
		closes := []float64{9, 11, 15}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.WilliamsRSignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("returns -100 when close equals lowest low", func() {
		highs := []float64{15, 14, 13}
		lows := []float64{10, 9, 8}
		closes := []float64{12, 10, 8}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.WilliamsRSignal)).To(BeNumerically("~", -100.0, 1e-10))
	})

	It("produces NaN for flat market", func() {
		highs := []float64{10, 10, 10}
		lows := []float64{10, 10, 10}
		closes := []float64{10, 10, 10}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.WilliamsRSignal))).To(BeTrue())
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		aaplHighs := []float64{12, 14}
		aaplLows := []float64{9, 9}
		aaplCloses := []float64{10, 11}
		msftHighs := []float64{20, 22}
		msftLows := []float64{18, 17}
		msftCloses := []float64{19, 20}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, msftHighs, msftLows, msftCloses}
		df, err := data.NewDataFrame(times, assets, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.WilliamsRSignal)).To(BeNumerically("~", -60.0, 1e-10))
		Expect(result.Value(msft, signal.WilliamsRSignal)).To(BeNumerically("~", -40.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
