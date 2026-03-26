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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// predictStrategy always rebalances to 100% SPY.
type predictStrategy struct {
	spy      asset.Asset
	schedule string
}

func (s *predictStrategy) Name() string { return "predict-test" }

func (s *predictStrategy) Setup(eng *engine.Engine) {
	s.spy = eng.Asset("SPY")
}

func (s *predictStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: s.schedule}
}

func (s *predictStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	df, err := eng.FetchAt(ctx, []asset.Asset{s.spy}, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil {
		return err
	}

	price := df.Value(s.spy, data.MetricClose)
	if price <= 0 {
		return nil
	}

	batch.Annotate("action", "buy SPY")

	return batch.RebalanceTo(ctx, portfolio.Allocation{
		Date:          eng.CurrentDate(),
		Members:       map[asset.Asset]float64{s.spy: 1.0},
		Justification: "always buy SPY",
	})
}

// Ensure predictStrategy satisfies the Strategy interface.
var _ engine.Strategy = (*predictStrategy)(nil)

var _ = Describe("PredictedPortfolio", func() {
	var (
		spyAsset      asset.Asset
		testAssets    []asset.Asset
		assetProvider *mockAssetProvider
		metrics       []data.Metric
		dataStart     time.Time
	)

	BeforeEach(func() {
		spyAsset = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets = []asset.Asset{spyAsset}
		assetProvider = &mockAssetProvider{assets: testAssets}
		metrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
		dataStart = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	})

	It("predicts trades mid-month for a monthly strategy", func() {
		testData := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, testData)

		// Schedule fires on the 5th of each month; Jan 5 falls within the backtest
		// window so currentDate is set, and PredictedPortfolio will predict Feb 5.
		strategy := &predictStrategy{schedule: "0 16 5 * *"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		backtestStart := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		backtestEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
		Expect(err).NotTo(HaveOccurred())

		predictedPortfolio, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predictedPortfolio).NotTo(BeNil())

		transactions := predictedPortfolio.Transactions()
		hasBuy := false
		for _, tx := range transactions {
			if tx.Type == asset.BuyTransaction {
				hasBuy = true
				break
			}
		}
		Expect(hasBuy).To(BeTrue(), "expected buy transactions in predicted portfolio")
	})

	It("predicts with minimal forward-fill for day-before", func() {
		testData := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, testData)

		strategy := &predictStrategy{schedule: "0 16 * * 1-5"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		backtestStart := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		backtestEnd := time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
		Expect(err).NotTo(HaveOccurred())

		predictedPortfolio, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predictedPortfolio).NotTo(BeNil())
	})

	It("does not mutate the original account", func() {
		testData := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, testData)

		strategy := &predictStrategy{schedule: "0 16 * * 1-5"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		backtestStart := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		backtestEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		originalPortfolio, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
		Expect(err).NotTo(HaveOccurred())

		originalCash := originalPortfolio.Cash()
		originalTxnCount := len(originalPortfolio.Transactions())

		_, err = eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())

		Expect(originalPortfolio.Cash()).To(Equal(originalCash), "original cash should not change after PredictedPortfolio")
		Expect(len(originalPortfolio.Transactions())).To(Equal(originalTxnCount), "original transaction count should not change after PredictedPortfolio")
	})

	It("includes annotations and justifications on predicted portfolio", func() {
		testData := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, testData)

		strategy := &predictStrategy{schedule: "0 16 * * 1-5"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		backtestStart := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		backtestEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
		Expect(err).NotTo(HaveOccurred())

		predictedPortfolio, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predictedPortfolio).NotTo(BeNil())

		annotations := predictedPortfolio.Annotations()
		Expect(annotations).NotTo(BeEmpty(), "predicted portfolio should have annotations")

		transactions := predictedPortfolio.Transactions()
		for _, tx := range transactions {
			if tx.Type == asset.BuyTransaction {
				Expect(tx.Justification).To(Equal("always buy SPY"), "buy transactions should have justification")
			}
		}
	})

	It("works with a daily strategy", func() {
		testData := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, testData)

		strategy := &predictStrategy{schedule: "0 16 * * 1-5"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		backtestStart := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		backtestEnd := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
		Expect(err).NotTo(HaveOccurred())

		predictedPortfolio, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predictedPortfolio).NotTo(BeNil())
	})

	It("works with a weekly strategy", func() {
		testData := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, testData)

		strategy := &predictStrategy{schedule: "0 16 * * 1"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		backtestStart := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		backtestEnd := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
		Expect(err).NotTo(HaveOccurred())

		predictedPortfolio, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predictedPortfolio).NotTo(BeNil())
	})

	It("returns error when no schedule is set", func() {
		strategy := &noScheduleStrategy{}
		eng := engine.New(strategy,
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		backtestStart := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		backtestEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), backtestStart, backtestEnd)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("schedule"))
	})
})
