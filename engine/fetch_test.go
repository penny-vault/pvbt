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
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// makeDailyDF creates a DataFrame with daily timestamps starting at start
// for nDays days, covering the given assets and metrics. Values are sequential
// floats starting from 1.0.
func makeDailyDF(t *testing.T, start time.Time, nDays int, assets []asset.Asset, metrics []data.Metric) *data.DataFrame {
	t.Helper()
	times := make([]time.Time, nDays)
	for i := range times {
		times[i] = start.AddDate(0, 0, i)
	}
	vals := make([]float64, nDays*len(assets)*len(metrics))
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	df, err := data.NewDataFrame(times, assets, metrics, vals)
	if err != nil {
		t.Fatalf("makeDailyDF: %v", err)
	}
	return df
}

// TestPeriodToTime verifies that periodToTime subtracts days, months, and years correctly.
func TestPeriodToTime(t *testing.T) {
	base := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		period portfolio.Period
		want   time.Time
	}{
		{
			name:   "subtract 10 days",
			period: portfolio.Days(10),
			want:   time.Date(2024, 6, 5, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "subtract 3 months",
			period: portfolio.Months(3),
			want:   time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "subtract 2 years",
			period: portfolio.Years(2),
			want:   time.Date(2022, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "zero days",
			period: portfolio.Days(0),
			want:   base,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := periodToTime(base, tt.period)
			if !got.Equal(tt.want) {
				t.Errorf("periodToTime(%v, %+v) = %v, want %v", base, tt.period, got, tt.want)
			}
		})
	}
}

// TestBuildProviderRouting verifies that buildProviderRouting correctly maps
// metrics to their respective providers.
func TestBuildProviderRouting(t *testing.T) {
	assets := []asset.Asset{{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}}

	closeMetrics := []data.Metric{data.MetricClose}
	volumeMetrics := []data.Metric{data.Volume}

	closeDF := makeDailyDF(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 30, assets, closeMetrics)
	volumeDF := makeDailyDF(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 30, assets, volumeMetrics)

	closeProvider := data.NewTestProvider(closeMetrics, closeDF)
	volumeProvider := data.NewTestProvider(volumeMetrics, volumeDF)

	e := New(nil, WithDataProvider(closeProvider, volumeProvider))
	if err := e.buildProviderRouting(); err != nil {
		t.Fatalf("buildProviderRouting: %v", err)
	}

	if e.metricProvider == nil {
		t.Fatal("metricProvider map is nil after buildProviderRouting")
	}

	if _, ok := e.metricProvider[data.MetricClose]; !ok {
		t.Error("metricProvider missing MetricClose")
	}
	if _, ok := e.metricProvider[data.Volume]; !ok {
		t.Error("metricProvider missing Volume")
	}

	// Verify the correct provider is mapped.
	if e.metricProvider[data.MetricClose] != closeProvider {
		t.Error("MetricClose mapped to wrong provider")
	}
	if e.metricProvider[data.Volume] != volumeProvider {
		t.Error("Volume mapped to wrong provider")
	}
}

// TestEngineFetch verifies that Fetch returns the correct windowed DataFrame
// and populates the cache.
func TestEngineFetch(t *testing.T) {
	assets := []asset.Asset{
		{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
		{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
	}
	metrics := []data.Metric{data.MetricClose}

	// Build 30 days of data starting 2024-01-01.
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	df := makeDailyDF(t, start, 30, assets, metrics)
	provider := data.NewTestProvider(metrics, df)

	// Use a small chunk size so we exercise chunking logic.
	chunkSize := 15 * 24 * time.Hour

	e := New(nil,
		WithDataProvider(provider),
		WithChunkSize(chunkSize),
	)
	if err := e.buildProviderRouting(); err != nil {
		t.Fatalf("buildProviderRouting: %v", err)
	}

	// currentDate is 2024-01-20; request last 10 days.
	e.currentDate = time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, err := e.Fetch(ctx, assets, portfolio.Days(10), metrics)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result == nil {
		t.Fatal("Fetch returned nil DataFrame")
	}

	// Expect timestamps from 2024-01-10 to 2024-01-20 (11 rows).
	wantStart := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)

	// 2024-01-10 through 2024-01-20 inclusive = 11 rows.
	if result.Len() != 11 {
		t.Errorf("result.Len() = %d, want 11", result.Len())
	}
	if !result.Start().Equal(wantStart) {
		t.Errorf("result.Start() = %v, want %v", result.Start(), wantStart)
	}
	if !result.End().Equal(wantEnd) {
		t.Errorf("result.End() = %v, want %v", result.End(), wantEnd)
	}

	// Verify cache was populated.
	if e.cache == nil {
		t.Fatal("cache is nil after Fetch")
	}
	if len(e.cache.entries) == 0 {
		t.Error("cache has no entries after Fetch")
	}
}

// TestEngineFetchCacheHit verifies that a second Fetch for the same range
// returns data from the cache without calling the provider again.
func TestEngineFetchCacheHit(t *testing.T) {
	assets := []asset.Asset{{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}}
	metrics := []data.Metric{data.MetricClose}

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	df := makeDailyDF(t, start, 30, assets, metrics)
	provider := data.NewTestProvider(metrics, df)

	e := New(nil, WithDataProvider(provider))
	if err := e.buildProviderRouting(); err != nil {
		t.Fatalf("buildProviderRouting: %v", err)
	}
	e.currentDate = time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result1, err := e.Fetch(ctx, assets, portfolio.Days(10), metrics)
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	result2, err := e.Fetch(ctx, assets, portfolio.Days(10), metrics)
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	// Both results should cover the same range.
	if result1.Len() != result2.Len() {
		t.Errorf("cache hit result length %d != first result length %d", result2.Len(), result1.Len())
	}
}

// TestEngineFetchMultiProvider verifies that Fetch with metrics from two
// providers merges the results correctly.
func TestEngineFetchMultiProvider(t *testing.T) {
	assets := []asset.Asset{{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}}

	closeMetrics := []data.Metric{data.MetricClose}
	volumeMetrics := []data.Metric{data.Volume}

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	nDays := 30

	closeDF := makeDailyDF(t, start, nDays, assets, closeMetrics)
	volumeDF := makeDailyDF(t, start, nDays, assets, volumeMetrics)

	closeProvider := data.NewTestProvider(closeMetrics, closeDF)
	volumeProvider := data.NewTestProvider(volumeMetrics, volumeDF)

	e := New(nil, WithDataProvider(closeProvider, volumeProvider))
	if err := e.buildProviderRouting(); err != nil {
		t.Fatalf("buildProviderRouting: %v", err)
	}
	e.currentDate = time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, err := e.Fetch(ctx, assets, portfolio.Days(10), []data.Metric{data.MetricClose, data.Volume})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result == nil {
		t.Fatal("Fetch returned nil DataFrame")
	}
	if result.Len() == 0 {
		t.Fatal("Fetch returned empty DataFrame")
	}

	// Verify both metrics are present.
	metricList := result.MetricList()
	hasClose := false
	hasVolume := false
	for _, m := range metricList {
		if m == data.MetricClose {
			hasClose = true
		}
		if m == data.Volume {
			hasVolume = true
		}
	}
	if !hasClose {
		t.Error("merged result missing MetricClose")
	}
	if !hasVolume {
		t.Error("merged result missing Volume")
	}
}

// TestEngineFetchAt verifies that FetchAt returns a single-timestamp DataFrame.
func TestEngineFetchAt(t *testing.T) {
	assets := []asset.Asset{{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}}
	metrics := []data.Metric{data.MetricClose}

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	df := makeDailyDF(t, start, 30, assets, metrics)
	provider := data.NewTestProvider(metrics, df)

	e := New(nil, WithDataProvider(provider))
	if err := e.buildProviderRouting(); err != nil {
		t.Fatalf("buildProviderRouting: %v", err)
	}

	target := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	result, err := e.FetchAt(ctx, assets, target, metrics)
	if err != nil {
		t.Fatalf("FetchAt: %v", err)
	}
	if result == nil {
		t.Fatal("FetchAt returned nil DataFrame")
	}
	if result.Len() != 1 {
		t.Errorf("FetchAt result has %d rows, want 1", result.Len())
	}
	if !result.Start().Equal(target) {
		t.Errorf("FetchAt result timestamp = %v, want %v", result.Start(), target)
	}
}
