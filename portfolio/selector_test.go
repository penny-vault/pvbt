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

	It("selects the asset with the highest positive value at each timestep", func() {
		// Two assets, three timesteps; SPY always higher.
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2, t3},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				// SPY: t1=5, t2=3, t3=8
				5, 3, 8,
				// AAPL: t1=2, t2=1, t3=4
				2, 1, 4,
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		// SPY wins every timestep, so only SPY should be in result.
		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("SPY001"))
	})

	It("falls back to specified assets when no assets are above zero", func() {
		// All signal values negative; BIL is present as a fallback candidate.
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MetricClose},
			[]float64{
				-1, // SPY
				-2, // AAPL
				-3, // BIL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero([]asset.Asset{bil})
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("BIL001"))
	})

	It("handles leadership changes across timesteps", func() {
		// SPY leads at t1, AAPL leads at t2 -- both should appear in result.
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				// SPY: t1=10, t2=1
				10, 1,
				// AAPL: t1=5, t2=20
				5, 20,
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		// Both assets are selected (each leads at one timestep).
		Expect(result.AssetList()).To(HaveLen(2))
	})

	It("uses fallback when all signal values are NaN", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MetricClose},
			[]float64{
				math.NaN(), // SPY
				math.NaN(), // AAPL
				math.NaN(), // BIL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero([]asset.Asset{bil})
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("BIL001"))
	})

	It("deterministically selects first asset when all values are equal positive", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				5, // SPY
				5, // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		// Both have equal values; the first asset in iteration wins (strict >).
		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("SPY001"))
	})

	It("returns empty result with nil fallback when no values are positive", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				-1, // SPY
				0,  // AAPL (zero is not above zero)
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(0))
	})

	It("trivially selects a single asset with positive signal", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{42},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("SPY001"))
	})

	It("treats +Inf as above zero and selects it", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				math.Inf(1), // SPY
				5,           // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("SPY001"))
	})

	It("treats -Inf as not above zero and falls back", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MetricClose},
			[]float64{
				math.Inf(-1), // SPY
				math.Inf(-1), // AAPL
				-1,           // BIL (also not positive, but selected via fallback)
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero([]asset.Asset{bil})
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("BIL001"))
	})

	It("selects the positive asset when mixed with NaN at the same timestep", func() {
		// SPY is positive, AAPL is NaN -- SPY should win.
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MetricClose},
			[]float64{
				10,           // SPY (positive)
				math.NaN(),   // AAPL (NaN, skipped)
				math.NaN(),   // BIL (NaN, skipped)
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero([]asset.Asset{bil})
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("SPY001"))
	})

	It("returns empty result for an empty DataFrame with zero timestamps", func() {
		// Zero timestamps, one asset, one metric -- data length is 0*1*1 = 0.
		df, err := data.NewDataFrame(
			nil,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			nil,
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero([]asset.Asset{bil})
		result := sel.Select(df)

		// No timesteps to iterate, so no assets are selected.
		Expect(result.AssetList()).To(HaveLen(0))
		Expect(result.Times()).To(HaveLen(0))
	})
})
