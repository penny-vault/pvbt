package broker_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// mockDataFetcher returns pre-configured intraday DataFrames.
type mockDataFetcher struct {
	df  *data.DataFrame
	err error
}

func (mf *mockDataFetcher) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return mf.df, mf.err
}

var _ = Describe("VWAPFill", func() {
	var (
		aapl asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		date = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("returns typical price (H+L+C)/3 when no DataFetcher is set", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{
			data.MetricHigh:  155.0,
			data.MetricLow:   145.0,
			data.MetricClose: 150.0,
		})
		model := broker.VWAP()

		result, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(150.0)) // (155+145+150)/3 = 150
		Expect(result.Quantity).To(Equal(100.0))
	})

	It("returns typical price when DataFetcher returns an error", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{
			data.MetricHigh:  155.0,
			data.MetricLow:   145.0,
			data.MetricClose: 150.0,
		})
		model := broker.VWAP()
		model.(broker.DataFetcherAware).SetDataFetcher(&mockDataFetcher{
			err: fmt.Errorf("no intraday data"),
		})

		result, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(150.0))
	})

	It("returns an error when no OHLC data is available", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{})
		model := broker.VWAP()

		_, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).To(HaveOccurred())
	})

	It("computes true VWAP from intraday bars when DataFetcher provides them", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{
			data.MetricHigh:  155.0,
			data.MetricLow:   145.0,
			data.MetricClose: 150.0,
		})

		// Build a 3-row intraday DataFrame with High, Low, Close, Volume.
		// Columns are ordered: for 1 asset with metrics [High, Low, Close, Volume]
		// column indices: 0=High, 1=Low, 2=Close, 3=Volume
		times := []time.Time{
			time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC),
			time.Date(2025, 6, 15, 14, 1, 0, 0, time.UTC),
			time.Date(2025, 6, 15, 14, 2, 0, 0, time.UTC),
		}
		assets := []asset.Asset{aapl}
		metrics := []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}

		// Bar 1: typical = (102+98+100)/3 = 100.0, volume = 1000
		// Bar 2: typical = (204+196+200)/3 = 200.0, volume = 2000
		// Bar 3: typical = (153+147+150)/3 = 150.0, volume = 3000
		// VWAP = (100*1000 + 200*2000 + 150*3000) / (1000+2000+3000)
		//      = (100000 + 400000 + 450000) / 6000
		//      = 950000 / 6000
		//      = 158.333...
		columns := [][]float64{
			{102.0, 204.0, 153.0},    // High
			{98.0, 196.0, 147.0},     // Low
			{100.0, 200.0, 150.0},    // Close
			{1000.0, 2000.0, 3000.0}, // Volume
		}

		intradayDF, err := data.NewDataFrame(times, assets, metrics, data.Tick, columns)
		Expect(err).NotTo(HaveOccurred())

		model := broker.VWAP()
		model.(broker.DataFetcherAware).SetDataFetcher(&mockDataFetcher{
			df: intradayDF,
		})

		result, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 50}, bar)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(BeNumerically("~", 158.3333, 0.001))
		Expect(result.Quantity).To(Equal(50.0))
	})

	It("falls back to typical price when intraday data has zero total volume", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{
			data.MetricHigh:  155.0,
			data.MetricLow:   145.0,
			data.MetricClose: 150.0,
		})

		times := []time.Time{
			time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC),
		}
		assets := []asset.Asset{aapl}
		metrics := []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}
		columns := [][]float64{
			{102.0}, // High
			{98.0},  // Low
			{100.0}, // Close
			{0.0},   // Volume = 0
		}

		intradayDF, err := data.NewDataFrame(times, assets, metrics, data.Tick, columns)
		Expect(err).NotTo(HaveOccurred())

		model := broker.VWAP()
		model.(broker.DataFetcherAware).SetDataFetcher(&mockDataFetcher{
			df: intradayDF,
		})

		result, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(150.0)) // fallback to daily typical price
	})
})
