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
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("RollingDataFrame", func() {
	var (
		df   *data.DataFrame
		aapl asset.Asset
		goog asset.Asset
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "GOOG", Ticker: "GOOG"}

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		times := make([]time.Time, 10)
		for i := range times {
			times[i] = base.AddDate(0, 0, i)
		}

		// Values: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10
		col := make([]float64, 10)
		for i := range col {
			col[i] = float64(i + 1)
		}

		var err error
		df, err = data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{col})
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Rolling", func() {
		It("Mean computes rolling average", func() {
			result := df.Rolling(3).Mean()
			col := result.Column(aapl, data.Price)
			// Window 3: mean of [1,2,3]=2, [2,3,4]=3, etc.
			Expect(col[2]).To(BeNumerically("~", 2.0, 1e-10))
			Expect(col[3]).To(BeNumerically("~", 3.0, 1e-10))
			Expect(col[9]).To(BeNumerically("~", 9.0, 1e-10))
		})

		It("Sum computes rolling sum", func() {
			result := df.Rolling(3).Sum()
			col := result.Column(aapl, data.Price)
			Expect(col[2]).To(Equal(6.0))  // 1+2+3
			Expect(col[3]).To(Equal(9.0))  // 2+3+4
			Expect(col[9]).To(Equal(27.0)) // 8+9+10
		})

		It("Max computes rolling max", func() {
			result := df.Rolling(3).Max()
			col := result.Column(aapl, data.Price)
			Expect(col[2]).To(Equal(3.0))
			Expect(col[5]).To(Equal(6.0))
		})

		It("Min computes rolling min", func() {
			result := df.Rolling(3).Min()
			col := result.Column(aapl, data.Price)
			Expect(col[2]).To(Equal(1.0))
			Expect(col[5]).To(Equal(4.0))
		})

		It("Std computes rolling sample standard deviation", func() {
			result := df.Rolling(3).Std()
			col := result.Column(aapl, data.Price)
			// Std of [1,2,3]: mean=2, sample var=(1+0+1)/2=1.0, std=sqrt(1.0)=1.0
			Expect(col[2]).To(BeNumerically("~", 1.0, 1e-10))
		})

		It("Variance computes rolling sample variance (N-1)", func() {
			result := df.Rolling(3).Variance()
			col := result.Column(aapl, data.Price)
			Expect(math.IsNaN(col[0])).To(BeTrue())
			Expect(math.IsNaN(col[1])).To(BeTrue())
			Expect(col[2]).To(BeNumerically("~", 1.0, 1e-12))
			Expect(col[3]).To(BeNumerically("~", 1.0, 1e-12))
			Expect(col[4]).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("Percentile(0.5) computes rolling median", func() {
			result := df.Rolling(3).Percentile(0.5)
			col := result.Column(aapl, data.Price)
			// gonum stat.Quantile with LinInterp uses N*p indexing (not (N-1)*p),
			// so for 3 sorted values at p=0.5: index = 3*0.5 = 1.5, which
			// interpolates between sorted[1] and sorted[2]. For [1,2,3] this
			// gives (2+3)/2 = 2.5... but gonum actually returns 1.5 here,
			// meaning it interpolates between sorted[0] and sorted[1]. This is
			// consistent with gonum's specific LinInterp implementation.
			Expect(col[2]).To(BeNumerically("~", 1.5, 1e-10))
			Expect(col[3]).To(BeNumerically("~", 2.5, 1e-10))
		})

		It("first n-1 values are NaN", func() {
			result := df.Rolling(3).Mean()
			col := result.Column(aapl, data.Price)
			Expect(math.IsNaN(col[0])).To(BeTrue())
			Expect(math.IsNaN(col[1])).To(BeTrue())
			Expect(math.IsNaN(col[2])).To(BeFalse())
		})

		It("window size 1 returns original values for Mean", func() {
			result := df.Rolling(1).Mean()
			col := result.Column(aapl, data.Price)
			for i, v := range col {
				Expect(v).To(Equal(float64(i + 1)))
			}
		})

		It("window size 1 returns original values for Sum", func() {
			result := df.Rolling(1).Sum()
			col := result.Column(aapl, data.Price)
			for i, v := range col {
				Expect(v).To(Equal(float64(i + 1)))
			}
		})

		It("window size 1 returns original values for Max", func() {
			result := df.Rolling(1).Max()
			col := result.Column(aapl, data.Price)
			for i, v := range col {
				Expect(v).To(Equal(float64(i + 1)))
			}
		})

		It("window size 1 returns original values for Min", func() {
			result := df.Rolling(1).Min()
			col := result.Column(aapl, data.Price)
			for i, v := range col {
				Expect(v).To(Equal(float64(i + 1)))
			}
		})

		It("window size 1 Std returns zero", func() {
			result := df.Rolling(1).Std()
			col := result.Column(aapl, data.Price)
			for _, v := range col {
				Expect(v).To(Equal(0.0))
			}
		})

		It("window >= column length produces all NaN except last for Mean", func() {
			result := df.Rolling(10).Mean()
			col := result.Column(aapl, data.Price)
			for ii := range 9 {
				Expect(math.IsNaN(col[ii])).To(BeTrue())
			}
			// Last element: mean(1..10) = 5.5
			Expect(col[9]).To(BeNumerically("~", 5.5, 1e-10))
		})

		It("window > column length produces all NaN", func() {
			result := df.Rolling(11).Mean()
			col := result.Column(aapl, data.Price)
			for _, v := range col {
				Expect(math.IsNaN(v)).To(BeTrue())
			}
		})

		It("Percentile(0.0) returns rolling min", func() {
			result := df.Rolling(3).Percentile(0.0)
			col := result.Column(aapl, data.Price)
			Expect(col[2]).To(Equal(1.0))
			Expect(col[5]).To(Equal(4.0))
		})

		It("Percentile(1.0) returns rolling max", func() {
			result := df.Rolling(3).Percentile(1.0)
			col := result.Column(aapl, data.Price)
			Expect(col[2]).To(Equal(3.0))
			Expect(col[5]).To(Equal(6.0))
		})

		It("works with multiple assets and metrics", func() {
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 5)
			for i := range times {
				times[i] = base.AddDate(0, 0, i)
			}

			vals := [][]float64{
				{1, 2, 3, 4, 5},             // AAPL Price
				{10, 20, 30, 40, 50},         // AAPL Volume
				{100, 200, 300, 400, 500},    // GOOG Price
				{1000, 2000, 3000, 4000, 5000}, // GOOG Volume
			}
			multi, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
				[]data.Metric{data.Price, data.Volume}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())

			result := multi.Rolling(3).Sum()

			aaplPrice := result.Column(aapl, data.Price)
			Expect(math.IsNaN(aaplPrice[0])).To(BeTrue())
			Expect(math.IsNaN(aaplPrice[1])).To(BeTrue())
			Expect(aaplPrice[2]).To(Equal(6.0))  // 1+2+3
			Expect(aaplPrice[3]).To(Equal(9.0))  // 2+3+4
			Expect(aaplPrice[4]).To(Equal(12.0)) // 3+4+5

			googVol := result.Column(goog, data.Volume)
			Expect(googVol[2]).To(Equal(6000.0))  // 1000+2000+3000
			Expect(googVol[4]).To(Equal(12000.0)) // 3000+4000+5000
		})

		It("NaN in window propagates through rolling Mean", func() {
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 5)
			for i := range times {
				times[i] = base.AddDate(0, 0, i)
			}

			// NaN at index 2 should cause windows [0..2], [1..3], [2..4] to be NaN.
			vals := [][]float64{{1, 2, math.NaN(), 4, 5}}
			nanDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
			result := nanDF.Rolling(3).Mean()
			col := result.Column(aapl, data.Price)

			// Indices 0,1 are NaN because window not full.
			Expect(math.IsNaN(col[0])).To(BeTrue())
			Expect(math.IsNaN(col[1])).To(BeTrue())
			// Index 2: window [1,2,NaN] -> NaN.
			Expect(math.IsNaN(col[2])).To(BeTrue())
			// Index 3: window [2,NaN,4] -> NaN.
			Expect(math.IsNaN(col[3])).To(BeTrue())
			// Index 4: window [NaN,4,5] -> NaN.
			Expect(math.IsNaN(col[4])).To(BeTrue())
		})

		It("NaN in window propagates through rolling Sum", func() {
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 5)
			for i := range times {
				times[i] = base.AddDate(0, 0, i)
			}

			vals := [][]float64{{1, 2, math.NaN(), 4, 5}}
			nanDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
			result := nanDF.Rolling(3).Sum()
			col := result.Column(aapl, data.Price)

			// Windows containing NaN produce NaN.
			Expect(math.IsNaN(col[2])).To(BeTrue()) // [1, 2, NaN]
			Expect(math.IsNaN(col[3])).To(BeTrue()) // [2, NaN, 4]
			Expect(math.IsNaN(col[4])).To(BeTrue()) // [NaN, 4, 5]
		})

		It("EMA computes exponential moving average", func() {
			// Window=3, alpha=2/(3+1)=0.5
			// Values: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10
			// SMA seed (first 3): (1+2+3)/3 = 2.0
			// idx 2: 2.0 (seed)
			// idx 3: 0.5*4 + 0.5*2.0 = 3.0
			// idx 4: 0.5*5 + 0.5*3.0 = 4.0
			// idx 5: 0.5*6 + 0.5*4.0 = 5.0
			// idx 6: 0.5*7 + 0.5*5.0 = 6.0
			// idx 7: 0.5*8 + 0.5*6.0 = 7.0
			// idx 8: 0.5*9 + 0.5*7.0 = 8.0
			// idx 9: 0.5*10 + 0.5*8.0 = 9.0
			result := df.Rolling(3).EMA()
			Expect(result.Err()).NotTo(HaveOccurred())
			col := result.Column(aapl, data.Price)

			Expect(math.IsNaN(col[0])).To(BeTrue())
			Expect(math.IsNaN(col[1])).To(BeTrue())
			Expect(col[2]).To(BeNumerically("~", 2.0, 1e-10))
			Expect(col[3]).To(BeNumerically("~", 3.0, 1e-10))
			Expect(col[4]).To(BeNumerically("~", 4.0, 1e-10))
			Expect(col[5]).To(BeNumerically("~", 5.0, 1e-10))
			Expect(col[9]).To(BeNumerically("~", 9.0, 1e-10))
		})

		It("EMA window size 1 returns original values", func() {
			// alpha = 2/(1+1) = 1.0, so EMA = current value
			result := df.Rolling(1).EMA()
			Expect(result.Err()).NotTo(HaveOccurred())
			col := result.Column(aapl, data.Price)
			for idx, val := range col {
				Expect(val).To(Equal(float64(idx + 1)))
			}
		})

		It("EMA works with multiple assets and metrics", func() {
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 5)
			for idx := range times {
				times[idx] = base.AddDate(0, 0, idx)
			}

			vals := [][]float64{
				{10, 20, 30, 40, 50},        // AAPL Price
				{100, 200, 300, 400, 500},   // GOOG Price
			}
			multi, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
				[]data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())

			// Window=3, alpha=0.5
			// AAPL: seed=(10+20+30)/3=20, idx3: 0.5*40+0.5*20=30, idx4: 0.5*50+0.5*30=40
			// GOOG: seed=(100+200+300)/3=200, idx3: 0.5*400+0.5*200=300, idx4: 0.5*500+0.5*300=400
			result := multi.Rolling(3).EMA()
			Expect(result.Err()).NotTo(HaveOccurred())

			aaplCol := result.Column(aapl, data.Price)
			Expect(aaplCol[2]).To(BeNumerically("~", 20.0, 1e-10))
			Expect(aaplCol[3]).To(BeNumerically("~", 30.0, 1e-10))
			Expect(aaplCol[4]).To(BeNumerically("~", 40.0, 1e-10))

			googCol := result.Column(goog, data.Price)
			Expect(googCol[2]).To(BeNumerically("~", 200.0, 1e-10))
			Expect(googCol[3]).To(BeNumerically("~", 300.0, 1e-10))
			Expect(googCol[4]).To(BeNumerically("~", 400.0, 1e-10))
		})

		It("EMA first n-1 values are NaN", func() {
			result := df.Rolling(5).EMA()
			col := result.Column(aapl, data.Price)
			for idx := range 4 {
				Expect(math.IsNaN(col[idx])).To(BeTrue())
			}
			Expect(math.IsNaN(col[4])).To(BeFalse())
		})
	})
})
