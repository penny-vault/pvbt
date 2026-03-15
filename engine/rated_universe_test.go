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
)

// testRatingProvider implements both DataProvider (via BatchProvider) and RatingProvider.
type testRatingProvider struct {
	metrics []data.Metric
	assets  map[int64][]asset.Asset // keyed by Unix seconds
}

func (p *testRatingProvider) Provides() []data.Metric { return p.metrics }
func (p *testRatingProvider) Close() error            { return nil }
func (p *testRatingProvider) Fetch(_ context.Context, _ data.DataRequest) (*data.DataFrame, error) {
	df, err := data.NewDataFrame(nil, nil, nil, nil)
	return df, err
}
func (p *testRatingProvider) RatedAssets(_ context.Context, _ string, _ data.RatingFilter, t time.Time) ([]asset.Asset, error) {
	return p.assets[t.Unix()], nil
}

var _ = Describe("Engine.RatedUniverse", func() {
	It("returns a universe wired with the engine's data source", func() {
		aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		t := time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC)

		provider := &testRatingProvider{
			metrics: []data.Metric{data.MetricClose},
			assets: map[int64][]asset.Asset{
				t.Unix(): {aapl},
			},
		}

		e := engine.New(&noScheduleStrategy{}, engine.WithDataProvider(provider))
		u := e.RatedUniverse("zacks-rank", data.RatingEq(1))

		got := u.Assets(t)
		Expect(got).To(ConsistOf(aapl))
	})

	It("panics when no provider implements RatingProvider", func() {
		provider := data.NewTestProvider([]data.Metric{data.MetricClose}, nil)

		e := engine.New(&noScheduleStrategy{}, engine.WithDataProvider(provider))

		Expect(func() {
			e.RatedUniverse("zacks-rank", data.RatingEq(1))
		}).To(Panic())
	})
})
