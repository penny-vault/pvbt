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
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("ResamplingProvider", func() {
	const numDays = 20

	var (
		testAsset       asset.Asset
		historicalFrame *data.DataFrame
		times           []time.Time
		histMetrics     []data.Metric
	)

	BeforeEach(func() {
		testAsset = asset.Asset{CompositeFigi: "FIGI-TEST", Ticker: "TEST"}

		base := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		times = make([]time.Time, numDays)
		for dayIdx := range numDays {
			times[dayIdx] = base.AddDate(0, 0, dayIdx)
		}

		histMetrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.SplitFactor}
		numMetrics := len(histMetrics)

		closePrices := make([]float64, numDays)
		adjClosePrices := make([]float64, numDays)
		dividends := make([]float64, numDays)
		splitFactors := make([]float64, numDays)

		for dayIdx := range numDays {
			price := 100.0 + float64(dayIdx)*0.5
			closePrices[dayIdx] = price
			adjClosePrices[dayIdx] = price
			dividends[dayIdx] = 0.0
			splitFactors[dayIdx] = 1.0
		}

		cols := make([][]float64, 1*numMetrics)
		cols[0*numMetrics+0] = closePrices
		cols[0*numMetrics+1] = adjClosePrices
		cols[0*numMetrics+2] = dividends
		cols[0*numMetrics+3] = splitFactors

		var err error
		historicalFrame, err = data.NewDataFrame(times, []asset.Asset{testAsset}, histMetrics, data.Daily, cols)
		Expect(err).NotTo(HaveOccurred())
	})

	It("satisfies the BatchProvider interface", func() {
		var _ data.BatchProvider = data.NewResamplingProvider(historicalFrame, &data.ReturnBootstrap{}, 42, histMetrics)
	})

	It("Close returns nil", func() {
		provider := data.NewResamplingProvider(historicalFrame, &data.ReturnBootstrap{}, 42, histMetrics)
		Expect(provider.Close()).To(Succeed())
	})

	It("Provides returns configured metrics", func() {
		metrics := []data.Metric{data.MetricClose, data.AdjClose}
		provider := data.NewResamplingProvider(historicalFrame, &data.ReturnBootstrap{}, 42, metrics)
		Expect(provider.Provides()).To(Equal(metrics))
	})

	Describe("Fetch", func() {
		var (
			provider   *data.ResamplingProvider
			req        data.DataRequest
			outMetrics []data.Metric
		)

		BeforeEach(func() {
			outMetrics = []data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow, data.Dividend, data.SplitFactor}
			provider = data.NewResamplingProvider(historicalFrame, &data.ReturnBootstrap{}, 42, outMetrics)
			req = data.DataRequest{
				Assets:    []asset.Asset{testAsset},
				Metrics:   outMetrics,
				Start:     times[0],
				End:       times[numDays-1],
				Frequency: data.Daily,
			}
		})

		It("returns a DataFrame with correct dimensions", func() {
			result, err := provider.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Len()).To(Equal(numDays))
			Expect(result.ColCount()).To(Equal(1 * len(outMetrics)))
		})

		It("zeroes out dividends", func() {
			result, err := provider.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			divCol := result.Column(testAsset, data.Dividend)
			Expect(divCol).To(HaveLen(numDays))
			for timeIdx, div := range divCol {
				Expect(div).To(BeZero(), "dividend at index %d should be zero", timeIdx)
			}
		})

		It("sets split factor to 1.0", func() {
			result, err := provider.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			sfCol := result.Column(testAsset, data.SplitFactor)
			Expect(sfCol).To(HaveLen(numDays))
			for timeIdx, sf := range sfCol {
				Expect(sf).To(BeNumerically("==", 1.0), "split factor at index %d should be 1.0", timeIdx)
			}
		})

		It("High is price * 1.005 and Low is price * 0.995", func() {
			result, err := provider.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			closeCol := result.Column(testAsset, data.MetricClose)
			highCol := result.Column(testAsset, data.MetricHigh)
			lowCol := result.Column(testAsset, data.MetricLow)

			for timeIdx := range numDays {
				Expect(highCol[timeIdx]).To(BeNumerically("~", closeCol[timeIdx]*1.005, 1e-9),
					"High at index %d", timeIdx)
				Expect(lowCol[timeIdx]).To(BeNumerically("~", closeCol[timeIdx]*0.995, 1e-9),
					"Low at index %d", timeIdx)
			}
		})

		It("produces valid prices (no NaN, no negative)", func() {
			result, err := provider.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			closeCol := result.Column(testAsset, data.MetricClose)
			for timeIdx, price := range closeCol {
				Expect(math.IsNaN(price)).To(BeFalse(), "price at index %d is NaN", timeIdx)
				Expect(price).To(BeNumerically(">", 0), "price at index %d should be positive", timeIdx)
			}
		})

		It("produces different results with different seeds", func() {
			provider1 := data.NewResamplingProvider(historicalFrame, &data.ReturnBootstrap{}, 1, outMetrics)
			provider2 := data.NewResamplingProvider(historicalFrame, &data.ReturnBootstrap{}, 2, outMetrics)

			result1, err := provider1.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			result2, err := provider2.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			close1 := result1.Column(testAsset, data.MetricClose)
			close2 := result2.Column(testAsset, data.MetricClose)

			// With different seeds, at least one price should differ
			// (they share the same first price, so compare from index 1).
			anyDiff := false
			for timeIdx := 1; timeIdx < numDays; timeIdx++ {
				if math.Abs(close1[timeIdx]-close2[timeIdx]) > 1e-12 {
					anyDiff = true
					break
				}
			}
			Expect(anyDiff).To(BeTrue(), "different seeds should produce different price paths")
		})

		It("is reproducible with the same seed", func() {
			provider1 := data.NewResamplingProvider(historicalFrame, &data.ReturnBootstrap{}, 99, outMetrics)
			provider2 := data.NewResamplingProvider(historicalFrame, &data.ReturnBootstrap{}, 99, outMetrics)

			result1, err := provider1.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			result2, err := provider2.Fetch(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			close1 := result1.Column(testAsset, data.MetricClose)
			close2 := result2.Column(testAsset, data.MetricClose)

			for timeIdx := range numDays {
				Expect(close1[timeIdx]).To(BeNumerically("~", close2[timeIdx], 1e-12),
					"same seed should produce identical price at index %d", timeIdx)
			}
		})
	})
})
