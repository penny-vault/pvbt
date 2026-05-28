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

// marketCloseHour is the regular-session NYSE close in Eastern time. EOD
// data for a given trading day is not expected to be available until after
// this hour has passed.
const marketCloseHour = 16

// adjustEndForExpectedEOD trims end down to the latest trading day for
// which EOD data should be available given the wall-clock now. The intent
// is to keep the step loop from ever reaching a date whose end-of-day bars
// the data feed has not had a chance to publish.
//
// Rules:
//   - If now is before today's 4 PM ET close (or today is not a trading
//     day), the latest expected EOD is the previous trading day. No data
//     call is made.
//   - If now is at or after today's 4 PM ET close, the latest expected
//     EOD is today. The strategy's assets are probed once for today; if
//     any are missing the result steps back exactly one trading day to
//     cover the in-flight ingest case.
//
// Genuine per-asset data gaps (delistings, broken feeds) are intentionally
// left to the broker's mid-backtest delisting path.
func (e *Engine) adjustEndForExpectedEOD(ctx context.Context, end, now time.Time) time.Time {
	nowLocal := now.In(nyc)
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, nyc)
	closeToday := time.Date(today.Year(), today.Month(), today.Day(), marketCloseHour, 0, 0, 0, nyc)

	marketStatus := tradecron.NewMarketStatus(&tradecron.RegularHours)

	var expectedLast time.Time
	if marketStatus.IsMarketDay(today) && !nowLocal.Before(closeToday) {
		expectedLast = today
		if !e.allStrategyAssetsPriced(ctx, today) {
			expectedLast = previousTradingDay(today, marketStatus)
		}
	} else {
		expectedLast = previousTradingDay(today, marketStatus)
	}

	expectedEnd := time.Date(expectedLast.Year(), expectedLast.Month(), expectedLast.Day(),
		23, 59, 59, int(time.Second-time.Nanosecond), nyc)

	if !end.After(expectedEnd) {
		return end
	}

	zerolog.Ctx(ctx).Warn().
		Time("requested_end", end).
		Time("adjusted_end", expectedEnd).
		Msg("end-of-day data not yet available; truncating backtest end")

	return expectedEnd
}

// previousTradingDay returns the most recent trading day strictly before
// `from`, using the provided market status for the holiday calendar.
func previousTradingDay(from time.Time, marketStatus *tradecron.MarketStatus) time.Time {
	candidate := from.AddDate(0, 0, -1)
	for !marketStatus.IsMarketDay(candidate) {
		candidate = candidate.AddDate(0, 0, -1)
	}

	return candidate
}

// allStrategyAssetsPriced reports whether every statically-known strategy
// asset (parent plus children, including the benchmark) has a non-NaN
// MetricClose on the given date. Strategies with only dynamic universes
// have nothing to probe and are reported as priced.
func (e *Engine) allStrategyAssetsPriced(ctx context.Context, date time.Time) bool {
	assets := collectStrategyAssets(e.strategy, e.benchmark)
	for _, child := range e.children {
		for _, childAsset := range collectStrategyAssets(child.strategy, asset.Asset{}) {
			assets = appendUniqueAsset(assets, childAsset)
		}
	}

	if len(assets) == 0 {
		return true
	}

	df, fetchErr := e.fetchRange(ctx, assets, []data.Metric{data.MetricClose}, date, date)
	if fetchErr != nil || df.Len() == 0 {
		return false
	}

	targetKey := data.DateKey(date)
	times := df.Times()

	for tIdx, ts := range times {
		if data.DateKey(ts) != targetKey {
			continue
		}

		for _, ast := range assets {
			col := df.Column(ast, data.MetricClose)
			if tIdx >= len(col) || math.IsNaN(col[tIdx]) {
				return false
			}
		}

		return true
	}

	return false
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
