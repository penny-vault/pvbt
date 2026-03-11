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
	"hash/fnv"
	"sort"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

const (
	defaultMaxBytes  int64         = 512 * 1024 * 1024    // 512 MB
	defaultChunkSize time.Duration = 365 * 24 * time.Hour // ~1 year
)

type dataCacheKey struct {
	assetsHash  uint64
	metricsHash uint64
	chunkStart  time.Time
}

type dataCache struct {
	entries   map[dataCacheKey]*data.DataFrame
	curBytes  int64
	maxBytes  int64
	chunkSize time.Duration
}

func newDataCache(maxBytes int64, chunkSize time.Duration) *dataCache {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	return &dataCache{
		entries:   make(map[dataCacheKey]*data.DataFrame),
		maxBytes:  maxBytes,
		chunkSize: chunkSize,
	}
}

func (c *dataCache) get(key dataCacheKey) (*data.DataFrame, bool) {
	df, ok := c.entries[key]
	return df, ok
}

func (c *dataCache) put(key dataCacheKey, df *data.DataFrame) {
	sz := estimateBytes(df)
	// If already present, subtract old size first.
	if old, ok := c.entries[key]; ok {
		c.curBytes -= estimateBytes(old)
	}
	c.entries[key] = df
	c.curBytes += sz
}

// evictBefore removes all entries whose chunk ends before t.
func (c *dataCache) evictBefore(t time.Time) {
	for key, df := range c.entries {
		chunkEnd := key.chunkStart.Add(c.chunkSize)
		if chunkEnd.Before(t) {
			c.curBytes -= estimateBytes(df)
			delete(c.entries, key)
		}
	}
}

// chunkBoundaries returns the start times of all chunks that cover [start, end].
// Chunks are aligned to the chunkSize from the epoch (2000-01-01).
func (c *dataCache) chunkBoundaries(start, end time.Time) []time.Time {
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	// Find the chunk boundary at or before start.
	elapsed := start.Sub(epoch)
	chunkIdx := elapsed / c.chunkSize
	if elapsed < 0 {
		chunkIdx--
	}
	chunkStart := epoch.Add(chunkIdx * c.chunkSize)

	var boundaries []time.Time
	for !chunkStart.After(end) {
		boundaries = append(boundaries, chunkStart)
		chunkStart = chunkStart.Add(c.chunkSize)
	}
	return boundaries
}

func estimateBytes(df *data.DataFrame) int64 {
	if df == nil {
		return 0
	}
	return int64(df.Len() * len(df.AssetList()) * len(df.MetricList()) * 8)
}

func hashAssets(assets []asset.Asset) uint64 {
	sorted := make([]string, len(assets))
	for i, a := range assets {
		sorted[i] = a.CompositeFigi
	}
	sort.Strings(sorted)

	h := fnv.New64a()
	for _, s := range sorted {
		h.Write([]byte(s))
		h.Write([]byte{0}) // separator
	}
	return h.Sum64()
}

func hashMetrics(metrics []data.Metric) uint64 {
	sorted := make([]string, len(metrics))
	for i, m := range metrics {
		sorted[i] = string(m)
	}
	sort.Strings(sorted)

	h := fnv.New64a()
	for _, s := range sorted {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return h.Sum64()
}
