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

var _ = Describe("OBV", func() {
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

	It("computes hand-calculated OBV correctly", func() {
		// Day 0: close=10, vol=100 (baseline, no comparison)
		// Day 1: close=12 (up),   vol=150 => OBV = 0 + 150 = 150
		// Day 2: close=11 (down), vol=130 => OBV = 150 - 130 = 20
		// Day 3: close=11 (flat), vol=90  => OBV = 20 + 0 = 20
		// Day 4: close=13 (up),   vol=200 => OBV = 20 + 200 = 220
		closes := []float64{10, 12, 11, 11, 13}
		volumes := []float64{100, 150, 130, 90, 200}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.OBVSignal}))
		Expect(result.Value(aapl, signal.OBVSignal)).To(BeNumerically("~", 220.0, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// AAPL: close goes up then down => OBV = 200 - 100 = 100
		// MSFT: close goes down then up => OBV = -500 + 600 = 100
		aaplCloses := []float64{10, 12, 11}
		aaplVolumes := []float64{300, 200, 100}
		msftCloses := []float64{50, 48, 52}
		msftVolumes := []float64{400, 500, 600}

		times := []time.Time{now.AddDate(0, 0, -2), now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplCloses, aaplVolumes, msftCloses, msftVolumes}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.OBVSignal)).To(BeNumerically("~", 100.0, 1e-10))
		Expect(result.Value(msft, signal.OBVSignal)).To(BeNumerically("~", 100.0, 1e-10))
	})

	It("computes correctly with minimum data", func() {
		// Exactly 2 rows -- the minimum for OBV.
		// Close goes up: OBV = +500.
		closes := []float64{10, 12}
		volumes := []float64{300, 500}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.OBVSignal)).To(BeNumerically("~", 500.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("handles flat prices", func() {
		closes := []float64{10, 10, 10}
		volumes := []float64{100, 200, 300}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		// Flat prices: no volume added or subtracted.
		Expect(result.Value(aapl, signal.OBVSignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
