package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("EqualWeight", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		t1   time.Time
		t2   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		t1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
	})

	It("assigns 1/N to all assets", func() {
		// 2 assets, 1 metric, 2 times => data length = 2*1*2 = 4
		// Layout: [SPY-Close@t1, SPY-Close@t2, AAPL-Close@t1, AAPL-Close@t2]
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{100, 101, 200, 201},
		)
		Expect(err).ToNot(HaveOccurred())

		plan := portfolio.EqualWeight(df)
		Expect(plan).To(HaveLen(2))

		for _, alloc := range plan {
			Expect(alloc.Members).To(HaveLen(2))
			Expect(alloc.Members[spy]).To(Equal(0.5))
			Expect(alloc.Members[aapl]).To(Equal(0.5))
		}
	})

	It("handles single asset", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{100},
		)
		Expect(err).ToNot(HaveOccurred())

		plan := portfolio.EqualWeight(df)
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})

	It("sets correct date on each allocation", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{100, 101},
		)
		Expect(err).ToNot(HaveOccurred())

		plan := portfolio.EqualWeight(df)
		Expect(plan).To(HaveLen(2))
		Expect(plan[0].Date).To(Equal(t1))
		Expect(plan[1].Date).To(Equal(t2))
	})
})

var _ = Describe("WeightedBySignal", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		t1   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		t1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	})

	It("weights proportionally by signal", func() {
		// 2 assets, 1 metric (MarketCap), 1 time => data length = 2
		// Layout: [SPY-MarketCap@t1, AAPL-MarketCap@t1]
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			[]float64{300, 100},
		)
		Expect(err).ToNot(HaveOccurred())

		plan := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(0.75))
		Expect(plan[0].Members[aapl]).To(Equal(0.25))
	})

	It("normalizes weights to sum to 1.0", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			[]float64{300, 100},
		)
		Expect(err).ToNot(HaveOccurred())

		plan := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(plan).To(HaveLen(1))

		sum := 0.0
		for _, w := range plan[0].Members {
			sum += w
		}
		Expect(sum).To(BeNumerically("~", 1.0, 1e-9))
	})
})
