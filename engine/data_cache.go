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
	"time"

	"github.com/penny-vault/pvbt/data"
)

const defaultMaxBytes int64 = 512 * 1024 * 1024 // 512 MB

var nyc *time.Location

func init() {
	var err error
	nyc, err = time.LoadLocation("America/New_York")
	if err != nil {
		panic("engine: load America/New_York: " + err.Error())
	}
}

// colCacheKey identifies a single cached column: one asset, one metric,
// one calendar-year chunk.
type colCacheKey struct {
	figi       string
	metric     data.Metric
	chunkStart int64 // Unix seconds, Jan 1 00:00 Eastern
}

// colCacheEntry holds a single time-series column.
type colCacheEntry struct {
	times  []time.Time
	values []float64
}

type dataCache struct {
	entries  map[colCacheKey]*colCacheEntry
	curBytes int64
	maxBytes int64
}

func newDataCache(maxBytes int64) *dataCache {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	return &dataCache{
		entries:  make(map[colCacheKey]*colCacheEntry),
		maxBytes: maxBytes,
	}
}

func (c *dataCache) get(key colCacheKey) (*colCacheEntry, bool) {
	e, ok := c.entries[key]
	return e, ok
}

func (c *dataCache) put(key colCacheKey, entry *colCacheEntry) {
	sz := estimateEntryBytes(entry)
	if old, ok := c.entries[key]; ok {
		c.curBytes -= estimateEntryBytes(old)
	}
	c.entries[key] = entry
	c.curBytes += sz
}

// evictBefore removes all entries whose chunk year is more than one year
// before t. We keep the previous year because lookback windows commonly
// span across year boundaries.
func (c *dataCache) evictBefore(t time.Time) {
	year := t.In(nyc).Year()
	for key, entry := range c.entries {
		chunkYear := time.Unix(key.chunkStart, 0).In(nyc).Year()
		if chunkYear < year-1 {
			c.curBytes -= estimateEntryBytes(entry)
			delete(c.entries, key)
		}
	}
}

// chunkStartFor returns the Unix seconds of Jan 1 00:00 Eastern for the
// year containing t (in Eastern time).
func chunkStartFor(t time.Time) int64 {
	et := t.In(nyc)
	jan1 := time.Date(et.Year(), 1, 1, 0, 0, 0, 0, nyc)
	return jan1.Unix()
}

// chunkYears returns the chunkStart values (Unix seconds) for every
// calendar year that overlaps [start, end].
func chunkYears(start, end time.Time) []int64 {
	startYear := start.In(nyc).Year()
	endYear := end.In(nyc).Year()
	years := make([]int64, 0, endYear-startYear+1)
	for y := startYear; y <= endYear; y++ {
		jan1 := time.Date(y, 1, 1, 0, 0, 0, 0, nyc)
		years = append(years, jan1.Unix())
	}
	return years
}

func estimateEntryBytes(e *colCacheEntry) int64 {
	if e == nil {
		return 0
	}
	return int64(len(e.values)*8 + len(e.times)*24)
}
