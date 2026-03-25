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

var _ = Describe("CCI", func() {
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

	It("computes hand-calculated CCI correctly", func() {
		highs := []float64{24, 25, 26}
		lows := []float64{22, 23, 22}
		closes := []float64{23, 24, 25}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.CCISignal}))

		tp2 := (26.0 + 22.0 + 25.0) / 3.0
		sma := (23.0 + 24.0 + tp2) / 3.0
		md := (math.Abs(23.0-sma) + math.Abs(24.0-sma) + math.Abs(tp2-sma)) / 3.0
		expected := (tp2 - sma) / (0.015 * md)
		Expect(result.Value(aapl, signal.CCISignal)).To(BeNumerically("~", expected, 1e-10))
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

		result := signal.CCI(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.CCISignal))).To(BeTrue())
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		aaplHighs := []float64{12, 14}
		aaplLows := []float64{9, 10}
		aaplCloses := []float64{10, 13}
		msftHighs := []float64{22, 20}
		msftLows := []float64{18, 17}
		msftCloses := []float64{20, 18}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, msftHighs, msftLows, msftCloses}
		df, err := data.NewDataFrame(times, assets, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplCCI := result.Value(aapl, signal.CCISignal)
		msftCCI := result.Value(msft, signal.CCISignal)
		Expect(aaplCCI).NotTo(Equal(msftCCI))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
