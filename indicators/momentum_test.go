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

package indicators_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/indicators"
	"github.com/penny-vault/pv-api/pgxmockhelper"
)

var _ = Describe("Momentum", func() {
	var (
		dbPool    pgxmock.PgxConnIface
		momentum  indicators.Indicator
		indicator *dataframe.DataFrame
		tz        *time.Location
		err       error
	)

	BeforeEach(func() {
		tz, _ = time.LoadLocation("America/New_York") // New York is the reference time

		momentum = &indicators.Momentum{
			Securities: []*data.Security{{
				Ticker:        "VFINX",
				CompositeFigi: "BBG000BHTMY2",
			}, {
				Ticker:        "PRIDX",
				CompositeFigi: "BBG000BBVR08",
			}},
			Periods: []int{1, 3, 6},
		}

		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		// Expect trading days transaction and query
		pgxmockhelper.MockHolidays(dbPool)
		pgxmockhelper.MockAssets(dbPool)
		manager := data.GetManagerInstance()
		manager.Reset()
	})

	Describe("Compute momentum indicator", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				pgxmockhelper.MockDBEodQuery(dbPool,
					[]string{
						"vfinx.csv",
						"pridx.csv",
						"dgs3mo.csv",
					},
					time.Date(1979, 7, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), "adj_close", "split_factor", "dividend")

				indicator, err = momentum.IndicatorForPeriod(context.Background(),
					time.Date(1980, time.January, 1, 0, 0, 0, 0, tz), time.Date(2021, time.January, 1, 0, 0, 0, 0, tz))
			})

			It("should not error", func() {
				Expect(err).To(BeNil())
			})

			It("should return a momentum indicator", func() {
				Expect(indicator).NotTo(BeNil())
			})

			It("should have an indicator for each trading day over the period", func() {
				Expect(indicator.Len()).To(Equal(379))
			})

			It("should have a series named 'Indicator'", func() {
				Expect(indicator.ColNames[0]).To(Equal(indicators.SeriesName))
			})

			It("should have correct starting value", func() {
				val := indicator.Vals[0][0]
				Expect(val).To(BeNumerically("~", 5.7988, .001))
			})

			It("should have correct starting date", func() {
				val := indicator.Dates[0]
				Expect(val).To(Equal(time.Date(1989, 6, 30, 16, 0, 0, 0, tz)))
			})

			It("should have correct ending value", func() {
				val := indicator.Vals[0][378]
				Expect(val).To(BeNumerically("~", 19.4716, .001))
			})

			It("should have correct ending date", func() {
				val := indicator.Dates[378]
				Expect(val).To(Equal(time.Date(2020, 12, 31, 16, 0, 0, 0, tz)))
			})
		})
	})
})
