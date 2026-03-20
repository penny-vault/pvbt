package engine

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// Test-only exports for black-box testing of dataCache.

// DataCacheForTest is a type alias for dataCache, allowing the black-box
// test file to hold references without knowing the concrete type name.
type DataCacheForTest = dataCache

// ColCacheEntryForTest is a type alias for colCacheEntry.
type ColCacheEntryForTest = colCacheEntry

// NewDataCacheForTest exposes newDataCache.
var NewDataCacheForTest = newDataCache

// ChunkYearsForTest exposes chunkYears.
var ChunkYearsForTest = chunkYears

// NYCForTest returns the engine-internal nyc time.Location.
func NYCForTest() *time.Location {
	return nyc
}

// ChunkStartForTest returns the Unix seconds of Jan 1 00:00 Eastern for the
// year containing t (in Eastern time).
func ChunkStartForTest(t time.Time) int64 {
	et := t.In(nyc)
	jan1 := time.Date(et.Year(), 1, 1, 0, 0, 0, 0, nyc)
	return jan1.Unix()
}

// NewColCacheKeyForTest constructs a colCacheKey from exported parameters.
func NewColCacheKeyForTest(figi string, metric data.Metric, chunkStart int64) colCacheKey {
	return colCacheKey{figi: figi, metric: metric, chunkStart: chunkStart}
}

// NewColCacheEntryForTest constructs a colCacheEntry from exported parameters.
func NewColCacheEntryForTest(times []time.Time, values []float64) *colCacheEntry {
	return &colCacheEntry{times: times, values: values}
}

// EntryValuesForTest returns the values slice from a colCacheEntry.
func EntryValuesForTest(entry *colCacheEntry) []float64 {
	return entry.values
}

// CurBytesForTest returns the current byte count tracked by the cache.
func CurBytesForTest(cache *dataCache) int64 {
	return cache.curBytes
}

// GetForTest exposes dataCache.get.
func GetForTest(cache *dataCache, key colCacheKey) (*colCacheEntry, bool) {
	return cache.get(key)
}

// PutForTest exposes dataCache.put.
func PutForTest(cache *dataCache, key colCacheKey, entry *colCacheEntry) {
	cache.put(key, entry)
}

// EvictBeforeForTest exposes dataCache.evictBefore.
func EvictBeforeForTest(cache *dataCache, t time.Time) {
	cache.evictBefore(t)
}

// WalkBackTradingDaysForTest exposes walkBackTradingDays.
var WalkBackTradingDaysForTest = walkBackTradingDays

// CollectStrategyAssetsForTest exposes collectStrategyAssets.
var CollectStrategyAssetsForTest func(strategy any, benchmark asset.Asset) []asset.Asset = collectStrategyAssets
