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

	"github.com/jackc/pgconn"
	"github.com/jdfergason/dataframe-go"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/indicators"
	"github.com/penny-vault/pv-api/pgxmockhelper"
)

var _ = Describe("Momentum", func() {
	var (
		dbPool    pgxmock.PgxConnIface
		momentum  indicators.Indicator
		manager   *data.Manager
		indicator *dataframe.DataFrame
		tz        *time.Location
		err       error
	)

	BeforeEach(func() {
		tz, _ = time.LoadLocation("America/New_York") // New York is the reference time
		manager = data.NewManager()

		momentum = &indicators.Momentum{
			Securities: []*data.Security{{
				Ticker:        "VFINX",
				CompositeFigi: "BBG000BHTMY2",
			}, {
				Ticker:        "PRIDX",
				CompositeFigi: "BBG000BBVR08",
			}},
			Periods: []int{1, 3, 6},
			Manager: manager,
		}

		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		// Expect trading days transaction and query
		pgxmockhelper.MockAssets(dbPool)
		pgxmockhelper.MockDBEodQuery(dbPool, []string{"riskfree.csv"},
			time.Date(1969, 12, 25, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC),
			time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC))
		pgxmockhelper.MockDBCorporateQuery(dbPool, []string{"riskfree_corporate.csv"},
			time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC))

		data.InitializeDataManager()
	})

	Describe("Compute momentum indicator", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				manager.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, tz)
				manager.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, tz)

				pgxmockhelper.MockDBEodQuery(dbPool,
					[]string{
						"vfinx.csv",
						"pridx.csv",
						"riskfree.csv",
					},
					time.Date(1979, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
					time.Date(1979, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))

				pgxmockhelper.MockDBCorporateQuery(dbPool,
					[]string{
						"vfinx_corporate.csv",
						"pridx_corporate.csv",
						"riskfree_corporate.csv",
					},
					time.Date(1979, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))

				dbPool.ExpectBegin()
				dbPool.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
				dbPool.ExpectQuery("SELECT").WillReturnRows(
					pgxmockhelper.NewCSVRows([]string{"market_holidays.csv"}, map[string]string{
						"event_date":  "date",
						"early_close": "bool",
						"close_time":  "int",
					}).Rows())
				dbPool.ExpectCommit()

				indicator, err = momentum.IndicatorForPeriod(context.Background(), manager.Begin, manager.End)
			})

			It("should not error", func() {
				Expect(err).To(BeNil())
			})

			It("should return a momentum indicator", func() {
				Expect(indicator).NotTo(BeNil())
			})

			It("should have an indicator for each trading day over the period", func() {
				Expect(indicator.NRows()).To(Equal(379))
			})

			It("should have a series named 'Indicator'", func() {
				_, err := indicator.NameToColumn(indicators.SeriesName)
				Expect(err).To(BeNil())
			})

			It("should have a date series", func() {
				_, err := indicator.NameToColumn(common.DateIdx)
				Expect(err).To(BeNil())
			})

			It("should have correct starting value", func() {
				row := indicator.Row(0, true)
				val, ok := row[indicators.SeriesName].(float64)
				Expect(ok).To(BeTrue())
				Expect(val).To(BeNumerically("~", 5.7988, .001))
			})

			It("should have correct starting date", func() {
				row := indicator.Row(0, true)
				val, ok := row[common.DateIdx].(time.Time)
				Expect(ok).To(BeTrue())
				Expect(val).To(Equal(time.Date(1989, 6, 30, 16, 0, 0, 0, tz)))
			})

			It("should have correct ending value", func() {
				row := indicator.Row(378, true)
				val, ok := row[indicators.SeriesName].(float64)
				Expect(ok).To(BeTrue())
				Expect(val).To(BeNumerically("~", 19.4716, .001))
			})

			It("should have correct ending date", func() {
				row := indicator.Row(378, true)
				val, ok := row[common.DateIdx].(time.Time)
				Expect(ok).To(BeTrue())
				Expect(val).To(Equal(time.Date(2020, 12, 31, 16, 0, 0, 0, tz)))
			})
		})
	})
})
