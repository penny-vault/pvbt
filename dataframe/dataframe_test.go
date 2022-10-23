// Copyright 2021-2022
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
	"github.com/rs/zerolog/log"
)

var _ = Describe("DataFrame", func() {
	Context("with no values", func() {
		var (
			df *dataframe.DataFrame
		)

		BeforeEach(func() {
			df = &dataframe.DataFrame{}
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
			df *dataframe.DataFrame
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
			df = &dataframe.DataFrame{
				ColNames: []string{"Col1"},
				Dates:    dates,
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
				Expect(df.Dates[0]).To(Equal(expectedA), "expected begin date")
				Expect(df.Dates[len(df.Dates)-1]).To(Equal(expectedB), "expected end date")
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
			log.Info().Times("dates", df.Dates).Msg("lockhart")
			Expect(df.Len()).To(Equal(expectedCnt), "expected count")
			if expectedCnt > 0 {
				Expect(df.Dates[0]).To(Equal(expectedStart), "expected start")
				Expect(df.Dates[len(df.Dates)-1]).To(Equal(expectedEnd), "expected end")
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
			df *dataframe.DataFrame
		)

		BeforeEach(func() {
			dates := make([]time.Time, 10)
			vals := make([]float64, 10)
			dt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			for idx := range dates {
				dates[idx] = dt
				dt = dt.AddDate(0, 0, 1)
				vals[idx] = math.NaN()
			}
			df = &dataframe.DataFrame{
				ColNames: []string{"Col1"},
				Dates:    dates,
				Vals:     [][]float64{vals},
			}
		})

		It("has length", func() {
			Expect(df.Len()).To(Equal(10))
		})

		It("drops NaNs", func() {
			Expect(df.Len()).To(Equal(10))
			df = df.Drop(math.NaN())
			Expect(df.Len()).To(Equal(0))
		})
	})
})
