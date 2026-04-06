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

package universe

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// compile-time check
var _ Universe = (*indexUniverse)(nil)

// indexUniverse resolves index membership from an IndexProvider. The provider
// owns the membership state; this type is a thin wrapper that delegates
// Assets calls and provides Window/At data access.
type indexUniverse struct {
	provider  data.IndexProvider
	indexName string
	ds        DataSource
}

// NewIndex creates an index universe backed by the given provider. The universe
// has no data source until SetDataSource is called (or it is created via
// engine.IndexUniverse()).
func NewIndex(provider data.IndexProvider, indexName string) *indexUniverse {
	return &indexUniverse{
		provider:  provider,
		indexName: indexName,
	}
}

// SetDataSource wires the universe to a data source.
func (u *indexUniverse) SetDataSource(ds DataSource) {
	u.ds = ds
}

// Assets returns the index members at the given date. The returned slice is
// borrowed from the provider and is only valid for the current engine step.
// Callers that need data across steps must copy. Dates must be monotonically
// increasing across calls.
func (u *indexUniverse) Assets(asOfDate time.Time) []asset.Asset {
	assets, _, err := u.provider.IndexMembers(context.Background(), u.indexName, asOfDate)
	if err != nil || len(assets) == 0 {
		return nil
	}

	return assets
}

// Constituents returns the index members with their weights at the given date.
// The returned slice is borrowed from the provider and is only valid for the
// current engine step. Since the provider is stateful and monotonically
// advancing, calling Constituents with the same date as a preceding Assets
// call is a no-op that returns the cached parallel slice.
func (u *indexUniverse) Constituents(asOfDate time.Time) []data.IndexConstituent {
	_, constituents, err := u.provider.IndexMembers(context.Background(), u.indexName, asOfDate)
	if err != nil || len(constituents) == 0 {
		return nil
	}

	return constituents
}

// Window returns a DataFrame covering [currentDate - lookback, currentDate]
// for the resolved assets and requested metrics.
func (u *indexUniverse) Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.IndexUniverse()?")
	}

	members := u.Assets(u.ds.CurrentDate())

	return u.ds.Fetch(ctx, members, lookback, metrics)
}

// At returns a single-row DataFrame at CurrentDate() for the resolved assets
// and requested metrics.
func (u *indexUniverse) At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.IndexUniverse()?")
	}

	now := u.ds.CurrentDate()
	members := u.Assets(now)

	return u.ds.FetchAt(ctx, members, now, metrics)
}

// CurrentDate returns the current simulation date from the data source, or
// zero time if no data source is set.
func (u *indexUniverse) CurrentDate() time.Time {
	if u.ds == nil {
		return time.Time{}
	}

	return u.ds.CurrentDate()
}

// SP500 creates a universe tracking S&P 500 membership historically.
func SP500(p data.IndexProvider) *indexUniverse {
	return NewIndex(p, "SPX")
}

// Nasdaq100 creates a universe tracking Nasdaq 100 membership historically.
func Nasdaq100(p data.IndexProvider) *indexUniverse {
	return NewIndex(p, "NDX")
}
