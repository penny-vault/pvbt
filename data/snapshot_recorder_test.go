package data_test

import (
	"context"
	"database/sql"

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
