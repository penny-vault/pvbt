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

			// Column-major layout: each inner slice is one column
			values := [][]float64{
				{100.0, 101.0}, // SPY close
				{99.0, 100.0},  // SPY adj_close
				{50.0, 51.0},   // TLT close
				{49.0, 50.0},   // TLT adj_close
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

	Describe("index member recording", func() {
		It("records index members from IndexMembers() call", func() {
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			members := []asset.Asset{spy}
			constituents := []data.IndexConstituent{
				{Asset: spy, Weight: 0.5},
			}

			nyc, _ := time.LoadLocation("America/New_York")
			date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

			stub := &stubIndexProvider{members: members, constituents: constituents}

			var err error
			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				IndexProvider: stub,
				AssetProvider: &stubAssetProvider{assets: members},
			})
			Expect(err).NotTo(HaveOccurred())

			resultAssets, resultConstituents, err := recorder.IndexMembers(ctx, "SP500", date)
			Expect(err).NotTo(HaveOccurred())
			Expect(resultAssets).To(Equal(members))
			Expect(resultConstituents).To(Equal(constituents))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM index_members").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))

			var weight float64
			Expect(db.QueryRow("SELECT weight FROM index_members LIMIT 1").Scan(&weight)).To(Succeed())
			Expect(weight).To(BeNumerically("~", 0.5, 0.001))
		})
	})

	Describe("rating recording", func() {
		It("records rated assets from RatedAssets() call", func() {
			rated := []asset.Asset{
				{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"},
			}

			nyc, _ := time.LoadLocation("America/New_York")
			date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

			stub := &stubRatingProvider{assets: rated}

			var err error
			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				RatingProvider: stub,
				AssetProvider:  &stubAssetProvider{assets: rated},
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := recorder.RatedAssets(ctx, "morningstar", data.RatingEq(5), date)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(rated))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM ratings").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))
		})
	})

	Describe("nil provider handling", func() {
		It("returns empty slice for IndexMembers when no IndexProvider", func() {
			var err error
			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: &stubAssetProvider{},
			})
			Expect(err).NotTo(HaveOccurred())

			nyc, _ := time.LoadLocation("America/New_York")
			resultAssets, resultConstituents, err := recorder.IndexMembers(ctx, "SP500", time.Date(2024, 1, 2, 16, 0, 0, 0, nyc))
			Expect(err).NotTo(HaveOccurred())
			Expect(resultAssets).To(BeNil())
			Expect(resultConstituents).To(BeNil())
		})

		It("returns empty slice for RatedAssets when no RatingProvider", func() {
			var err error
			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: &stubAssetProvider{},
			})
			Expect(err).NotTo(HaveOccurred())

			nyc, _ := time.LoadLocation("America/New_York")
			result, err := recorder.RatedAssets(ctx, "morningstar", data.RatingEq(5), time.Date(2024, 1, 2, 16, 0, 0, 0, nyc))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
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

type stubIndexProvider struct {
	members      []asset.Asset
	constituents []data.IndexConstituent
}

func (s *stubIndexProvider) IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, []data.IndexConstituent, error) {
	return s.members, s.constituents, nil
}

type stubRatingProvider struct {
	assets []asset.Asset
}

func (s *stubRatingProvider) RatedAssets(ctx context.Context, analyst string, filter data.RatingFilter, t time.Time) ([]asset.Asset, error) {
	return s.assets, nil
}
