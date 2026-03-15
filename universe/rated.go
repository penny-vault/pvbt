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
var _ Universe = (*ratedUniverse)(nil)

// ratedUniverse resolves universe membership from a RatingProvider and caches
// results in memory keyed by Unix seconds.
type ratedUniverse struct {
	provider data.RatingProvider
	analyst  string
	filter   data.RatingFilter
	ds       DataSource

	mu    sync.Mutex
	cache map[int64][]asset.Asset // keyed by Unix seconds
}

// NewRated creates a rated universe backed by the given provider. The universe
// has no data source until SetDataSource is called (or it is created via
// engine.RatedUniverse()).
func NewRated(provider data.RatingProvider, analyst string, filter data.RatingFilter) *ratedUniverse {
	return &ratedUniverse{
		provider: provider,
		analyst:  analyst,
		filter:   filter,
		cache:    make(map[int64][]asset.Asset),
	}
}

// SetDataSource wires the universe to a data source.
func (u *ratedUniverse) SetDataSource(ds DataSource) {
	u.ds = ds
}

// Assets returns the rated assets at time t, sorted by ticker. Results are
// cached. Errors from the provider are swallowed and nil is returned.
func (u *ratedUniverse) Assets(asOfDate time.Time) []asset.Asset {
	u.mu.Lock()
	defer u.mu.Unlock()

	key := asOfDate.Unix()
	if members, ok := u.cache[key]; ok {
		return members
	}

	members, err := u.provider.RatedAssets(context.Background(), u.analyst, u.filter, asOfDate)
	if err != nil || len(members) == 0 {
		return nil
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].Ticker < members[j].Ticker
	})

	u.cache[key] = members

	return members
}

// Prefetch pre-populates the cache for every day in [start, end].
func (u *ratedUniverse) Prefetch(ctx context.Context, start, end time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		key := date.Unix()
		if _, ok := u.cache[key]; ok {
			continue
		}

		members, err := u.provider.RatedAssets(ctx, u.analyst, u.filter, date)
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
func (u *ratedUniverse) Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.RatedUniverse()?")
	}

	members := u.Assets(u.ds.CurrentDate())

	return u.ds.Fetch(ctx, members, lookback, metrics)
}

// At returns a single-row DataFrame at time t for the resolved assets and
// requested metrics.
func (u *ratedUniverse) At(ctx context.Context, asOfDate time.Time, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.RatedUniverse()?")
	}

	members := u.Assets(asOfDate)

	return u.ds.FetchAt(ctx, members, asOfDate, metrics)
}

// CurrentDate returns the current simulation date from the data source, or
// zero time if no data source is set.
func (u *ratedUniverse) CurrentDate() time.Time {
	if u.ds == nil {
		return time.Time{}
	}

	return u.ds.CurrentDate()
}
