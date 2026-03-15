package engine_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/engine"
)

var _ = Describe("dataCache", func() {
	var nyc *time.Location

	BeforeEach(func() {
		nyc = engine.NYCForTest()
	})

	Describe("ChunkStartForTest", func() {
		It("maps a mid-year date to Jan 1 of that year in Eastern time", func() {
			date := time.Date(2025, 6, 15, 16, 0, 0, 0, nyc)
			got := engine.ChunkStartForTest(date)
			want := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
			Expect(got).To(Equal(want))
		})

		It("converts UTC to Eastern before computing the chunk year", func() {
			// 03:00 UTC on Jan 1 = 22:00 Dec 31 ET, so chunk should be 2025.
			date := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
			got := engine.ChunkStartForTest(date)
			want := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
			Expect(got).To(Equal(want))
		})
	})

	Describe("chunkYears", func() {
		It("returns chunks for each year spanned", func() {
			start := time.Date(2025, 11, 1, 16, 0, 0, 0, nyc)
			end := time.Date(2026, 2, 1, 16, 0, 0, 0, nyc)
			got := engine.ChunkYearsForTest(start, end)
			Expect(got).To(HaveLen(2))
			Expect(got[0]).To(Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()))
			Expect(got[1]).To(Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, nyc).Unix()))
		})

		It("returns a single chunk when start and end are in the same year", func() {
			start := time.Date(2025, 3, 1, 16, 0, 0, 0, nyc)
			end := time.Date(2025, 9, 1, 16, 0, 0, 0, nyc)
			got := engine.ChunkYearsForTest(start, end)
			Expect(got).To(HaveLen(1))
		})
	})

	Describe("get and put", func() {
		It("returns a miss for an unknown key", func() {
			cache := engine.NewDataCacheForTest(0)
			key := engine.NewColCacheKeyForTest("FIGI-A", "close", engine.ChunkStartForTest(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc)))
			_, ok := engine.GetForTest(cache, key)
			Expect(ok).To(BeFalse())
		})

		It("returns a hit after putting an entry", func() {
			cache := engine.NewDataCacheForTest(0)
			key := engine.NewColCacheKeyForTest("FIGI-A", "close", engine.ChunkStartForTest(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc)))
			entry := engine.NewColCacheEntryForTest(
				[]time.Time{time.Date(2025, 6, 1, 16, 0, 0, 0, nyc)},
				[]float64{100.0},
			)
			engine.PutForTest(cache, key, entry)
			got, ok := engine.GetForTest(cache, key)
			Expect(ok).To(BeTrue())
			Expect(engine.EntryValuesForTest(got)[0]).To(Equal(100.0))
		})
	})

	Describe("evictBefore", func() {
		It("removes old entries but keeps the previous and current year", func() {
			cache := engine.NewDataCacheForTest(0)
			key2023 := engine.NewColCacheKeyForTest("FIGI-A", "close", engine.ChunkStartForTest(time.Date(2023, 6, 1, 0, 0, 0, 0, nyc)))
			key2024 := engine.NewColCacheKeyForTest("FIGI-A", "close", engine.ChunkStartForTest(time.Date(2024, 6, 1, 0, 0, 0, 0, nyc)))
			key2025 := engine.NewColCacheKeyForTest("FIGI-A", "close", engine.ChunkStartForTest(time.Date(2025, 6, 1, 0, 0, 0, 0, nyc)))
			entry := engine.NewColCacheEntryForTest([]time.Time{}, []float64{})
			engine.PutForTest(cache, key2023, entry)
			engine.PutForTest(cache, key2024, entry)
			engine.PutForTest(cache, key2025, entry)

			engine.EvictBeforeForTest(cache, time.Date(2025, 3, 1, 0, 0, 0, 0, nyc))

			_, ok2023 := engine.GetForTest(cache, key2023)
			_, ok2024 := engine.GetForTest(cache, key2024)
			_, ok2025 := engine.GetForTest(cache, key2025)
			Expect(ok2023).To(BeFalse(), "expected 2023 entry to be evicted")
			Expect(ok2024).To(BeTrue(), "expected 2024 entry to remain (previous year)")
			Expect(ok2025).To(BeTrue(), "expected 2025 entry to remain")
		})
	})

	Describe("heterogeneous time axes", func() {
		It("caches entries with different time axes independently", func() {
			cache := engine.NewDataCacheForTest(0)
			yr := engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc))

			dailyTimes := []time.Time{
				time.Date(2025, 1, 2, 16, 0, 0, 0, nyc),
				time.Date(2025, 1, 3, 16, 0, 0, 0, nyc),
				time.Date(2025, 1, 6, 16, 0, 0, 0, nyc),
			}
			engine.PutForTest(cache, engine.NewColCacheKeyForTest("FIGI-A", "close", yr),
				engine.NewColCacheEntryForTest(dailyTimes, []float64{100, 101, 102}))

			quarterlyTimes := []time.Time{
				time.Date(2025, 1, 3, 16, 0, 0, 0, nyc),
			}
			engine.PutForTest(cache, engine.NewColCacheKeyForTest("FIGI-A", "revenue", yr),
				engine.NewColCacheEntryForTest(quarterlyTimes, []float64{5000}))

			_, ok1 := engine.GetForTest(cache, engine.NewColCacheKeyForTest("FIGI-A", "close", yr))
			_, ok2 := engine.GetForTest(cache, engine.NewColCacheKeyForTest("FIGI-A", "revenue", yr))
			Expect(ok1).To(BeTrue())
			Expect(ok2).To(BeTrue())
		})
	})

	Describe("bytes tracking", func() {
		It("tracks curBytes on put", func() {
			cache := engine.NewDataCacheForTest(0)
			key := engine.NewColCacheKeyForTest("FIGI-A", "close", engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc)))
			entry := engine.NewColCacheEntryForTest(
				make([]time.Time, 10),
				make([]float64, 10),
			)
			engine.PutForTest(cache, key, entry)
			expected := int64(10*8 + 10*24) // 320
			Expect(engine.CurBytesForTest(cache)).To(Equal(expected))
		})

		It("updates curBytes correctly on overwrite", func() {
			cache := engine.NewDataCacheForTest(0)
			key := engine.NewColCacheKeyForTest("FIGI-A", "close", engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc)))
			entry := engine.NewColCacheEntryForTest(
				make([]time.Time, 10),
				make([]float64, 10),
			)
			engine.PutForTest(cache, key, entry)

			entry2 := engine.NewColCacheEntryForTest(
				make([]time.Time, 5),
				make([]float64, 5),
			)
			engine.PutForTest(cache, key, entry2)
			expected := int64(5*8 + 5*24) // 160
			Expect(engine.CurBytesForTest(cache)).To(Equal(expected))
		})
	})
})
