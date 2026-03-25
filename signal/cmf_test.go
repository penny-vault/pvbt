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

var _ = Describe("CMF", func() {
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

	It("computes hand-calculated CMF correctly", func() {
		// H=[12,15,14], L=[8,10,11], C=[10,14,12], V=[1000,2000,1500]
		// Bar 0: MFM=((10-8)-(12-10))/(12-8)=(2-2)/4=0,    MFV=0
		// Bar 1: MFM=((14-10)-(15-14))/(15-10)=(4-1)/5=0.6, MFV=0.6*2000=1200
		// Bar 2: MFM=((12-11)-(14-12))/(14-11)=(1-2)/3=-1/3, MFV=-1/3*1500=-500
		// CMF = (0+1200-500)/(1000+2000+1500) = 700/4500 = 7/45
		highs := []float64{12, 15, 14}
		lows := []float64{8, 10, 11}
		closes := []float64{10, 14, 12}
		volumes := []float64{1000, 2000, 1500}

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

		result := signal.CMF(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.CMFSignal}))
		Expect(result.Value(aapl, signal.CMFSignal)).To(BeNumerically("~", 700.0/4500.0, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// AAPL: H=[10,12], L=[8,9], C=[9,11], V=[1000,2000]
		// Bar 0: MFM=((9-8)-(10-9))/(10-8)=(1-1)/2=0, MFV=0
		// Bar 1: MFM=((11-9)-(12-11))/(12-9)=(2-1)/3=1/3, MFV=1/3*2000=2000/3
		// CMF AAPL = (2000/3) / (1000+2000) = 2000/(3*3000) = 2/9

		// MSFT: H=[50,55], L=[45,48], C=[47,54], V=[3000,4000]
		// Bar 0: MFM=((47-45)-(50-47))/(50-45)=(2-3)/5=-0.2, MFV=-0.2*3000=-600
		// Bar 1: MFM=((54-48)-(55-54))/(55-48)=(6-1)/7=5/7, MFV=5/7*4000=20000/7
		// CMF MSFT = (-600+20000/7) / (3000+4000) = (-4200/7+20000/7)/7000 = (15800/7)/7000

		aaplHighs := []float64{10, 12}
		aaplLows := []float64{8, 9}
		aaplCloses := []float64{9, 11}
		aaplVolumes := []float64{1000, 2000}

		msftHighs := []float64{50, 55}
		msftLows := []float64{45, 48}
		msftCloses := []float64{47, 54}
		msftVolumes := []float64{3000, 4000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{
			aaplHighs, aaplLows, aaplCloses, aaplVolumes,
			msftHighs, msftLows, msftCloses, msftVolumes,
		}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplCMF := result.Value(aapl, signal.CMFSignal)
		msftCMF := result.Value(msft, signal.CMFSignal)
		Expect(aaplCMF).NotTo(BeNumerically("~", msftCMF, 1e-6))
		Expect(aaplCMF).To(BeNumerically("~", 2.0/9.0, 1e-10))
		Expect(msftCMF).To(BeNumerically("~", (15800.0/7.0)/7000.0, 1e-10))
	})

	It("returns NaN when total volume is zero", func() {
		highs := []float64{10, 12}
		lows := []float64{8, 9}
		closes := []float64{9, 11}
		volumes := []float64{0, 0}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.CMFSignal))).To(BeTrue())
	})

	It("computes correctly with minimum data (2 rows)", func() {
		// H=[10,12], L=[8,8], C=[10,8], V=[1000,2000]
		// Bar 0: MFM=((10-8)-(10-10))/(10-8)=(2-0)/2=1.0, MFV=1.0*1000=1000
		// Bar 1: MFM=((8-8)-(12-8))/(12-8)=(0-4)/4=-1.0,  MFV=-1.0*2000=-2000
		// CMF = (1000-2000)/(1000+2000) = -1000/3000 = -1/3
		highs := []float64{10, 12}
		lows := []float64{8, 8}
		closes := []float64{10, 8}
		volumes := []float64{1000, 2000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.CMFSignal)).To(BeNumerically("~", -1.0/3.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
