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

type dimensionStrategy struct {
	dimensionToSet string
	metrics        []data.Metric
	assets         []asset.Asset
	fetched        *data.DataFrame
	fetchErr       error
}

func (s *dimensionStrategy) Name() string { return "dimensionStrategy" }

func (s *dimensionStrategy) Setup(eng *engine.Engine) {
	if s.dimensionToSet != "" {
		eng.SetFundamentalDimension(s.dimensionToSet)
	}
}

func (s *dimensionStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *dimensionStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	s.fetched, s.fetchErr = eng.FetchAt(ctx, s.assets, eng.CurrentDate(), s.metrics)
	return nil
}

var _ = Describe("SetFundamentalDimension", func() {
	It("accepts valid dimensions", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}

		closeDF := makeDailyDF(
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			90, testAssets, []data.Metric{data.MetricClose},
		)
		provider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)
		assetProv := &mockAssetProvider{assets: testAssets}

		strategy := &dimensionStrategy{
			dimensionToSet: "MRQ",
			metrics:        []data.Metric{data.MetricClose},
			assets:         testAssets,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects invalid dimensions", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}

		closeDF := makeDailyDF(
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			90, testAssets, []data.Metric{data.MetricClose},
		)
		provider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)
		assetProv := &mockAssetProvider{assets: testAssets}

		strategy := &dimensionStrategy{
			dimensionToSet: "INVALID",
			metrics:        []data.Metric{data.MetricClose},
			assets:         testAssets,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid fundamental dimension"))
	})
})
