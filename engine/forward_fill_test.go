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

package engine_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
)

var _ = Describe("ForwardFillTo", func() {
	var spy asset.Asset

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
	})

	It("fills daily data across multiple days", func() {
		lastDate := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		targetDate := time.Date(2024, 1, 19, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{lastDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		result, err := engine.ForwardFillTo(df, targetDate)
		Expect(err).NotTo(HaveOccurred())

		// Original + 4 days (16th, 17th, 18th, 19th)
		Expect(result.Len()).To(Equal(5))
		Expect(result.End()).To(Equal(targetDate))

		lastValue := result.ValueAt(spy, data.MetricClose, targetDate)
		Expect(lastValue).To(Equal(500.0))
	})

	It("is a no-op when DataFrame already covers the target date", func() {
		targetDate := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{targetDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		result, err := engine.ForwardFillTo(df, targetDate)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
	})

	It("returns error for Tick frequency", func() {
		lastDate := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		targetDate := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{lastDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Tick,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = engine.ForwardFillTo(df, targetDate)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tick"))
	})

	It("fills weekly data with 7-day spacing", func() {
		lastDate := time.Date(2024, 1, 5, 16, 0, 0, 0, time.UTC)
		targetDate := time.Date(2024, 1, 26, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{lastDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Weekly,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		result, err := engine.ForwardFillTo(df, targetDate)
		Expect(err).NotTo(HaveOccurred())

		// Original + 3 weeks (12th, 19th, 26th)
		Expect(result.Len()).To(Equal(4))
		Expect(result.End()).To(Equal(targetDate))
	})

	It("fills monthly data with 1-month spacing", func() {
		lastDate := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		targetDate := time.Date(2024, 3, 15, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{lastDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Monthly,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		result, err := engine.ForwardFillTo(df, targetDate)
		Expect(err).NotTo(HaveOccurred())

		// Original + 2 months (Feb 15, Mar 15)
		Expect(result.Len()).To(Equal(3))
	})
})
