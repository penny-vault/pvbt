// Copyright 2021-2022
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

package data

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/alphadose/haxmap"
	"github.com/rs/zerolog/log"
)

type CacheItem struct {
	Values []float64
	Period *Interval
}

type SecurityMetricCache struct {
	sizeBytes    int64
	maxSizeBytes int64
	values       map[string][]*CacheItem
	lastSeen     *haxmap.HashMap[string, time.Time]
	periods      []time.Time
	locker       sync.RWMutex
}

type pair struct {
	key  string
	last time.Time
}

// ByAge implements sort.Interface for []Person based on
// the Age field.
type ByDate []pair

func (a ByDate) Len() int           { return len(a) }
func (a ByDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDate) Less(i, j int) bool { return a[i].last.Before(a[j].last) }

// Functions

func NewSecurityMetricCache(sz int64, periods []time.Time) *SecurityMetricCache {
	return &SecurityMetricCache{
		sizeBytes:    0,
		maxSizeBytes: sz,
		values:       make(map[string][]*CacheItem, 10),
		lastSeen:     haxmap.New[string, time.Time](),
		periods:      periods,
		locker:       sync.RWMutex{},
	}
}

// Check returns if the requested range is in the cache. If the range is not completely covered by the cache
// returns false and a list of intervals covered by the cache that partially match the requested range.
func (cache *SecurityMetricCache) Check(security *Security, metric Metric, begin, end time.Time) (bool, []*Interval) {
	return false, nil
}

// Get returns the requested data over the range. If the data does not return ErrDoesNotExist
func (cache *SecurityMetricCache) Get(security *Security, metric Metric, begin, end time.Time) ([]float64, error) {
	return nil, errors.New("not implemented")
}

// Set adds the data for the specified security and metric to the cache
func (cache *SecurityMetricCache) Set(security *Security, metric Metric, begin, end time.Time, val []float64) error {
	cache.locker.Lock()
	defer cache.locker.Unlock()

	toAddBytes := int64(len(val) * 8)

	if cache.maxSizeBytes < toAddBytes {
		return ErrDataLargerThanCache
	}

	newTotalSize := toAddBytes + cache.sizeBytes
	if newTotalSize > cache.maxSizeBytes {
		cache.deleteLRU(toAddBytes)
	}

	k := key(security, metric)

	// create an interval and check that it's valid
	interval := &Interval{
		Begin: begin,
		End:   end,
	}

	if err := interval.Valid(); err != nil {
		log.Error().Err(err).Msg("cannot set cache value with invalid interval")
		return ErrInvalidTimeRange
	}

	// check if this key already exists
	var items []*CacheItem
	var ok bool

	if items, ok = cache.values[k]; !ok {
		items = []*CacheItem{}
	}

	items, sizeAdded := cache.merge(&CacheItem{
		Values: val,
		Period: interval,
	}, items)

	cache.values[k] = items
	cache.lastSeen.Set(k, time.Now())
	cache.sizeBytes += int64(sizeAdded * 8)

	return nil
}

// Count returns the number of securities + metrics in the cache
func (cache *SecurityMetricCache) Count() int {
	return len(cache.values)
}

func (cache *SecurityMetricCache) Size() int64 {
	return cache.sizeBytes
}

// Private Implementation

func key(security *Security, metric Metric) string {
	return fmt.Sprintf("%s:%s", security.CompositeFigi, metric)
}

func (cache *SecurityMetricCache) deleteLRU(bytesToDelete int64) {
	lastAccess := make([]pair, 0, cache.lastSeen.Len())
	cache.lastSeen.ForEach(func(s string, t time.Time) {
		lastAccess = append(lastAccess, pair{
			key:  s,
			last: t,
		})
	})

	sort.Sort(ByDate(lastAccess))

	cleared := int64(0)
	for _, keyPair := range lastAccess {
		entry := cache.values[keyPair.key]
		delete(cache.values, keyPair.key)

		for _, item := range entry {
			cleared += int64(len(item.Values) * 8)
		}

		if cleared > bytesToDelete {
			cache.sizeBytes -= cleared
			break
		}
	}
}

// merge takes a list of intervals and adds a new interval to the list. If the new interval
// overlaps with an existing interval they are merged, otherwise it is inserted in the
// interval.Begin time sorted location in the list
func (cache *SecurityMetricCache) merge(new *CacheItem, items []*CacheItem) ([]*CacheItem, int) {
	if len(items) == 0 {
		return []*CacheItem{new}, len(new.Values)
	}

	insertIdx := -1
	for idx, item := range items {
		if item.Period.Contains(new.Period) {
			// nothing to be done data already in cache
			return items, 0
		}

		if item.Period.Contiguous(new.Period) {
			// need to merge
			// new interval
			mergedInterval := &Interval{
				Begin: minTime(new.Period.Begin, item.Period.Begin),
				End:   maxTime(new.Period.End, item.Period.Begin),
			}

			added := 0
			// new values before current values
			mergedValues := make([]float64, 0, len(item.Values))
			if new.Period.Begin.Before(item.Period.Begin) {
				startIdx := sort.Search(len(cache.periods), func(i int) bool {
					idxVal := cache.periods[i]
					return (idxVal.After(new.Period.Begin) || idxVal.Equal(new.Period.Begin))
				})

				searchVal := item.Period.Begin.AddDate(0, 0, -1)
				endIdx := sort.Search(len(cache.periods), func(i int) bool {
					idxVal := cache.periods[i]
					return (idxVal.After(searchVal) || idxVal.Equal(searchVal))
				})

				numDates := endIdx - startIdx
				added += numDates
				mergedValues = new.Values[:numDates]
			}

			mergedValues = append(mergedValues, item.Values...)

			// new values after item.Values
			if new.Period.End.After(item.Period.End) {
				periodStartIdx := sort.Search(len(cache.periods), func(i int) bool {
					idxVal := cache.periods[i]
					return (idxVal.After(new.Period.Begin) || idxVal.Equal(new.Period.Begin))
				})

				searchVal := item.Period.End.AddDate(0, 0, 1)
				sliceStartIdx := sort.Search(len(cache.periods), func(i int) bool {
					idxVal := cache.periods[i]
					return (idxVal.After(searchVal) || idxVal.Equal(searchVal))
				})

				startIdx := sliceStartIdx - periodStartIdx
				added += len(new.Values[startIdx:])
				mergedValues = append(mergedValues, new.Values[startIdx:]...)
			}

			item.Period = mergedInterval
			item.Values = mergedValues

			return items, added
		}

		if item.Period.Begin.After(new.Period.Begin) {
			// insert at the index
			insertIdx = idx
		}
	}

	if insertIdx != -1 {
		items = insert(items, insertIdx, new)
	}

	return items, len(new.Values)
}

func insert(orig []*CacheItem, index int, value *CacheItem) []*CacheItem {
	if index < 0 {
		return orig
	}

	if index >= len(orig) {
		return append(orig, value)
	}

	orig = append(orig[:index+1], orig[index:]...)
	orig[index] = value

	return orig
}

func minTime(a, b time.Time) time.Time {
	if a.After(b) {
		return b
	}

	return a
}

func maxTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}
