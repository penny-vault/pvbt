package engine_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// warmupStrategy is a test strategy that declares warmup via Describe().
type warmupStrategy struct {
	assets     []asset.Asset
	warmupDays int
}

func (s *warmupStrategy) Name() string { return "warmupStrategy" }

func (s *warmupStrategy) Setup(_ *engine.Engine) {}

func (s *warmupStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
	return nil
}

func (s *warmupStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		Schedule: "0 16 * * 1-5",
		Warmup:   s.warmupDays,
	}
}

var _ = Describe("walkBackTradingDays", func() {
	It("returns the same date for 0 days", func() {
		from := time.Date(2024, 2, 5, 16, 0, 0, 0, time.UTC)
		result, err := engine.WalkBackTradingDaysForTest(from, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Format("2006-01-02")).To(Equal("2024-02-05"))
	})

	It("walks back 5 trading days skipping weekends", func() {
		// Monday 2024-02-12
		from := time.Date(2024, 2, 12, 16, 0, 0, 0, time.UTC)
		result, err := engine.WalkBackTradingDaysForTest(from, 5)
		Expect(err).NotTo(HaveOccurred())
		// 5 trading days back from Feb 12 (Mon): Feb 5 (Mon)
		Expect(result.Format("2006-01-02")).To(Equal("2024-02-05"))
	})

	It("walks back a large number of trading days", func() {
		from := time.Date(2024, 6, 3, 16, 0, 0, 0, time.UTC)
		result, err := engine.WalkBackTradingDaysForTest(from, 252)
		Expect(err).NotTo(HaveOccurred())
		// ~1 year of trading days back from June 2024
		Expect(result.Year()).To(Equal(2023))
	})

	It("returns an error for negative days", func() {
		from := time.Date(2024, 2, 12, 16, 0, 0, 0, time.UTC)
		_, err := engine.WalkBackTradingDaysForTest(from, -1)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("collectStrategyAssets", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		msft asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
	})

	It("collects asset fields from a strategy struct", func() {
		type testStrategy struct {
			RiskOn  asset.Asset
			RiskOff asset.Asset
		}
		strategy := &testStrategy{RiskOn: spy, RiskOff: aapl}
		assets := engine.CollectStrategyAssetsForTest(strategy, asset.Asset{})
		Expect(assets).To(ContainElement(spy))
		Expect(assets).To(ContainElement(aapl))
	})

	It("collects assets from static universe fields", func() {
		type testStrategy struct {
			Universe universe.Universe
		}
		staticU := universe.NewStaticWithSource([]asset.Asset{aapl, msft}, nil)
		strategy := &testStrategy{Universe: staticU}
		assets := engine.CollectStrategyAssetsForTest(strategy, asset.Asset{})
		Expect(assets).To(ContainElement(aapl))
		Expect(assets).To(ContainElement(msft))
	})

	It("includes the benchmark when set", func() {
		type testStrategy struct {
			RiskOn asset.Asset
		}
		strategy := &testStrategy{RiskOn: aapl}
		assets := engine.CollectStrategyAssetsForTest(strategy, spy)
		Expect(assets).To(ContainElement(spy))
		Expect(assets).To(ContainElement(aapl))
	})

	It("deduplicates by CompositeFigi", func() {
		type testStrategy struct {
			Asset1 asset.Asset
			Asset2 asset.Asset
		}
		strategy := &testStrategy{Asset1: spy, Asset2: spy}
		assets := engine.CollectStrategyAssetsForTest(strategy, asset.Asset{})
		count := 0
		for _, assetItem := range assets {
			if assetItem.CompositeFigi == spy.CompositeFigi {
				count++
			}
		}
		Expect(count).To(Equal(1))
	})

	It("skips zero-value asset fields", func() {
		type testStrategy struct {
			Optional asset.Asset
		}
		strategy := &testStrategy{}
		assets := engine.CollectStrategyAssetsForTest(strategy, asset.Asset{})
		Expect(assets).To(BeEmpty())
	})
})

var _ = Describe("validateWarmup", func() {
	var (
		aapl          asset.Asset
		msft          asset.Asset
		testAssets    []asset.Asset
		assetProvider *mockAssetProvider
		metrics       []data.Metric
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		testAssets = []asset.Asset{aapl, msft}
		assetProvider = &mockAssetProvider{assets: testAssets}
		metrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
	})

	Context("warmup is 0", func() {
		It("returns the original start date unchanged", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &warmupStrategy{warmupDays: 0, assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())
		})
	})

	Context("strict mode", func() {
		It("succeeds when warmup data is available", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &warmupStrategy{warmupDays: 14, assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())
		})

		It("errors when warmup data is insufficient", func() {
			dataStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 60, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &warmupStrategy{warmupDays: 14, assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("warmup"))
		})
	})

	Context("permissive mode", func() {
		It("adjusts start date forward when warmup data is partial", func() {
			dataStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 200, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &warmupStrategy{warmupDays: 5, assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000),
				engine.WithDateRangeMode(engine.DateRangeModePermissive),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())
		})

		It("errors when no valid start date exists before end", func() {
			dataStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 10, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &warmupStrategy{warmupDays: 100, assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000),
				engine.WithDateRangeMode(engine.DateRangeModePermissive),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("warmup"))
		})
	})

	Context("negative warmup", func() {
		It("errors during backtest initialization", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &warmupStrategy{warmupDays: -5, assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("negative"))
		})
	})
})
