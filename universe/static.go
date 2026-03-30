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
var _ Universe = (*StaticUniverse)(nil)

// StaticUniverse is a fixed set of assets that does not change over time.
type StaticUniverse struct {
	members []asset.Asset
	ds      DataSource
}

func (u *StaticUniverse) Assets(_ time.Time) []asset.Asset { return u.members }

func (u *StaticUniverse) Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.Universe()?")
	}

	return u.ds.Fetch(ctx, u.members, lookback, metrics)
}

func (u *StaticUniverse) At(ctx context.Context, t time.Time, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.Universe()?")
	}

	return u.ds.FetchAt(ctx, u.members, t, metrics)
}

func (u *StaticUniverse) CurrentDate() time.Time {
	if u.ds == nil {
		return time.Time{}
	}

	return u.ds.CurrentDate()
}

// NewStatic creates a static universe from explicit ticker symbols.
// The universe has no data source until it is created via engine.Universe().
func NewStatic(tickers ...string) *StaticUniverse {
	members := make([]asset.Asset, len(tickers))
	for i, t := range tickers {
		members[i] = asset.Asset{Ticker: t}
	}

	return &StaticUniverse{members: members}
}

// NewStaticWithSource creates a static universe wired to a data source.
func NewStaticWithSource(assets []asset.Asset, ds DataSource) *StaticUniverse {
	return &StaticUniverse{members: assets, ds: ds}
}
