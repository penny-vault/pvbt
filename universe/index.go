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

// indexUniverse resolves index membership from an IndexProvider and caches
// results in memory. It fetches on demand when Assets is called for a time
// outside the cached range, or all at once if Prefetch is called.
type indexUniverse struct {
	provider  data.IndexProvider
	indexName string

	mu         sync.Mutex
	cachedFrom time.Time
	cachedTo   time.Time
	cache      map[time.Time][]asset.Asset
}

func (u *indexUniverse) Assets(t time.Time) []asset.Asset {
	u.mu.Lock()
	defer u.mu.Unlock()

	if members, ok := u.cache[t]; ok {
		return members
	}

	members, err := u.provider.IndexMembers(context.Background(), u.indexName, t)
	if err != nil {
		return nil
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].Ticker < members[j].Ticker
	})

	if u.cache == nil {
		u.cache = make(map[time.Time][]asset.Asset)
	}
	u.cache[t] = members

	return members
}

func (u *indexUniverse) Prefetch(ctx context.Context, start, end time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Already covers the requested range.
	if !u.cachedFrom.IsZero() && !start.Before(u.cachedFrom) && !end.After(u.cachedTo) {
		return nil
	}

	// Walk each day in the range and fetch membership. The provider
	// may optimize this internally, but from our side we cache each
	// distinct date we'll need.
	for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
		if _, ok := u.cache[t]; ok {
			continue
		}

		members, err := u.provider.IndexMembers(ctx, u.indexName, t)
		if err != nil {
			return err
		}

		sort.Slice(members, func(i, j int) bool {
			return members[i].Ticker < members[j].Ticker
		})

		if u.cache == nil {
			u.cache = make(map[time.Time][]asset.Asset)
		}
		u.cache[t] = members
	}

	u.cachedFrom = start
	u.cachedTo = end

	return nil
}

func (u *indexUniverse) Window(_ portfolio.Period, _ ...data.Metric) (*data.DataFrame, error) {
	return nil, fmt.Errorf("indexUniverse has no data source; was it created via engine.Universe()?")
}

func (u *indexUniverse) At(_ time.Time, _ ...data.Metric) (*data.DataFrame, error) {
	return nil, fmt.Errorf("indexUniverse has no data source; was it created via engine.Universe()?")
}

// SP500 creates a universe tracking S&P 500 membership historically.
func SP500(p data.IndexProvider) Universe {
	return &indexUniverse{provider: p, indexName: "SP500"}
}

// Nasdaq100 creates a universe tracking Nasdaq 100 membership historically.
func Nasdaq100(p data.IndexProvider) Universe {
	return &indexUniverse{provider: p, indexName: "NASDAQ100"}
}
