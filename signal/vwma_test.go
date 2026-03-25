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

var _ = Describe("VWMA", func() {
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

	It("computes hand-calculated VWMA correctly", func() {
		// closes=[10,12,14], volumes=[100,200,300]
		// VWMA = (10*100 + 12*200 + 14*300) / (100+200+300)
		//      = (1000 + 2400 + 4200) / 600
		//      = 7600 / 600 = 12.666...
		closes := []float64{10, 12, 14}
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

		result := signal.VWMA(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.VWMASignal}))
		Expect(result.Value(aapl, signal.VWMASignal)).To(BeNumerically("~", 7600.0/600.0, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// AAPL: (10*100 + 20*100) / (100+100) = 3000/200 = 15
		// MSFT: (50*300 + 60*100) / (300+100) = (15000+6000)/400 = 21000/400 = 52.5
		aaplCloses := []float64{10, 20}
		aaplVolumes := []float64{100, 100}
		msftCloses := []float64{50, 60}
		msftVolumes := []float64{300, 100}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplCloses, aaplVolumes, msftCloses, msftVolumes}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.VWMASignal)).To(BeNumerically("~", 15.0, 1e-10))
		Expect(result.Value(msft, signal.VWMASignal)).To(BeNumerically("~", 52.5, 1e-10))
	})

	It("returns NaN when total volume is zero", func() {
		closes := []float64{10, 12}
		volumes := []float64{0, 0}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.VWMASignal))).To(BeTrue())
	})

	It("computes correctly with minimum data", func() {
		// 1 row: close=42, volume=1000 => VWMA = 42*1000/1000 = 42
		closes := []float64{42}
		volumes := []float64{1000}

		times := []time.Time{now}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(1))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.VWMASignal)).To(BeNumerically("~", 42.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
