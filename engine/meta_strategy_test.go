// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package engine_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// --- Child strategies for meta-strategy tests ---

// spyOnlyStrategy buys SPY with all available cash on every compute date.
type spyOnlyStrategy struct {
	Assets universe.Universe `default:"SPY"`
}

func (s *spyOnlyStrategy) Name() string           { return "spyOnly" }
func (s *spyOnlyStrategy) Setup(_ *engine.Engine)  {}
func (s *spyOnlyStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *spyOnlyStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	members := s.Assets.Assets(eng.CurrentDate())
	if len(members) == 0 {
		return nil
	}

	target := members[0] // SPY
	priceDF, err := eng.FetchAt(ctx, members, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil || priceDF == nil {
		return nil
	}

	price := priceDF.ValueAt(target, data.MetricClose, eng.CurrentDate())
	if math.IsNaN(price) || price <= 0 {
		return nil
	}

	// Buy SPY with all cash if not already held.
	currentShares := fund.Position(target)
	totalValue := fund.Cash()
	fund.Holdings(func(held asset.Asset, qty float64) {
		holdingPrice := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
		if !math.IsNaN(holdingPrice) {
			totalValue += qty * holdingPrice
		}
	})

	targetShares := math.Floor(totalValue / price)
	diff := targetShares - currentShares
	if diff > 0 {
		batch.Order(ctx, target, portfolio.Buy, diff)
	} else if diff < 0 {
		batch.Order(ctx, target, portfolio.Sell, -diff)
	}

	return nil
}

// tltOnlyStrategy buys TLT with all available cash on every compute date.
type tltOnlyStrategy struct {
	Assets universe.Universe `default:"TLT"`
}

func (s *tltOnlyStrategy) Name() string           { return "tltOnly" }
func (s *tltOnlyStrategy) Setup(_ *engine.Engine)  {}
func (s *tltOnlyStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *tltOnlyStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	members := s.Assets.Assets(eng.CurrentDate())
	if len(members) == 0 {
		return nil
	}

	target := members[0] // TLT
	priceDF, err := eng.FetchAt(ctx, members, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil || priceDF == nil {
		return nil
	}

	price := priceDF.ValueAt(target, data.MetricClose, eng.CurrentDate())
	if math.IsNaN(price) || price <= 0 {
		return nil
	}

	currentShares := fund.Position(target)
	totalValue := fund.Cash()
	fund.Holdings(func(held asset.Asset, qty float64) {
		holdingPrice := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
		if !math.IsNaN(holdingPrice) {
			totalValue += qty * holdingPrice
		}
	})

	targetShares := math.Floor(totalValue / price)
	diff := targetShares - currentShares
	if diff > 0 {
		batch.Order(ctx, target, portfolio.Buy, diff)
	} else if diff < 0 {
		batch.Order(ctx, target, portfolio.Sell, -diff)
	}

	return nil
}

// --- Meta-strategy ---

// testMetaStrategy combines spyOnlyStrategy (60%) and tltOnlyStrategy (40%).
type testMetaStrategy struct {
	SPYChild *spyOnlyStrategy `pvbt:"spy-child" weight:"0.60"`
	TLTChild *tltOnlyStrategy `pvbt:"tlt-child" weight:"0.40"`
}

func (s *testMetaStrategy) Name() string           { return "testMeta" }
func (s *testMetaStrategy) Setup(_ *engine.Engine)  {}
func (s *testMetaStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *testMetaStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	alloc, err := eng.ChildAllocations()
	if err != nil {
		return err
	}

	return batch.RebalanceTo(ctx, alloc)
}

// testMetaStrategyWithOverride uses overridden weights of 80/20 instead of 60/40.
type testMetaStrategyWithOverride struct {
	SPYChild *spyOnlyStrategy `pvbt:"spy-child" weight:"0.60"`
	TLTChild *tltOnlyStrategy `pvbt:"tlt-child" weight:"0.40"`
}

func (s *testMetaStrategyWithOverride) Name() string           { return "testMetaOverride" }
func (s *testMetaStrategyWithOverride) Setup(_ *engine.Engine)  {}
func (s *testMetaStrategyWithOverride) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *testMetaStrategyWithOverride) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	overrideWeights := map[string]float64{
		"spy-child": 0.80,
		"tlt-child": 0.20,
	}

	alloc, err := eng.ChildAllocations(overrideWeights)
	if err != nil {
		return err
	}

	return batch.RebalanceTo(ctx, alloc)
}

// testMetaStrategyWithChildCheck verifies ChildPortfolios inside Compute and
// records the result for later assertion.
type testMetaStrategyWithChildCheck struct {
	SPYChild *spyOnlyStrategy `pvbt:"spy-child" weight:"0.60"`
	TLTChild *tltOnlyStrategy `pvbt:"tlt-child" weight:"0.40"`

	// ChildPortfolioNames is populated during Compute for later assertion.
	ChildPortfolioNames []string
}

func (s *testMetaStrategyWithChildCheck) Name() string           { return "testMetaChildCheck" }
func (s *testMetaStrategyWithChildCheck) Setup(_ *engine.Engine)  {}
func (s *testMetaStrategyWithChildCheck) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *testMetaStrategyWithChildCheck) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	childPortfolios := eng.ChildPortfolios()
	s.ChildPortfolioNames = make([]string, 0, len(childPortfolios))
	for childName := range childPortfolios {
		s.ChildPortfolioNames = append(s.ChildPortfolioNames, childName)
	}

	alloc, err := eng.ChildAllocations()
	if err != nil {
		return err
	}

	return batch.RebalanceTo(ctx, alloc)
}

// makeLowPriceTestData creates a DataFrame with low, stable prices suitable
// for meta-strategy tests where child accounts start with limited capital.
// Prices are ~10-14 per share so that child accounts (starting with $100)
// can buy several shares.
//
// Layout: column-major -- (assetIdx * numMetrics + metricIdx) * numDays + dayIdx.
func makeLowPriceTestData(start time.Time, numDays int, testAssets []asset.Asset, metrics []data.Metric) *data.DataFrame {
	times := make([]time.Time, numDays)
	for dayIdx := range times {
		day := start.AddDate(0, 0, dayIdx)
		times[dayIdx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
	}

	numAssets := len(testAssets)
	numMetrics := len(metrics)
	vals := make([]float64, numDays*numAssets*numMetrics)

	for assetIdx := 0; assetIdx < numAssets; assetIdx++ {
		for metricIdx := 0; metricIdx < numMetrics; metricIdx++ {
			colStart := (assetIdx*numMetrics + metricIdx) * numDays
			for dayIdx := 0; dayIdx < numDays; dayIdx++ {
				basePrice := 10.0 + float64(assetIdx)*2.0 + float64(dayIdx)*0.01

				switch metrics[metricIdx] {
				case data.Dividend:
					vals[colStart+dayIdx] = 0.0
				case data.SplitFactor:
					vals[colStart+dayIdx] = 1.0
				case data.MetricHigh:
					vals[colStart+dayIdx] = basePrice * 1.01
				case data.MetricLow:
					vals[colStart+dayIdx] = basePrice * 0.99
				default:
					vals[colStart+dayIdx] = basePrice
				}
			}
		}
	}

	df, err := data.NewDataFrame(times, testAssets, metrics, data.Daily, vals)
	Expect(err).NotTo(HaveOccurred())
	return df
}

// --- Test suite ---

var _ = Describe("MetaStrategy", func() {
	var (
		spy           asset.Asset
		tlt           asset.Asset
		allAssets     []asset.Asset
		assetProvider *mockAssetProvider
		metrics       []data.Metric
		provider      data.DataProvider
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		tlt = asset.Asset{CompositeFigi: "FIGI-TLT", Ticker: "TLT"}
		allAssets = []asset.Asset{spy, tlt}
		assetProvider = &mockAssetProvider{assets: allAssets}
		metrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor}

		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		dataFrame := makeLowPriceTestData(dataStart, 400, allAssets, metrics)
		provider = data.NewTestProvider(metrics, dataFrame)
	})

	Context("complete backtest", func() {
		It("runs without error and produces a non-nil portfolio", func() {
			strategy := &testMetaStrategy{}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			backtestStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			backtestEnd := time.Date(2024, 4, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())
		})
	})

	Context("parent portfolio holds underlying assets", func() {
		It("has positions in SPY and/or TLT with recorded transactions", func() {
			strategy := &testMetaStrategy{}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			backtestStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			backtestEnd := time.Date(2024, 4, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
			Expect(err).NotTo(HaveOccurred())

			// Parent portfolio should hold underlying assets (SPY and/or TLT).
			spyPosition := fund.Position(spy)
			tltPosition := fund.Position(tlt)
			Expect(spyPosition + tltPosition).To(BeNumerically(">", 0),
				"expected parent portfolio to hold SPY and/or TLT, got SPY=%.0f TLT=%.0f", spyPosition, tltPosition)

			// Verify transactions were recorded.
			txns := fund.Transactions()
			Expect(len(txns)).To(BeNumerically(">", 0), "expected recorded transactions")

			hasBuy := false
			for _, txn := range txns {
				if txn.Type == portfolio.BuyTransaction {
					hasBuy = true
					break
				}
			}
			Expect(hasBuy).To(BeTrue(), "expected at least one buy transaction")
		})
	})

	Context("approximate weight split", func() {
		It("holds SPY and TLT in approximately 60/40 ratio", func() {
			strategy := &testMetaStrategy{}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			backtestStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			backtestEnd := time.Date(2024, 4, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
			Expect(err).NotTo(HaveOccurred())

			spyValue := fund.PositionValue(spy)
			tltValue := fund.PositionValue(tlt)
			totalPositionValue := spyValue + tltValue
			Expect(totalPositionValue).To(BeNumerically(">", 0),
				"expected non-zero total position value")

			spyWeight := spyValue / totalPositionValue
			tltWeight := tltValue / totalPositionValue

			// Allow generous tolerance since prices drift daily and we buy
			// whole shares, but the weights should be approximately 60/40.
			Expect(spyWeight).To(BeNumerically("~", 0.60, 0.15),
				"SPY weight %.2f not close to 0.60", spyWeight)
			Expect(tltWeight).To(BeNumerically("~", 0.40, 0.15),
				"TLT weight %.2f not close to 0.40", tltWeight)
		})
	})

	Context("ChildPortfolios returns correct entries", func() {
		It("reports spy-child and tlt-child from within Compute", func() {
			strategy := &testMetaStrategyWithChildCheck{}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			backtestStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			backtestEnd := time.Date(2024, 4, 30, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
			Expect(err).NotTo(HaveOccurred())

			// The strategy recorded ChildPortfolioNames during every Compute call.
			Expect(strategy.ChildPortfolioNames).To(ContainElement("spy-child"))
			Expect(strategy.ChildPortfolioNames).To(ContainElement("tlt-child"))
		})
	})

	Context("PredictedPortfolio with meta-strategy", func() {
		It("returns a non-nil portfolio with transactions after a backtest", func() {
			strategy := &testMetaStrategy{}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			backtestStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			backtestEnd := time.Date(2024, 4, 30, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
			Expect(err).NotTo(HaveOccurred())

			predictedPortfolio, predErr := eng.PredictedPortfolio(context.Background())
			Expect(predErr).NotTo(HaveOccurred())
			Expect(predictedPortfolio).NotTo(BeNil())

			txns := predictedPortfolio.Transactions()
			Expect(len(txns)).To(BeNumerically(">", 0),
				"expected predicted portfolio to have transactions")

			hasBuy := false
			for _, txn := range txns {
				if txn.Type == portfolio.BuyTransaction {
					hasBuy = true
					break
				}
			}
			Expect(hasBuy).To(BeTrue(), "expected at least one buy transaction in predicted portfolio")
		})
	})

	Context("ChildAllocations with overrides", func() {
		It("reflects 80/20 weight split when overrides are used", func() {
			strategy := &testMetaStrategyWithOverride{}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			backtestStart := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			backtestEnd := time.Date(2024, 4, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
			Expect(err).NotTo(HaveOccurred())

			spyValue := fund.PositionValue(spy)
			tltValue := fund.PositionValue(tlt)
			totalPositionValue := spyValue + tltValue
			Expect(totalPositionValue).To(BeNumerically(">", 0),
				"expected non-zero total position value")

			spyWeight := spyValue / totalPositionValue
			tltWeight := tltValue / totalPositionValue

			// With 80/20 overrides, SPY should be notably higher than the
			// default 60/40 split, and TLT notably lower.
			Expect(spyWeight).To(BeNumerically("~", 0.80, 0.15),
				"SPY weight %.2f not close to 0.80 with override", spyWeight)
			Expect(tltWeight).To(BeNumerically("~", 0.20, 0.15),
				"TLT weight %.2f not close to 0.20 with override", tltWeight)
		})
	})
})
