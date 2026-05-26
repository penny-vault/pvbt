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

package data_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("intraday period constructors", func() {
	Describe("MinuteBars", func() {
		It("captures N and routes as intraday", func() {
			period := data.MinuteBars(60)

			Expect(period.N).To(Equal(60))
			Expect(period.Unit).To(Equal(data.UnitMinuteBar))
			Expect(period.IsIntraday()).To(BeTrue())
			Expect(period.TimeOfDay).To(BeEmpty())
		})

		It("Before subtracts N minutes from the reference", func() {
			ref := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
			start := data.MinuteBars(30).Before(ref)

			Expect(start).To(Equal(ref.Add(-30 * time.Minute)))
		})
	})

	Describe("DailyAtTime", func() {
		It("parses a single time-of-day", func() {
			period := data.DailyAtTime("10:00", 5)

			Expect(period.N).To(Equal(5))
			Expect(period.Unit).To(Equal(data.UnitDailyAtTime))
			Expect(period.IsIntraday()).To(BeTrue())
			Expect(period.TimeOfDay).To(HaveLen(1))
			Expect(period.TimeOfDay[0].Hour).To(Equal(10))
			Expect(period.TimeOfDay[0].Minute).To(Equal(0))
			Expect(period.TimeOfDay[0].MinutesSinceMidnight()).To(Equal(600))
		})

		It("parses multiple comma-separated times", func() {
			period := data.DailyAtTime("10:00,14:30", 5)

			Expect(period.TimeOfDay).To(HaveLen(2))
			Expect(period.TimeOfDay[0].MinutesSinceMidnight()).To(Equal(600))
			Expect(period.TimeOfDay[1].MinutesSinceMidnight()).To(Equal(870))
		})

		It("panics on malformed input", func() {
			Expect(func() { data.DailyAtTime("invalid", 5) }).To(Panic())
			Expect(func() { data.DailyAtTime("25:00", 5) }).To(Panic())
			Expect(func() { data.DailyAtTime("10:99", 5) }).To(Panic())
		})

		It("Before walks back N calendar days from reference", func() {
			ref := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
			start := data.DailyAtTime("10:00", 5).Before(ref)

			Expect(start).To(Equal(ref.AddDate(0, 0, -5)))
		})
	})

	Describe("daily periods", func() {
		It("are not intraday", func() {
			Expect(data.Days(60).IsIntraday()).To(BeFalse())
			Expect(data.Months(13).IsIntraday()).To(BeFalse())
			Expect(data.Years(2).IsIntraday()).To(BeFalse())
			Expect(data.YTD().IsIntraday()).To(BeFalse())
		})
	})
})

var _ = Describe("IntradayMetric", func() {
	It("recognises raw OHLCV", func() {
		Expect(data.IntradayMetric(data.MetricOpen)).To(BeTrue())
		Expect(data.IntradayMetric(data.MetricHigh)).To(BeTrue())
		Expect(data.IntradayMetric(data.MetricLow)).To(BeTrue())
		Expect(data.IntradayMetric(data.MetricClose)).To(BeTrue())
		Expect(data.IntradayMetric(data.Volume)).To(BeTrue())
	})

	It("recognises adjusted OHLCV", func() {
		Expect(data.IntradayMetric(data.AdjOpen)).To(BeTrue())
		Expect(data.IntradayMetric(data.AdjHigh)).To(BeTrue())
		Expect(data.IntradayMetric(data.AdjLow)).To(BeTrue())
		Expect(data.IntradayMetric(data.AdjClose)).To(BeTrue())
		Expect(data.IntradayMetric(data.AdjVolume)).To(BeTrue())
	})

	It("rejects non-OHLCV metrics", func() {
		Expect(data.IntradayMetric(data.Dividend)).To(BeFalse())
		Expect(data.IntradayMetric(data.SplitFactor)).To(BeFalse())
		Expect(data.IntradayMetric(data.PE)).To(BeFalse())
	})
})
