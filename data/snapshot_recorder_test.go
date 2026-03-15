package data_test

import (
	"context"
	"database/sql"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"

	_ "modernc.org/sqlite"
)

var _ = Describe("SnapshotRecorder", func() {
	var (
		ctx      context.Context
		recorder *data.SnapshotRecorder
		dbPath   string
	)

	BeforeEach(func() {
		ctx = context.Background()
		tmpDir := GinkgoT().TempDir()
		dbPath = tmpDir + "/test-snapshot.db"
	})

	AfterEach(func() {
		if recorder != nil {
			Expect(recorder.Close()).To(Succeed())
		}
	})

	Describe("asset recording", func() {
		It("records assets from Assets() call", func() {
			stubAssets := []asset.Asset{
				{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"},
				{CompositeFigi: "BBG000BHTK15", Ticker: "TLT"},
			}
			stub := &stubAssetProvider{assets: stubAssets}

			var err error
			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: stub,
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := recorder.Assets(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(stubAssets))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			// Verify data was written to SQLite.
			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM assets").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(2))
		})

		It("records asset from LookupAsset() call", func() {
			expected := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			stub := &stubAssetProvider{lookupResult: expected}

			var err error
			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: stub,
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := recorder.LookupAsset(ctx, "SPY")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(expected))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM assets").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))
		})
	})

	Describe("batch data recording", func() {
		It("records eod data from Fetch() call", func() {
			// Build a DataFrame with 2 assets, 2 dates, 2 eod metrics.
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			tlt := asset.Asset{CompositeFigi: "BBG000BHTK15", Ticker: "TLT"}
			assets := []asset.Asset{spy, tlt}
			metrics := []data.Metric{data.MetricClose, data.AdjClose}

			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			times := []time.Time{
				time.Date(2024, 1, 2, 16, 0, 0, 0, nyc),
				time.Date(2024, 1, 3, 16, 0, 0, 0, nyc),
			}

			// Column-major layout: [spy_close_t0, spy_close_t1, spy_adjclose_t0, spy_adjclose_t1,
			//                       tlt_close_t0, tlt_close_t1, tlt_adjclose_t0, tlt_adjclose_t1]
			values := []float64{
				100.0, 101.0, 99.0, 100.0,
				50.0, 51.0, 49.0, 50.0,
			}

			df, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			stub := data.NewTestProvider(metrics, df)

			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := recorder.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   metrics,
				Start:     times[0],
				End:       times[1],
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM eod").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(4)) // 2 assets * 2 dates

			Expect(db.QueryRow("SELECT count(*) FROM assets").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(2))
		})
	})
})

// -- stubs --

type stubAssetProvider struct {
	assets       []asset.Asset
	lookupResult asset.Asset
}

func (s *stubAssetProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
	return s.assets, nil
}

func (s *stubAssetProvider) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	return s.lookupResult, nil
}
