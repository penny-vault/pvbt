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
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/penny-vault/pvbt/universe"
)

// walkBackTradingDays finds the trading date that is `days` trading days
// before `from`. It uses a forward-walk approach since TradeCron only
// supports Next(). Returns an error if days is negative.
func walkBackTradingDays(from time.Time, days int) (time.Time, error) {
	if days < 0 {
		return time.Time{}, fmt.Errorf("walkBackTradingDays: days must be non-negative, got %d", days)
	}

	if days == 0 {
		return from, nil
	}

	daily, err := tradecron.New("@close * * *", tradecron.RegularHours)
	if err != nil {
		return time.Time{}, fmt.Errorf("walkBackTradingDays: creating daily schedule: %w", err)
	}

	// Estimate calendar days needed. Start with 2x multiplier to account
	// for weekends and holidays. Retry with doubled offset up to 3 times.
	multiplier := 2

	const maxAttempts = 3

	for attempt := range maxAttempts {
		calendarDays := days * multiplier * (1 << attempt) // 2x, 4x, 8x
		estimatedStart := from.AddDate(0, 0, -calendarDays)

		// Walk forward from estimated start, collecting trading days.
		var tradingDays []time.Time

		cur := daily.Next(estimatedStart.Add(-time.Nanosecond))

		for !cur.After(from) {
			tradingDays = append(tradingDays, cur)
			cur = daily.Next(cur.Add(time.Nanosecond))
		}

		if len(tradingDays) >= days+1 {
			// We have enough trading days. The target is at index
			// len(tradingDays) - 1 - days (counting back from `from`).
			targetIdx := len(tradingDays) - 1 - days
			return tradingDays[targetIdx], nil
		}
	}

	return time.Time{}, fmt.Errorf("walkBackTradingDays: could not find %d trading days before %s after %d attempts",
		days, from.Format("2006-01-02"), maxAttempts)
}

// validateWarmup checks that all strategy assets have sufficient data
// in the warmup window. It returns the (possibly adjusted) start date.
func (e *Engine) validateWarmup(ctx context.Context, start, end time.Time) (time.Time, error) {
	// Extract warmup from Describe() if available.
	if desc, ok := e.strategy.(Descriptor); ok {
		e.warmup = desc.Describe().Warmup
	}

	if e.warmup == 0 {
		return start, nil
	}

	if e.warmup < 0 {
		return time.Time{}, fmt.Errorf("engine: negative warmup value %d", e.warmup)
	}

	// Resolve start to the first scheduled trading date.
	firstTradeDate := e.schedule.Next(start.Add(-time.Nanosecond))

	// Collect assets to validate.
	strategyAssets := collectStrategyAssets(e.strategy, e.benchmark)
	if len(strategyAssets) == 0 {
		return start, nil
	}

	// Check warmup data availability.
	warmupStart, err := walkBackTradingDays(firstTradeDate, e.warmup)
	if err != nil {
		return time.Time{}, fmt.Errorf("engine: computing warmup start: %w", err)
	}

	insufficientAssets, checkErr := e.checkWarmupData(ctx, strategyAssets, warmupStart, firstTradeDate)
	if checkErr != nil {
		return time.Time{}, fmt.Errorf("engine: checking warmup data: %w", checkErr)
	}

	if len(insufficientAssets) == 0 {
		return start, nil
	}

	// Handle insufficient data based on mode.
	if e.dateRangeMode == DateRangeModeStrict {
		return time.Time{}, fmt.Errorf("engine: %s", e.formatWarmupShortfall(ctx, insufficientAssets, warmupStart, firstTradeDate))
	}

	// Permissive mode: scan forward to find a valid start date.
	daily, dailyErr := tradecron.New("@close * * *", tradecron.RegularHours)
	if dailyErr != nil {
		return time.Time{}, fmt.Errorf("engine: creating daily schedule for permissive scan: %w", dailyErr)
	}

	candidate := daily.Next(firstTradeDate.Add(time.Nanosecond))
	for !candidate.After(end) {
		candidateWarmupStart, walkErr := walkBackTradingDays(candidate, e.warmup)
		if walkErr != nil {
			return time.Time{}, fmt.Errorf("engine: computing warmup start for candidate %s: %w",
				candidate.Format("2006-01-02"), walkErr)
		}

		insufficientAssets, checkErr = e.checkWarmupData(ctx, insufficientAssets, candidateWarmupStart, candidate)
		if checkErr != nil {
			return time.Time{}, fmt.Errorf("engine: checking warmup data at %s: %w",
				candidate.Format("2006-01-02"), checkErr)
		}

		if len(insufficientAssets) == 0 {
			return candidate, nil
		}

		candidate = daily.Next(candidate.Add(time.Nanosecond))
	}

	return time.Time{}, fmt.Errorf("engine: no valid start date with sufficient warmup data before %s",
		end.Format("2006-01-02"))
}

// checkWarmupData fetches MetricClose over the warmup window and returns
// assets that have fewer than e.warmup non-NaN values.
func (e *Engine) checkWarmupData(ctx context.Context, assets []asset.Asset, warmupStart, tradeStart time.Time) ([]asset.Asset, error) {
	df, err := e.fetchRange(ctx, assets, []data.Metric{data.MetricClose}, warmupStart, tradeStart)
	if err != nil {
		return nil, err
	}

	var insufficient []asset.Asset

	for _, assetItem := range assets {
		col := df.Column(assetItem, data.MetricClose)
		nonNaN := 0

		for _, val := range col {
			if !math.IsNaN(val) {
				nonNaN++
			}
		}

		if nonNaN < e.warmup {
			insufficient = append(insufficient, assetItem)
		}
	}

	return insufficient, nil
}

// formatWarmupShortfall builds an error message listing each asset and
// how many trading days it is short by.
func (e *Engine) formatWarmupShortfall(ctx context.Context, assets []asset.Asset, warmupStart, tradeStart time.Time) string {
	df, err := e.fetchRange(ctx, assets, []data.Metric{data.MetricClose}, warmupStart, tradeStart)
	if err != nil {
		return fmt.Sprintf("assets with insufficient warmup data (fetch error: %v)", err)
	}

	var details []string

	for _, assetItem := range assets {
		col := df.Column(assetItem, data.MetricClose)
		nonNaN := 0

		for _, val := range col {
			if !math.IsNaN(val) {
				nonNaN++
			}
		}

		shortBy := e.warmup - nonNaN
		if shortBy > 0 {
			details = append(details, fmt.Sprintf("%s (short by %d days)", assetItem.Ticker, shortBy))
		}
	}

	return fmt.Sprintf("insufficient warmup data: %s", strings.Join(details, ", "))
}

// collectStrategyAssets reflects over the strategy struct to find all
// asset.Asset and static universe.Universe fields. It also includes
// the benchmark if set. Returns a deduplicated slice.
func collectStrategyAssets(strategy any, benchmark asset.Asset) []asset.Asset {
	seen := make(map[string]bool)

	var result []asset.Asset

	addAsset := func(assetToAdd asset.Asset) {
		if assetToAdd == (asset.Asset{}) || assetToAdd.CompositeFigi == "" {
			return
		}

		if seen[assetToAdd.CompositeFigi] {
			return
		}

		seen[assetToAdd.CompositeFigi] = true
		result = append(result, assetToAdd)
	}

	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		addAsset(benchmark)
		return result
	}

	targetType := val.Type()

	for fieldIdx := 0; fieldIdx < targetType.NumField(); fieldIdx++ {
		field := targetType.Field(fieldIdx)
		if !field.IsExported() {
			continue
		}

		fieldValue := val.Field(fieldIdx)

		// Check for asset.Asset fields.
		if field.Type == assetType {
			addAsset(fieldValue.Interface().(asset.Asset))
			continue
		}

		// Check for universe.Universe fields (interface).
		if field.Type.Implements(universeType) && !fieldValue.IsNil() {
			universeVal := fieldValue.Interface().(universe.Universe)
			// Only extract assets from static universes.
			if _, isStatic := universeVal.(*universe.StaticUniverse); isStatic {
				for _, member := range universeVal.Assets(time.Time{}) {
					addAsset(member)
				}
			}
		}
	}

	addAsset(benchmark)

	return result
}
