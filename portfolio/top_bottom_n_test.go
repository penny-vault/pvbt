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

var _ = Describe("TopN", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		msft asset.Asset
		goog asset.Asset

		t1 time.Time
		t2 time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL001", Ticker: "AAPL"}
		msft = asset.Asset{CompositeFigi: "MSFT001", Ticker: "MSFT"}
		goog = asset.Asset{CompositeFigi: "GOOG001", Ticker: "GOOG"}

		t1 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	})

	It("selects the N assets with the highest metric values", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, msft, goog},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{10, 30, 20, 5},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(2, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(msft, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(goog, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects fewer than N when not enough valid assets exist", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{10, math.NaN()},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(3, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("excludes NaN values from ranking", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, msft},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{10, math.NaN(), 5},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(2, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(msft, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects the single asset in a single-asset DataFrame", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{42},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(1, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
	})

	It("selects no assets when all values are NaN", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{math.NaN(), math.NaN()},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(2, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects all assets when N exceeds asset count", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{10, 20},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(5, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(1.0))
	})

	It("handles leadership changes across timesteps", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl, msft},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{
				30, 5,  // SPY
				10, 20, // AAPL
				20, 25, // MSFT
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(2, data.MetricClose)
		result := sel.Select(df)

		// t1: SPY=30, MSFT=20 are top 2
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(msft, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))

		// t2: MSFT=25, AAPL=20 are top 2
		Expect(result.ValueAt(msft, portfolio.Selected, t2)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(0.0))
	})

	It("handles zero timesteps without panic", func() {
		df, err := data.NewDataFrame(
			nil,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			nil,
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(1, data.MetricClose)
		result := sel.Select(df)

		Expect(result.Times()).To(HaveLen(0))
	})

	It("ranks mixed positive and negative values correctly", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, msft},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{-5, 10, -20},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(2, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(msft, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("panics when n < 1", func() {
		Expect(func() { portfolio.TopN(0, data.MetricClose) }).To(Panic())
		Expect(func() { portfolio.TopN(-1, data.MetricClose) }).To(Panic())
	})

	It("selects exactly N assets when ties exist", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, msft, goog},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{10, 10, 10, 5},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(2, data.MetricClose)
		result := sel.Select(df)

		selectedCount := 0.0
		for _, a := range []asset.Asset{spy, aapl, msft, goog} {
			selectedCount += result.ValueAt(a, portfolio.Selected, t1)
		}
		Expect(selectedCount).To(Equal(2.0))
	})

	It("handles asset with NaN at some timesteps but valid at others", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{
				10, 15,        // SPY
				math.NaN(), 5, // AAPL: NaN at t1, valid at t2
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.TopN(1, data.MetricClose)
		result := sel.Select(df)

		// t1: only SPY valid
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))

		// t2: SPY=15 > AAPL=5
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(0.0))
	})
})

var _ = Describe("BottomN", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		msft asset.Asset
		goog asset.Asset

		t1 time.Time
		t2 time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL001", Ticker: "AAPL"}
		msft = asset.Asset{CompositeFigi: "MSFT001", Ticker: "MSFT"}
		goog = asset.Asset{CompositeFigi: "GOOG001", Ticker: "GOOG"}

		t1 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	})

	It("selects the N assets with the lowest metric values", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, msft, goog},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{10, 30, 20, 5},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.BottomN(2, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(goog, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(msft, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("handles leadership changes across timesteps", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl, msft},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{
				5, 30,  // SPY
				10, 1,  // AAPL
				20, 15, // MSFT
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.BottomN(2, data.MetricClose)
		result := sel.Select(df)

		// t1: SPY=5, AAPL=10 are bottom 2
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(msft, portfolio.Selected, t1)).To(Equal(0.0))

		// t2: AAPL=1, MSFT=15 are bottom 2
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(1.0))
		Expect(result.ValueAt(msft, portfolio.Selected, t2)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(0.0))
	})

	It("ranks mixed positive and negative values correctly", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, msft},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{-5, 10, -20},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.BottomN(2, data.MetricClose)
		result := sel.Select(df)

		Expect(result.ValueAt(msft, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("panics when n < 1", func() {
		Expect(func() { portfolio.BottomN(0, data.MetricClose) }).To(Panic())
		Expect(func() { portfolio.BottomN(-1, data.MetricClose) }).To(Panic())
	})
})
