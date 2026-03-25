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

var _ = Describe("StochasticFast", func() {
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

	It("computes hand-calculated fast stochastic correctly", func() {
		// 7-day data with 5-day %K period. Need period.N + 2 = 7 rows.
		// %K windows (5-day rolling):
		// Window [0-4]: HH=14, LL=8,  C=11 -> %K = (11-8)/(14-8)*100 = 50.0
		// Window [1-5]: HH=15, LL=8,  C=14 -> %K = (14-8)/(15-8)*100 = 600/7
		// Window [2-6]: HH=15, LL=9,  C=12 -> %K = (12-9)/(15-9)*100 = 50.0
		// %D = SMA of 3 %K = (50 + 600/7 + 50) / 3
		highs := []float64{12, 11, 13, 14, 12, 15, 13}
		lows := []float64{9, 8, 10, 11, 9, 10, 11}
		closes := []float64{10, 10, 12, 13, 11, 14, 12}

		times := make([]time.Time, 7)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-6)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		Expect(result.Value(aapl, signal.StochasticKSignal)).To(BeNumerically("~", 50.0, 1e-10))
		Expect(result.Value(aapl, signal.StochasticDSignal)).To(BeNumerically("~", (50.0+600.0/7.0+50.0)/3.0, 1e-10))
	})

	It("produces NaN for flat market", func() {
		highs := []float64{10, 10, 10, 10, 10}
		lows := []float64{10, 10, 10, 10, 10}
		closes := []float64{10, 10, 10, 10, 10}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.StochasticKSignal))).To(BeTrue())
		Expect(math.IsNaN(result.Value(aapl, signal.StochasticDSignal))).To(BeTrue())
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		aaplHighs := []float64{12, 14, 13, 15}
		aaplLows := []float64{9, 10, 11, 12}
		aaplCloses := []float64{11, 13, 12, 14}
		msftHighs := []float64{22, 20, 24, 21}
		msftLows := []float64{18, 17, 20, 18}
		msftCloses := []float64{20, 19, 23, 20}

		times := make([]time.Time, 4)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-3)
		}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, msftHighs, msftLows, msftCloses}
		df, err := data.NewDataFrame(times, assets, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplK := result.Value(aapl, signal.StochasticKSignal)
		msftK := result.Value(msft, signal.StochasticKSignal)
		Expect(aaplK).NotTo(Equal(msftK))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})

var _ = Describe("StochasticSlow", func() {
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

	It("computes hand-calculated slow stochastic correctly", func() {
		// 9-day data, period=3, smoothing=3.
		// Raw %K (3-day rolling, 7 values):
		// [0-2]: (12-8)/(13-8)*100=80, [1-3]: (13-8)/(14-8)*100=250/3,
		// [2-4]: (11-9)/(14-9)*100=40, [3-5]: (14-9)/(15-9)*100=250/3,
		// [4-6]: (12-9)/(15-9)*100=50, [5-7]: (15-10)/(16-10)*100=250/3,
		// [6-8]: (13-10)/(16-10)*100=50
		//
		// Slow %K (SMA of 3 raw %K, 5 values):
		// [0]=(80+250/3+40)/3, [1]=(250/3+40+250/3)/3, [2]=(40+250/3+50)/3,
		// [3]=(250/3+50+250/3)/3, [4]=(50+250/3+50)/3
		//
		// Slow %D = SMA of last 3 Slow %K = (SlowK[2]+SlowK[3]+SlowK[4])/3
		highs := []float64{12, 11, 13, 14, 12, 15, 13, 16, 14}
		lows := []float64{9, 8, 10, 11, 9, 10, 11, 12, 10}
		closes := []float64{10, 10, 12, 13, 11, 14, 12, 15, 13}

		times := make([]time.Time, 9)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-8)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		expectedSlowK := (50.0 + 250.0/3.0 + 50.0) / 3.0
		Expect(result.Value(aapl, signal.StochasticSlowKSignal)).To(BeNumerically("~", expectedSlowK, 1e-6))

		slowK2 := (40.0 + 250.0/3.0 + 50.0) / 3.0
		slowK3 := (250.0/3.0 + 50.0 + 250.0/3.0) / 3.0
		expectedSlowD := (slowK2 + slowK3 + expectedSlowK) / 3.0
		Expect(result.Value(aapl, signal.StochasticSlowDSignal)).To(BeNumerically("~", expectedSlowD, 1e-6))
	})

	It("works with non-default smoothing period", func() {
		highs := []float64{12, 11, 13, 14, 12, 15, 13, 16, 14, 13}
		lows := []float64{9, 8, 10, 11, 9, 10, 11, 12, 10, 9}
		closes := []float64{10, 10, 12, 13, 11, 14, 12, 15, 13, 11}

		times := make([]time.Time, 10)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-9)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		slowK := result.Value(aapl, signal.StochasticSlowKSignal)
		slowD := result.Value(aapl, signal.StochasticSlowDSignal)
		Expect(math.IsNaN(slowK)).To(BeFalse())
		Expect(math.IsNaN(slowD)).To(BeFalse())
		Expect(slowK).To(BeNumerically(">=", 0.0))
		Expect(slowK).To(BeNumerically("<=", 100.0))
	})

	It("produces NaN for flat market", func() {
		nn := 10
		highs := make([]float64, nn)
		lows := make([]float64, nn)
		closes := make([]float64, nn)
		for ii := range nn {
			highs[ii] = 10
			lows[ii] = 10
			closes[ii] = 10
		}

		times := make([]time.Time, nn)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-nn+1)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.StochasticSlowKSignal))).To(BeTrue())
		Expect(math.IsNaN(result.Value(aapl, signal.StochasticSlowDSignal))).To(BeTrue())
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
