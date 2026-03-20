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
	"github.com/penny-vault/pvbt/universe"
)

// hydrateStrategy has default-tagged fields that the engine should hydrate
// before Setup is called. Compute records field values for assertion.
type hydrateStrategy struct {
	FloatVal    float64           `default:"3.14"`
	IntVal      int               `default:"42"`
	StringVal   string            `default:"hello"`
	BoolVal     bool              `default:"true"`
	DurationVal time.Duration     `default:"5m"`
	AssetVal    asset.Asset       `default:"AAPL"`
	UniverseVal universe.Universe `default:"AAPL,GOOG"`
	PreSet      float64           `default:"99.0"`

	// captured during Compute for assertions
	computeCalled bool
}

func (s *hydrateStrategy) Name() string { return "hydrateStrategy" }

func (s *hydrateStrategy) Setup(_ *engine.Engine) {}

func (s *hydrateStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *hydrateStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
	s.computeCalled = true
	return nil
}

var _ = Describe("Hydration", func() {
	var (
		aapl          asset.Asset
		goog          asset.Asset
		assetProvider *mockAssetProvider
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		assetProvider = &mockAssetProvider{assets: []asset.Asset{aapl, goog}}
	})

	It("re-wires pre-set universe fields with the engine data source", func() {
		metrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 90, []asset.Asset{aapl, goog}, metrics)
		provider := data.NewTestProvider(metrics, df)

		// Simulate what CLI applyStrategyFlags does: set the universe field
		// to a StaticUniverse without a data source.
		strategy := &hydrateStrategy{
			UniverseVal: universe.NewStatic("AAPL", "GOOG"),
		}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		// The engine should have re-wired the universe with a data source.
		// Verify by checking the assets are resolved (have CompositeFigi).
		members := strategy.UniverseVal.Assets(time.Now())
		Expect(members).To(HaveLen(2))
		Expect(members[0].CompositeFigi).To(Equal("FIGI-AAPL"))
		Expect(members[1].CompositeFigi).To(Equal("FIGI-GOOG"))
	})

	It("populates default-tagged fields before Compute runs", func() {
		metrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 90, []asset.Asset{aapl, goog}, metrics)
		provider := data.NewTestProvider(metrics, df)

		strategy := &hydrateStrategy{PreSet: 1.0}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.computeCalled).To(BeTrue())

		Expect(strategy.FloatVal).To(Equal(3.14))
		Expect(strategy.IntVal).To(Equal(42))
		Expect(strategy.StringVal).To(Equal("hello"))
		Expect(strategy.BoolVal).To(BeTrue())
		Expect(strategy.DurationVal).To(Equal(5 * time.Minute))
		Expect(strategy.AssetVal.CompositeFigi).To(Equal("FIGI-AAPL"))
		Expect(strategy.UniverseVal).NotTo(BeNil())

		members := strategy.UniverseVal.Assets(time.Now())
		Expect(members).To(HaveLen(2))

		// PreSet should NOT be overwritten since it was non-zero.
		Expect(strategy.PreSet).To(Equal(1.0))
	})
})
