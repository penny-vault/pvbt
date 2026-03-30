package data_test

import (
	"context"
	"database/sql"
	"math"
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

			values := [][]float64{{100.0, 101.0}, {99.0, 100.0}}

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
		It("replays recorded index members with weights", func() {
			// Seed via recorder.
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			members := []asset.Asset{spy}
			constituents := []data.IndexConstituent{{Asset: spy, Weight: 0.75}}
			nyc, _ := time.LoadLocation("America/New_York")
			date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				IndexProvider: &stubIndexProvider{members: members, constituents: constituents},
				AssetProvider: &stubAssetProvider{assets: members},
			})
			Expect(err).NotTo(HaveOccurred())

			_, _, err = recorder.IndexMembers(ctx, "SP500", date)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Close()).To(Succeed())

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			resultAssets, resultConstituents, err := snap.IndexMembers(ctx, "SP500", date)
			Expect(err).NotTo(HaveOccurred())
			Expect(resultAssets).To(HaveLen(1))
			Expect(resultAssets[0].Ticker).To(Equal("SPY"))
			Expect(resultConstituents).To(HaveLen(1))
			Expect(resultConstituents[0].Weight).To(BeNumerically("~", 0.75, 0.001))
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

	Describe("round-trip", func() {
		It("returns data when query dates have different time-of-day than stored dates", func() {
			// This reproduces the bug: data is recorded with 4pm Eastern timestamps
			// (from the PV database), but the engine queries with midnight timestamps
			// (from user-specified start/end dates). The snapshot must return data
			// regardless of time-of-day differences on the same calendar date.
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}
			metrics := []data.Metric{data.MetricClose, data.AdjClose}

			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			// Record with 4pm Eastern (market close time).
			storedTimes := []time.Time{
				time.Date(2024, 6, 3, 16, 0, 0, 0, nyc),
				time.Date(2024, 6, 4, 16, 0, 0, 0, nyc),
				time.Date(2024, 6, 5, 16, 0, 0, 0, nyc),
			}

			values := [][]float64{
				{100.0, 101.0, 102.0}, // close
				{99.0, 100.0, 101.0},  // adj_close
			}

			originalDF, err := data.NewDataFrame(storedTimes, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			stub := data.NewTestProvider(metrics, originalDF)
			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets: assets, Metrics: metrics,
				Start: storedTimes[0], End: storedTimes[2],
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Close()).To(Succeed())

			// Query with midnight Eastern (how engine constructs dates from CLI --start/--end).
			queryStart := time.Date(2024, 6, 1, 0, 0, 0, 0, nyc)
			queryEnd := time.Date(2024, 6, 30, 0, 0, 0, 0, nyc)

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   metrics,
				Start:     queryStart,
				End:       queryEnd,
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Len()).To(Equal(3), "expected 3 rows but got %d -- query dates may not match stored dates", result.Len())

			spyClose := result.Column(spy, data.MetricClose)
			Expect(spyClose[0]).To(BeNumerically("~", 100.0, 0.001))
		})

		It("returns correct data for multiple assets", func() {
			// Reproduces bug: PRIDX returns NaN for all rows except the first
			// while VFINX returns correct values for all rows.
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "VFINX"}
			tlt := asset.Asset{CompositeFigi: "BBG000BHTK15", Ticker: "PRIDX"}
			assets := []asset.Asset{spy, tlt}
			metrics := []data.Metric{data.MetricClose, data.AdjClose}

			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			times := []time.Time{
				time.Date(2024, 1, 2, 16, 0, 0, 0, nyc),
				time.Date(2024, 2, 1, 16, 0, 0, 0, nyc),
				time.Date(2024, 3, 1, 16, 0, 0, 0, nyc),
				time.Date(2024, 4, 1, 16, 0, 0, 0, nyc),
				time.Date(2024, 5, 1, 16, 0, 0, 0, nyc),
			}

			// 2 assets * 2 metrics * 5 times = 4 columns (column-major)
			values := [][]float64{
				{100, 110, 120, 130, 140}, // VFINX close
				{99, 109, 119, 129, 139},  // VFINX adj_close
				{50, 55, 60, 65, 70},      // PRIDX close
				{49, 54, 59, 64, 69},      // PRIDX adj_close
			}

			originalDF, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			// Verify original data is correct before recording.
			Expect(originalDF.Column(tlt, data.AdjClose)).To(Equal([]float64{49, 54, 59, 64, 69}))

			stub := data.NewTestProvider(metrics, originalDF)
			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets: assets, Metrics: metrics,
				Start: times[0], End: times[len(times)-1],
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Close()).To(Succeed())

			// Replay.
			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   metrics,
				Start:     time.Date(2024, 1, 1, 0, 0, 0, 0, nyc),
				End:       time.Date(2024, 6, 1, 0, 0, 0, 0, nyc),
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Len()).To(Equal(5))

			// Verify BOTH assets have all values (not just the first row).
			vfinxAdj := result.Column(spy, data.AdjClose)
			pridxAdj := result.Column(tlt, data.AdjClose)

			for idx := range times {
				Expect(vfinxAdj[idx]).To(BeNumerically("~", float64(99+idx*10), 0.01),
					"VFINX adj_close mismatch at index %d", idx)
				Expect(pridxAdj[idx]).To(BeNumerically("~", float64(49+idx*5), 0.01),
					"PRIDX adj_close mismatch at index %d -- got NaN?", idx)
			}
		})

		It("returns correct data when assets are recorded in separate Fetch calls", func() {
			// Reproduces real-world scenario: engine fetches VFINX in one call,
			// then PRIDX in a separate call. Both get INSERT OR REPLACE'd into
			// the same eod table. Verify the second asset's data survives.
			vfinx := asset.Asset{CompositeFigi: "BBG000BHTMY2", Ticker: "VFINX"}
			pridx := asset.Asset{CompositeFigi: "BBG000BRKZ91", Ticker: "PRIDX"}
			metrics := []data.Metric{data.MetricClose, data.AdjClose}

			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			times := []time.Time{
				time.Date(2024, 1, 31, 16, 0, 0, 0, nyc),
				time.Date(2024, 2, 29, 16, 0, 0, 0, nyc),
				time.Date(2024, 3, 28, 16, 0, 0, 0, nyc),
			}

			// First DataFrame: VFINX only.
			vfinxValues := [][]float64{
				{440, 460, 470}, // close
				{432, 452, 462}, // adj_close
			}
			vfinxDF, err := data.NewDataFrame(times, []asset.Asset{vfinx}, metrics, data.Daily, vfinxValues)
			Expect(err).NotTo(HaveOccurred())

			// Second DataFrame: PRIDX only.
			pridxValues := [][]float64{
				{58, 60, 62}, // close
				{57, 59, 61}, // adj_close
			}
			pridxDF, err := data.NewDataFrame(times, []asset.Asset{pridx}, metrics, data.Daily, pridxValues)
			Expect(err).NotTo(HaveOccurred())

			// Record VFINX first, then PRIDX (separate Fetch calls).
			vfinxStub := data.NewTestProvider(metrics, vfinxDF)
			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: vfinxStub,
				AssetProvider: &stubAssetProvider{assets: []asset.Asset{vfinx, pridx}},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets: []asset.Asset{vfinx}, Metrics: metrics,
				Start: times[0], End: times[2],
			})
			Expect(err).NotTo(HaveOccurred())

			// Swap stub to return PRIDX data.
			pridxStub := data.NewTestProvider(metrics, pridxDF)
			recorder2, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: pridxStub,
				AssetProvider: &stubAssetProvider{assets: []asset.Asset{vfinx, pridx}},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder2.Fetch(ctx, data.DataRequest{
				Assets: []asset.Asset{pridx}, Metrics: metrics,
				Start: times[0], End: times[2],
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder2.Close()).To(Succeed())

			// Replay both assets together.
			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.Fetch(ctx, data.DataRequest{
				Assets:    []asset.Asset{vfinx, pridx},
				Metrics:   metrics,
				Start:     time.Date(2024, 1, 1, 0, 0, 0, 0, nyc),
				End:       time.Date(2024, 4, 1, 0, 0, 0, 0, nyc),
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Len()).To(Equal(3))

			// Both assets should have all 3 rows populated.
			vfinxAdjResult := result.Column(vfinx, data.AdjClose)
			pridxAdjResult := result.Column(pridx, data.AdjClose)

			Expect(vfinxAdjResult).To(HaveLen(3))
			Expect(pridxAdjResult).To(HaveLen(3))

			for idx := range times {
				Expect(math.IsNaN(vfinxAdjResult[idx])).To(BeFalse(),
					"VFINX adj_close is NaN at index %d", idx)
				Expect(math.IsNaN(pridxAdjResult[idx])).To(BeFalse(),
					"PRIDX adj_close is NaN at index %d -- second asset data lost?", idx)
			}

			Expect(pridxAdjResult[0]).To(BeNumerically("~", 57.0, 0.01))
			Expect(pridxAdjResult[1]).To(BeNumerically("~", 59.0, 0.01))
			Expect(pridxAdjResult[2]).To(BeNumerically("~", 61.0, 0.01))
		})

		It("preserves existing columns when recording different metrics for the same asset+date", func() {
			// Reproduces: engine fetches close+adj_close for VFINX+PRIDX, then later
			// fetches dividend for the same assets+dates. The second INSERT OR REPLACE
			// must not NULL out the close+adj_close that were already written.
			vfinx := asset.Asset{CompositeFigi: "BBG000BHTMY2", Ticker: "VFINX"}
			pridx := asset.Asset{CompositeFigi: "BBG000BRKZ91", Ticker: "PRIDX"}
			allAssets := []asset.Asset{vfinx, pridx}

			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			times := []time.Time{
				time.Date(2024, 1, 31, 16, 0, 0, 0, nyc),
				time.Date(2024, 2, 29, 16, 0, 0, 0, nyc),
			}

			// First fetch: close + adj_close for both assets.
			priceMetrics := []data.Metric{data.MetricClose, data.AdjClose}
			priceValues := [][]float64{
				{440, 460}, // VFINX close
				{432, 452}, // VFINX adj_close
				{58, 60},   // PRIDX close
				{57, 59},   // PRIDX adj_close
			}
			priceDF, err := data.NewDataFrame(times, allAssets, priceMetrics, data.Daily, priceValues)
			Expect(err).NotTo(HaveOccurred())

			priceStub := data.NewTestProvider(priceMetrics, priceDF)
			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: priceStub,
				AssetProvider: &stubAssetProvider{assets: allAssets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets: allAssets, Metrics: priceMetrics,
				Start: times[0], End: times[1],
			})
			Expect(err).NotTo(HaveOccurred())

			// Second fetch: ONLY dividend for the same assets+dates.
			// INSERT OR REPLACE must NOT overwrite close/adj_close with NULL.
			divOnlyMetrics := []data.Metric{data.Dividend}
			divOnlyValues := [][]float64{
				{0.5, 0.0}, // VFINX dividend
				{0.3, 0.0}, // PRIDX dividend
			}
			divOnlyDF, err := data.NewDataFrame(times, allAssets, divOnlyMetrics, data.Daily, divOnlyValues)
			Expect(err).NotTo(HaveOccurred())

			divStub := data.NewTestProvider(divOnlyMetrics, divOnlyDF)
			recorder2, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: divStub,
				AssetProvider: &stubAssetProvider{assets: allAssets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder2.Fetch(ctx, data.DataRequest{
				Assets: allAssets, Metrics: divOnlyMetrics,
				Start: times[0], End: times[1],
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder2.Close()).To(Succeed())

			// Replay: close and adj_close must still be present for PRIDX.
			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.Fetch(ctx, data.DataRequest{
				Assets:    allAssets,
				Metrics:   []data.Metric{data.MetricClose, data.AdjClose},
				Start:     time.Date(2024, 1, 1, 0, 0, 0, 0, nyc),
				End:       time.Date(2024, 3, 1, 0, 0, 0, 0, nyc),
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Len()).To(Equal(2))

			// PRIDX adj_close must NOT be NaN.
			pridxAdj := result.Column(pridx, data.AdjClose)
			Expect(pridxAdj).To(HaveLen(2))
			Expect(math.IsNaN(pridxAdj[0])).To(BeFalse(), "PRIDX adj_close[0] is NaN after second INSERT OR REPLACE")
			Expect(math.IsNaN(pridxAdj[1])).To(BeFalse(), "PRIDX adj_close[1] is NaN after second INSERT OR REPLACE")
			Expect(pridxAdj[0]).To(BeNumerically("~", 57.0, 0.01))
			Expect(pridxAdj[1]).To(BeNumerically("~", 59.0, 0.01))
		})

		It("reads snapshots with RFC3339 dates (backward compatibility)", func() {
			// Snapshots created before the date-format fix stored dates as RFC3339.
			// The provider must handle both formats.
			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.CreateSnapshotSchema(db)).To(Succeed())

			_, err = db.Exec("INSERT INTO assets (composite_figi, ticker) VALUES ('BBG000BLNNH6', 'SPY')")
			Expect(err).NotTo(HaveOccurred())

			// Store dates in RFC3339 format (the old format).
			_, err = db.Exec(`INSERT INTO eod (composite_figi, event_date, close, adj_close) VALUES
				('BBG000BLNNH6', '2024-06-03T16:00:00-04:00', 100.0, 99.0),
				('BBG000BLNNH6', '2024-06-04T16:00:00-04:00', 101.0, 100.0),
				('BBG000BLNNH6', '2024-06-05T16:00:00-04:00', 102.0, 101.0)`)
			Expect(err).NotTo(HaveOccurred())
			db.Close()

			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}

			// Query with midnight dates (how engine constructs them).
			result, err := snap.Fetch(ctx, data.DataRequest{
				Assets:    []asset.Asset{spy},
				Metrics:   []data.Metric{data.MetricClose, data.AdjClose},
				Start:     time.Date(2024, 6, 1, 0, 0, 0, 0, nyc),
				End:       time.Date(2024, 6, 30, 0, 0, 0, 0, nyc),
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Len()).To(Equal(3), "old RFC3339 snapshots must still be readable")
		})

		It("record then replay produces identical data", func() {
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			tlt := asset.Asset{CompositeFigi: "BBG000BHTK15", Ticker: "TLT"}
			assets := []asset.Asset{spy, tlt}
			metrics := []data.Metric{data.MetricClose, data.AdjClose}

			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			times := []time.Time{
				time.Date(2024, 1, 2, 16, 0, 0, 0, nyc),
				time.Date(2024, 1, 3, 16, 0, 0, 0, nyc),
				time.Date(2024, 1, 4, 16, 0, 0, 0, nyc),
			}

			// 2 assets * 2 metrics * 3 times = 4 columns
			values := [][]float64{
				{100.0, 101.0, 102.0}, // SPY close
				{99.0, 100.0, 101.0},  // SPY adj_close
				{50.0, 51.0, 52.0},    // TLT close
				{49.0, 50.0, 51.0},    // TLT adj_close
			}

			originalDF, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			stub := data.NewTestProvider(metrics, originalDF)
			recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			req := data.DataRequest{
				Assets: assets, Metrics: metrics,
				Start: times[0], End: times[2], Frequency: data.Daily,
			}

			_, err = recorder.Fetch(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Close()).To(Succeed())

			// Replay.
			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			replayedDF, err := snap.Fetch(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(replayedDF).NotTo(BeNil())

			// Compare all values element-by-element.
			for _, assetItem := range assets {
				for _, metric := range metrics {
					original := originalDF.Column(assetItem, metric)
					replayed := replayedDF.Column(assetItem, metric)
					Expect(len(replayed)).To(Equal(len(original)))
					for idx := range original {
						Expect(replayed[idx]).To(BeNumerically("~", original[idx], 0.001),
							"mismatch at %s/%s index %d", assetItem.Ticker, metric, idx)
					}
				}
			}
		})
	})
})
