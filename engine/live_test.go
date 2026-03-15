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
	"github.com/penny-vault/pvbt/tradecron"
)

// liveStrategy is a minimal strategy that sets a schedule and does nothing in Compute.
type liveStrategy struct{}

func (s *liveStrategy) Name() string { return "liveStrategy" }

func (s *liveStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic("liveStrategy.Setup: " + err.Error())
	}
	eng.Schedule(tc)
}

func (s *liveStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
	return nil
}

var _ = Describe("RunLive", func() {
	Context("context cancellation", func() {
		It("closes the channel when the context is cancelled", func() {
			aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
			testAssets := []asset.Asset{aapl}

			dataStart := time.Now().AddDate(0, 0, -30)
			metrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
			df := makeDailyTestData(dataStart, 60, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)
			assetProvider := &mockAssetProvider{assets: testAssets}

			eng := engine.New(&liveStrategy{},
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			ch, err := eng.RunLive(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ch).NotTo(BeNil())

			// Drain the channel until it is closed (context timeout triggers close).
			for range ch {
			}
			// Reaching here means the channel was closed without panicking.
		})
	})

	Context("validation", func() {
		It("returns an error when no schedule is set", func() {
			aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
			assetProvider := &mockAssetProvider{assets: []asset.Asset{aapl}}

			eng := engine.New(&noScheduleStrategy{},
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			_, err := eng.RunLive(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("schedule"))
		})
	})
})
