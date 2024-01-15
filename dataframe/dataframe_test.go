// Copyright 2021-2023
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

package dataframe_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pv-api/dataframe"
)

const DIFFERENT = "Different"

var _ = Describe("DataFrame", func() {
	Context("with no values", func() {
		var (
			df *dataframe.DataFrame[time.Time]
		)

		BeforeEach(func() {
			df = &dataframe.DataFrame[time.Time]{}
		})

		It("has zero length", func() {
			Expect(df.Len()).To(Equal(0))
		})

		It("has zero columns", func() {
			Expect(df.ColCount()).To(Equal(0))
		})

		It("has no column names", func() {
			Expect(len(df.ColNames)).To(Equal(0))
		})

		It("does not error on breakout", func() {
			dfMap := df.Breakout()
			Expect(len(dfMap)).To(Equal(0))
		})

		It("does not error on drop", func() {
			df = df.Drop(1)
			Expect(df.Len()).To(Equal(0))
		})

		It("does not error on trim", func() {
			df = df.Trim(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC))
			Expect(df.Len()).To(Equal(0))
		})

		It("does not error on frequency", func() {
			df = df.Frequency(dataframe.Weekly)
			Expect(df.Len()).To(Equal(0))
		})
	})

	Context("with 2 years of values and a single column", func() {
		var (
			df *dataframe.DataFrame[time.Time]
		)

		BeforeEach(func() {
			dates := make([]time.Time, 730)
			vals := make([]float64, 730)
			dt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			for idx := range dates {
				dates[idx] = dt
				dt = dt.AddDate(0, 0, 1)
				vals[idx] = float64(idx)
			}
			df = &dataframe.DataFrame[time.Time]{
				ColNames: []string{"Col1"},
				Index:    dates,
				Vals:     [][]float64{vals},
			}
		})

		It("has length", func() {
			Expect(df.Len()).To(Equal(730))
		})

		It("has 1 column", func() {
			Expect(df.ColCount()).To(Equal(1))
		})

		It("has a column name", func() {
			Expect(len(df.ColNames)).To(Equal(1))
		})

		It("does not error on breakout", func() {
			dfMap := df.Breakout()
			_, ok := dfMap["Col1"]
			Expect(ok).To(BeTrue())
		})

		It("can remove all 0s with drop", func() {
			Expect(df.Len()).To(Equal(730))
			df = df.Drop(0)
			Expect(df.Len()).To(Equal(729))
			Expect(df.Vals[0][0]).To(BeNumerically("==", 1.0))
		})

		DescribeTable("trims values by date range", func(a, b time.Time, expectedLen int, expectedA, expectedB time.Time) {
			df = df.Trim(a, b)
			Expect(df.Len()).To(Equal(expectedLen))
			if expectedLen > 1 {
				Expect(df.Index[0]).To(Equal(expectedA), "expected begin date")
				Expect(df.Index[len(df.Index)-1]).To(Equal(expectedB), "expected end date")
			}
		},
			Entry("whole range", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC), 730, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC)),
			Entry("range that does not exist in dataframe (left)", time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2019, 12, 30, 0, 0, 0, 0, time.UTC), 0, time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2019, 12, 30, 0, 0, 0, 0, time.UTC)),
			Entry("range that does not exist in dataframe (right)", time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2023, 12, 30, 0, 0, 0, 0, time.UTC), 0, time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2023, 12, 30, 0, 0, 0, 0, time.UTC)),
			Entry("range that touches start but not end", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC), 5, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC)),
			Entry("range that touches end but not start", time.Date(2021, 12, 27, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC), 4, time.Date(2021, 12, 27, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC)),
			Entry("range that starts before begin", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC), 5, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC)),
			Entry("range that extends beyond the end", time.Date(2021, 12, 27, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 31, 0, 0, 0, 0, time.UTC), 4, time.Date(2021, 12, 27, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC)),
			Entry("range in the middle of dataframe", time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 6, 5, 0, 0, 0, 0, time.UTC), 5, time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 6, 5, 0, 0, 0, 0, time.UTC)),
			Entry("single date", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), 1, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
			Entry("inverted range", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), 0, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
			Entry("end on start", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), 1, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
			Entry("start on end", time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 1, time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC)),
		)

		DescribeTable("test frequency filter", func(frequency dataframe.Frequency, expectedCnt int, expectedStart, expectedEnd time.Time) {
			df = df.Frequency(frequency)
			Expect(df.Len()).To(Equal(expectedCnt), "expected count")
			if expectedCnt > 0 {
				Expect(df.Index[0]).To(Equal(expectedStart), "expected start")
				Expect(df.Index[len(df.Index)-1]).To(Equal(expectedEnd), "expected end")
			}
		},
			Entry("daily", dataframe.Daily, 522, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 30, 0, 0, 0, 0, time.UTC)),
			Entry("weekly", dataframe.Weekly, 104, time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 24, 0, 0, 0, 0, time.UTC)),
			Entry("week begin", dataframe.WeekBegin, 104, time.Date(2020, 1, 6, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 27, 0, 0, 0, 0, time.UTC)),
			Entry("week end", dataframe.WeekEnd, 104, time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 24, 0, 0, 0, 0, time.UTC)),
			Entry("month begin", dataframe.MonthBegin, 24, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 12, 1, 0, 0, 0, 0, time.UTC)),
			Entry("month end", dataframe.MonthEnd, 23, time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2021, 11, 30, 0, 0, 0, 0, time.UTC)),
			Entry("monthly", dataframe.Monthly, 23, time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2021, 11, 30, 0, 0, 0, 0, time.UTC)),
			Entry("year begin", dataframe.YearBegin, 2, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)),
			Entry("year end", dataframe.YearEnd, 1, time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC), time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)),
			Entry("annually", dataframe.Annually, 1, time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC), time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)),
		)
	})

	Context("with NaN values in dataframe", func() {
		var (
			df *dataframe.DataFrame[time.Time]
		)

		BeforeEach(func() {
			dates := make([]time.Time, 10)
			vals := make([]float64, 10)
			dt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			for idx := range dates {
				dates[idx] = dt
				dt = dt.AddDate(0, 0, 1)
				if idx < 5 {
					vals[idx] = float64(idx)
				} else {
					vals[idx] = math.NaN()
				}
			}
			df = &dataframe.DataFrame[time.Time]{
				ColNames: []string{"Col1"},
				Index:    dates,
				Vals:     [][]float64{vals},
			}
		})

		It("has length", func() {
			Expect(df.Len()).To(Equal(10))
		})

		It("drops NaNs", func() {
			Expect(df.Len()).To(Equal(10))
			df = df.Drop(math.NaN())
			Expect(df.Len()).To(Equal(5), "length")
			Expect(df.Vals[0]).To(Equal([]float64{0.0, 1.0, 2.0, 3.0, 4.0}), "vals")
			Expect(df.Index).To(Equal([]time.Time{
				time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC),
			}), "dates")
		})
	})

	Context("multi-column with NaN values in dataframe", func() {
		var (
			df *dataframe.DataFrame[time.Time]
		)

		BeforeEach(func() {
			dates := make([]time.Time, 10)
			dt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			for idx := range dates {
				dates[idx] = dt
				dt = dt.AddDate(0, 0, 1)
			}

			vals1 := make([]float64, 10)
			vals2 := make([]float64, 10)

			for idx := range dates {
				if idx < 5 {
					vals1[idx] = float64(idx)
				} else {
					vals1[idx] = math.NaN()
				}

				if idx < 6 {
					vals2[idx] = float64(idx)
				} else {
					vals2[idx] = math.NaN()
				}
			}

			df = &dataframe.DataFrame[time.Time]{
				ColNames: []string{"Col1", "Col2"},
				Index:    dates,
				Vals:     [][]float64{vals1, vals2},
			}
		})

		It("has length", func() {
			Expect(df.Len()).To(Equal(10))
		})

		It("drops NaNs", func() {
			Expect(df.Len()).To(Equal(10))
			df = df.Drop(math.NaN())
			Expect(df.Len()).To(Equal(5), "length")
			Expect(df.ColCount()).To(Equal(2), "col count")
			Expect(df.Vals[0]).To(Equal([]float64{0.0, 1.0, 2.0, 3.0, 4.0}), "vals1")
			Expect(df.Vals[1]).To(Equal([]float64{0.0, 1.0, 2.0, 3.0, 4.0}), "vals2")
			Expect(df.Index).To(Equal([]time.Time{
				time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC),
			}), "dates")
		})
	})

	Context("multi-column", func() {
		var (
			df *dataframe.DataFrame[time.Time]
		)

		BeforeEach(func() {
			dates := make([]time.Time, 10)
			dt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			for idx := range dates {
				dates[idx] = dt
				dt = dt.AddDate(0, 0, 1)
			}

			vals1 := []float64{1, 2, 3, 4, 5, 6, 7, math.NaN(), 9, math.NaN()}
			vals2 := []float64{1, 3, 2, 4, 6, 5, 6, math.NaN(), 10, 1}

			df = &dataframe.DataFrame[time.Time]{
				ColNames: []string{"Col1", "Col2"},
				Index:    dates,
				Vals:     [][]float64{vals1, vals2},
			}
		})

		It("can fetch last row", func() {
			last := df.Last()
			Expect(len(last.Vals)).To(Equal(len(df.Vals)), "length of value array")
			Expect(last.ColCount()).To(Equal(df.ColCount()), "column count")
			Expect(last.ColNames).To(Equal(df.ColNames), "column names")
			Expect(last.Len()).To(Equal(1), "row length")
			Expect(math.IsNaN(last.Vals[0][0])).To(BeTrue(), "col 0 value")
			Expect(last.Vals[1][0]).To(Equal(1.0), "col 1 value")
		})

		It("can take idxmax", func() {
			Expect(df.Len()).To(Equal(10))
			idxmax := df.IdxMax()
			Expect(idxmax.Len()).To(Equal(10), "length")
			Expect(idxmax.ColCount()).To(Equal(1), "col count")

			Expect(idxmax.Vals[0][0]).To(Equal(0.0), "vals[0]")
			Expect(idxmax.Vals[0][1]).To(Equal(1.0), "vals[1]")
			Expect(idxmax.Vals[0][2]).To(Equal(0.0), "vals[2]")
			Expect(idxmax.Vals[0][3]).To(Equal(0.0), "vals[3]")
			Expect(idxmax.Vals[0][4]).To(Equal(1.0), "vals[4]")
			Expect(idxmax.Vals[0][5]).To(Equal(0.0), "vals[5]")
			Expect(idxmax.Vals[0][6]).To(Equal(0.0), "vals[6]")
			Expect(math.IsNaN(idxmax.Vals[0][7])).To(BeTrue(), "vals[7]")
			Expect(idxmax.Vals[0][8]).To(Equal(1.0), "vals[8]")
			Expect(math.IsNaN(idxmax.Vals[0][9])).To(BeTrue(), "vals[9]")

			Expect(idxmax.Index).To(Equal([]time.Time{
				time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 6, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 7, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 8, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 9, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 10, 0, 0, 0, 0, time.UTC),
			}), "dates")
		})
	})

	Context("with 5 values for checking math functions", func() {
		var (
			df *dataframe.DataFrame[time.Time]
		)

		BeforeEach(func() {
			dates := make([]time.Time, 5)
			vals := make([]float64, 5)
			dt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			for idx := range dates {
				dates[idx] = dt
				dt = dt.AddDate(0, 0, 1)
				vals[idx] = float64(idx)
			}
			df = &dataframe.DataFrame[time.Time]{
				ColNames: []string{"Col1"},
				Index:    dates,
				Vals:     [][]float64{vals},
			}
		})

		It("lag shouldn't change the original dataframe", func() {
			df.Lag(2)
			Expect(math.IsNaN(df.Vals[0][0])).To(BeFalse())
		})

		It("lag 0 shouldn't shift the data frame", func() {
			df2 := df.Lag(0)
			Expect(df2.Vals[0][0]).To(BeNumerically("~", 0.0))
		})

		It("lag 2 shifts data frame by 2 values", func() {
			df2 := df.Lag(2)
			Expect(len(df.Vals[0])).To(Equal(5))
			Expect(math.IsNaN(df2.Vals[0][0])).To(BeTrue())
			Expect(math.IsNaN(df2.Vals[0][1])).To(BeTrue())
			Expect(df2.Vals[0][2]).To(Equal(0.0))
			Expect(df2.Vals[0][3]).To(Equal(1.0))
			Expect(df2.Vals[0][4]).To(Equal(2.0))
		})

		It("lag 6 results in all NaNs", func() {
			df2 := df.Lag(6)
			Expect(len(df.Vals[0])).To(Equal(5))
			Expect(math.IsNaN(df2.Vals[0][0])).To(BeTrue())
			Expect(math.IsNaN(df2.Vals[0][1])).To(BeTrue())
			Expect(math.IsNaN(df2.Vals[0][2])).To(BeTrue())
			Expect(math.IsNaN(df2.Vals[0][3])).To(BeTrue())
			Expect(math.IsNaN(df2.Vals[0][4])).To(BeTrue())
		})

		It("can divide same named columns", func() {
			df2 := df.Copy()
			df3 := df.Div(df2)
			Expect(df3.Len()).To(Equal(5))
			Expect(math.IsNaN(df3.Vals[0][0])).To(BeTrue())
			Expect(df3.Vals[0][1]).To(Equal(1.0))
			Expect(df3.Vals[0][2]).To(Equal(1.0))
			Expect(df3.Vals[0][3]).To(Equal(1.0))
			Expect(df3.Vals[0][4]).To(Equal(1.0))
		})

		It("different named columns do not divide", func() {
			df2 := df.Copy()
			df2.ColNames[0] = DIFFERENT
			df3 := df.Div(df2)
			Expect(df3.Len()).To(Equal(5))
			Expect(df3.Vals[0][0]).To(Equal(0.0))
			Expect(df3.Vals[0][1]).To(Equal(1.0))
			Expect(df3.Vals[0][2]).To(Equal(2.0))
			Expect(df3.Vals[0][3]).To(Equal(3.0))
			Expect(df3.Vals[0][4]).To(Equal(4.0))
		})

		It("non-column aligned dfs still divide", func() {
			df2 := df.Copy()
			df2.ColNames[0] = DIFFERENT
			df2.ColNames = append(df2.ColNames, "Col1")
			df2.Vals = append(df2.Vals, []float64{2, 2, 2, 2, 2})
			df3 := df.Div(df2)
			Expect(df3.Len()).To(Equal(5))
			Expect(df3.Vals[0][0]).To(Equal(0.0))
			Expect(df3.Vals[0][1]).To(Equal(0.5))
			Expect(df3.Vals[0][2]).To(Equal(1.0))
			Expect(df3.Vals[0][3]).To(Equal(1.5))
			Expect(df3.Vals[0][4]).To(Equal(2.0))
		})

		It("can multiply same named columns", func() {
			df2 := df.Copy()
			df3 := df.Mul(df2)
			Expect(df3.Len()).To(Equal(5))
			Expect(df3.Vals[0][0]).To(Equal(0.0))
			Expect(df3.Vals[0][1]).To(Equal(1.0))
			Expect(df3.Vals[0][2]).To(Equal(4.0))
			Expect(df3.Vals[0][3]).To(Equal(9.0))
			Expect(df3.Vals[0][4]).To(Equal(16.0))
		})

		It("different named columns do not multiply", func() {
			df2 := df.Copy()
			df2.ColNames[0] = DIFFERENT
			df3 := df.Div(df2)
			Expect(df3.Len()).To(Equal(5))
			Expect(df3.Vals[0][0]).To(Equal(0.0))
			Expect(df3.Vals[0][1]).To(Equal(1.0))
			Expect(df3.Vals[0][2]).To(Equal(2.0))
			Expect(df3.Vals[0][3]).To(Equal(3.0))
			Expect(df3.Vals[0][4]).To(Equal(4.0))
		})

		It("non-column aligned dfs still multiply", func() {
			df2 := df.Copy()
			df2.ColNames[0] = DIFFERENT
			df2.ColNames = append(df2.ColNames, "Col1")
			df2.Vals = append(df2.Vals, []float64{2, 2, 2, 2, 2})
			df3 := df.Mul(df2)
			Expect(df3.Len()).To(Equal(5))
			Expect(df3.Vals[0][0]).To(Equal(0.0))
			Expect(df3.Vals[0][1]).To(Equal(2.0))
			Expect(df3.Vals[0][2]).To(Equal(4.0))
			Expect(df3.Vals[0][3]).To(Equal(6.0))
			Expect(df3.Vals[0][4]).To(Equal(8.0))
		})

		It("can add a vector to the dataframe", func() {
			df3 := df.AddVec([]float64{1, 2, 3, 4, 5})
			Expect(df3.Len()).To(Equal(5))
			Expect(df3.Vals[0][0]).To(Equal(1.0))
			Expect(df3.Vals[0][1]).To(Equal(3.0))
			Expect(df3.Vals[0][2]).To(Equal(5.0))
			Expect(df3.Vals[0][3]).To(Equal(7.0))
			Expect(df3.Vals[0][4]).To(Equal(9.0))
		})

		It("computes rolling sum scaled with period 2", func() {
			df3 := df.RollingSumScaled(2, 2)
			Expect(df3.Len()).To(Equal(5))
			Expect(math.IsNaN(df3.Vals[0][0])).To(BeTrue())
			Expect(df3.Vals[0][1]).To(Equal(2.0))  // (0 + 1) * 2 = 2
			Expect(df3.Vals[0][2]).To(Equal(6.0))  // (1 + 2) * 2 = 6
			Expect(df3.Vals[0][3]).To(Equal(10.0)) // (2 + 3) * 2 = 10
			Expect(df3.Vals[0][4]).To(Equal(14.0)) // (3 + 4) * 2 = 14
		})

		It("computes rolling sum scaled with period 3", func() {
			df3 := df.RollingSumScaled(3, 2)
			Expect(df3.Len()).To(Equal(5))
			Expect(math.IsNaN(df3.Vals[0][0])).To(BeTrue())
			Expect(math.IsNaN(df3.Vals[0][1])).To(BeTrue())
			Expect(df3.Vals[0][2]).To(Equal(6.0))  // (0 + 1 + 2) * 2 = 6
			Expect(df3.Vals[0][3]).To(Equal(12.0)) // (1 + 2 + 3) * 2 = 12
			Expect(df3.Vals[0][4]).To(Equal(18.0)) // (2 + 3 + 4) * 2 = 18
		})
	})
})
