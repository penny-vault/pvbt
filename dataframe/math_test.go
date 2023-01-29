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
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/dataframe"
)

var _ = Describe("When computing the SMA", func() {
	Context("with 5 values", func() {
		var (
			df1 *dataframe.DataFrame
			tz  *time.Location
		)

		BeforeEach(func() {
			tz = common.GetTimezone()

			df1 = &dataframe.DataFrame{
				Dates: []time.Time{
					time.Date(2021, time.January, 1, 0, 0, 0, 0, tz),
					time.Date(2021, time.February, 1, 0, 0, 0, 0, tz),
					time.Date(2021, time.March, 1, 0, 0, 0, 0, tz),
					time.Date(2021, time.April, 1, 0, 0, 0, 0, tz),
					time.Date(2021, time.May, 1, 0, 0, 0, 0, tz),
				},
				Vals:     [][]float64{{1.0, 2.0, 3.0, 4.0, 5.0}},
				ColNames: []string{"test"},
			}
		})

		It("yields correct results for lookback of 0", func() {
			sma := df1.SMA(0)
			Expect(sma.Len()).To(Equal(5))

			// Confirm that the timeAxis has all the expected values
			Expect(sma.Dates).To(Equal([]time.Time{
				time.Date(2021, time.January, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.February, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.March, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.April, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.May, 1, 0, 0, 0, 0, tz),
			}))

			// Confirm that col1 has all the expected values
			col1 := sma.Vals[0]
			Expect(math.IsNaN(col1[0])).Should(BeTrue())
			Expect(math.IsNaN(col1[1])).Should(BeTrue())
			Expect(math.IsNaN(col1[2])).Should(BeTrue())
			Expect(math.IsNaN(col1[3])).Should(BeTrue())
			Expect(math.IsNaN(col1[4])).Should(BeTrue())
		})

		It("yields correct results for lookback of 2", func() {
			sma := df1.SMA(2)
			Expect(sma.Len()).To(Equal(5))

			// Confirm that the timeAxis has all the expected values
			Expect(sma.Dates).To(Equal([]time.Time{
				time.Date(2021, time.January, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.February, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.March, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.April, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.May, 1, 0, 0, 0, 0, tz),
			}))

			// Confirm that col1 has all the expected values
			col1 := sma.Vals[0]
			Expect(math.IsNaN(col1[0])).Should(BeTrue())
			Expect(col1[1]).Should(Equal(1.5))
			Expect(col1[2]).Should(Equal(2.5))
			Expect(col1[3]).Should(Equal(3.5))
			Expect(col1[4]).Should(Equal(4.5))
		})

		It("yields correct results for lookback of 3", func() {
			sma := df1.SMA(3)
			Expect(sma.Len()).To(Equal(5))

			// Confirm that the timeAxis has all the expected values
			Expect(sma.Dates).To(Equal([]time.Time{
				time.Date(2021, time.January, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.February, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.March, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.April, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.May, 1, 0, 0, 0, 0, tz),
			}))

			// Confirm that col1 has all the expected values
			col1 := sma.Vals[0]
			Expect(math.IsNaN(col1[0])).Should(BeTrue())
			Expect(math.IsNaN(col1[1])).Should(BeTrue())
			Expect(col1[2]).Should(Equal(2.0))
			Expect(col1[3]).Should(Equal(3.0))
			Expect(col1[4]).Should(Equal(4.0))
		})

		It("yields correct results for lookback of 5", func() {
			sma := df1.SMA(5)
			Expect(sma.Len()).To(Equal(5))

			// Confirm that the timeAxis has all the expected values
			Expect(sma.Dates).To(Equal([]time.Time{
				time.Date(2021, time.January, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.February, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.March, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.April, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.May, 1, 0, 0, 0, 0, tz),
			}))

			// Confirm that col1 has all the expected values
			col1 := sma.Vals[0]
			Expect(math.IsNaN(col1[0])).Should(BeTrue())
			Expect(math.IsNaN(col1[1])).Should(BeTrue())
			Expect(math.IsNaN(col1[2])).Should(BeTrue())
			Expect(math.IsNaN(col1[3])).Should(BeTrue())
			Expect(col1[4]).Should(Equal(3.0))
		})

		It("yields correct results for lookback of 6", func() {
			sma := df1.SMA(6)
			Expect(sma.Len()).To(Equal(5))

			// Confirm that the timeAxis has all the expected values
			Expect(sma.Dates).To(Equal([]time.Time{
				time.Date(2021, time.January, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.February, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.March, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.April, 1, 0, 0, 0, 0, tz),
				time.Date(2021, time.May, 1, 0, 0, 0, 0, tz),
			}))

			// Confirm that col1 has all the expected values
			col1 := sma.Vals[0]
			Expect(math.IsNaN(col1[0])).Should(BeTrue())
			Expect(math.IsNaN(col1[1])).Should(BeTrue())
			Expect(math.IsNaN(col1[2])).Should(BeTrue())
			Expect(math.IsNaN(col1[3])).Should(BeTrue())
			Expect(math.IsNaN(col1[4])).Should(BeTrue())
		})
	})
})
