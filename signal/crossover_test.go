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

var _ = Describe("Crossover", func() {
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

	It("returns +1 signal in uptrend (fast SMA > slow SMA)", func() {
		// Prices: [10,20,30,40,50,60,70]
		// fast=Days(2): last 2 = [60,70], SMA=65
		// slow=Days(5): last 5 = [30,40,50,60,70], SMA=50
		// signal = +1
		times := make([]time.Time, 7)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-6)
		}
		vals := [][]float64{{10, 20, 30, 40, 50, 60, 70}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Crossover(ctx, uu, portfolio.Days(2), portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(ConsistOf(
			signal.CrossoverFastSignal,
			signal.CrossoverSlowSignal,
			signal.CrossoverSignal,
		))
		Expect(result.Value(aapl, signal.CrossoverFastSignal)).To(BeNumerically("~", 65.0, 1e-10))
		Expect(result.Value(aapl, signal.CrossoverSlowSignal)).To(BeNumerically("~", 50.0, 1e-10))
		Expect(result.Value(aapl, signal.CrossoverSignal)).To(BeNumerically("~", 1.0, 1e-10))
	})

	It("returns -1 signal in downtrend (fast SMA < slow SMA)", func() {
		// Prices: [70,60,50,40,30,20,10]
		// fast=Days(2): last 2 = [20,10], SMA=15
		// slow=Days(5): last 5 = [50,40,30,20,10], SMA=30
		// signal = -1
		times := make([]time.Time, 7)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-6)
		}
		vals := [][]float64{{70, 60, 50, 40, 30, 20, 10}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Crossover(ctx, uu, portfolio.Days(2), portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.CrossoverFastSignal)).To(BeNumerically("~", 15.0, 1e-10))
		Expect(result.Value(aapl, signal.CrossoverSlowSignal)).To(BeNumerically("~", 30.0, 1e-10))
		Expect(result.Value(aapl, signal.CrossoverSignal)).To(BeNumerically("~", -1.0, 1e-10))
	})

	It("uses custom metric when provided", func() {
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{{10, 20, 30, 40, 50}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Crossover(ctx, uu, portfolio.Days(2), portfolio.Days(4), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.CrossoverSignal)).To(BeNumerically("~", 1.0, 1e-10))
	})

	It("returns error on degenerate window (fewer than 2 rows)", func() {
		times := []time.Time{now}
		vals := [][]float64{{100}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Crossover(ctx, uu, portfolio.Days(1), portfolio.Days(1))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("connection refused")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Crossover(ctx, uu, portfolio.Days(2), portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("connection refused"))
	})
})
