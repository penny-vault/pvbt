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

// testIndexProvider implements both DataProvider (via BatchProvider) and IndexProvider.
type testIndexProvider struct {
	metrics []data.Metric
	members map[int64][]asset.Asset // keyed by Unix seconds
}

func (p *testIndexProvider) Provides() []data.Metric { return p.metrics }
func (p *testIndexProvider) Close() error            { return nil }
func (p *testIndexProvider) Fetch(_ context.Context, _ data.DataRequest) (*data.DataFrame, error) {
	df, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
	return df, err
}
func (p *testIndexProvider) IndexMembers(_ context.Context, _ string, t time.Time) ([]asset.Asset, error) {
	return p.members[t.Unix()], nil
}

var _ = Describe("Engine.IndexUniverse", func() {
	It("returns a universe wired with the engine's data source", func() {
		aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		t := time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC)

		provider := &testIndexProvider{
			metrics: []data.Metric{data.MetricClose},
			members: map[int64][]asset.Asset{
				t.Unix(): {aapl},
			},
		}

		eng := engine.New(&noScheduleStrategy{}, engine.WithDataProvider(provider))
		u := eng.IndexUniverse("SP500")

		got := u.Assets(t)
		Expect(got).To(ConsistOf(aapl))
	})

	It("panics when no provider implements IndexProvider", func() {
		provider := data.NewTestProvider([]data.Metric{data.MetricClose}, nil)

		eng := engine.New(&noScheduleStrategy{}, engine.WithDataProvider(provider))

		Expect(func() {
			eng.IndexUniverse("SP500")
		}).To(Panic())
	})
})
