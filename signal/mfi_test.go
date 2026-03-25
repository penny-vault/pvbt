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

var _ = Describe("MFI", func() {
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

	It("computes hand-calculated MFI correctly with period=3", func() {
		// 4 bars total (period+1 so first bar is the baseline for TP comparison).
		// Bar 0: H=12, L=8,  C=10, V=1000 => TP=10.0         (baseline)
		// Bar 1: H=14, L=10, C=13, V=2000 => TP=12.333..     (up) posFlow += TP*V
		// Bar 2: H=13, L=9,  C=11, V=1500 => TP=11.0         (down) negFlow += TP*V
		// Bar 3: H=15, L=11, C=14, V=1800 => TP=13.333..     (up) posFlow += TP*V
		highs := []float64{12, 14, 13, 15}
		lows := []float64{8, 10, 9, 11}
		closes := []float64{10, 13, 11, 14}
		volumes := []float64{1000, 2000, 1500, 1800}

		times := make([]time.Time, 4)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-3)
		}

		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.MFISignal}))

		tp0 := (highs[0] + lows[0] + closes[0]) / 3.0
		tp1 := (highs[1] + lows[1] + closes[1]) / 3.0
		tp2 := (highs[2] + lows[2] + closes[2]) / 3.0
		tp3 := (highs[3] + lows[3] + closes[3]) / 3.0

		posFlow := 0.0
		negFlow := 0.0

		if tp1 > tp0 {
			posFlow += tp1 * volumes[1]
		} else {
			negFlow += tp1 * volumes[1]
		}

		if tp2 > tp1 {
			posFlow += tp2 * volumes[2]
		} else {
			negFlow += tp2 * volumes[2]
		}

		if tp3 > tp2 {
			posFlow += tp3 * volumes[3]
		} else {
			negFlow += tp3 * volumes[3]
		}

		ratio := posFlow / negFlow
		expected := 100.0 - 100.0/(1.0+ratio)

		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", expected, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// AAPL: rising TP across 3 bars => MFI = 100
		aaplHighs := []float64{10, 12, 14}
		aaplLows := []float64{8, 10, 12}
		aaplCloses := []float64{9, 11, 13}
		aaplVolumes := []float64{100, 200, 300}

		// MSFT: falling TP across 3 bars => MFI = 0
		msftHighs := []float64{14, 12, 10}
		msftLows := []float64{12, 10, 8}
		msftCloses := []float64{13, 11, 9}
		msftVolumes := []float64{300, 200, 100}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}

		vals := [][]float64{
			aaplHighs, aaplLows, aaplCloses, aaplVolumes,
			msftHighs, msftLows, msftCloses, msftVolumes,
		}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", 100.0, 1e-10))
		Expect(result.Value(msft, signal.MFISignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("returns MFI=100 when all flows are positive", func() {
		// Monotonically rising TP => all money flow positive.
		highs := []float64{10, 12, 14}
		lows := []float64{8, 10, 12}
		closes := []float64{9, 11, 13}
		volumes := []float64{100, 200, 300}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}

		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", 100.0, 1e-10))
	})

	It("returns MFI=0 when all flows are negative", func() {
		// Monotonically falling TP => all money flow negative.
		highs := []float64{14, 12, 10}
		lows := []float64{12, 10, 8}
		closes := []float64{13, 11, 9}
		volumes := []float64{300, 200, 100}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}

		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("computes correctly with minimum data (period=1, 2 bars)", func() {
		// period=1 fetches 2 bars; rising TP => MFI=100.
		highs := []float64{10, 12}
		lows := []float64{8, 9}
		closes := []float64{9, 11}
		volumes := []float64{1000, 2000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(1))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", 100.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
