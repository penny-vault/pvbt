package engine

import (
	"testing"
	"time"
)

func TestChunkStartFor(t *testing.T) {
	// Mid-year date should map to Jan 1 of that year in Eastern time.
	date := time.Date(2025, 6, 15, 16, 0, 0, 0, nyc)
	got := chunkStartFor(date)
	want := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
	if got != want {
		t.Errorf("chunkStartFor(%v) = %d, want %d", date, got, want)
	}
}

func TestChunkStartForUTC(t *testing.T) {
	// A UTC time on Jan 1 at 03:00 UTC is still Dec 31 in Eastern.
	date := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	got := chunkStartFor(date)
	// 03:00 UTC = 22:00 Dec 31 ET, so chunk should be 2025.
	want := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
	if got != want {
		t.Errorf("chunkStartFor(%v) = %d, want %d", date, got, want)
	}
}

func TestChunkYears(t *testing.T) {
	start := time.Date(2025, 11, 1, 16, 0, 0, 0, nyc)
	end := time.Date(2026, 2, 1, 16, 0, 0, 0, nyc)
	got := chunkYears(start, end)
	if len(got) != 2 {
		t.Fatalf("chunkYears: got %d chunks, want 2", len(got))
	}
	want2025 := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
	want2026 := time.Date(2026, 1, 1, 0, 0, 0, 0, nyc).Unix()
	if got[0] != want2025 {
		t.Errorf("chunkYears[0] = %d, want %d", got[0], want2025)
	}
	if got[1] != want2026 {
		t.Errorf("chunkYears[1] = %d, want %d", got[1], want2026)
	}
}

func TestChunkYearsSingleYear(t *testing.T) {
	start := time.Date(2025, 3, 1, 16, 0, 0, 0, nyc)
	end := time.Date(2025, 9, 1, 16, 0, 0, 0, nyc)
	got := chunkYears(start, end)
	if len(got) != 1 {
		t.Fatalf("chunkYears: got %d chunks, want 1", len(got))
	}
}

func TestDataCacheGetMiss(t *testing.T) {
	c := newDataCache(0)
	key := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc))}
	_, ok := c.get(key)
	if ok {
		t.Error("expected cache miss")
	}
}

func TestDataCachePutGet(t *testing.T) {
	c := newDataCache(0)
	key := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc))}
	entry := &colCacheEntry{
		times:  []time.Time{time.Date(2025, 6, 1, 16, 0, 0, 0, nyc)},
		values: []float64{100.0},
	}
	c.put(key, entry)
	got, ok := c.get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.values[0] != 100.0 {
		t.Errorf("value = %f, want 100.0", got.values[0])
	}
}

func TestDataCacheEvictBefore(t *testing.T) {
	c := newDataCache(0)
	key2024 := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2024, 6, 1, 0, 0, 0, 0, nyc))}
	key2025 := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc))}
	entry := &colCacheEntry{times: []time.Time{}, values: []float64{}}
	c.put(key2024, entry)
	c.put(key2025, entry)

	// Evict before a 2025 date: should remove 2024, keep 2025.
	c.evictBefore(time.Date(2025, 3, 1, 0, 0, 0, 0, nyc))
	_, ok2024 := c.get(key2024)
	_, ok2025 := c.get(key2025)
	if ok2024 {
		t.Error("expected 2024 entry to be evicted")
	}
	if !ok2025 {
		t.Error("expected 2025 entry to remain")
	}
}

func TestDataCacheBytesTracking(t *testing.T) {
	c := newDataCache(0)
	key := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc))}
	entry := &colCacheEntry{
		times:  make([]time.Time, 10),
		values: make([]float64, 10),
	}
	c.put(key, entry)
	expected := int64(10*8 + 10*24) // 320
	if c.curBytes != expected {
		t.Errorf("curBytes = %d, want %d", c.curBytes, expected)
	}

	// Overwrite should update bytes correctly.
	entry2 := &colCacheEntry{
		times:  make([]time.Time, 5),
		values: make([]float64, 5),
	}
	c.put(key, entry2)
	expected2 := int64(5*8 + 5*24) // 160
	if c.curBytes != expected2 {
		t.Errorf("curBytes after overwrite = %d, want %d", c.curBytes, expected2)
	}
}
