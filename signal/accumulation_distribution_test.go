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

var _ = Describe("AccumulationDistribution", func() {
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

	It("computes hand-calculated A/D correctly", func() {
		// H=[12,15,14], L=[8,10,11], C=[10,14,12], V=[1000,2000,1500]
		// Bar 0: MFM = ((10-8)-(12-10))/(12-8) = (2-2)/4 = 0, MFV = 0
		// Bar 1: MFM = ((14-10)-(15-14))/(15-10) = (4-1)/5 = 0.6, MFV = 0.6*2000 = 1200
		// Bar 2: MFM = ((12-11)-(14-12))/(14-11) = (1-2)/3 = -1/3, MFV = -1/3*1500 = -500
		// A/D = 0 + 1200 + (-500) = 700
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

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.AccumulationDistributionSignal}))
		Expect(result.Value(aapl, signal.AccumulationDistributionSignal)).To(BeNumerically("~", 700.0, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// AAPL: H=[10,12], L=[8,9], C=[9,11], V=[1000,2000]
		// Bar 0: MFM=((9-8)-(10-9))/(10-8)=0/2=0, MFV=0
		// Bar 1: MFM=((11-9)-(12-11))/(12-9)=(2-1)/3=1/3, MFV=1/3*2000=666.67
		// A/D AAPL ~ 666.67

		// MSFT: H=[50,55], L=[45,48], C=[47,54], V=[3000,4000]
		// Bar 0: MFM=((47-45)-(50-47))/(50-45)=(2-3)/5=-0.2, MFV=-0.2*3000=-600
		// Bar 1: MFM=((54-48)-(55-54))/(55-48)=(6-1)/7=5/7, MFV=5/7*4000~2857.14
		// A/D MSFT ~ -600 + 2857.14 ~ 2257.14

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

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplAD := result.Value(aapl, signal.AccumulationDistributionSignal)
		msftAD := result.Value(msft, signal.AccumulationDistributionSignal)
		Expect(aaplAD).NotTo(BeNumerically("~", msftAD, 1.0))
		Expect(aaplAD).To(BeNumerically("~", 2000.0/3.0, 1e-6))
		Expect(msftAD).To(BeNumerically("~", -600.0+20000.0/7.0, 1e-6))
	})

	It("yields MFM=0 and A/D=0 when High equals Low", func() {
		highs := []float64{10, 10, 10}
		lows := []float64{10, 10, 10}
		closes := []float64{10, 10, 10}
		volumes := []float64{500, 1000, 750}

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

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.AccumulationDistributionSignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("computes correctly with minimum data (2 rows)", func() {
		// H=[10,12], L=[8,8], C=[9,11], V=[1000,2000]
		// Bar 0: MFM=((9-8)-(10-9))/(10-8)=(1-1)/2=0, MFV=0
		// Bar 1: MFM=((11-8)-(12-11))/(12-8)=(3-1)/4=0.5, MFV=0.5*2000=1000
		// A/D = 1000
		highs := []float64{10, 12}
		lows := []float64{8, 8}
		closes := []float64{9, 11}
		volumes := []float64{1000, 2000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.AccumulationDistributionSignal)).To(BeNumerically("~", 1000.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
