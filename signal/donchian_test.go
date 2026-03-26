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

var _ = Describe("DonchianChannels", func() {
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

	It("computes correct upper, middle, and lower channels", func() {
		// AAPL: H=[12,15,11,14,13], L=[9,10,8,11,10]
		//   Upper = max(H) = 15, Lower = min(L) = 8, Middle = (15+8)/2 = 11.5
		// GOOG: H=[120,150,110,140,130], L=[90,100,80,110,100]
		//   Upper = max(H) = 150, Lower = min(L) = 80, Middle = (150+80)/2 = 115
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{
			{12, 15, 11, 14, 13},       // AAPL High
			{9, 10, 8, 11, 10},         // AAPL Low
			{120, 150, 110, 140, 130},  // GOOG High
			{90, 100, 80, 110, 100},    // GOOG Low
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
			[]data.Metric{data.MetricHigh, data.MetricLow}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.DonchianChannels(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(ConsistOf(
			signal.DonchianUpperSignal,
			signal.DonchianMiddleSignal,
			signal.DonchianLowerSignal,
		))

		Expect(result.Value(aapl, signal.DonchianUpperSignal)).To(BeNumerically("~", 15.0, 1e-10))
		Expect(result.Value(aapl, signal.DonchianLowerSignal)).To(BeNumerically("~", 8.0, 1e-10))
		Expect(result.Value(aapl, signal.DonchianMiddleSignal)).To(BeNumerically("~", 11.5, 1e-10))

		Expect(result.Value(goog, signal.DonchianUpperSignal)).To(BeNumerically("~", 150.0, 1e-10))
		Expect(result.Value(goog, signal.DonchianLowerSignal)).To(BeNumerically("~", 80.0, 1e-10))
		Expect(result.Value(goog, signal.DonchianMiddleSignal)).To(BeNumerically("~", 115.0, 1e-10))
	})

	It("works with a single data point", func() {
		// A single row is valid: upper=high, lower=low, middle=midpoint.
		times := []time.Time{now}
		vals := [][]float64{{12}, {9}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.DonchianChannels(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.DonchianUpperSignal)).To(BeNumerically("~", 12.0, 1e-10))
		Expect(result.Value(aapl, signal.DonchianLowerSignal)).To(BeNumerically("~", 9.0, 1e-10))
		Expect(result.Value(aapl, signal.DonchianMiddleSignal)).To(BeNumerically("~", 10.5, 1e-10))
	})

	It("returns error when DataFrame is empty", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.DonchianChannels(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("DonchianChannels"))
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.DonchianChannels(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
