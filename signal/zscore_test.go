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

var _ = Describe("ZScore", func() {
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

	It("computes hand-calculated z-score correctly", func() {
		// Prices: 100, 102, 98, 104, 106
		// Mean = (100+102+98+104+106)/5 = 102
		// Variance = ((100-102)^2 + (102-102)^2 + (98-102)^2 + (104-102)^2 + (106-102)^2) / 5
		//          = (4 + 0 + 16 + 4 + 16) / 5 = 8
		// StdDev = sqrt(8) = 2.8284...
		// Z = (106 - 102) / 2.8284... = 1.4142...
		prices := []float64{100, 102, 98, 104, 106}
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.ZScoreSignal}))
		Expect(result.Value(aapl, signal.ZScoreSignal)).To(BeNumerically("~", math.Sqrt(2), 1e-10))
	})

	It("computes independently per asset", func() {
		// AAPL: 100, 100, 100, 100, 110 => mean=102, stddev>0, z>0
		// GOOG: 200, 200, 200, 200, 190 => mean=198, stddev>0, z<0
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		vals := [][]float64{
			{100, 100, 100, 100, 110},
			{200, 200, 200, 200, 190},
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.ZScoreSignal)).To(BeNumerically(">", 0))
		Expect(result.Value(goog, signal.ZScoreSignal)).To(BeNumerically("<", 0))
	})

	It("uses custom metric when provided", func() {
		prices := []float64{50, 60, 70}
		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(2), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.ZScoreSignal)).To(BeNumerically(">", 0))
	})

	It("returns error on constant price series (zero stddev)", func() {
		prices := []float64{100, 100, 100}
		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("zero"))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(10))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
