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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("TestProvider", func() {
	var (
		provider *data.TestProvider
		aapl     asset.Asset
		goog     asset.Asset
		times    []time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "GOOG", Ticker: "GOOG"}

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		times = make([]time.Time, 5)
		for i := range times {
			times[i] = base.AddDate(0, 0, i)
		}

		vals := [][]float64{
			{100, 101, 102, 103, 104}, // AAPL Price
			{200, 202, 204, 206, 208}, // GOOG Price
		}
		frame, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())
		provider = data.NewTestProvider([]data.Metric{data.Price}, frame)
	})

	It("Provides returns configured metrics", func() {
		Expect(provider.Provides()).To(Equal([]data.Metric{data.Price}))
	})

	It("Close returns nil", func() {
		Expect(provider.Close()).To(Succeed())
	})

	It("Fetch narrows by asset", func() {
		req := data.DataRequest{
			Assets:  []asset.Asset{aapl},
			Metrics: []data.Metric{data.Price},
			Start:   times[0],
			End:     times[4],
		}
		result, err := provider.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ColCount()).To(Equal(1))
		Expect(result.Value(aapl, data.Price)).To(Equal(104.0))
	})

	It("Fetch narrows by time range", func() {
		req := data.DataRequest{
			Assets:  []asset.Asset{aapl, goog},
			Metrics: []data.Metric{data.Price},
			Start:   times[1],
			End:     times[3],
		}
		result, err := provider.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(3))
		Expect(result.Start()).To(Equal(times[1]))
		Expect(result.End()).To(Equal(times[3]))
	})

	It("Fetch with no matching assets returns empty frame", func() {
		unknown := asset.Asset{CompositeFigi: "UNKNOWN", Ticker: "UNK"}
		req := data.DataRequest{
			Assets:  []asset.Asset{unknown},
			Metrics: []data.Metric{data.Price},
			Start:   times[0],
			End:     times[4],
		}
		result, err := provider.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(0))
	})

	It("Fetch with no matching time range returns empty frame", func() {
		far := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
		req := data.DataRequest{
			Assets:  []asset.Asset{aapl},
			Metrics: []data.Metric{data.Price},
			Start:   far,
			End:     far.AddDate(0, 0, 1),
		}
		result, err := provider.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(0))
	})
})
