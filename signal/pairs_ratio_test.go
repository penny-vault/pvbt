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

var _ = Describe("PairsRatio", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		msft asset.Asset
		spy  asset.Asset
		efa  asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		spy = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		efa = asset.Asset{CompositeFigi: "FIGI-EFA", Ticker: "EFA"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("produces a z-score of price ratio against a single reference", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		// AAPL and SPY track closely, then AAPL spikes at the end.
		aaplPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		for ii := range 20 {
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			aaplPrices[ii] = 150.0 + float64(ii)*0.75 // Ratio ~1.5 throughout.
		}

		aaplPrices[19] = 200.0 // Spike the ratio at the end.

		aaplDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		Expect(err).NotTo(HaveOccurred())

		spyDF, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{"PairsRatio_SPY"}))

		// Ratio spiked up at the end, so z-score should be positive.
		Expect(result.Value(aapl, "PairsRatio_SPY")).To(BeNumerically(">", 0))
	})

	It("produces metrics for multiple reference assets", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		aaplPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		efaPrices := make([]float64, 20)
		for ii := range 20 {
			aaplPrices[ii] = 150.0 + float64(ii)*1.0
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			efaPrices[ii] = 50.0 + float64(ii)*0.3
		}

		aaplDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		Expect(err).NotTo(HaveOccurred())

		refDF, err := data.NewDataFrame(times, []asset.Asset{spy, efa}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices, efaPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: refDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy, efa}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())

		metricList := result.MetricList()
		Expect(metricList).To(HaveLen(2))
		Expect(metricList).To(ContainElement(data.Metric("PairsRatio_SPY")))
		Expect(metricList).To(ContainElement(data.Metric("PairsRatio_EFA")))
	})

	It("computes independently per primary asset", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		aaplPrices := make([]float64, 20)
		msftPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		for ii := range 20 {
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			aaplPrices[ii] = 150.0 + float64(ii)*0.75
			msftPrices[ii] = 120.0 + float64(ii)*0.60
		}

		aaplPrices[19] = 200.0 // AAPL ratio spikes up.
		msftPrices[19] = 100.0 // MSFT ratio drops.

		primaryDF, err := data.NewDataFrame(times, []asset.Asset{aapl, msft}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices, msftPrices})
		Expect(err).NotTo(HaveOccurred())

		spyDF, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: primaryDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl, msft}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())

		Expect(result.Value(aapl, "PairsRatio_SPY")).To(BeNumerically(">", 0))
		Expect(result.Value(msft, "PairsRatio_SPY")).To(BeNumerically("<", 0))
	})

	It("returns error when reference price is zero", func() {
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		aaplPrices := []float64{100, 101, 102, 103, 104}
		spyPrices := []float64{100, 0, 102, 103, 104} // Zero price.

		aaplDF, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		spyDF, _ := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(4), refU)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("zero"))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		aaplDF, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})
		spyDF, _ := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(0), refU)
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		spyDF, _ := data.NewDataFrame([]time.Time{now}, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
