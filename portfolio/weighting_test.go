package portfolio_test

import (
	"math"
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
			data.Daily,
			[]float64{100, 101, 200, 201},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1, 1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1, 1})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
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
			data.Daily,
			[]float64{100},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})

	It("sets correct date on each allocation", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100, 101},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1, 1})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
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
			data.Daily,
			[]float64{300, 100},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(0.75))
		Expect(plan[0].Members[aapl]).To(Equal(0.25))
	})

	It("normalizes weights to sum to 1.0", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{300, 100},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		sum := 0.0
		for _, w := range plan[0].Members {
			sum += w
		}
		Expect(sum).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("falls back to equal weight when all signal values are negative", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{-10, -20},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("skips NaN values and weights positive values proportionally", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{300, math.NaN()},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		// SPY gets 300/300 = 1.0, AAPL omitted (NaN metric, zero weight)
		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})

	It("assigns weight 1.0 to a single asset", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{500},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})

	It("falls back to equal weight when all signal values are zero", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{0, 0},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("ignores negative values and weights positive values proportionally", func() {
		bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{
				300, // SPY
				-50, // AAPL (negative, should be ignored)
				100, // BIL
			},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(bil, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		// positive sum = 300 + 100 = 400
		// SPY = 300/400 = 0.75, AAPL omitted (negative metric), BIL = 100/400 = 0.25
		Expect(plan[0].Members).To(HaveLen(2))
		Expect(plan[0].Members[spy]).To(Equal(0.75))
		Expect(plan[0].Members[bil]).To(Equal(0.25))
	})

	It("computes weights independently at each timestep", func() {
		t2 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{
				// SPY: t1=300, t2=100
				300, 100,
				// AAPL: t1=100, t2=300
				100, 300,
			},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1, 1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1, 1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(2))

		// t1: SPY=300/(300+100)=0.75, AAPL=100/400=0.25
		Expect(plan[0].Date).To(Equal(t1))
		Expect(plan[0].Members[spy]).To(Equal(0.75))
		Expect(plan[0].Members[aapl]).To(Equal(0.25))

		// t2: SPY=100/(100+300)=0.25, AAPL=300/400=0.75
		Expect(plan[1].Date).To(Equal(t2))
		Expect(plan[1].Members[spy]).To(Equal(0.25))
		Expect(plan[1].Members[aapl]).To(Equal(0.75))
	})

	It("falls back to equal weight when all signal values are NaN at a timestep", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{math.NaN(), math.NaN()},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		// All NaN => sum is 0 => falls back to equal weight.
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("correctly weights a single timestep", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{200, 800},
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Date).To(Equal(t1))

		// SPY = 200/1000 = 0.2, AAPL = 800/1000 = 0.8
		Expect(plan[0].Members[spy]).To(Equal(0.2))
		Expect(plan[0].Members[aapl]).To(Equal(0.8))

		sum := 0.0
		for _, w := range plan[0].Members {
			sum += w
		}
		Expect(sum).To(BeNumerically("~", 1.0, 1e-9))
	})
})

var _ = Describe("EqualWeight edge cases", func() {
	It("returns empty plan for a DataFrame with zero timestamps", func() {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		// Zero timestamps, one asset, one metric -- data length is 0.
		df, err := data.NewDataFrame(
			nil,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(0))
	})

	It("returns allocations with empty members for a DataFrame with zero assets", func() {
		t1 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

		// One timestamp, zero assets, one metric -- data length is 0.
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			nil,
			[]data.Metric{data.MetricClose, portfolio.Selected},
			data.Daily,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Date).To(Equal(t1))
		Expect(plan[0].Members).To(HaveLen(0))
	})
})

var _ = Describe("EqualWeight with Selected column", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		bil  asset.Asset
		t1   time.Time
		t2   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
		t1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
	})

	It("returns error when Selected column is missing", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = portfolio.EqualWeight(df)
		Expect(err).To(HaveOccurred())
	})

	It("assigns equal weight only to selected assets at each timestep", func() {
		// 3 assets, 2 timesteps. SPY selected at t1, AAPL selected at t2.
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{
				100, 101, // SPY
				200, 201, // AAPL
				50, 51, // BIL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		// Insert Selected columns: SPY=1 at t1, 0 at t2
		Expect(df.Insert(spy, portfolio.Selected, []float64{1, 0})).To(Succeed())
		// AAPL=0 at t1, 1 at t2
		Expect(df.Insert(aapl, portfolio.Selected, []float64{0, 1})).To(Succeed())
		// BIL=0 at both
		Expect(df.Insert(bil, portfolio.Selected, []float64{0, 0})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(2))

		// t1: only SPY selected => weight 1.0
		Expect(plan[0].Date).To(Equal(t1))
		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))

		// t2: only AAPL selected => weight 1.0
		Expect(plan[1].Date).To(Equal(t2))
		Expect(plan[1].Members).To(HaveLen(1))
		Expect(plan[1].Members[aapl]).To(Equal(1.0))
	})

	It("assigns equal weight when multiple assets selected at same timestep", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100, 200, 50},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(bil, portfolio.Selected, []float64{0})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members).To(HaveLen(2))
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("treats fractional Selected > 0 as selected", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{0.5})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1.0})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		// Both selected (magnitude ignored), equal weight
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("treats NaN in Selected column as not selected", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1.0})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{math.NaN()})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})

	It("produces empty members when no assets are selected at a timestep", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{0})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{0})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members).To(HaveLen(0))
	})
})

var _ = Describe("WeightedBySignal with Selected column", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		bil  asset.Asset
		t1   time.Time
		t2   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
		t1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
	})

	It("returns error when Selected column is missing", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{300},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).To(HaveOccurred())
	})

	It("weights only selected assets by signal", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{300, 100, 500},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(bil, portfolio.Selected, []float64{0})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		Expect(plan[0].Members).To(HaveLen(2))
		Expect(plan[0].Members[spy]).To(Equal(0.75))
		Expect(plan[0].Members[aapl]).To(Equal(0.25))
	})

	It("uses per-timestep selection for weighting", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{
				300, 100, // SPY
				100, 300, // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1, 0})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{0, 1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(2))

		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))

		Expect(plan[1].Members).To(HaveLen(1))
		Expect(plan[1].Members[aapl]).To(Equal(1.0))
	})

	It("falls back to equal weight among selected when all signal values are zero", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{0, 0, 500},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(bil, portfolio.Selected, []float64{0})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		Expect(plan[0].Members).To(HaveLen(2))
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("discards zero signal values in normalization", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{300, 0},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		// SPY=300/300=1.0, AAPL discarded (zero signal, omitted from map)
		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})
})
