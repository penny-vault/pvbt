package engine

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("dataCache", func() {
	Describe("chunkStartFor", func() {
		It("maps a mid-year date to Jan 1 of that year in Eastern time", func() {
			date := time.Date(2025, 6, 15, 16, 0, 0, 0, nyc)
			got := chunkStartFor(date)
			want := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
			Expect(got).To(Equal(want))
		})

		It("converts UTC to Eastern before computing the chunk year", func() {
			// 03:00 UTC on Jan 1 = 22:00 Dec 31 ET, so chunk should be 2025.
			date := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
			got := chunkStartFor(date)
			want := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
			Expect(got).To(Equal(want))
		})
	})

	Describe("chunkYears", func() {
		It("returns chunks for each year spanned", func() {
			start := time.Date(2025, 11, 1, 16, 0, 0, 0, nyc)
			end := time.Date(2026, 2, 1, 16, 0, 0, 0, nyc)
			got := chunkYears(start, end)
			Expect(got).To(HaveLen(2))
			Expect(got[0]).To(Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()))
			Expect(got[1]).To(Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, nyc).Unix()))
		})

		It("returns a single chunk when start and end are in the same year", func() {
			start := time.Date(2025, 3, 1, 16, 0, 0, 0, nyc)
			end := time.Date(2025, 9, 1, 16, 0, 0, 0, nyc)
			got := chunkYears(start, end)
			Expect(got).To(HaveLen(1))
		})
	})

	Describe("get and put", func() {
		It("returns a miss for an unknown key", func() {
			cache := newDataCache(0)
			key := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc))}
			_, ok := cache.get(key)
			Expect(ok).To(BeFalse())
		})

		It("returns a hit after putting an entry", func() {
			cache := newDataCache(0)
			key := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc))}
			entry := &colCacheEntry{
				times:  []time.Time{time.Date(2025, 6, 1, 16, 0, 0, 0, nyc)},
				values: []float64{100.0},
			}
			cache.put(key, entry)
			got, ok := cache.get(key)
			Expect(ok).To(BeTrue())
			Expect(got.values[0]).To(Equal(100.0))
		})
	})

	Describe("evictBefore", func() {
		It("removes old entries but keeps the previous and current year", func() {
			cache := newDataCache(0)
			key2023 := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2023, 6, 1, 0, 0, 0, 0, nyc))}
			key2024 := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2024, 6, 1, 0, 0, 0, 0, nyc))}
			key2025 := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc))}
			entry := &colCacheEntry{times: []time.Time{}, values: []float64{}}
			cache.put(key2023, entry)
			cache.put(key2024, entry)
			cache.put(key2025, entry)

			cache.evictBefore(time.Date(2025, 3, 1, 0, 0, 0, 0, nyc))

			_, ok2023 := cache.get(key2023)
			_, ok2024 := cache.get(key2024)
			_, ok2025 := cache.get(key2025)
			Expect(ok2023).To(BeFalse(), "expected 2023 entry to be evicted")
			Expect(ok2024).To(BeTrue(), "expected 2024 entry to remain (previous year)")
			Expect(ok2025).To(BeTrue(), "expected 2025 entry to remain")
		})
	})

	Describe("heterogeneous time axes", func() {
		It("caches entries with different time axes independently", func() {
			cache := newDataCache(0)
			yr := chunkStartFor(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc))

			dailyTimes := []time.Time{
				time.Date(2025, 1, 2, 16, 0, 0, 0, nyc),
				time.Date(2025, 1, 3, 16, 0, 0, 0, nyc),
				time.Date(2025, 1, 6, 16, 0, 0, 0, nyc),
			}
			cache.put(colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: yr},
				&colCacheEntry{times: dailyTimes, values: []float64{100, 101, 102}})

			quarterlyTimes := []time.Time{
				time.Date(2025, 1, 3, 16, 0, 0, 0, nyc),
			}
			cache.put(colCacheKey{figi: "FIGI-A", metric: "revenue", chunkStart: yr},
				&colCacheEntry{times: quarterlyTimes, values: []float64{5000}})

			_, ok1 := cache.get(colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: yr})
			_, ok2 := cache.get(colCacheKey{figi: "FIGI-A", metric: "revenue", chunkStart: yr})
			Expect(ok1).To(BeTrue())
			Expect(ok2).To(BeTrue())
		})
	})

	Describe("bytes tracking", func() {
		It("tracks curBytes on put", func() {
			cache := newDataCache(0)
			key := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc))}
			entry := &colCacheEntry{
				times:  make([]time.Time, 10),
				values: make([]float64, 10),
			}
			cache.put(key, entry)
			expected := int64(10*8 + 10*24) // 320
			Expect(cache.curBytes).To(Equal(expected))
		})

		It("updates curBytes correctly on overwrite", func() {
			cache := newDataCache(0)
			key := colCacheKey{figi: "FIGI-A", metric: "close", chunkStart: chunkStartFor(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc))}
			entry := &colCacheEntry{
				times:  make([]time.Time, 10),
				values: make([]float64, 10),
			}
			cache.put(key, entry)

			entry2 := &colCacheEntry{
				times:  make([]time.Time, 5),
				values: make([]float64, 5),
			}
			cache.put(key, entry2)
			expected := int64(5*8 + 5*24) // 160
			Expect(cache.curBytes).To(Equal(expected))
		})
	})
})
