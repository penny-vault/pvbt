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
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/rs/zerolog"
)

// staleDataTolerance is the maximum number of trailing trading days that can
// be missing before a per-asset NaN is interpreted as a delisting rather
// than a transient data lag. A backtest run shortly after the close (or on
// a holiday) will commonly trail by 1-2 trading days while the data feed
// catches up; truncating end keeps those runs from spuriously liquidating
// every held position.
const staleDataTolerance = 2

// adjustEndForStaleData detects when the requested end is 1-2 trading days
// past the latest available close data and returns a truncated end aligned
// with the last fully-priced trading day. Larger gaps fall through to the
// existing per-asset delisting logic so genuinely abandoned data sources
// still surface as delistings.
//
// The probe uses the strategy's statically-known assets (asset fields plus
// the benchmark and explicit static universes). Strategies that rely solely
// on dynamic universes have nothing to probe and skip the check.
func (e *Engine) adjustEndForStaleData(ctx context.Context, end time.Time) time.Time {
	probeAssets := collectStrategyAssets(e.strategy, e.benchmark)
	for _, child := range e.children {
		for _, childAsset := range collectStrategyAssets(child.strategy, asset.Asset{}) {
			probeAssets = appendUniqueAsset(probeAssets, childAsset)
		}
	}

	if len(probeAssets) == 0 {
		return end
	}

	lookbackStart, walkErr := walkBackTradingDays(end, staleDataTolerance+3)
	if walkErr != nil {
		return end
	}

	df, fetchErr := e.fetchRange(ctx, probeAssets, []data.Metric{data.MetricClose}, lookbackStart, end)
	if fetchErr != nil || df.Len() == 0 {
		return end
	}

	times := df.Times()
	latest := time.Time{}

	for tIdx := len(times) - 1; tIdx >= 0; tIdx-- {
		if times[tIdx].After(end) {
			continue
		}

		for _, ast := range probeAssets {
			col := df.Column(ast, data.MetricClose)
			if tIdx >= len(col) {
				continue
			}

			if !math.IsNaN(col[tIdx]) {
				latest = times[tIdx]
				break
			}
		}

		if !latest.IsZero() {
			break
		}
	}

	if latest.IsZero() || !latest.Before(end) {
		return end
	}

	gap, gapErr := tradingDayGap(latest, end)
	if gapErr != nil || gap == 0 {
		return end
	}

	if gap > staleDataTolerance {
		return end
	}

	zerolog.Ctx(ctx).Warn().
		Time("requested_end", end).
		Time("adjusted_end", latest).
		Int("missing_trading_days", gap).
		Msg("trailing close data is stale; ending backtest at the last fully-priced trading day")

	return latest
}

// tradingDayGap counts trading days strictly after `from` up to and
// including `to`. Returns 0 if `to` is on or before `from`.
func tradingDayGap(from, to time.Time) (int, error) {
	if !to.After(from) {
		return 0, nil
	}

	daily, err := tradecron.New("@close * * *", tradecron.RegularHours)
	if err != nil {
		return 0, err
	}

	gap := 0

	cur := daily.Next(from.Add(time.Nanosecond))
	for !cur.After(to) {
		gap++
		cur = daily.Next(cur.Add(time.Nanosecond))
	}

	return gap, nil
}

func appendUniqueAsset(assets []asset.Asset, candidate asset.Asset) []asset.Asset {
	if candidate == (asset.Asset{}) || candidate.CompositeFigi == "" {
		return assets
	}

	for _, existing := range assets {
		if existing.CompositeFigi == candidate.CompositeFigi {
			return assets
		}
	}

	return append(assets, candidate)
}
