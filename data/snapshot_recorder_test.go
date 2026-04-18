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
			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			stubAssets := []asset.Asset{
				{
					CompositeFigi:   "BBG000BLNNH6",
					Ticker:          "SPY",
					Name:            "SPDR S&P 500 ETF Trust",
					AssetType:       asset.AssetTypeETF,
					PrimaryExchange: asset.ExchangeNYSE,
					Sector:          "",
					Industry:        "",
					SICCode:         6726,
					CIK:             "0000884394",
					Listed:          time.Date(1993, 1, 22, 0, 0, 0, 0, nyc),
				},
				{
					CompositeFigi:   "BBG000BHTK15",
					Ticker:          "TLT",
					Name:            "iShares 20+ Year Treasury Bond ETF",
					AssetType:       asset.AssetTypeETF,
					PrimaryExchange: asset.ExchangeNASDAQ,
					Sector:          "",
					Industry:        "",
					SICCode:         0,
					CIK:             "0000088525",
					Listed:          time.Date(2002, 7, 22, 0, 0, 0, 0, nyc),
				},
			}
			stub := &stubAssetProvider{assets: stubAssets}

			var recErr error
			recorder, recErr = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: stub,
			})
			Expect(recErr).NotTo(HaveOccurred())

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

			var name, assetType, exchange, cik string
			var sicCode int
			Expect(db.QueryRow(
				"SELECT name, asset_type, primary_exchange, sic_code, cik FROM assets WHERE ticker = 'SPY'",
			).Scan(&name, &assetType, &exchange, &sicCode, &cik)).To(Succeed())
			Expect(name).To(Equal("SPDR S&P 500 ETF Trust"))
			Expect(assetType).To(Equal("ETF"))
			Expect(exchange).To(Equal("NYSE"))
			Expect(sicCode).To(Equal(6726))
			Expect(cik).To(Equal("0000884394"))
		})

		It("records asset from LookupAsset() call", func() {
			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			expected := asset.Asset{
				CompositeFigi:   "BBG000BLNNH6",
				Ticker:          "SPY",
				Name:            "SPDR S&P 500 ETF Trust",
				AssetType:       asset.AssetTypeETF,
				PrimaryExchange: asset.ExchangeNYSE,
				SICCode:         6726,
				CIK:             "0000884394",
				Listed:          time.Date(1993, 1, 22, 0, 0, 0, 0, nyc),
			}
			stub := &stubAssetProvider{lookupResult: expected}

			var recErr error
			recorder, recErr = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: stub,
			})
			Expect(recErr).NotTo(HaveOccurred())

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

			var name, assetType, exchange, cik string
			var sicCode int
			Expect(db.QueryRow(
				"SELECT name, asset_type, primary_exchange, sic_code, cik FROM assets WHERE ticker = 'SPY'",
			).Scan(&name, &assetType, &exchange, &sicCode, &cik)).To(Succeed())
			Expect(name).To(Equal("SPDR S&P 500 ETF Trust"))
			Expect(assetType).To(Equal("ETF"))
			Expect(exchange).To(Equal("NYSE"))
			Expect(sicCode).To(Equal(6726))
			Expect(cik).To(Equal("0000884394"))
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

	Describe("fundamentals recording with metadata", func() {
		It("persists date_key, report_period, and dimension from the wrapped provider", func() {
			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}

			filing := time.Date(2024, 5, 2, 16, 0, 0, 0, nyc)
			dateKey := time.Date(2024, 3, 31, 0, 0, 0, 0, nyc)
			reportPeriod := time.Date(2024, 3, 30, 0, 0, 0, 0, nyc)

			times := []time.Time{filing}
			metrics := []data.Metric{
				data.WorkingCapital,
				data.FundamentalsDateKey,
				data.FundamentalsReportPeriod,
			}

			values := [][]float64{
				{120_000_000.0},                // SPY WorkingCapital
				{float64(dateKey.Unix())},      // SPY FundamentalsDateKey
				{float64(reportPeriod.Unix())}, // SPY FundamentalsReportPeriod
			}

			df, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			stub := &dimensionedTestProvider{
				TestProvider: data.NewTestProvider(metrics, df),
				dimension:    "MRQ",
			}

			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   metrics,
				Start:     filing,
				End:       filing,
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var (
				dateKeyStr, reportPeriodStr, dimStr string
				wc                                  float64
			)
			err = db.QueryRow(
				`SELECT date_key, report_period, dimension, working_capital
				   FROM fundamentals
				  WHERE composite_figi = ?`,
				spy.CompositeFigi,
			).Scan(&dateKeyStr, &reportPeriodStr, &dimStr, &wc)
			Expect(err).NotTo(HaveOccurred())

			Expect(dateKeyStr).To(Equal("2024-03-31"))
			Expect(reportPeriodStr).To(Equal("2024-03-30"))
			Expect(dimStr).To(Equal("MRQ"))
			Expect(wc).To(BeNumerically("==", 120_000_000.0))
		})

		It("writes NULL date_key/report_period when the DataFrame omits them", func() {
			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}

			filing := time.Date(2024, 5, 2, 16, 0, 0, 0, nyc)
			times := []time.Time{filing}
			metrics := []data.Metric{data.WorkingCapital}
			values := [][]float64{{120_000_000.0}}

			df, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			stub := data.NewTestProvider(metrics, df) // no Dimension() method

			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   metrics,
				Start:     filing,
				End:       filing,
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var (
				dateKeyNull, reportPeriodNull sql.NullString
				dimStr                        string
			)
			err = db.QueryRow(
				`SELECT date_key, report_period, dimension
				   FROM fundamentals
				  WHERE composite_figi = ?`,
				spy.CompositeFigi,
			).Scan(&dateKeyNull, &reportPeriodNull, &dimStr)
			Expect(err).NotTo(HaveOccurred())

			Expect(dateKeyNull.Valid).To(BeFalse())
			Expect(reportPeriodNull.Valid).To(BeFalse())
			Expect(dimStr).To(Equal("ARQ")) // fallback default
		})
	})

	Describe("FetchFundamentalsByDateKey", func() {
		It("implements FundamentalsByDateKeyProvider so Engine can snapshot by-date-key queries", func() {
			var _ data.FundamentalsByDateKeyProvider = (*data.SnapshotRecorder)(nil)
		})

		It("delegates to the wrapped provider and records the result for replay", func() {
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}
			dateKey := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)
			maxEventDate := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			stub := &stubFundamentalsByDateKeyProvider{
				values:    map[string]float64{spy.CompositeFigi: 120_000_000.0},
				dimension: "ARQ",
			}

			var recErr error
			recorder, recErr = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(recErr).NotTo(HaveOccurred())

			df, err := recorder.FetchFundamentalsByDateKey(ctx, assets,
				[]data.Metric{data.WorkingCapital}, dateKey, "ARQ", maxEventDate)
			Expect(err).NotTo(HaveOccurred())
			Expect(df.Column(spy, data.WorkingCapital)[0]).To(BeNumerically("==", 120_000_000.0))
			Expect(stub.calls).To(Equal(1))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var (
				dateKeyStr, dimStr string
				wc                 float64
			)
			err = db.QueryRow(
				`SELECT date_key, dimension, working_capital
				   FROM fundamentals
				  WHERE composite_figi = ?`,
				spy.CompositeFigi,
			).Scan(&dateKeyStr, &dimStr, &wc)
			Expect(err).NotTo(HaveOccurred())

			Expect(dateKeyStr).To(Equal("2024-03-31"))
			Expect(dimStr).To(Equal("ARQ"))
			Expect(wc).To(BeNumerically("==", 120_000_000.0))
		})

		It("populates date_key when a row already exists from the daily Fetch path", func() {
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}
			dateKey := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

			dailyDF, err := data.NewDataFrame(
				[]time.Time{dateKey},
				assets,
				[]data.Metric{data.WorkingCapital},
				data.Daily,
				[][]float64{{50_000_000}},
			)
			Expect(err).NotTo(HaveOccurred())

			stub := &stubFundamentalsByDateKeyProvider{
				TestProvider: data.NewTestProvider([]data.Metric{data.WorkingCapital}, dailyDF),
				values:       map[string]float64{spy.CompositeFigi: 120_000_000.0},
				dimension:    "ARQ",
			}

			var recErr error
			recorder, recErr = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(recErr).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   []data.Metric{data.WorkingCapital},
				Start:     dateKey,
				End:       dateKey,
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.FetchFundamentalsByDateKey(ctx, assets,
				[]data.Metric{data.WorkingCapital}, dateKey, "ARQ", dateKey)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var dateKeyStr sql.NullString
			err = db.QueryRow(
				`SELECT date_key FROM fundamentals WHERE composite_figi = ?`,
				spy.CompositeFigi,
			).Scan(&dateKeyStr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dateKeyStr.Valid).To(BeTrue(), "date_key should be populated by FetchFundamentalsByDateKey even when a row already exists")
			Expect(dateKeyStr.String).To(Equal("2024-12-31"))
		})

		It("errors when no wrapped provider supports FundamentalsByDateKey", func() {
			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}
			dateKey := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

			plainStub := data.NewTestProvider([]data.Metric{data.WorkingCapital}, nil)

			var recErr error
			recorder, recErr = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: plainStub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(recErr).NotTo(HaveOccurred())

			_, err := recorder.FetchFundamentalsByDateKey(ctx, assets,
				[]data.Metric{data.WorkingCapital}, dateKey, "ARQ", dateKey)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no wrapped provider"))
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

type dimensionedTestProvider struct {
	*data.TestProvider
	dimension string
}

func (p *dimensionedTestProvider) Dimension() string { return p.dimension }

type stubFundamentalsByDateKeyProvider struct {
	*data.TestProvider
	values    map[string]float64
	dimension string
	calls     int
}

func (p *stubFundamentalsByDateKeyProvider) Dimension() string { return p.dimension }

func (p *stubFundamentalsByDateKeyProvider) FetchFundamentalsByDateKey(
	_ context.Context,
	assets []asset.Asset,
	metrics []data.Metric,
	dateKey time.Time,
	_ string,
	_ time.Time,
) (*data.DataFrame, error) {
	p.calls++

	times := []time.Time{dateKey}
	columns := make([][]float64, len(assets)*len(metrics))

	for aIdx, aa := range assets {
		for mIdx := range metrics {
			val, ok := p.values[aa.CompositeFigi]
			if !ok {
				columns[aIdx*len(metrics)+mIdx] = []float64{0.0}
				continue
			}

			columns[aIdx*len(metrics)+mIdx] = []float64{val}
		}
	}

	return data.NewDataFrame(times, assets, metrics, data.Daily, columns)
}
