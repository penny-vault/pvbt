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

var _ = Describe("ATR", func() {
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

	It("computes hand-calculated ATR correctly", func() {
		// H=[12,11,13,14,12], L=[9,8,10,11,9], C=[10,10,12,13,11]
		// TR from row 1:
		//   TR[0]=max(11-8, |11-10|, |8-10|) = max(3,1,2) = 3
		//   TR[1]=max(13-10, |13-10|, |10-10|) = max(3,3,0) = 3
		//   TR[2]=max(14-11, |14-12|, |11-12|) = max(3,2,1) = 3
		//   TR[3]=max(12-9, |12-13|, |9-13|) = max(3,1,4) = 4
		// atrPeriod=4, ATR = (3+3+3+4)/4 = 3.25
		highs := []float64{12, 11, 13, 14, 12}
		lows := []float64{9, 8, 10, 11, 9}
		closes := []float64{10, 10, 12, 13, 11}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		// Columns layout: [asset0/High, asset0/Low, asset0/Close]
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.ATRSignal}))
		Expect(result.Value(aapl, signal.ATRSignal)).To(BeNumerically("~", 3.25, 1e-10))
	})

	It("uses |high - prevClose| as TR when gap up dominates", func() {
		// Gap up: prevClose=10, high=16, low=14
		// TR = max(16-14, |16-10|, |14-10|) = max(2, 6, 4) = 6
		highs := []float64{10, 16}
		lows := []float64{8, 14}
		closes := []float64{10, 15}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, uu, portfolio.Days(1))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.ATRSignal)).To(BeNumerically("~", 6.0, 1e-10))
	})

	It("uses |low - prevClose| as TR when gap down dominates", func() {
		// Gap down: prevClose=20, high=14, low=13
		// TR = max(14-13, |14-20|, |13-20|) = max(1, 6, 7) = 7
		highs := []float64{20, 14}
		lows := []float64{18, 13}
		closes := []float64{20, 13}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, uu, portfolio.Days(1))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.ATRSignal)).To(BeNumerically("~", 7.0, 1e-10))
	})

	It("returns error on degenerate window (fewer than 2 rows)", func() {
		highs := []float64{12}
		lows := []float64{9}
		closes := []float64{10}

		times := []time.Time{now}
		vals := [][]float64{highs, lows, closes}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
