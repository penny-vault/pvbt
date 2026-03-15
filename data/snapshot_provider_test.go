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

var _ = Describe("SnapshotProvider", func() {
	var (
		ctx    context.Context
		dbPath string
	)

	BeforeEach(func() {
		ctx = context.Background()
		dbPath = GinkgoT().TempDir() + "/test-snapshot.db"
	})

	// Helper: seed a snapshot database with known data.
	seedDB := func() {
		db, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.CreateSnapshotSchema(db)).To(Succeed())

		_, err = db.Exec("INSERT INTO assets (composite_figi, ticker) VALUES ('BBG000BLNNH6', 'SPY'), ('BBG000BHTK15', 'TLT')")
		Expect(err).NotTo(HaveOccurred())
		db.Close()
	}

	Describe("Assets", func() {
		It("returns all assets from the snapshot", func() {
			seedDB()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			assets, err := snap.Assets(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(assets).To(HaveLen(2))
			Expect(assets[0].Ticker).To(Equal("SPY"))
		})
	})

	Describe("LookupAsset", func() {
		It("finds an asset by ticker", func() {
			seedDB()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.LookupAsset(ctx, "SPY")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.CompositeFigi).To(Equal("BBG000BLNNH6"))
		})

		It("returns error for unknown ticker", func() {
			seedDB()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			_, err = snap.LookupAsset(ctx, "NOPE")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Fetch", func() {
		It("replays eod data as a DataFrame", func() {
			// Seed a snapshot with eod data via the recorder.
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}
			metrics := []data.Metric{data.MetricClose, data.AdjClose}

			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			times := []time.Time{
				time.Date(2024, 1, 2, 16, 0, 0, 0, nyc),
				time.Date(2024, 1, 3, 16, 0, 0, 0, nyc),
			}

			values := []float64{100.0, 101.0, 99.0, 100.0}

			df, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			stub := data.NewTestProvider(metrics, df)
			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets: assets, Metrics: metrics, Start: times[0], End: times[1],
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Close()).To(Succeed())

			// Now replay.
			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   metrics,
				Start:     times[0],
				End:       times[1],
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Verify the replayed data matches the original.
			// SPY close at t0=100.0, t1=101.0; adj_close t0=99.0, t1=100.0
			spyClose := result.Column(spy, data.MetricClose)
			Expect(spyClose[0]).To(BeNumerically("~", 100.0, 0.001))
			Expect(spyClose[1]).To(BeNumerically("~", 101.0, 0.001))

			spyAdj := result.Column(spy, data.AdjClose)
			Expect(spyAdj[0]).To(BeNumerically("~", 99.0, 0.001))
			Expect(spyAdj[1]).To(BeNumerically("~", 100.0, 0.001))
		})
	})

	Describe("Provides", func() {
		It("returns metrics for tables that have data", func() {
			// Seed with eod data only.
			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.CreateSnapshotSchema(db)).To(Succeed())

			_, err = db.Exec("INSERT INTO assets (composite_figi, ticker) VALUES ('BBG000BLNNH6', 'SPY')")
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec("INSERT INTO eod (composite_figi, event_date, close) VALUES ('BBG000BLNNH6', '2024-01-02T16:00:00-05:00', 100.0)")
			Expect(err).NotTo(HaveOccurred())
			db.Close()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			provided := snap.Provides()
			Expect(provided).To(ContainElement(data.MetricClose))
			Expect(provided).To(ContainElement(data.MetricOpen))
			Expect(provided).NotTo(ContainElement(data.PE)) // metrics table is empty
		})

		It("returns empty when no tables have data", func() {
			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.CreateSnapshotSchema(db)).To(Succeed())
			db.Close()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			Expect(snap.Provides()).To(BeEmpty())
		})
	})

	Describe("IndexMembers", func() {
		It("replays recorded index members", func() {
			// Seed via recorder.
			members := []asset.Asset{{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}}
			nyc, _ := time.LoadLocation("America/New_York")
			date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				IndexProvider: &stubIndexProvider{members: members},
				AssetProvider: &stubAssetProvider{assets: members},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.IndexMembers(ctx, "SP500", date)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Close()).To(Succeed())

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.IndexMembers(ctx, "SP500", date)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].Ticker).To(Equal("SPY"))
		})
	})

	Describe("RatedAssets", func() {
		It("replays recorded rated assets", func() {
			rated := []asset.Asset{{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}}
			nyc, _ := time.LoadLocation("America/New_York")
			date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				RatingProvider: &stubRatingProvider{assets: rated},
				AssetProvider:  &stubAssetProvider{assets: rated},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.RatedAssets(ctx, "morningstar", data.RatingEq(5), date)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Close()).To(Succeed())

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.RatedAssets(ctx, "morningstar", data.RatingEq(5), date)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].Ticker).To(Equal("SPY"))
		})
	})
})
