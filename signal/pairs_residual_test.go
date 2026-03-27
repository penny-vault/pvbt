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

var _ = Describe("PairsResidual", func() {
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

	It("produces a z-score of residuals against a single reference", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		// AAPL tracks SPY closely at first, then diverges up.
		aaplPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		for ii := range 20 {
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			aaplPrices[ii] = 100.0 + float64(ii)*0.5
		}

		aaplPrices[19] = 130.0 // Diverge up at the end.

		aaplDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		Expect(err).NotTo(HaveOccurred())

		spyDF, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{"PairsResidual_SPY"}))

		// AAPL diverged up from SPY, so the residual z-score should be positive.
		Expect(result.Value(aapl, "PairsResidual_SPY")).To(BeNumerically(">", 0))
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
			aaplPrices[ii] = 100.0 + float64(ii)*1.0
			spyPrices[ii] = 100.0 + float64(ii)*0.8
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

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		metricList := result.MetricList()
		Expect(metricList).To(HaveLen(2))
		Expect(metricList).To(ContainElement(data.Metric("PairsResidual_SPY")))
		Expect(metricList).To(ContainElement(data.Metric("PairsResidual_EFA")))
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
			aaplPrices[ii] = 100.0 + float64(ii)*0.5
			msftPrices[ii] = 80.0 + float64(ii)*0.5
		}

		aaplPrices[19] = 130.0 // AAPL diverges up.
		msftPrices[19] = 70.0  // MSFT diverges down.

		primaryDF, err := data.NewDataFrame(times, []asset.Asset{aapl, msft}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices, msftPrices})
		Expect(err).NotTo(HaveOccurred())

		spyDF, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: primaryDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl, msft}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())

		// AAPL diverged up => positive; MSFT diverged down => negative.
		Expect(result.Value(aapl, "PairsResidual_SPY")).To(BeNumerically(">", 0))
		Expect(result.Value(msft, "PairsResidual_SPY")).To(BeNumerically("<", 0))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		aaplDF, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})
		spyDF, _ := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(0), refU)
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates primary fetch error", func() {
		ds := &errorDataSource{err: errors.New("primary unavailable")}
		refDF, _ := data.NewDataFrame([]time.Time{now}, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})
		refDS := &mockDataSource{currentDate: now, fetchResult: refDF}

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("primary unavailable"))
	})

	It("propagates reference fetch error", func() {
		aaplDF, _ := data.NewDataFrame([]time.Time{now}, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})
		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &errorDataSource{err: errors.New("ref unavailable")}

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("ref unavailable"))
	})
})
