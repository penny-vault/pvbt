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

var _ = Describe("RSI", func() {
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

	It("computes RSI with Wilder smoothing (known reference)", func() {
		// Classic RSI reference data with 15 prices and period=14.
		// Expected RSI ~72.97 (tolerance 0.01).
		prices := []float64{44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84, 46.08, 45.89, 46.03, 45.61, 46.28, 46.28}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, uu, portfolio.Days(14))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.RSISignal}))
		Expect(result.Value(aapl, signal.RSISignal)).To(BeNumerically("~", 72.984, 0.01))
	})

	It("returns RSI near 100 when all changes are gains", func() {
		// Prices [10..24]: all positive changes, avgLoss=0, RSI=100.
		prices := make([]float64, 15)
		for ii := range prices {
			prices[ii] = float64(10 + ii)
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

		result := signal.RSI(ctx, uu, portfolio.Days(14))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.RSISignal)).To(BeNumerically("~", 100.0, 1e-10))
	})

	It("returns RSI of 0 when all changes are losses", func() {
		// Monotonically decreasing prices: all changes are negative, avgGain=0, RSI=0.
		prices := make([]float64, 15)
		for ii := range prices {
			prices[ii] = float64(24 - ii)
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

		result := signal.RSI(ctx, uu, portfolio.Days(14))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.RSISignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("uses custom metric when provided", func() {
		prices := []float64{44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84, 46.08, 45.89, 46.03, 45.61, 46.28, 46.28}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, uu, portfolio.Days(14), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.RSISignal}))
		Expect(result.Value(aapl, signal.RSISignal)).To(BeNumerically("~", 72.984, 0.01))
	})

	It("returns error on degenerate window (fewer than 3 rows)", func() {
		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{{100, 110}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, uu, portfolio.Days(1))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("timeout")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, uu, portfolio.Days(14))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("timeout"))
	})
})
