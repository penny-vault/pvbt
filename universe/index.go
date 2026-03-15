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
	"sort"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// compile-time check
var _ Universe = (*indexUniverse)(nil)

// indexUniverse resolves index membership from an IndexProvider and caches
// results in memory. It fetches on demand when Assets is called for a time
// outside the cached range, or all at once if Prefetch is called.
type indexUniverse struct {
	provider  data.IndexProvider
	indexName string
	ds        DataSource

	mu    sync.Mutex
	cache map[int64][]asset.Asset // keyed by Unix seconds
}

// NewIndex creates an index universe backed by the given provider. The universe
// has no data source until SetDataSource is called (or it is created via
// engine.IndexUniverse()).
func NewIndex(provider data.IndexProvider, indexName string) *indexUniverse {
	return &indexUniverse{
		provider:  provider,
		indexName: indexName,
		cache:     make(map[int64][]asset.Asset),
	}
}

// SetDataSource wires the universe to a data source.
func (u *indexUniverse) SetDataSource(ds DataSource) {
	u.ds = ds
}

func (u *indexUniverse) Assets(asOfDate time.Time) []asset.Asset {
	u.mu.Lock()
	defer u.mu.Unlock()

	key := asOfDate.Unix()
	if members, ok := u.cache[key]; ok {
		return members
	}

	members, err := u.provider.IndexMembers(context.Background(), u.indexName, asOfDate)
	if err != nil || len(members) == 0 {
		return nil
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].Ticker < members[j].Ticker
	})

	u.cache[key] = members

	return members
}

func (u *indexUniverse) Prefetch(ctx context.Context, start, end time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		key := date.Unix()
		if _, ok := u.cache[key]; ok {
			continue
		}

		members, err := u.provider.IndexMembers(ctx, u.indexName, date)
		if err != nil {
			return err
		}

		if len(members) > 0 {
			sort.Slice(members, func(i, j int) bool {
				return members[i].Ticker < members[j].Ticker
			})
			u.cache[key] = members
		}
	}

	return nil
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

// At returns a single-row DataFrame at time t for the resolved assets and
// requested metrics.
func (u *indexUniverse) At(ctx context.Context, asOfDate time.Time, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.IndexUniverse()?")
	}

	members := u.Assets(asOfDate)

	return u.ds.FetchAt(ctx, members, asOfDate, metrics)
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
	return NewIndex(p, "SP500")
}

// Nasdaq100 creates a universe tracking Nasdaq 100 membership historically.
func Nasdaq100(p data.IndexProvider) *indexUniverse {
	return NewIndex(p, "NASDAQ100")
}
