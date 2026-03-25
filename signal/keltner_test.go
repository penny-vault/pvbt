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

var _ = Describe("KeltnerChannels", func() {
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

	It("computes hand-calculated Keltner Channels for multiple assets", func() {
		// AAPL: Close=[100,102,101,104,103], High=[101,103,102,105,104], Low=[99,101,100,103,102]
		//   EMA(5) of Close: seed = SMA = (100+102+101+104+103)/5 = 102.0
		//   Only 5 points so EMA = 102.0
		//   TR from row 1: [3, 2, 4, 2], atrPeriod=4, ATR = (3+2+4+2)/4 = 2.75
		//   Upper = 102.0 + 2*2.75 = 107.5, Lower = 102.0 - 2*2.75 = 96.5
		//
		// GOOG: Close=[200,204,202,208,206], High=[202,206,204,210,208], Low=[198,202,200,206,204]
		//   EMA(5) of Close: seed = SMA = (200+204+202+208+206)/5 = 204.0
		//   Only 5 points so EMA = 204.0
		//   TR from row 1: [6, 4, 8, 4], atrPeriod=4, ATR = (6+4+8+4)/4 = 5.5
		//   Upper = 204.0 + 2*5.5 = 215.0, Lower = 204.0 - 2*5.5 = 193.0
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{
			{101, 103, 102, 105, 104},  // AAPL High
			{99, 101, 100, 103, 102},   // AAPL Low
			{100, 102, 101, 104, 103},  // AAPL Close
			{202, 206, 204, 210, 208},  // GOOG High
			{198, 202, 200, 206, 204},  // GOOG Low
			{200, 204, 202, 208, 206},  // GOOG Close
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
			data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.KeltnerChannels(ctx, uu, portfolio.Days(4), 2.0)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(ConsistOf(
			signal.KeltnerUpperSignal,
			signal.KeltnerMiddleSignal,
			signal.KeltnerLowerSignal,
		))

		Expect(result.Value(aapl, signal.KeltnerMiddleSignal)).To(BeNumerically("~", 102.0, 1e-10))
		Expect(result.Value(aapl, signal.KeltnerUpperSignal)).To(BeNumerically("~", 107.5, 1e-10))
		Expect(result.Value(aapl, signal.KeltnerLowerSignal)).To(BeNumerically("~", 96.5, 1e-10))

		Expect(result.Value(goog, signal.KeltnerMiddleSignal)).To(BeNumerically("~", 204.0, 1e-10))
		Expect(result.Value(goog, signal.KeltnerUpperSignal)).To(BeNumerically("~", 215.0, 1e-10))
		Expect(result.Value(goog, signal.KeltnerLowerSignal)).To(BeNumerically("~", 193.0, 1e-10))
	})

	It("uses custom metric for center line when provided", func() {
		// Use AdjClose instead of Close for the EMA center line.
		// ATR still uses High/Low/Close.
		// AdjClose=[90,92,91,94,93] differs from Close=[100,102,101,104,103].
		// EMA(5) of AdjClose: SMA seed = (90+92+91+94+93)/5 = 92.0.
		// Only 5 points so EMA = 92.0.
		adjCloses := []float64{90, 92, 91, 94, 93}
		closes := []float64{100, 102, 101, 104, 103}
		highs := []float64{101, 103, 102, 105, 104}
		lows := []float64{99, 101, 100, 103, 102}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{highs, lows, closes, adjCloses}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.AdjClose},
			data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.KeltnerChannels(ctx, uu, portfolio.Days(4), 2.0, data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		// Center line must reflect AdjClose EMA (92.0), not Close EMA (102.0).
		Expect(result.Value(aapl, signal.KeltnerMiddleSignal)).To(BeNumerically("~", 92.0, 1e-10))
	})

	It("returns error on degenerate window (fewer than 2 rows)", func() {
		times := []time.Time{now}
		vals := [][]float64{{101}, {99}, {100}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
			data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.KeltnerChannels(ctx, uu, portfolio.Days(0), 2.0)
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.KeltnerChannels(ctx, uu, portfolio.Days(4), 2.0)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
