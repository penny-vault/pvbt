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

var _ = Describe("Volatility", func() {
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

	It("computes rolling std dev of returns", func() {
		// Prices: 100, 110, 105, 115, 120
		// Returns (Pct(1)): NaN, 0.10, -0.04545, 0.09524, 0.04348
		// After Drop(NaN): 0.10, -0.04545, 0.09524, 0.04348
		// Std of those 4 returns (sample, N-1)
		times := make([]time.Time, 5)
		for i := range times {
			times[i] = now.AddDate(0, 0, i-4)
		}
		vals := [][]float64{{100, 110, 105, 115, 120}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Volatility(ctx, u, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.VolatilitySignal}))

		// Hand-compute expected std dev.
		returns := []float64{
			(110.0 - 100.0) / 100.0,
			(105.0 - 110.0) / 110.0,
			(115.0 - 105.0) / 105.0,
			(120.0 - 115.0) / 115.0,
		}
		mean := 0.0
		for _, r := range returns {
			mean += r
		}
		mean /= float64(len(returns))
		variance := 0.0
		for _, r := range returns {
			d := r - mean
			variance += d * d
		}
		variance /= float64(len(returns) - 1)
		expectedStd := math.Sqrt(variance)

		Expect(result.Value(aapl, signal.VolatilitySignal)).To(BeNumerically("~", expectedStd, 1e-10))
	})

	It("uses custom metric when provided", func() {
		times := make([]time.Time, 4)
		for i := range times {
			times[i] = now.AddDate(0, 0, i-3)
		}
		vals := [][]float64{{50, 55, 52, 58}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Volatility(ctx, u, portfolio.Days(3), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.VolatilitySignal}))
	})

	It("returns error on degenerate window (fewer than 3 rows)", func() {
		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{{100, 110}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Volatility(ctx, u, portfolio.Days(1))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("timeout")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Volatility(ctx, u, portfolio.Days(10))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("timeout"))
	})
})
