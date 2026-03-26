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

var _ = Describe("MACD", func() {
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

	It("returns all three MACD metrics", func() {
		// 10 prices, fast=3, slow=5, signal=2
		prices := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, uu, portfolio.Days(3), portfolio.Days(5), portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(ConsistOf(
			signal.MACDLineSignal,
			signal.MACDSignalLineSignal,
			signal.MACDHistogramSignal,
		))
	})

	It("MACD line is positive in uptrend", func() {
		// 30 rising prices: [100, 102, 104, ...], fast=5, slow=15, signal=4
		// In a steady uptrend, shorter EMA > longer EMA, so MACD line > 0.
		prices := make([]float64, 30)
		for ii := range prices {
			prices[ii] = 100 + float64(ii)*2
		}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, uu, portfolio.Days(5), portfolio.Days(15), portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MACDLineSignal)).To(BeNumerically(">", 0))
	})

	It("histogram equals MACD line minus signal line", func() {
		prices := make([]float64, 30)
		for ii := range prices {
			prices[ii] = 100 + float64(ii)*2
		}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, uu, portfolio.Days(5), portfolio.Days(15), portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())

		macdLine := result.Value(aapl, signal.MACDLineSignal)
		signalLine := result.Value(aapl, signal.MACDSignalLineSignal)
		histogram := result.Value(aapl, signal.MACDHistogramSignal)

		Expect(histogram).To(BeNumerically("~", macdLine-signalLine, 1e-10))
	})

	It("produces exact numeric output for hand-calculable inputs", func() {
		// Prices: [2, 4, 6, 8, 10]; fast=2, slow=3, signal=2.
		// EMA seed is SMA of first window values; alpha=2/(n+1).
		//
		// Fast EMA (alpha=2/3, window=2):
		//   idx=0: NaN
		//   idx=1: mean(2,4) = 3
		//   idx=2: (2/3)*6 + (1/3)*3 = 5
		//   idx=3: (2/3)*8 + (1/3)*5 = 7
		//   idx=4: (2/3)*10 + (1/3)*7 = 9
		//
		// Slow EMA (alpha=1/2, window=3):
		//   idx=0: NaN, idx=1: NaN
		//   idx=2: mean(2,4,6) = 4
		//   idx=3: (1/2)*8 + (1/2)*4 = 6
		//   idx=4: (1/2)*10 + (1/2)*6 = 8
		//
		// MACD line (fast-slow, after Drop NaN): [1, 1, 1]
		//
		// Signal EMA (alpha=2/3, window=2) of [1,1,1]:
		//   idx=0: NaN
		//   idx=1: mean(1,1) = 1
		//   idx=2: (2/3)*1 + (1/3)*1 = 1
		//
		// Last values: MACDLine=1, SignalLine=1, Histogram=0.
		prices := []float64{2, 4, 6, 8, 10}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, uu, portfolio.Days(2), portfolio.Days(3), portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MACDLineSignal)).To(BeNumerically("~", 1.0, 1e-10))
		Expect(result.Value(aapl, signal.MACDSignalLineSignal)).To(BeNumerically("~", 1.0, 1e-10))
		Expect(result.Value(aapl, signal.MACDHistogramSignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("uses custom metric when provided", func() {
		prices := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, uu, portfolio.Days(3), portfolio.Days(5), portfolio.Days(2), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.MetricList()).To(ConsistOf(
			signal.MACDLineSignal,
			signal.MACDSignalLineSignal,
			signal.MACDHistogramSignal,
		))
	})

	It("returns error on degenerate window (fewer than 2 rows)", func() {
		times := []time.Time{now}
		vals := [][]float64{{100}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, uu, portfolio.Days(3), portfolio.Days(5), portfolio.Days(2))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("service unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, uu, portfolio.Days(3), portfolio.Days(5), portfolio.Days(2))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("service unavailable"))
	})
})
