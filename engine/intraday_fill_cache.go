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

package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// intradayFillCache memoizes the next-minute-bar window fetched during a single
// intra-day firing. Order fills route through nextMinuteBar once per order (via
// SimulatedBroker.Submit) plus once for the batch prefetch
// (prefetchBrokerPrices); against a remote ClickHouse source each call is an
// independent round-trip for the same [currentTime+1m, currentTime+6m] window.
// The cache holds the window fetched for the union of requested assets so every
// per-order lookup within the firing is served locally. It is keyed by the
// firing timestamp and is invalid once currentTime advances to the next firing,
// so it never serves stale bars across firings.
type intradayFillCache struct {
	firingTime time.Time
	assets     []asset.Asset
	covered    map[string]bool
	window     *data.DataFrame
}

// covers reports whether the cached window already includes every requested
// asset.
func (c *intradayFillCache) covers(assets []asset.Asset) bool {
	for _, want := range assets {
		if !c.covered[want.CompositeFigi] {
			return false
		}
	}

	return true
}

// intradayFillWindow returns the minute-bar window covering the requested
// assets for the current firing, issuing an IntradayFetch only on a cache miss.
// A miss within the same firing widens the fetch to the union of previously
// cached and newly requested assets, so an asset fetched earlier in the firing
// is never evicted by a later lookup for a different one.
func (e *Engine) intradayFillWindow(
	ctx context.Context,
	provider IntradayProvider,
	assets []asset.Asset,
	metrics []data.Metric,
	start, end time.Time,
) (*data.DataFrame, error) {
	if e.intradayFill != nil && e.intradayFill.firingTime.Equal(e.currentTime) {
		if e.intradayFill.covers(assets) {
			return e.intradayFill.window, nil
		}

		assets = unionAssets(e.intradayFill.assets, assets)
	}

	window, err := provider.IntradayFetch(ctx, assets, metrics, start, end, nil)
	if err != nil {
		return nil, fmt.Errorf("engine: fetch intraday fill window: %w", err)
	}

	covered := make(map[string]bool, len(assets))
	for _, held := range assets {
		covered[held.CompositeFigi] = true
	}

	e.intradayFill = &intradayFillCache{
		firingTime: e.currentTime,
		assets:     assets,
		covered:    covered,
		window:     window,
	}

	return window, nil
}

// unionAssets returns the assets in base followed by any in extra not already
// present, deduplicating by composite figi while preserving order.
func unionAssets(base, extra []asset.Asset) []asset.Asset {
	seen := make(map[string]bool, len(base)+len(extra))
	union := make([]asset.Asset, 0, len(base)+len(extra))

	for _, group := range [][]asset.Asset{base, extra} {
		for _, held := range group {
			if seen[held.CompositeFigi] {
				continue
			}

			seen[held.CompositeFigi] = true
			union = append(union, held)
		}
	}

	return union
}
