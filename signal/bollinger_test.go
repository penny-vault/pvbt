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

var _ = Describe("BollingerBands", func() {
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

	It("computes correct upper, middle, and lower bands", func() {
		// AAPL prices: [10, 20, 30, 40, 50], mean=30
		// sample std = sqrt(sum((x-30)^2)/4) = sqrt((400+100+0+100+400)/4) = sqrt(250) ~= 15.8114
		// GOOG prices: [100, 200, 300, 400, 500], mean=300, std=sqrt(25000) ~= 158.114
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{
			{10, 20, 30, 40, 50},
			{100, 200, 300, 400, 500},
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.BollingerBands(ctx, uu, portfolio.Days(4), 2.0)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(ConsistOf(
			signal.BollingerUpperSignal,
			signal.BollingerMiddleSignal,
			signal.BollingerLowerSignal,
		))

		aaplStd := math.Sqrt(250.0)
		Expect(result.Value(aapl, signal.BollingerMiddleSignal)).To(BeNumerically("~", 30.0, 1e-10))
		Expect(result.Value(aapl, signal.BollingerUpperSignal)).To(BeNumerically("~", 30.0+2*aaplStd, 1e-10))
		Expect(result.Value(aapl, signal.BollingerLowerSignal)).To(BeNumerically("~", 30.0-2*aaplStd, 1e-10))

		googStd := math.Sqrt(25000.0)
		Expect(result.Value(goog, signal.BollingerMiddleSignal)).To(BeNumerically("~", 300.0, 1e-10))
		Expect(result.Value(goog, signal.BollingerUpperSignal)).To(BeNumerically("~", 300.0+2*googStd, 1e-10))
		Expect(result.Value(goog, signal.BollingerLowerSignal)).To(BeNumerically("~", 300.0-2*googStd, 1e-10))
	})

	It("uses custom metric when provided", func() {
		// AdjClose values: [10, 20, 30, 40], mean=25
		times := make([]time.Time, 4)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-3)
		}
		vals := [][]float64{{10, 20, 30, 40}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.BollingerBands(ctx, uu, portfolio.Days(3), 2.0, data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.BollingerMiddleSignal)).To(BeNumerically("~", 25.0, 1e-10))
	})

	It("returns error on degenerate window (fewer than 2 rows)", func() {
		times := []time.Time{now}
		vals := [][]float64{{100}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.BollingerBands(ctx, uu, portfolio.Days(0), 2.0)
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.BollingerBands(ctx, uu, portfolio.Days(10), 2.0)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
