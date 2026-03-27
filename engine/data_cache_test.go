package engine_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
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

	Describe("chunkEntry", func() {
		var (
			aapl   asset.Asset
			msft   asset.Asset
			yr2025 int64
		)

		BeforeEach(func() {
			aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
			msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
			yr2025 = engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc))
		})

		It("initializes all values to NaN", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
				time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			Expect(math.IsNaN(val)).To(BeTrue())
		})

		It("stores and retrieves a value by day offset", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			day := engine.DayOffsetForTest(ce, time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			engine.ChunkEntrySetForTest(ce, day, 0, 0, 150.0)
			val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
				time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			Expect(val).To(Equal(150.0))
		})

		It("returns NaN for an unknown asset", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			val := engine.ChunkEntryGetForTest(ce, "FIGI-UNKNOWN", data.MetricClose,
				time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			Expect(math.IsNaN(val)).To(BeTrue())
		})

		It("returns NaN for an unknown metric", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.Volume,
				time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			Expect(math.IsNaN(val)).To(BeTrue())
		})

		It("detects present and missing columns", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-AAPL", data.MetricClose)).To(BeTrue())
			Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-AAPL", data.Volume)).To(BeFalse())
			Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-MSFT", data.MetricClose)).To(BeFalse())
		})

		Context("expand", func() {
			It("adds new assets and preserves existing values", func() {
				ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
				day := engine.DayOffsetForTest(ce, time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				engine.ChunkEntrySetForTest(ce, day, 0, 0, 150.0)

				engine.ChunkEntryExpandForTest(ce, []asset.Asset{msft}, nil)

				val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
					time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				Expect(val).To(Equal(150.0))

				Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-MSFT", data.MetricClose)).To(BeTrue())
				val2 := engine.ChunkEntryGetForTest(ce, "FIGI-MSFT", data.MetricClose,
					time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				Expect(math.IsNaN(val2)).To(BeTrue())
			})

			It("adds new metrics and preserves existing values", func() {
				ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
				day := engine.DayOffsetForTest(ce, time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				engine.ChunkEntrySetForTest(ce, day, 0, 0, 150.0)

				engine.ChunkEntryExpandForTest(ce, nil, []data.Metric{data.Volume})

				val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
					time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				Expect(val).To(Equal(150.0))

				Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-AAPL", data.Volume)).To(BeTrue())
			})

			It("is a no-op when all assets and metrics already present", func() {
				ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
				day := engine.DayOffsetForTest(ce, time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				engine.ChunkEntrySetForTest(ce, day, 0, 0, 150.0)

				engine.ChunkEntryExpandForTest(ce, []asset.Asset{aapl}, []data.Metric{data.MetricClose})

				val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
					time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				Expect(val).To(Equal(150.0))
			})
		})
	})

	Describe("dataCache", func() {
		It("returns nil for an unknown chunk", func() {
			dc := engine.NewDataCacheForTest(0)
			Expect(engine.GetChunkForTest(dc, 0)).To(BeNil())
		})

		It("stores and retrieves a chunk", func() {
			dc := engine.NewDataCacheForTest(0)
			aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
			yr := engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, engine.NYCForTest()))
			ce := engine.NewChunkEntryForTest(yr, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			engine.PutChunkForTest(dc, yr, ce)
			Expect(engine.GetChunkForTest(dc, yr)).To(Equal(ce))
		})

		It("tracks bytes", func() {
			dc := engine.NewDataCacheForTest(0)
			aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
			yr := engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, engine.NYCForTest()))
			ce := engine.NewChunkEntryForTest(yr, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			engine.PutChunkForTest(dc, yr, ce)
			Expect(engine.CurBytesForTest(dc)).To(Equal(int64(366*1*1*8 + 366*24)))
		})

		It("evicts old chunks but keeps previous and current year", func() {
			dc := engine.NewDataCacheForTest(0)
			loc := engine.NYCForTest()
			aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}

			yr2023 := engine.ChunkStartForTest(time.Date(2023, 6, 1, 0, 0, 0, 0, loc))
			yr2024 := engine.ChunkStartForTest(time.Date(2024, 6, 1, 0, 0, 0, 0, loc))
			yr2025 := engine.ChunkStartForTest(time.Date(2025, 6, 1, 0, 0, 0, 0, loc))

			for _, yr := range []int64{yr2023, yr2024, yr2025} {
				engine.PutChunkForTest(dc, yr, engine.NewChunkEntryForTest(yr, []asset.Asset{aapl}, []data.Metric{data.MetricClose}))
			}

			engine.EvictBeforeForTest(dc, time.Date(2025, 3, 1, 0, 0, 0, 0, loc))

			Expect(engine.GetChunkForTest(dc, yr2023)).To(BeNil(), "expected 2023 chunk evicted")
			Expect(engine.GetChunkForTest(dc, yr2024)).NotTo(BeNil(), "expected 2024 chunk kept")
			Expect(engine.GetChunkForTest(dc, yr2025)).NotTo(BeNil(), "expected 2025 chunk kept")
		})
	})
})
