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

var _ = Describe("MaxAboveZero", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		bil  asset.Asset

		t1 time.Time
		t2 time.Time
		t3 time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL001", Ticker: "AAPL"}
		bil = asset.Asset{CompositeFigi: "BIL001", Ticker: "BIL"}

		t1 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
		t3 = time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)
	})

	It("marks the asset with the highest positive value as selected", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2, t3},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{
				5, 3, 8, // SPY
				2, 1, 4, // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(2))

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t3)).To(Equal(1.0))

		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t3)).To(Equal(0.0))
	})

	It("inserts fallback assets when no asset is above zero", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{-1, -2},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{90},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(3))

		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))

		Expect(result.ValueAt(bil, data.MetricClose, t1)).To(Equal(90.0))
	})

	It("handles leadership changes across timesteps", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{
				10, 1, // SPY
				5, 20, // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(1.0))
	})

	It("uses fallback when all values are NaN", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{math.NaN(), math.NaN()},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{90},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects first asset when all values are equal positive", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{5, 5},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects no assets with nil fallback when none are positive", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{-1, 0},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("uses fallback at some timesteps and regular selection at others", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{
				10, -5, // SPY
				5, -3,  // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{90, 91},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(0.0))

		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(bil, portfolio.Selected, t2)).To(Equal(1.0))
	})

	It("trivially selects a single asset with positive value", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{42},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
	})

	It("treats +Inf as above zero and selects it", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{math.Inf(1), 5},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("treats -Inf as not above zero and falls back", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{math.Inf(-1), math.Inf(-1)},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{90},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects the positive asset when mixed with NaN at the same timestep", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{10, math.NaN()},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects overlapping fallback asset normally when it has positive value", func() {
		// BIL is in both the input DataFrame and the fallback DataFrame.
		// BIL has a positive value (80) in the input, so it should be
		// selected normally -- the fallback path should NOT trigger.
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{-1, 80},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{90},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		// BIL selected via normal path (highest positive), not fallback.
		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		// BIL's price is NOT overwritten -- fallback didn't trigger.
		Expect(result.ValueAt(bil, data.MetricClose, t1)).To(Equal(80.0))
	})

	It("selects overlapping fallback asset via fallback at timesteps where no input asset qualifies", func() {
		// BIL is in both input and fallback. At t1 SPY wins normally.
		// At t2 nothing is positive, so fallback triggers and BIL
		// should be marked Selected via the fallback path.
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{
				10, -5, // SPY
				-1, -2, // BIL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{90, 91},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		// t1: SPY wins normally.
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(0.0))

		// t2: fallback triggers, BIL selected.
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(bil, portfolio.Selected, t2)).To(Equal(1.0))
	})

	It("returns empty DataFrame with Selected for zero timestamps", func() {
		df, err := data.NewDataFrame(
			nil,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			nil,
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.Times()).To(HaveLen(0))
	})
})
