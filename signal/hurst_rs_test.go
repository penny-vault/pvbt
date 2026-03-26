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

var _ = Describe("HurstRS", func() {
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

	It("produces H > 0.5 for a trending series", func() {
		// Monotonically rising prices => strongly trending.
		prices := make([]float64, 64)
		for ii := range prices {
			prices[ii] = 100.0 + float64(ii)*2.0
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.HurstRSSignal}))
		Expect(result.Value(aapl, signal.HurstRSSignal)).To(BeNumerically(">", 0.5))
	})

	It("produces H < 0.5 for a mean-reverting series", func() {
		// Alternating up/down prices => mean-reverting.
		prices := make([]float64, 64)
		for ii := range prices {
			if ii%2 == 0 {
				prices[ii] = 100.0
			} else {
				prices[ii] = 110.0
			}
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.HurstRSSignal)).To(BeNumerically("<", 0.5))
	})

	It("produces value between 0 and 1", func() {
		// Arbitrary price series.
		prices := []float64{100, 103, 97, 105, 99, 108, 95, 110, 100, 102,
			98, 106, 94, 112, 101, 99, 107, 93, 111, 100,
			104, 96, 108, 92, 113, 101, 97, 109, 91, 114, 100, 105}

		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(len(prices)-1))
		Expect(result.Err()).NotTo(HaveOccurred())

		hh := result.Value(aapl, signal.HurstRSSignal)
		Expect(hh).To(BeNumerically(">=", 0))
		Expect(hh).To(BeNumerically("<=", 1))
	})

	It("computes independently per asset", func() {
		// AAPL trending, GOOG mean-reverting.
		aaplPrices := make([]float64, 64)
		googPrices := make([]float64, 64)
		for ii := range 64 {
			aaplPrices[ii] = 100.0 + float64(ii)*2.0
			if ii%2 == 0 {
				googPrices[ii] = 100.0
			} else {
				googPrices[ii] = 110.0
			}
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		vals := [][]float64{aaplPrices, googPrices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.HurstRSSignal)).To(BeNumerically(">", 0.5))
		Expect(result.Value(goog, signal.HurstRSSignal)).To(BeNumerically("<", 0.5))
	})

	It("returns error on insufficient data", func() {
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100, 101, 102, 103, 104}})
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		// 5 data points yields 4 returns, which is not enough for multiple sub-period sizes.
		result := signal.HurstRS(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
