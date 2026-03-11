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
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

func makeTestDF(t *testing.T, nTimes int, assets []asset.Asset, metrics []data.Metric) *data.DataFrame {
	t.Helper()
	times := make([]time.Time, nTimes)
	for i := range times {
		times[i] = time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC)
	}
	vals := make([]float64, nTimes*len(assets)*len(metrics))
	for i := range vals {
		vals[i] = float64(i)
	}
	df, err := data.NewDataFrame(times, assets, metrics, vals)
	if err != nil {
		t.Fatalf("NewDataFrame: %v", err)
	}
	return df
}

func TestCachePutGet(t *testing.T) {
	c := newDataCache(0, 0) // use defaults

	assets := []asset.Asset{{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}}
	metrics := []data.Metric{data.MetricClose}
	df := makeTestDF(t, 5, assets, metrics)

	key := dataCacheKey{
		assetsHash:  hashAssets(assets),
		metricsHash: hashMetrics(metrics),
		chunkStart:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	// Should not exist yet.
	if _, ok := c.get(key); ok {
		t.Fatal("expected cache miss before put")
	}

	c.put(key, df)

	got, ok := c.get(key)
	if !ok {
		t.Fatal("expected cache hit after put")
	}
	if got != df {
		t.Fatal("expected identical DataFrame pointer")
	}

	if c.curBytes != estimateBytes(df) {
		t.Fatalf("curBytes = %d, want %d", c.curBytes, estimateBytes(df))
	}
}

func TestCacheEviction(t *testing.T) {
	chunkSize := 365 * 24 * time.Hour
	c := newDataCache(0, chunkSize)

	assets := []asset.Asset{{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}}
	metrics := []data.Metric{data.MetricClose}
	df1 := makeTestDF(t, 3, assets, metrics)
	df2 := makeTestDF(t, 3, assets, metrics)

	key1 := dataCacheKey{
		assetsHash:  hashAssets(assets),
		metricsHash: hashMetrics(metrics),
		chunkStart:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	key2 := dataCacheKey{
		assetsHash:  hashAssets(assets),
		metricsHash: hashMetrics(metrics),
		chunkStart:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	c.put(key1, df1)
	c.put(key2, df2)

	if len(c.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(c.entries))
	}

	// Evict chunks whose end is before 2025-07-01.
	// key1 chunk ends at 2024-01-01 + 365d = 2024-12-31 -> before 2025-07-01, evicted.
	// key2 chunk ends at 2025-01-01 + 365d = 2025-12-31 -> NOT before 2025-07-01, kept.
	c.evictBefore(time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC))

	if _, ok := c.get(key1); ok {
		t.Fatal("expected key1 to be evicted")
	}
	if _, ok := c.get(key2); !ok {
		t.Fatal("expected key2 to remain")
	}

	expectedBytes := estimateBytes(df2)
	if c.curBytes != expectedBytes {
		t.Fatalf("curBytes = %d, want %d", c.curBytes, expectedBytes)
	}
}

func TestCacheSizeEstimation(t *testing.T) {
	assets := []asset.Asset{
		{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
		{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
	}
	metrics := []data.Metric{data.MetricClose, data.MetricOpen, data.MetricHigh}
	df := makeTestDF(t, 10, assets, metrics)

	got := estimateBytes(df)
	want := int64(10 * 2 * 3 * 8) // 480
	if got != want {
		t.Fatalf("estimateBytes = %d, want %d", got, want)
	}
}

func TestCacheChunkBoundaries(t *testing.T) {
	chunkSize := 365 * 24 * time.Hour
	c := newDataCache(0, chunkSize)

	start := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	boundaries := c.chunkBoundaries(start, end)

	// Epoch is 2000-01-01. With 365-day chunks:
	// Chunk 24 starts at 2000-01-01 + 24*365d = day 8760
	// We need the chunk at or before 2024-03-15 and all chunks up to 2025-06-01.
	if len(boundaries) < 2 {
		t.Fatalf("expected at least 2 boundaries, got %d", len(boundaries))
	}

	// First boundary must be at or before start.
	if boundaries[0].After(start) {
		t.Fatalf("first boundary %v is after start %v", boundaries[0], start)
	}

	// Last boundary must be at or before end.
	if boundaries[len(boundaries)-1].After(end) {
		t.Fatalf("last boundary %v is after end %v", boundaries[len(boundaries)-1], end)
	}

	// Last boundary + chunkSize must be after end (otherwise we'd need another chunk).
	lastEnd := boundaries[len(boundaries)-1].Add(chunkSize)
	if !lastEnd.After(end) {
		t.Fatalf("last chunk end %v does not cover end %v", lastEnd, end)
	}

	// Consecutive boundaries should differ by chunkSize.
	for i := 1; i < len(boundaries); i++ {
		diff := boundaries[i].Sub(boundaries[i-1])
		if diff != chunkSize {
			t.Fatalf("boundary gap = %v, want %v", diff, chunkSize)
		}
	}
}

func TestHashAssets(t *testing.T) {
	a := []asset.Asset{
		{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
		{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
		{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"},
	}
	b := []asset.Asset{
		{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"},
		{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
		{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
	}

	h1 := hashAssets(a)
	h2 := hashAssets(b)
	if h1 != h2 {
		t.Fatalf("hashAssets differs for same assets in different order: %d != %d", h1, h2)
	}

	// Different assets should (very likely) produce a different hash.
	c := []asset.Asset{
		{CompositeFigi: "FIGI-TSLA", Ticker: "TSLA"},
	}
	h3 := hashAssets(c)
	if h3 == h1 {
		t.Fatal("hashAssets unexpectedly equal for different asset sets")
	}
}
